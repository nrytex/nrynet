package advanced

import (
	"context"

	netx "github.com/nrytex/nrynet/internal/advanced"
)

type RendezvousService struct {
	server *netx.RendezvousServer
}

func ListenRendezvous(addr string) (*RendezvousService, error) {
	server, err := netx.ListenRendezvous(addr)
	if err != nil {
		return nil, err
	}
	return &RendezvousService{server: server}, nil
}

func (s *RendezvousService) Addr() string {
	return s.server.Addr().String()
}

func (s *RendezvousService) Run(ctx context.Context) error {
	return s.server.Serve(ctx)
}

func (s *RendezvousService) Close() error {
	return s.server.Close()
}
