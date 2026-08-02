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
)

const (
	udpIdleTimeout     = 2 * time.Minute
	udpWorkerCount     = 32
	udpPacketQueueSize = 256
	udpMaxSessions     = 4096
	udpMaxP2PSessions  = 128
)

type udpVisitorPacket struct {
	addr    *net.UDPAddr
	payload []byte
}

type udpRuntime struct {
	manager *Manager
	tunnel  model.Tunnel
	conn    *net.UDPConn
	done    chan struct{}
	packets chan udpVisitorPacket
	once    sync.Once

	mu       sync.Mutex
	sessions map[string]*udpVisitorSession
	byID     map[string]*udpVisitorSession
	p2pCount int
}

type udpVisitorSession struct {
	id       string
	addr     *net.UDPAddr
	lastSeen time.Time
	path     string
	p2pMu    sync.Mutex
	p2p      *p2pDirectSession
	closed   bool
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
	for range udpWorkerCount {
		go runtime.workerLoop()
	}
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
		packets:  make(chan udpVisitorPacket, udpPacketQueueSize),
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
		packet := udpVisitorPacket{addr: addr, payload: append([]byte(nil), buffer[:n]...)}
		select {
		case r.packets <- packet:
		case <-r.done:
			return
		default:
		}
	}
}

func (r *udpRuntime) workerLoop() {
	for {
		select {
		case packet := <-r.packets:
			r.handleVisitorPacket(packet.addr, packet.payload)
		case <-r.done:
			return
		}
	}
}

func (r *udpRuntime) handleVisitorPacket(addr *net.UDPAddr, data []byte) {
	if !visitorAllowed(addr, r.tunnel.IPAllowlist) {
		r.recordDenied(addr)
		return
	}
	session := r.session(addr)
	if session == nil {
		return
	}
	if r.manager.tryP2PUDPPacket(r.tunnel, session, data) {
		r.recordTraffic(int64(len(data)), 0)
		r.recordPath("p2p.direct", session)
		return
	}
	err := r.manager.hub.SendUDPPacket(r.tunnel.ClientID, r.tunnel, session.id, data)
	if err != nil {
		r.manager.removeUDPSession(r.tunnel.ID, session.id)
		return
	}
	r.recordTraffic(int64(len(data)), 0)
	r.recordPath("p2p.fallback", session)
}

func (r *udpRuntime) session(addr *net.UDPAddr) *udpVisitorSession {
	key := addr.String()
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	session := r.sessions[key]
	if session == nil {
		if len(r.sessions) >= udpMaxSessions {
			return nil
		}
		session = &udpVisitorSession{id: uuid.NewString(), addr: addr, lastSeen: now}
		r.sessions[key] = session
		r.byID[session.id] = session
		r.manager.active.Add(1)
		return session
	}
	session.lastSeen = now
	return session
}

func (r *udpRuntime) acquireP2P() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.p2pCount >= udpMaxP2PSessions {
		return false
	}
	r.p2pCount++
	return true
}

func (r *udpRuntime) releaseP2P() {
	r.mu.Lock()
	if r.p2pCount > 0 {
		r.p2pCount--
	}
	r.mu.Unlock()
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
	var removed []*udpVisitorSession
	for key, session := range r.sessions {
		if session.lastSeen.Before(cutoff) {
			delete(r.sessions, key)
			delete(r.byID, session.id)
			r.manager.active.Add(-1)
			removed = append(removed, session)
		}
	}
	r.mu.Unlock()
	for _, session := range removed {
		session.closeP2P()
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
	sessions := make([]*udpVisitorSession, 0, count)
	for _, session := range r.sessions {
		sessions = append(sessions, session)
	}
	r.sessions = make(map[string]*udpVisitorSession)
	r.byID = make(map[string]*udpVisitorSession)
	r.mu.Unlock()
	for _, session := range sessions {
		session.closeP2P()
	}
	r.manager.active.Add(-int64(count))
}

func (m *Manager) disconnectUDPClient(clientID string) {
	m.mu.Lock()
	runtimes := make([]*udpRuntime, 0)
	for _, runtime := range m.udpRuntimes {
		if runtime.tunnel.ClientID == clientID {
			runtimes = append(runtimes, runtime)
		}
	}
	m.mu.Unlock()
	for _, runtime := range runtimes {
		runtime.clearSessions()
	}
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

func (r *udpRuntime) recordPath(event string, session *udpVisitorSession) {
	r.mu.Lock()
	if session.path == event {
		r.mu.Unlock()
		return
	}
	session.path = event
	r.mu.Unlock()
	_ = r.manager.store.RecordEvent(context.Background(), "info", event,
		"UDP packet routed", map[string]any{"tunnel_id": r.tunnel.ID, "session_id": session.id})
}
