package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/nat-link/nat-link/internal/auth"
	"github.com/nat-link/nat-link/internal/config"
	"github.com/nat-link/nat-link/internal/storage"
	"github.com/nat-link/nat-link/server/api"
	clienthub "github.com/nat-link/nat-link/server/client"
	"github.com/nat-link/nat-link/server/relay"
	"github.com/nat-link/nat-link/server/tunnel"
)

type App struct {
	config  config.Config
	store   *storage.Store
	server  *http.Server
	data    net.Listener
	broker  *relay.Broker
	tunnels *tunnel.Manager
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
	hub := clienthub.NewHub(store, authService, cfg.Server.HeartbeatTimeout)
	broker := relay.NewBroker(authService, store, 30*time.Second)
	tunnelManager := tunnel.NewManager(store, hub, broker)
	dataListener, err := listenData(cfg.Server)
	if err != nil {
		store.Close()
		return nil, auth.BootstrapResult{}, fmt.Errorf("listen for data connections: %w", err)
	}
	router := api.NewRouter(store, authService, time.Now(), tunnelManager)
	router.GET("/agent/connect", hub.Handle)
	server := &http.Server{Addr: cfg.Server.Listen, Handler: router, ReadHeaderTimeout: 10 * time.Second}
	return &App{config: cfg, store: store, server: server, data: dataListener,
		broker: broker, tunnels: tunnelManager}, bootstrap, nil
}

func listenData(cfg config.ServerConfig) (net.Listener, error) {
	listener, err := net.Listen("tcp", cfg.DataListen)
	if err != nil {
		return nil, fmt.Errorf("listen for data connections: %w", err)
	}
	if !cfg.TLS.Enabled {
		return listener, nil
	}
	certificate, err := tls.LoadX509KeyPair(cfg.TLS.CertFile, cfg.TLS.KeyFile)
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("load data TLS certificate: %w", err)
	}
	tlsConfig := &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS13}
	return tls.NewListener(listener, tlsConfig), nil
}

func (a *App) Run() error {
	if err := a.tunnels.Restore(context.Background()); err != nil {
		fmt.Printf("restore tunnels: %v\n", err)
	}
	go func() { _ = a.broker.Run(a.data) }()
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
	dataErr := a.data.Close()
	tunnelErr := a.tunnels.Close()
	storeErr := a.store.Close()
	return errors.Join(serverErr, dataErr, tunnelErr, storeErr)
}

func (a *App) Address() string {
	return fmt.Sprintf("http://%s", a.config.Server.Listen)
}
