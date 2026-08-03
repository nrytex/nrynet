package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/nrytex/nrynet/internal/model"
)

type ClientHello struct {
	Name     string
	DeviceID string
	IP       string
	OS       string
	Version  string
}

func (s *Store) UpsertClient(ctx context.Context, tokenID string, hello ClientHello) (model.Client, error) {
	now := time.Now().UTC()
	id := uuid.NewString()
	result, err := s.db.ExecContext(ctx, `INSERT INTO clients
        (id, name, device_id, token_id, status, disabled, ip, os, version, last_online, created_at)
        SELECT ?, ?, ?, ?, 'online', 0, ?, ?, ?, ?, ?
        WHERE NOT EXISTS (SELECT 1 FROM revoked_devices WHERE device_id = ?)
        ON CONFLICT(device_id) DO UPDATE SET name=excluded.name,
        status='online', ip=excluded.ip, os=excluded.os, version=excluded.version,
		last_online=excluded.last_online WHERE clients.token_id=excluded.token_id`, id, hello.Name, hello.DeviceID, tokenID,
		hello.IP, hello.OS, hello.Version, now, now, hello.DeviceID)
	if err != nil {
		return model.Client{}, err
	}
	if err := requireChanged(result); err != nil {
		return model.Client{}, errors.New("device_id is bound to another token or has been revoked")
	}
	return s.GetClientByDevice(ctx, hello.DeviceID)
}

func (s *Store) GetClientByDevice(ctx context.Context, deviceID string) (model.Client, error) {
	return scanClient(s.db.QueryRowContext(ctx, `SELECT id, name, device_id, token_id, status,
        disabled, ip, os, version, last_online, created_at FROM clients WHERE device_id = ?`, deviceID))
}

func (s *Store) GetClient(ctx context.Context, id string) (model.Client, error) {
	return scanClient(s.db.QueryRowContext(ctx, `SELECT id, name, device_id, token_id, status,
        disabled, ip, os, version, last_online, created_at FROM clients WHERE id = ?`, id))
}

func scanClient(row *sql.Row) (model.Client, error) {
	var client model.Client
	err := row.Scan(&client.ID, &client.Name, &client.DeviceID, &client.TokenID, &client.Status,
		&client.Disabled, &client.IP, &client.OS, &client.Version, &client.LastOnline, &client.CreatedAt)
	return client, err
}

func (s *Store) ListClients(ctx context.Context) ([]model.Client, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, device_id, token_id, status, disabled,
        ip, os, version, last_online, created_at FROM clients ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	clients := make([]model.Client, 0)
	for rows.Next() {
		var client model.Client
		if err := rows.Scan(&client.ID, &client.Name, &client.DeviceID, &client.TokenID,
			&client.Status, &client.Disabled, &client.IP, &client.OS, &client.Version,
			&client.LastOnline, &client.CreatedAt); err != nil {
			return nil, err
		}
		clients = append(clients, client)
	}
	return clients, rows.Err()
}

func (s *Store) ListClientsByToken(ctx context.Context, tokenID string) ([]model.Client, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, device_id, token_id, status, disabled,
        ip, os, version, last_online, created_at FROM clients WHERE token_id = ? ORDER BY created_at DESC`, tokenID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	clients := make([]model.Client, 0)
	for rows.Next() {
		var client model.Client
		if err := rows.Scan(&client.ID, &client.Name, &client.DeviceID, &client.TokenID,
			&client.Status, &client.Disabled, &client.IP, &client.OS, &client.Version,
			&client.LastOnline, &client.CreatedAt); err != nil {
			return nil, err
		}
		clients = append(clients, client)
	}
	return clients, rows.Err()
}

func (s *Store) SetClientStatus(ctx context.Context, id, status string) error {
	result, err := s.db.ExecContext(ctx,
		"UPDATE clients SET status = ?, last_online = ? WHERE id = ?", status, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	return requireChanged(result)
}

func (s *Store) UpdateClient(ctx context.Context, id, name string, disabled *bool) error {
	if name != "" {
		if _, err := s.db.ExecContext(ctx, "UPDATE clients SET name = ? WHERE id = ?", name, id); err != nil {
			return err
		}
	}
	if disabled != nil {
		return s.setClientDisabled(ctx, id, *disabled)
	}
	if name == "" {
		return errors.New("no client changes supplied")
	}
	return nil
}

func (s *Store) setClientDisabled(ctx context.Context, id string, disabled bool) error {
	result, err := s.db.ExecContext(ctx, "UPDATE clients SET disabled = ? WHERE id = ?", disabled, id)
	if err != nil {
		return err
	}
	return requireChanged(result)
}

func (s *Store) DeleteClient(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO revoked_devices(device_id, revoked_at)
        SELECT device_id, ? FROM clients WHERE id = ? ON CONFLICT(device_id) DO NOTHING`, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM traffic_logs WHERE tunnel_id IN
        (SELECT id FROM tunnels WHERE client_id = ?)`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM tunnels WHERE client_id = ?", id); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM clients WHERE id = ?", id)
	if err != nil {
		return err
	}
	if err := requireChanged(result); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RevokeClientDevice(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `INSERT INTO revoked_devices(device_id, revoked_at)
        SELECT device_id, ? FROM clients WHERE id = ?
        ON CONFLICT(device_id) DO UPDATE SET revoked_at = excluded.revoked_at`, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	return requireChanged(result)
}

func (s *Store) UpdateClientToken(ctx context.Context, id, tokenID string) error {
	result, err := s.db.ExecContext(ctx, "UPDATE clients SET token_id = ? WHERE id = ?", tokenID, id)
	if err != nil {
		return err
	}
	return requireChanged(result)
}
