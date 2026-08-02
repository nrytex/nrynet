package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/nat-link/nat-link/internal/model"
	"github.com/nat-link/nat-link/internal/protocol"
)

const udpIdleTimeout = 2 * time.Minute

type udpRuntime struct {
	manager *Manager
	tunnel  model.Tunnel
	conn    *net.UDPConn
	done    chan struct{}
	once    sync.Once

	mu       sync.Mutex
	sessions map[string]*udpVisitorSession
	byID     map[string]*udpVisitorSession
}

type udpVisitorSession struct {
	id       string
	addr     *net.UDPAddr
	lastSeen time.Time
}

func (m *Manager) startUDP(tunnel model.Tunnel) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.udpRuntimes[tunnel.ID]; exists {
		return nil
	}
	conn, err := listenUDP(tunnel.RemotePort)
	if err != nil {
		return err
	}
	runtime := newUDPRuntime(m, tunnel, conn)
	m.udpRuntimes[tunnel.ID] = runtime
	go runtime.readLoop()
	go runtime.reapLoop()
	return nil
}

func listenUDP(port int) (*net.UDPConn, error) {
	address := net.JoinHostPort("", strconv.Itoa(port))
	udpAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, fmt.Errorf("resolve udp port %d: %w", port, err)
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, fmt.Errorf("listen on udp remote port %d: %w", port, err)
	}
	return conn, nil
}

func newUDPRuntime(manager *Manager, tunnel model.Tunnel, conn *net.UDPConn) *udpRuntime {
	return &udpRuntime{
		manager:  manager,
		tunnel:   tunnel,
		conn:     conn,
		done:     make(chan struct{}),
		sessions: make(map[string]*udpVisitorSession),
		byID:     make(map[string]*udpVisitorSession),
	}
}

func (r *udpRuntime) readLoop() {
	buffer := make([]byte, 64*1024)
	for {
		n, addr, err := r.conn.ReadFromUDP(buffer)
		if err != nil {
			return
		}
		r.handleVisitorPacket(addr, buffer[:n])
	}
}

func (r *udpRuntime) handleVisitorPacket(addr *net.UDPAddr, data []byte) {
	if !visitorAllowed(addr, r.tunnel.IPAllowlist) {
		r.recordDenied(addr)
		return
	}
	session := r.session(addr)
	payload := append([]byte(nil), data...)
	err := r.manager.hub.SendUDPPacket(r.tunnel.ClientID, r.tunnel, session.id, payload)
	if err != nil {
		r.manager.removeUDPSession(r.tunnel.ID, session.id)
		return
	}
	r.recordTraffic(int64(len(payload)), 0)
}

func (r *udpRuntime) session(addr *net.UDPAddr) *udpVisitorSession {
	key := addr.String()
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	session := r.sessions[key]
	if session == nil {
		session = &udpVisitorSession{id: uuid.NewString(), addr: addr, lastSeen: now}
		r.sessions[key] = session
		r.byID[session.id] = session
		r.manager.active.Add(1)
		return session
	}
	session.lastSeen = now
	return session
}

func (r *udpRuntime) sendToVisitor(sessionID string, payload []byte) error {
	session := r.findSession(sessionID)
	if session == nil {
		return errors.New("udp visitor session not found")
	}
	if _, err := r.conn.WriteToUDP(payload, session.addr); err != nil {
		return err
	}
	r.recordTraffic(0, int64(len(payload)))
	return nil
}

func (r *udpRuntime) findSession(sessionID string) *udpVisitorSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	session := r.byID[sessionID]
	if session != nil {
		session.lastSeen = time.Now()
	}
	return session
}

func (r *udpRuntime) reapLoop() {
	ticker := time.NewTicker(udpIdleTimeout / 2)
	defer ticker.Stop()
	for {
		select {
		case <-r.done:
			return
		case <-ticker.C:
			r.removeIdle(time.Now().Add(-udpIdleTimeout))
		}
	}
}

func (r *udpRuntime) removeIdle(cutoff time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, session := range r.sessions {
		if session.lastSeen.Before(cutoff) {
			delete(r.sessions, key)
			delete(r.byID, session.id)
			r.manager.active.Add(-1)
		}
	}
}

func (r *udpRuntime) close() error {
	r.once.Do(func() {
		close(r.done)
		r.clearSessions()
	})
	return r.conn.Close()
}

func (r *udpRuntime) clearSessions() {
	r.mu.Lock()
	count := len(r.sessions)
	r.sessions = make(map[string]*udpVisitorSession)
	r.byID = make(map[string]*udpVisitorSession)
	r.mu.Unlock()
	r.manager.active.Add(-int64(count))
}

func (r *udpRuntime) recordDenied(addr *net.UDPAddr) {
	_ = r.manager.store.RecordEvent(context.Background(), "warn", "tunnel.denied",
		"Visitor denied by IP allowlist", map[string]any{
			"tunnel_id": r.tunnel.ID, "visitor": addr.String(),
		})
}

func (r *udpRuntime) recordTraffic(upload, download int64) {
	_ = r.manager.store.RecordTraffic(context.Background(), r.tunnel.ID, upload, download)
	r.manager.broker.RecordBytes(upload + download)
}

func (m *Manager) HandleUDPPacket(clientID string, message protocol.ControlMessage) {
	payload, err := protocol.DecodePayload[protocol.UDPPacketPayload](message)
	if err != nil || len(payload.Payload) == 0 {
		return
	}
	runtime := m.udpRuntimeFor(clientID, message.TunnelID)
	if runtime == nil {
		return
	}
	_ = runtime.sendToVisitor(message.RequestID, payload.Payload)
}

func (m *Manager) udpRuntimeFor(clientID, tunnelID string) *udpRuntime {
	m.mu.Lock()
	defer m.mu.Unlock()
	runtime := m.udpRuntimes[tunnelID]
	if runtime == nil || (clientID != "" && runtime.tunnel.ClientID != clientID) {
		return nil
	}
	return runtime
}

func (m *Manager) removeUDPSession(tunnelID, sessionID string) {
	runtime := m.udpRuntimeFor("", tunnelID)
	if runtime != nil {
		runtime.removeSession(sessionID)
	}
}

func (r *udpRuntime) removeSession(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	session := r.byID[sessionID]
	if session == nil {
		return
	}
	delete(r.sessions, session.addr.String())
	delete(r.byID, sessionID)
	r.manager.active.Add(-1)
}
