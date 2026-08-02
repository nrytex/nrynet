package app

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nat-link/nat-link/internal/auth"
	"github.com/nat-link/nat-link/internal/config"
	"github.com/nat-link/nat-link/internal/storage"
	"github.com/nat-link/nat-link/server/api"
	clienthub "github.com/nat-link/nat-link/server/client"
	"github.com/nat-link/nat-link/server/dashboard"
	"github.com/nat-link/nat-link/server/gateway"
	"github.com/nat-link/nat-link/server/relay"
	"github.com/nat-link/nat-link/server/tunnel"
)

type App struct {
	config  config.Config
	store   *storage.Store
	server  *http.Server
	data    net.Listener
	web     net.Listener
	broker  *relay.Broker
	gateway *gateway.Gateway
	tunnels *tunnel.Manager
}

func New(ctx context.Context, cfg config.Config) (*App, auth.BootstrapResult, error) {
	store, err := storage.Open(cfg.Server.Database)
	if err != nil {
		return nil, auth.BootstrapResult{}, err
	}
	if err := applyStoredSettings(ctx, store, &cfg); err != nil {
		store.Close()
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
	webListener, err := net.Listen("tcp", cfg.Server.HTTPListen)
	if err != nil {
		_ = dataListener.Close()
		store.Close()
		return nil, auth.BootstrapResult{}, fmt.Errorf("listen for HTTP tunnels: %w", err)
	}
	webGateway := gateway.New(store, tunnelManager)
	router := api.NewRouterWithOptions(store, authService, time.Now(), api.RouterOptions{
		Runtime: tunnelManager, Settings: safeSettings(cfg),
	})
	router.GET("/agent/connect", hub.Handle)
	dashboardHandler := dashboard.Handler()
	router.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") || strings.HasPrefix(c.Request.URL.Path, "/agent/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
			return
		}
		dashboardHandler.ServeHTTP(c.Writer, c.Request)
	})
	server := &http.Server{Addr: cfg.Server.Listen, Handler: router, ReadHeaderTimeout: 10 * time.Second}
	return &App{config: cfg, store: store, server: server, data: dataListener, web: webListener,
		broker: broker, gateway: webGateway, tunnels: tunnelManager}, bootstrap, nil
}

func safeSettings(cfg config.Config) []api.SettingItem {
	return []api.SettingItem{
		{Key: "server.listen", Value: cfg.Server.Listen, Description: "Dashboard and control API address; restart required", Mutable: true},
		{Key: "server.data_listen", Value: cfg.Server.DataListen, Description: "TCP relay data address; restart required", Mutable: true},
		{Key: "server.http_listen", Value: cfg.Server.HTTPListen, Description: "HTTP and HTTPS gateway; restart required", Mutable: true},
		{Key: "server.tls.enabled", Value: cfg.Server.TLS.Enabled, Description: "Configured through YAML certificate settings"},
		{Key: "server.heartbeat_timeout", Value: cfg.Server.HeartbeatText, Description: "Agent offline timeout; restart required", Mutable: true},
	}
}

func applyStoredSettings(ctx context.Context, store *storage.Store, cfg *config.Config) error {
	stringSettings := []struct {
		key    string
		target *string
	}{
		{"server.listen", &cfg.Server.Listen},
		{"server.data_listen", &cfg.Server.DataListen},
		{"server.http_listen", &cfg.Server.HTTPListen},
		{"server.heartbeat_timeout", &cfg.Server.HeartbeatText},
	}
	for _, setting := range stringSettings {
		value, err := store.GetSetting(ctx, "config."+setting.key)
		if err == nil {
			*setting.target = value
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	duration, err := time.ParseDuration(cfg.Server.HeartbeatText)
	if err != nil {
		return fmt.Errorf("stored heartbeat timeout: %w", err)
	}
	cfg.Server.HeartbeatTimeout = duration
	if value, err := store.GetSetting(ctx, "config.server.tls.enabled"); err == nil {
		cfg.Server.TLS.Enabled, err = strconv.ParseBool(value)
		return err
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return nil
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
	go func() { _ = a.gateway.Run(a.web) }()
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
	webErr := a.web.Close()
	tunnelErr := a.tunnels.Close()
	storeErr := a.store.Close()
	return errors.Join(serverErr, dataErr, webErr, tunnelErr, storeErr)
}

func (a *App) Address() string {
	return fmt.Sprintf("http://%s", a.config.Server.Listen)
}
