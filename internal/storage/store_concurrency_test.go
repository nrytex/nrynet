package storage

import (
	"path/filepath"
	"testing"
)

func TestFileStoreUsesWALAndConcurrentConnections(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "concurrent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if got := store.DB().Stats().MaxOpenConnections; got != sqliteMaxOpenConnections {
		t.Fatalf("MaxOpenConnections=%d, want %d", got, sqliteMaxOpenConnections)
	}
	var mode string
	if err := store.DB().QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode=%q, want wal", mode)
	}
}

func TestMemoryStoreKeepsSingleConnection(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if got := store.DB().Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("MaxOpenConnections=%d, want 1", got)
	}
}
