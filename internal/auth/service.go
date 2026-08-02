package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/nat-link/nat-link/internal/config"
	"github.com/nat-link/nat-link/internal/storage"
)

const serverSecretKey = "server_secret"

type Service struct {
	store  *storage.Store
	secret []byte
	ttl    time.Duration
}

type BootstrapResult struct {
	Created      bool   `json:"created"`
	Username     string `json:"username"`
	Password     string `json:"password,omitempty"`
	ServerSecret string `json:"server_secret,omitempty"`
}

func New(ctx context.Context, store *storage.Store, ttl time.Duration) (*Service, error) {
	secret, err := store.GetSetting(ctx, serverSecretKey)
	if errors.Is(err, sql.ErrNoRows) {
		secret, err = config.RandomSecret(32)
		if err == nil {
			err = store.SetSetting(ctx, serverSecretKey, secret)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("load server secret: %w", err)
	}
	return &Service{store: store, secret: []byte(secret), ttl: ttl}, nil
}

func (s *Service) Bootstrap(ctx context.Context, username, password string) (BootstrapResult, error) {
	if username == "" {
		username = "admin"
	}
	var count int
	if err := s.store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM admins").Scan(&count); err != nil {
		return BootstrapResult{}, err
	}
	if count > 0 {
		return BootstrapResult{Username: username}, nil
	}
	generated := password == ""
	if generated {
		var err error
		password, err = config.RandomSecret(12)
		if err != nil {
			return BootstrapResult{}, err
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return BootstrapResult{}, err
	}
	_, err = s.store.DB().ExecContext(ctx, `INSERT INTO admins(id, username, password_hash, created_at)
        VALUES(?, ?, ?, ?)`, uuid.NewString(), username, string(hash), time.Now().UTC())
	if err != nil {
		return BootstrapResult{}, err
	}
	result := BootstrapResult{Created: true, Username: username, ServerSecret: string(s.secret)}
	if generated {
		result.Password = password
	}
	return result, nil
}

func (s *Service) Login(ctx context.Context, username, password string) (string, error) {
	var id, hash string
	err := s.store.DB().QueryRowContext(ctx,
		"SELECT id, password_hash FROM admins WHERE username = ?", username).Scan(&id, &hash)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return "", errors.New("invalid username or password")
	}
	now := time.Now()
	claims := jwt.MapClaims{"sub": id, "username": username, "iat": now.Unix(), "exp": now.Add(s.ttl).Unix()}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

func (s *Service) VerifyJWT(value string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(value, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return s.secret, nil
	})
	if err != nil || !token.Valid {
		return nil, errors.New("invalid session")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid session claims")
	}
	return claims, nil
}

func HashToken(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func TokenParts(value string) (string, string, error) {
	parts := strings.SplitN(value, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("invalid agent token")
	}
	return parts[0], parts[1], nil
}
