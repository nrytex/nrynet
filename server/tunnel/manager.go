package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"

	"github.com/nat-link/nat-link/internal/model"
	"github.com/nat-link/nat-link/internal/storage"
	clienthub "github.com/nat-link/nat-link/server/client"
	"github.com/nat-link/nat-link/server/relay"
)

type Manager struct {
	store  *storage.Store
	hub    *clienthub.Hub
	broker *relay.Broker

	mu        sync.Mutex
	listeners map[string]net.Listener
	active    atomic.Int64
}

func NewManager(store *storage.Store, hub *clienthub.Hub, broker *relay.Broker) *Manager {
	return &Manager{store: store, hub: hub, broker: broker, listeners: make(map[string]net.Listener)}
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
	if tunnel.Protocol != "tcp" {
		return fmt.Errorf("%s tunnel runtime is not available yet", tunnel.Protocol)
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
	m.mu.Unlock()
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
}

func (m *Manager) ActiveConnections() int64 {
	return m.active.Load()
}

func (m *Manager) Close() error {
	m.mu.Lock()
	listeners := make([]net.Listener, 0, len(m.listeners))
	for _, listener := range m.listeners {
		listeners = append(listeners, listener)
	}
	m.listeners = make(map[string]net.Listener)
	m.mu.Unlock()
	var errs []error
	for _, listener := range listeners {
		errs = append(errs, listener.Close())
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
	_ = m.broker.Wait(requestID, pending)
}
