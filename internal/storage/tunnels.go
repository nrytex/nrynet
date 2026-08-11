package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nrytex/nrynet/internal/model"
)

func (s *Store) CreateTunnel(ctx context.Context, tunnel model.Tunnel) (model.Tunnel, error) {
	if err := validateTunnel(tunnel); err != nil {
		return model.Tunnel{}, err
	}
	tunnel.ID = uuid.NewString()
	tunnel.Protocol = strings.ToLower(tunnel.Protocol)
	if tunnel.Protocol == "visitor_webrtc" {
		tunnel.VisitorToken = newVisitorToken(tunnel.VisitorToken)
	}
	tunnel.Domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(tunnel.Domain), "."))
	tunnel.Status = "stopped"
	tunnel.CreatedAt = time.Now().UTC()
	tunnel.UpdatedAt = tunnel.CreatedAt
	allowlist, _ := json.Marshal(tunnel.IPAllowlist)
	_, err := s.db.ExecContext(ctx, `INSERT INTO tunnels
	        (id, client_id, name, protocol, local_host, local_port, remote_port, domain, visitor_token,
	        status, ip_allowlist, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tunnel.ID, tunnel.ClientID, tunnel.Name, tunnel.Protocol, tunnel.LocalHost,
		tunnel.LocalPort, tunnel.RemotePort, tunnel.Domain, tunnel.VisitorToken, tunnel.Status,
		string(allowlist), tunnel.CreatedAt, tunnel.UpdatedAt)
	return tunnel, err
}

func validateTunnel(tunnel model.Tunnel) error {
	if tunnel.Name == "" || tunnel.ClientID == "" || tunnel.LocalHost == "" {
		return errors.New("name, client_id, and local_host are required")
	}
	if tunnel.LocalPort < 1 || tunnel.LocalPort > 65535 {
		return errors.New("local_port must be between 1 and 65535")
	}
	protocol := strings.ToLower(tunnel.Protocol)
	if protocol != "tcp" && protocol != "p2p" && protocol != "http" && protocol != "https" && protocol != "udp" && protocol != "visitor_webrtc" {
		return errors.New("protocol must be tcp, p2p, http, https, udp, or visitor_webrtc")
	}
	if (protocol == "tcp" || protocol == "p2p" || protocol == "udp") && (tunnel.RemotePort < 1 || tunnel.RemotePort > 65535) {
		return errors.New("remote_port must be between 1 and 65535")
	}
	if protocol == "visitor_webrtc" && tunnel.RemotePort != 0 {
		return errors.New("remote_port must be empty for visitor_webrtc tunnels")
	}
	if (protocol == "http" || protocol == "https") && tunnel.Domain == "" {
		return errors.New("domain is required for HTTP and HTTPS tunnels")
	}
	if (protocol == "http" || protocol == "https") && tunnel.Domain != "" {
		if _, err := NormalizeDomain(tunnel.Domain); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GetTunnel(ctx context.Context, id string) (model.Tunnel, error) {
	row := s.db.QueryRowContext(ctx, tunnelSelect+" WHERE id = ?", id)
	return scanTunnel(row)
}

func (s *Store) FindDomainTunnel(ctx context.Context, protocol, domain string) (model.Tunnel, error) {
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))
	row := s.db.QueryRowContext(ctx, tunnelSelect+
		" WHERE protocol = ? AND lower(domain) = ? AND status = 'running'", protocol, domain)
	return scanTunnel(row)
}

func (s *Store) ListTunnels(ctx context.Context) ([]model.Tunnel, error) {
	rows, err := s.db.QueryContext(ctx, tunnelSelect+" ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tunnels := make([]model.Tunnel, 0)
	for rows.Next() {
		tunnel, err := scanTunnel(rows)
		if err != nil {
			return nil, err
		}
		tunnels = append(tunnels, tunnel)
	}
	return tunnels, rows.Err()
}

func (s *Store) ListClientTunnels(ctx context.Context, clientID string) ([]model.Tunnel, error) {
	rows, err := s.db.QueryContext(ctx, tunnelSelect+" WHERE client_id = ? ORDER BY created_at", clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tunnels := make([]model.Tunnel, 0)
	for rows.Next() {
		tunnel, err := scanTunnel(rows)
		if err != nil {
			return nil, err
		}
		tunnels = append(tunnels, tunnel)
	}
	return tunnels, rows.Err()
}

const tunnelSelect = `SELECT id, client_id, name, protocol, local_host, local_port,
    remote_port, domain, visitor_token, status, ip_allowlist, created_at, updated_at FROM tunnels`

type scanner interface {
	Scan(...any) error
}

func scanTunnel(row scanner) (model.Tunnel, error) {
	var tunnel model.Tunnel
	var allowlist string
	err := row.Scan(&tunnel.ID, &tunnel.ClientID, &tunnel.Name, &tunnel.Protocol,
		&tunnel.LocalHost, &tunnel.LocalPort, &tunnel.RemotePort, &tunnel.Domain, &tunnel.VisitorToken,
		&tunnel.Status, &allowlist, &tunnel.CreatedAt, &tunnel.UpdatedAt)
	if err == nil {
		err = json.Unmarshal([]byte(allowlist), &tunnel.IPAllowlist)
	}
	return tunnel, err
}

func (s *Store) UpdateTunnel(ctx context.Context, tunnel model.Tunnel) (model.Tunnel, error) {
	if err := validateTunnel(tunnel); err != nil {
		return model.Tunnel{}, err
	}
	tunnel.Protocol = strings.ToLower(tunnel.Protocol)
	if tunnel.Protocol == "visitor_webrtc" {
		if tunnel.VisitorToken == "" {
			current, err := s.GetTunnel(ctx, tunnel.ID)
			if err != nil {
				return model.Tunnel{}, err
			}
			tunnel.VisitorToken = current.VisitorToken
		}
		tunnel.VisitorToken = newVisitorToken(tunnel.VisitorToken)
	}
	tunnel.Domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(tunnel.Domain), "."))
	tunnel.UpdatedAt = time.Now().UTC()
	allowlist, _ := json.Marshal(tunnel.IPAllowlist)
	result, err := s.db.ExecContext(ctx, `UPDATE tunnels SET client_id=?, name=?, protocol=?,
		local_host=?, local_port=?, remote_port=?, domain=?, visitor_token=?, ip_allowlist=?, updated_at=? WHERE id=?`,
		tunnel.ClientID, tunnel.Name, tunnel.Protocol, tunnel.LocalHost, tunnel.LocalPort,
		tunnel.RemotePort, tunnel.Domain, tunnel.VisitorToken, string(allowlist), tunnel.UpdatedAt, tunnel.ID)
	if err != nil {
		return model.Tunnel{}, err
	}
	if err := requireChanged(result); err != nil {
		return model.Tunnel{}, err
	}
	return s.GetTunnel(ctx, tunnel.ID)
}

func (s *Store) SetTunnelStatus(ctx context.Context, id, status string) error {
	if status != "running" && status != "stopped" && status != "error" {
		return errors.New("invalid tunnel status")
	}
	result, err := s.db.ExecContext(ctx, "UPDATE tunnels SET status=?, updated_at=? WHERE id=?",
		status, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	return requireChanged(result)
}

func (s *Store) DeleteTunnel(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM tunnels WHERE id = ?", id)
	if err != nil {
		return err
	}
	return requireChanged(result)
}

var _ scanner = (*sql.Row)(nil)
