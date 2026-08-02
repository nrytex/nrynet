package storage

import (
	"context"
	"encoding/json"
	"time"

	"github.com/nat-link/nat-link/internal/model"
)

type EventFilter struct {
	Level   string
	Keyword string
	Limit   int
	Offset  int
}

func (s *Store) RecordEvent(ctx context.Context, level, event, message string, fields map[string]any) error {
	encoded, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO event_logs(level, event, message, fields, created_at)
        VALUES(?, ?, ?, ?, ?)`, level, event, message, string(encoded), time.Now().UTC())
	return err
}

func (s *Store) ListEvents(ctx context.Context, filter EventFilter) ([]model.Event, error) {
	if filter.Limit < 0 || filter.Limit > 500 {
		filter.Limit = 100
	}
	query := `SELECT id, level, event, message, fields, created_at FROM event_logs
        WHERE (? = '' OR level = ?) AND (? = '' OR message LIKE '%' || ? || '%' OR event LIKE '%' || ? || '%')
		ORDER BY created_at DESC`
	args := []any{filter.Level, filter.Level, filter.Keyword, filter.Keyword, filter.Keyword}
	if filter.Limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, filter.Limit, filter.Offset)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]model.Event, 0)
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func scanEvent(row scanner) (model.Event, error) {
	var event model.Event
	var fields string
	err := row.Scan(&event.ID, &event.Level, &event.Event, &event.Message, &fields, &event.CreatedAt)
	if err == nil {
		err = json.Unmarshal([]byte(fields), &event.Fields)
	}
	return event, err
}

func (s *Store) ClearEvents(ctx context.Context, before time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM event_logs WHERE created_at < ?", before.UTC())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
