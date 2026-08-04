package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	netx "github.com/nrytex/nrynet/internal/advanced"
	"github.com/nrytex/nrynet/internal/model"
	"github.com/nrytex/nrynet/internal/storage"
	clienthub "github.com/nrytex/nrynet/server/client"
	"github.com/nrytex/nrynet/server/relay"
)

type Manager struct {
	store  *storage.Store
	hub    *clienthub.Hub
	broker *relay.Broker

	mu          sync.Mutex
	listeners   map[string]net.Listener
	udpRuntimes map[string]*udpRuntime
	relayBinds  map[string]RelayBinding
	registry    *netx.RelayRegistry
	binder      RelayBinder
	rdvAddress  string
	active      atomic.Int64
}

func NewManager(store *storage.Store, hub *clienthub.Hub, broker *relay.Broker) *Manager {
	manager := &Manager{
		store:       store,
		hub:         hub,
		broker:      broker,
		listeners:   make(map[string]net.Listener),
		udpRuntimes: make(map[string]*udpRuntime),
		relayBinds:  make(map[string]RelayBinding),
	}
	hub.SetUDPPacketHandler(manager.HandleUDPPacket)
	return manager
}

func (m *Manager) Restore(ctx context.Context) error {
	tunnels, err := m.store.ListTunnels(ctx)
	if err != nil {
		return err
	}
	var restoreErrors []error
	for _, item := range tunnels {
		if item.Status != "running" {
			continue
		}
		if err := m.start(item); err != nil {
			_ = m.store.SetTunnelStatus(ctx, item.ID, "error")
			restoreErrors = append(restoreErrors, fmt.Errorf("restore %s: %w", item.Name, err))
		}
	}
	return errors.Join(restoreErrors...)
}

func (m *Manager) StartTunnel(ctx context.Context, id string) error {
	tunnel, err := m.store.GetTunnel(ctx, id)
	if err != nil {
		return err
	}
	if err := m.start(tunnel); err != nil {
		_ = m.store.SetTunnelStatus(ctx, id, "error")
		return err
	}
	if err := m.store.SetTunnelStatus(ctx, id, "running"); err != nil {
		_ = m.stop(id)
		return err
	}
	_ = m.store.RecordEvent(ctx, "info", "tunnel.started", "Tunnel started", map[string]any{
		"tunnel_id": tunnel.ID, "name": tunnel.Name, "remote_port": tunnel.RemotePort,
	})
	return m.SyncClient(ctx, tunnel.ClientID)
}

func (m *Manager) start(tunnel model.Tunnel) error {
	if tunnel.Protocol == "http" || tunnel.Protocol == "https" {
		return nil
	}
	if m.tryRelayStart(tunnel) {
		return nil
	}
	if tunnel.Protocol == "udp" {
		return m.startUDP(tunnel)
	}
	if tunnel.Protocol != "tcp" {
		return fmt.Errorf("%s tunnel runtime is not available yet", tunnel.Protocol)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.listeners[tunnel.ID]; exists {
		return nil
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(tunnel.RemotePort)))
	if err != nil {
		return fmt.Errorf("listen on remote port %d: %w", tunnel.RemotePort, err)
	}
	m.listeners[tunnel.ID] = listener
	go m.acceptLoop(tunnel, listener)
	return nil
}

func (m *Manager) RouteVisitor(tunnelID string, visitor net.Conn) error {
	tunnel, err := m.store.GetTunnel(context.Background(), tunnelID)
	if err != nil {
		return err
	}
	if tunnel.Status != "running" {
		return errors.New("tunnel is not running")
	}
	if !visitorAllowed(visitor.RemoteAddr(), tunnel.IPAllowlist) {
		return errors.New("visitor is not in the tunnel IP allowlist")
	}
	m.handleVisitor(tunnel, visitor)
	return nil
}

func (m *Manager) StopTunnel(ctx context.Context, id string) error {
	tunnel, err := m.store.GetTunnel(ctx, id)
	if err != nil {
		return err
	}
	if err := m.stop(id); err != nil {
		return err
	}
	if err := m.store.SetTunnelStatus(ctx, id, "stopped"); err != nil {
		return err
	}
	_ = m.store.RecordEvent(ctx, "info", "tunnel.stopped", "Tunnel stopped", map[string]any{
		"tunnel_id": tunnel.ID, "name": tunnel.Name,
	})
	return m.SyncClient(ctx, tunnel.ClientID)
}

func (m *Manager) stop(id string) error {
	m.mu.Lock()
	listener := m.listeners[id]
	delete(m.listeners, id)
	udp := m.udpRuntimes[id]
	delete(m.udpRuntimes, id)
	relayBind := m.relayBinds[id]
	delete(m.relayBinds, id)
	registry := m.registry
	m.mu.Unlock()
	if registry != nil {
		registry.ReleaseTunnel(id)
	}
	if relayBind != nil {
		return relayBind.Close()
	}
	if udp != nil {
		return udp.close()
	}
	if listener == nil {
		return nil
	}
	return listener.Close()
}

func (m *Manager) SyncClient(ctx context.Context, clientID string) error {
	tunnels, err := m.store.ListClientTunnels(ctx, clientID)
	if err != nil {
		return err
	}
	_ = m.hub.SendSnapshot(clientID, tunnels)
	return nil
}

func (m *Manager) DisconnectClient(clientID string) {
	m.hub.Disconnect(clientID)
	m.broker.DisconnectClient(clientID)
	m.disconnectUDPClient(clientID)
}

func (m *Manager) ClientConnectedAt(clientID string) (time.Time, bool) {
	return m.hub.ConnectedAt(clientID)
}

func (m *Manager) ActiveConnections() int64 {
	return m.active.Load()
}

func (m *Manager) BandwidthBPS() int64 {
	return m.broker.BandwidthBPS()
}

func (m *Manager) Close() error {
	m.mu.Lock()
	listeners := make([]net.Listener, 0, len(m.listeners))
	for _, listener := range m.listeners {
		listeners = append(listeners, listener)
	}
	udpRuntimes := make([]*udpRuntime, 0, len(m.udpRuntimes))
	for _, runtime := range m.udpRuntimes {
		udpRuntimes = append(udpRuntimes, runtime)
	}
	m.listeners = make(map[string]net.Listener)
	m.udpRuntimes = make(map[string]*udpRuntime)
	relayBinds := make([]RelayBinding, 0, len(m.relayBinds))
	for _, binding := range m.relayBinds {
		relayBinds = append(relayBinds, binding)
	}
	m.relayBinds = make(map[string]RelayBinding)
	m.mu.Unlock()
	var errs []error
	for _, listener := range listeners {
		errs = append(errs, listener.Close())
	}
	for _, runtime := range udpRuntimes {
		errs = append(errs, runtime.close())
	}
	for _, binding := range relayBinds {
		errs = append(errs, binding.Close())
	}
	return errors.Join(errs...)
}

func (m *Manager) acceptLoop(tunnel model.Tunnel, listener net.Listener) {
	for {
		visitor, err := listener.Accept()
		if err != nil {
			return
		}
		if !visitorAllowed(visitor.RemoteAddr(), tunnel.IPAllowlist) {
			_ = m.store.RecordEvent(context.Background(), "warn", "tunnel.denied",
				"Visitor denied by IP allowlist", map[string]any{
					"tunnel_id": tunnel.ID, "visitor": visitor.RemoteAddr().String(),
				})
			_ = visitor.Close()
			continue
		}
		go m.handleVisitor(tunnel, visitor)
	}
}

func (m *Manager) handleVisitor(tunnel model.Tunnel, visitor net.Conn) {
	requestID := uuid.NewString()
	pending, err := m.broker.RegisterPending(requestID, visitor, tunnel, func(upload, download int64) {
		_ = m.store.RecordTraffic(context.Background(), tunnel.ID, upload, download)
	})
	if err != nil {
		_ = visitor.Close()
		return
	}
	if err := m.hub.OpenConnection(tunnel.ClientID, tunnel, requestID); err != nil {
		m.broker.Cancel(requestID, pending)
		return
	}
	m.active.Add(1)
	defer m.active.Add(-1)
	err = m.broker.Wait(requestID, pending)
	m.recordConnectionFailure(tunnel.ID, requestID, err)
}
