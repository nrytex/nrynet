package storage

import (
	"context"
	"errors"
	"time"

	"github.com/nrytex/nrynet/internal/model"
)

type TrafficDelta struct {
	TunnelID string
	Upload   int64
	Download int64
}

func (s *Store) RecordTraffic(ctx context.Context, tunnelID string, upload, download int64) error {
	if upload < 0 || download < 0 {
		return errors.New("traffic values cannot be negative")
	}
	if upload == 0 && download == 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO traffic_logs(tunnel_id, upload, download, created_at)
        VALUES(?, ?, ?, ?)`, tunnelID, upload, download, time.Now().UTC())
	return err
}

func (s *Store) RecordTrafficBatch(ctx context.Context, items []TrafficDelta) error {
	if len(items) == 0 {
		return nil
	}
	for _, item := range items {
		if item.TunnelID == "" || item.Upload < 0 || item.Download < 0 {
			return errors.New("invalid traffic delta")
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statement, err := tx.PrepareContext(ctx, `INSERT INTO traffic_logs(tunnel_id, upload, download, created_at)
        VALUES(?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer statement.Close()
	now := time.Now().UTC()
	for _, item := range items {
		if item.Upload == 0 && item.Download == 0 {
			continue
		}
		if _, err := statement.ExecContext(ctx, item.TunnelID, item.Upload, item.Download, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) TrafficSummary(ctx context.Context, since time.Time) (model.TrafficSummary, error) {
	var summary model.TrafficSummary
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(upload), 0), COALESCE(SUM(download), 0)
        FROM traffic_logs WHERE created_at >= ?`, since.UTC()).Scan(&summary.Upload, &summary.Download)
	return summary, err
}

func (s *Store) TrafficForClient(ctx context.Context, clientID string, since time.Time) (model.TrafficSummary, error) {
	var summary model.TrafficSummary
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(l.upload), 0), COALESCE(SUM(l.download), 0)
		FROM traffic_logs l JOIN tunnels t ON t.id = l.tunnel_id
		WHERE t.client_id = ? AND l.created_at >= ?`, clientID, since.UTC()).Scan(
		&summary.Upload, &summary.Download,
	)
	return summary, err
}

func (s *Store) TrafficByTunnel(ctx context.Context, since time.Time) ([]model.TrafficTarget, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.id, t.name, COALESCE(SUM(l.upload), 0),
        COALESCE(SUM(l.download), 0) FROM tunnels t LEFT JOIN traffic_logs l
        ON l.tunnel_id=t.id AND l.created_at >= ? GROUP BY t.id, t.name
        ORDER BY SUM(COALESCE(l.upload, 0) + COALESCE(l.download, 0)) DESC`, since.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTrafficTargets(rows)
}

func (s *Store) TrafficByClient(ctx context.Context, since time.Time) ([]model.TrafficTarget, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.id, c.name, COALESCE(SUM(l.upload), 0),
        COALESCE(SUM(l.download), 0) FROM clients c LEFT JOIN tunnels t ON t.client_id=c.id
        LEFT JOIN traffic_logs l ON l.tunnel_id=t.id AND l.created_at >= ? GROUP BY c.id, c.name
        ORDER BY SUM(COALESCE(l.upload, 0) + COALESCE(l.download, 0)) DESC`, since.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTrafficTargets(rows)
}

func scanTrafficTargets(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]model.TrafficTarget, error) {
	items := make([]model.TrafficTarget, 0)
	for rows.Next() {
		var item model.TrafficTarget
		if err := rows.Scan(&item.ID, &item.Name, &item.Upload, &item.Download); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func RangeStart(value string, now time.Time) (time.Time, error) {
	now = now.UTC()
	switch value {
	case "today", "":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), nil
	case "month":
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC), nil
	default:
		return time.Time{}, errors.New("range must be today or month")
	}
}
