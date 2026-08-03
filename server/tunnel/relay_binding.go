package tunnel

import (
	"context"
	"errors"
	"net"
	"strconv"

	netx "github.com/nrytex/nrynet/internal/advanced"
	"github.com/nrytex/nrynet/internal/model"
)

type RelayBinder interface {
	BindTunnel(model.Tunnel, netx.TunnelAssignment, func(net.Conn)) (RelayBinding, error)
}

type RelayBinding interface {
	Close() error
	Address() string
}

func (m *Manager) SetRelayRegistry(registry *netx.RelayRegistry, binder RelayBinder) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registry = registry
	m.binder = binder
}

func (m *Manager) ReassignRelayTunnel(ctx context.Context, id string) error {
	tunnel, err := m.store.GetTunnel(ctx, id)
	if err != nil {
		return err
	}
	registry, binder := m.relayDependencies()
	if registry == nil || binder == nil {
		return errors.New("relay runtime is disabled")
	}
	m.closeRelayBinding(id)
	assignment, err := registry.ReassignTunnel(id)
	if err != nil {
		registry.ReleaseTunnel(id)
		return m.start(tunnel)
	}
	if !m.bindRelayTunnel(tunnel, registry, binder, assignment) {
		return errors.New("bind reassigned relay")
	}
	return nil
}

func (m *Manager) ReassignUnhealthyRelayTunnels(ctx context.Context) {
	registry, _ := m.relayDependencies()
	if registry == nil {
		return
	}
	for _, tunnelID := range registry.UnhealthyAssignments() {
		if err := m.ReassignRelayTunnel(ctx, tunnelID); err != nil {
			_ = m.store.RecordEvent(ctx, "warn", "relay.reassign_failed", "Relay reassignment failed", map[string]any{"tunnel_id": tunnelID, "error": err.Error()})
		}
	}
}

func (m *Manager) AssignAvailableRelayTunnels(ctx context.Context) {
	registry, binder := m.relayDependencies()
	if registry == nil || binder == nil {
		return
	}
	tunnels, err := m.store.ListTunnels(ctx)
	if err != nil {
		return
	}
	for _, item := range tunnels {
		if item.Status != "running" || item.Protocol != "tcp" {
			continue
		}
		if _, assigned := registry.Assignment(item.ID); assigned {
			continue
		}
		if !m.closeLocalListener(item.ID) {
			continue
		}
		assignment, err := registry.AssignTunnel(item.ID)
		if err == nil && m.bindRelayTunnel(item, registry, binder, assignment) {
			continue
		}
		registry.ReleaseTunnel(item.ID)
		_ = m.start(item)
	}
}

func (m *Manager) RouteRelayVisitor(nodeID, tunnelID, visitorAddr string, visitor net.Conn) error {
	registry, _ := m.relayDependencies()
	if registry == nil || !registry.IsAssignedTo(tunnelID, nodeID) {
		return errors.New("relay is not assigned to tunnel")
	}
	host, portText, err := net.SplitHostPort(visitorAddr)
	if err != nil {
		return errors.New("relay visitor address is invalid")
	}
	ip := net.ParseIP(host)
	port, err := strconv.Atoi(portText)
	if ip == nil || err != nil || port < 1 || port > 65535 {
		return errors.New("relay visitor address is invalid")
	}
	remote := &net.TCPAddr{IP: ip, Port: port}
	return m.routeRelayVisitor(tunnelID, remoteAddrConn{Conn: visitor, remote: remote})
}

func (m *Manager) routeRelayVisitor(tunnelID string, visitor net.Conn) error {
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
	go m.handleVisitor(tunnel, visitor)
	return nil
}

type remoteAddrConn struct {
	net.Conn
	remote net.Addr
}

func (c remoteAddrConn) RemoteAddr() net.Addr { return c.remote }

func (m *Manager) tryRelayStart(tunnel model.Tunnel) bool {
	registry, binder := m.relayDependencies()
	if registry == nil || binder == nil || tunnel.Protocol != "tcp" {
		return false
	}
	assignment, err := registry.AssignTunnel(tunnel.ID)
	if err != nil {
		return false
	}
	return m.bindRelayTunnel(tunnel, registry, binder, assignment)
}

func (m *Manager) closeRelayBinding(id string) {
	m.mu.Lock()
	binding := m.relayBinds[id]
	delete(m.relayBinds, id)
	m.mu.Unlock()
	if binding != nil {
		_ = binding.Close()
	}
}

func (m *Manager) closeLocalListener(id string) bool {
	m.mu.Lock()
	listener := m.listeners[id]
	delete(m.listeners, id)
	m.mu.Unlock()
	if listener == nil {
		return false
	}
	_ = listener.Close()
	return true
}

func (m *Manager) relayDependencies() (*netx.RelayRegistry, RelayBinder) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.registry, m.binder
}

func (m *Manager) bindRelayTunnel(
	tunnel model.Tunnel,
	registry *netx.RelayRegistry,
	binder RelayBinder,
	assignment netx.TunnelAssignment,
) bool {
	binding, err := binder.BindTunnel(tunnel, assignment, func(conn net.Conn) {
		m.handleVisitor(tunnel, conn)
	})
	if err != nil {
		registry.ReleaseTunnel(tunnel.ID)
		return false
	}
	m.mu.Lock()
	m.relayBinds[tunnel.ID] = binding
	m.mu.Unlock()
	return true
}
