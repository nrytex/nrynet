package auth

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/nrytex/nrynet/internal/config"
	"github.com/nrytex/nrynet/internal/model"
)

func (s *Service) CreateAgentToken(ctx context.Context, name string) (model.Token, string, error) {
	if name == "" {
		return model.Token{}, "", errors.New("token name is required")
	}
	id := uuid.NewString()
	secret, err := config.RandomSecret(24)
	if err != nil {
		return model.Token{}, "", err
	}
	prefix := secret[:8]
	now := time.Now().UTC()
	_, err = s.store.DB().ExecContext(ctx, `INSERT INTO tokens
        (id, name, prefix, secret_hash, disabled, created_at) VALUES(?, ?, ?, ?, 0, ?)`,
		id, name, prefix, HashToken(secret), now)
	if err != nil {
		return model.Token{}, "", err
	}
	return model.Token{ID: id, Name: name, Prefix: prefix, CreatedAt: now}, id + "." + secret, nil
}

func (s *Service) AuthenticateAgent(ctx context.Context, value string) (model.Token, error) {
	id, secret, err := TokenParts(value)
	if err != nil {
		return model.Token{}, err
	}
	var token model.Token
	var hash string
	err = s.store.DB().QueryRowContext(ctx, `SELECT id, name, prefix, secret_hash, disabled,
        last_used, created_at FROM tokens WHERE id = ?`, id).Scan(
		&token.ID, &token.Name, &token.Prefix, &hash, &token.Disabled, &token.LastUsed, &token.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) || token.Disabled {
		return model.Token{}, errors.New("agent token is invalid or disabled")
	}
	if err != nil {
		return model.Token{}, fmt.Errorf("load agent token: %w", err)
	}
	actual, expected := []byte(HashToken(secret)), []byte(hash)
	if len(actual) != len(expected) || subtle.ConstantTimeCompare(actual, expected) != 1 {
		return model.Token{}, errors.New("agent token is invalid or disabled")
	}
	now := time.Now().UTC()
	_, _ = s.store.DB().ExecContext(ctx, "UPDATE tokens SET last_used = ? WHERE id = ?", now, id)
	token.LastUsed = &now
	return token, nil
}
