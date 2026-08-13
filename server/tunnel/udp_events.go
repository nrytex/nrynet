package tunnel

import (
	"context"
	"net"

	"github.com/nrytex/nrynet/internal/protocol"
)

func (r *udpRuntime) recordDenied(addr *net.UDPAddr) {
	r.manager.recordEvent(context.Background(), "warn", "tunnel.denied",
		"Visitor denied by IP allowlist", map[string]any{
			"tunnel_id": r.tunnel.ID, "visitor": addr.String(),
		})
}

func (r *udpRuntime) recordTraffic(upload, download int64) {
	r.manager.recordTraffic(r.tunnel.ID, upload, download)
}

func (r *udpRuntime) recordPath(event string, session *udpVisitorSession) {
	r.mu.Lock()
	if session.path == event {
		r.mu.Unlock()
		return
	}
	session.path = event
	r.mu.Unlock()
	r.manager.recordEvent(context.Background(), "info", event,
		"UDP packet routed", map[string]any{"tunnel_id": r.tunnel.ID, "session_id": session.id})
	path := protocol.TunnelPathRelay
	if event == "p2p.direct" {
		path = protocol.TunnelPathP2P
	}
	r.manager.notifyTunnelPath(r.tunnel, path)
}
