package storage

import "context"

type Counts struct {
	OnlineClients int `json:"online_clients"`
	TotalClients  int `json:"total_clients"`
	ActiveTunnels int `json:"active_tunnels"`
	TotalTunnels  int `json:"total_tunnels"`
}

func (s *Store) Counts(ctx context.Context) (Counts, error) {
	var counts Counts
	err := s.db.QueryRowContext(ctx, `SELECT
        (SELECT COUNT(*) FROM clients WHERE status='online' AND disabled=0),
        (SELECT COUNT(*) FROM clients),
        (SELECT COUNT(*) FROM tunnels WHERE status='running'),
        (SELECT COUNT(*) FROM tunnels)`).Scan(&counts.OnlineClients, &counts.TotalClients,
		&counts.ActiveTunnels, &counts.TotalTunnels)
	return counts, err
}
