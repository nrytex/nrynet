package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

func TestAutoSubdomainConfigReadsAtomicSnapshots(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "auto-domain.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first := AutoSubdomainConfig{Enabled: true, BaseDomain: "first.example.com"}
	second := AutoSubdomainConfig{Enabled: false, BaseDomain: "second.example.com"}
	if err := store.SetAutoSubdomainConfig(ctx, first); err != nil {
		t.Fatal(err)
	}

	errors := make(chan error, 4)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			cfg := first
			if i%2 == 1 {
				cfg = second
			}
			if err := store.SetAutoSubdomainConfig(ctx, cfg); err != nil {
				errors <- err
				return
			}
		}
	}()
	for reader := 0; reader < 3; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				cfg, err := store.AutoSubdomainConfig(ctx)
				if err != nil {
					errors <- err
					return
				}
				if cfg != first && cfg != second {
					errors <- fmt.Errorf("torn auto-subdomain config: %+v", cfg)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
}
