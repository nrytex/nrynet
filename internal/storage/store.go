package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

const sqliteMaxOpenConnections = 8

func Open(path string) (*Store, error) {
	dsn := sqliteDSN(path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if !isMemoryDatabase(path) {
		if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
			db.Close()
			return nil, fmt.Errorf("enable sqlite WAL: %w", err)
		}
		db.SetMaxOpenConns(sqliteMaxOpenConnections)
		db.SetMaxIdleConns(sqliteMaxOpenConnections)
	}
	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func sqliteDSN(path string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
}

func isMemoryDatabase(path string) bool {
	value := strings.ToLower(path)
	return value == ":memory:" || strings.Contains(value, "mode=memory")
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range migrations {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("database migration: %w", err)
		}
	}
	if err := ensureVisitorTokenColumn(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func ensureVisitorTokenColumn(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, "PRAGMA table_info(tunnels)")
	if err != nil {
		return fmt.Errorf("inspect tunnels schema: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return fmt.Errorf("read tunnels schema: %w", err)
		}
		if name == "visitor_token" {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read tunnels schema: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "ALTER TABLE tunnels ADD COLUMN visitor_token TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("migrate visitor token: %w", err)
	}
	return nil
}

func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	return value, err
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES(?, ?)
        ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}
