package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/nat-link/nat-link/internal/auth"
	"github.com/nat-link/nat-link/internal/config"
	"github.com/nat-link/nat-link/internal/storage"
	"github.com/nat-link/nat-link/server/api"
)

type App struct {
	config config.Config
	store  *storage.Store
	server *http.Server
}

func New(ctx context.Context, cfg config.Config) (*App, auth.BootstrapResult, error) {
	store, err := storage.Open(cfg.Server.Database)
	if err != nil {
		return nil, auth.BootstrapResult{}, err
	}
	authService, err := auth.New(ctx, store, cfg.Server.JWTTTL)
	if err != nil {
		store.Close()
		return nil, auth.BootstrapResult{}, err
	}
	bootstrap, err := authService.Bootstrap(ctx, cfg.Server.Bootstrap.AdminUsername, cfg.Server.Bootstrap.AdminPassword)
	if err != nil {
		store.Close()
		return nil, auth.BootstrapResult{}, err
	}
	router := api.NewRouter(store, authService, time.Now())
	server := &http.Server{Addr: cfg.Server.Listen, Handler: router, ReadHeaderTimeout: 10 * time.Second}
	return &App{config: cfg, store: store, server: server}, bootstrap, nil
}

func (a *App) Run() error {
	var err error
	if a.config.Server.TLS.Enabled {
		err = a.server.ListenAndServeTLS(a.config.Server.TLS.CertFile, a.config.Server.TLS.KeyFile)
	} else {
		err = a.server.ListenAndServe()
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (a *App) Shutdown(ctx context.Context) error {
	serverErr := a.server.Shutdown(ctx)
	storeErr := a.store.Close()
	return errors.Join(serverErr, storeErr)
}

func (a *App) Address() string {
	return fmt.Sprintf("http://%s", a.config.Server.Listen)
}
