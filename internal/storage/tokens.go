package storage

import (
	"context"
	"database/sql"
	"errors"

	"github.com/nat-link/nat-link/internal/model"
)

func (s *Store) ListTokens(ctx context.Context) ([]model.Token, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, prefix, disabled, last_used, created_at
        FROM tokens ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tokens := make([]model.Token, 0)
	for rows.Next() {
		var token model.Token
		if err := rows.Scan(&token.ID, &token.Name, &token.Prefix, &token.Disabled,
			&token.LastUsed, &token.CreatedAt); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, rows.Err()
}

func (s *Store) GetToken(ctx context.Context, id string) (model.Token, error) {
	var token model.Token
	err := s.db.QueryRowContext(ctx, `SELECT id, name, prefix, disabled, last_used, created_at
        FROM tokens WHERE id = ?`, id).Scan(&token.ID, &token.Name, &token.Prefix,
		&token.Disabled, &token.LastUsed, &token.CreatedAt)
	return token, err
}

func (s *Store) SetTokenDisabled(ctx context.Context, id string, disabled bool) error {
	result, err := s.db.ExecContext(ctx, "UPDATE tokens SET disabled = ? WHERE id = ?", disabled, id)
	if err != nil {
		return err
	}
	return requireChanged(result)
}

func (s *Store) DeleteToken(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM traffic_logs WHERE tunnel_id IN
        (SELECT tunnels.id FROM tunnels JOIN clients ON clients.id = tunnels.client_id WHERE clients.token_id = ?)`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tunnels WHERE client_id IN
        (SELECT id FROM clients WHERE token_id = ?)`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM clients WHERE token_id = ?", id); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM tokens WHERE id = ?", id)
	if err != nil {
		return err
	}
	if err := requireChanged(result); err != nil {
		return err
	}
	return tx.Commit()
}

func requireChanged(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return errors.New("resource not found")
	}
	return nil
}
