package gateway

import (
	"context"
	"net"
	"time"

	"github.com/nrytex/nrynet/internal/storage"
)

type Router interface {
	RouteVisitor(tunnelID string, visitor net.Conn) error
}

type Gateway struct {
	store   *storage.Store
	router  Router
	timeout time.Duration
}

func New(store *storage.Store, router Router) *Gateway {
	return &Gateway{store: store, router: router, timeout: 10 * time.Second}
}

func (g *Gateway) Run(listener net.Listener) error {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		go g.handle(conn)
	}
}

func (g *Gateway) handle(conn net.Conn) {
	_ = conn.SetReadDeadline(time.Now().Add(g.timeout))
	visitor, protocol, domain, err := sniffConnection(conn)
	if err != nil {
		_ = conn.Close()
		return
	}
	_ = conn.SetReadDeadline(time.Time{})
	tunnel, err := g.store.FindDomainTunnel(context.Background(), protocol, domain)
	if err != nil {
		_ = conn.Close()
		return
	}
	if err := g.router.RouteVisitor(tunnel.ID, visitor); err != nil {
		_ = visitor.Close()
	}
}
