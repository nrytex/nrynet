package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	netx "github.com/nrytex/nrynet/internal/advanced"
	"github.com/nrytex/nrynet/internal/auth"
	"github.com/nrytex/nrynet/internal/config"
	"github.com/nrytex/nrynet/internal/storage"
	serveradvanced "github.com/nrytex/nrynet/server/advanced"
	"github.com/nrytex/nrynet/server/api"
	clienthub "github.com/nrytex/nrynet/server/client"
	"github.com/nrytex/nrynet/server/dashboard"
	"github.com/nrytex/nrynet/server/gateway"
	"github.com/nrytex/nrynet/server/relay"
	"github.com/nrytex/nrynet/server/tunnel"
)

type App struct {
	config    config.Config
	store     *storage.Store
	server    *http.Server
	plain     *http.Server
	plainCtrl net.Listener
	data      net.Listener
	plainData net.Listener
	web       net.Listener
	broker    *relay.Broker
	gateway   *gateway.Gateway
	tunnels   *tunnel.Manager
	quic      *serveradvanced.QUICControlServer
	rdv       *serveradvanced.RendezvousService
	relayDone chan struct{}
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
	if err := config.ValidateServerTransport(cfg.Server); err != nil {
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
	tunnelManager.SetRendezvousAddress(cfg.Server.PublicRendezvous)
	registry := netx.NewRelayRegistry(cfg.Server.HeartbeatTimeout)
	relayToken := cfg.Server.RelayAPIToken
	binder := &serveradvanced.RemoteRelayNode{Token: relayToken, BrokerAddress: cfg.Server.PublicDataAddress, BrokerTLS: cfg.Server.TLS.Enabled, BrokerServerName: publicDataHostname(cfg.Server.PublicDataAddress), Registry: registry}
	tunnelManager.SetRelayRegistry(registry, binder)
	broker.SetRelayVisitorHandler(relayToken, tunnelManager.RouteRelayVisitor)
	dataListener, err := listenData(cfg.Server.DataListen, cfg.Server.TLS)
	if err != nil {
		store.Close()
		return nil, auth.BootstrapResult{}, fmt.Errorf("listen for data connections: %w", err)
	}
	plainDataListener, err := listenPlainData(cfg.Server)
	if err != nil {
		_ = dataListener.Close()
		store.Close()
		return nil, auth.BootstrapResult{}, err
	}
	webListener, err := net.Listen("tcp", cfg.Server.HTTPListen)
	if err != nil {
		_ = dataListener.Close()
		_ = closeOptionalListener(plainDataListener)
		store.Close()
		return nil, auth.BootstrapResult{}, fmt.Errorf("listen for HTTP tunnels: %w", err)
	}
	webGateway := gateway.New(store, tunnelManager)
	certificatePin, err := serverCertificatePin(cfg.Server)
	if err != nil {
		_ = dataListener.Close()
		_ = closeOptionalListener(plainDataListener)
		_ = webListener.Close()
		store.Close()
		return nil, auth.BootstrapResult{}, err
	}
	router := api.NewRouterWithOptions(store, authService, time.Now(), api.RouterOptions{
		Runtime: tunnelManager, Settings: safeSettings(cfg),
		RelayRegistry: registry, RelayToken: relayToken, CertificatePin: certificatePin,
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
	server := &http.Server{
		Addr: cfg.Server.Listen, Handler: router, ReadHeaderTimeout: 10 * time.Second,
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13},
	}
	plainServer, plainControlListener, err := listenPlainControl(cfg.Server, router)
	if err != nil {
		_ = dataListener.Close()
		_ = closeOptionalListener(plainDataListener)
		_ = webListener.Close()
		store.Close()
		return nil, auth.BootstrapResult{}, err
	}
	quicServer, err := listenQUIC(cfg.Server, authService, hub, broker)
	if err != nil {
		_ = dataListener.Close()
		_ = closeOptionalListener(plainDataListener)
		_ = closeOptionalListener(plainControlListener)
		_ = webListener.Close()
		store.Close()
		return nil, auth.BootstrapResult{}, err
	}
	rdv, err := serveradvanced.ListenRendezvous(cfg.Server.RendezvousListen)
	if err != nil {
		_ = dataListener.Close()
		_ = closeOptionalListener(plainDataListener)
		_ = closeOptionalListener(plainControlListener)
		_ = webListener.Close()
		_ = quicServer.Close()
		store.Close()
		return nil, auth.BootstrapResult{}, err
	}
	return &App{config: cfg, store: store, server: server, plain: plainServer, plainCtrl: plainControlListener,
		data: dataListener, plainData: plainDataListener, web: webListener,
		broker: broker, gateway: webGateway, tunnels: tunnelManager, quic: quicServer, rdv: rdv, relayDone: make(chan struct{})}, bootstrap, nil
}

func listenQUIC(
	cfg config.ServerConfig,
	authService *auth.Service,
	hub *clienthub.Hub,
	broker *relay.Broker,
) (*serveradvanced.QUICControlServer, error) {
	certificate, err := quicCertificate(cfg)
	if err != nil {
		return nil, err
	}
	return serveradvanced.ListenQUIC(cfg.QUICListen, netx.ServerTLSConfig(certificate), authService, hub, broker)
}

func quicCertificate(cfg config.ServerConfig) (tls.Certificate, error) {
	if !cfg.TLS.Enabled {
		return netx.SelfSignedCertificate()
	}
	cert, err := tls.LoadX509KeyPair(cfg.TLS.CertFile, cfg.TLS.KeyFile)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("load QUIC TLS certificate: %w", err)
	}
	return cert, nil
}

func (a *App) Run() error {
	if err := a.tunnels.Restore(context.Background()); err != nil {
		fmt.Printf("restore tunnels: %v\n", err)
	}
	errCh := make(chan error, 8)
	go reportServeError(errCh, "data listener", func() error { return a.broker.Run(a.data) })
	if a.plainData != nil {
		go reportServeError(errCh, "plaintext data listener", func() error { return a.broker.Run(a.plainData) })
	}
	go reportServeError(errCh, "HTTP tunnel gateway", func() error { return a.gateway.Run(a.web) })
	go reportServeError(errCh, "QUIC listener", func() error { return a.quic.Serve(context.Background()) })
	go reportServeError(errCh, "rendezvous listener", func() error { return a.rdv.Run(context.Background()) })
	if a.plain != nil {
		go reportServeError(errCh, "plaintext control server", func() error { return a.plain.Serve(a.plainCtrl) })
	}
	go a.monitorRelays()
	if a.config.Server.TLS.Enabled {
		go reportServeError(errCh, "control server", func() error {
			return a.server.ListenAndServeTLS(a.config.Server.TLS.CertFile, a.config.Server.TLS.KeyFile)
		})
	} else {
		go reportServeError(errCh, "control server", a.server.ListenAndServe)
	}
	return <-errCh
}

func (a *App) Shutdown(ctx context.Context) error {
	select {
	case <-a.relayDone:
	default:
		close(a.relayDone)
	}
	serverErr := a.server.Shutdown(ctx)
	plainErr := shutdownOptionalServer(ctx, a.plain)
	plainCtrlErr := closeOptionalListener(a.plainCtrl)
	dataErr := a.data.Close()
	plainDataErr := closeOptionalListener(a.plainData)
	webErr := a.web.Close()
	quicErr := a.quic.Close()
	rdvErr := a.rdv.Close()
	tunnelErr := a.tunnels.Close()
	storeErr := a.store.Close()
	return errors.Join(serverErr, plainErr, plainCtrlErr, dataErr, plainDataErr, webErr, quicErr, rdvErr, tunnelErr, storeErr)
}

func reportServeError(errCh chan<- error, name string, serve func() error) {
	if err := serve(); err != nil && !isNormalServeClose(err) {
		errCh <- fmt.Errorf("%s: %w", name, err)
		return
	}
	errCh <- nil
}

func isNormalServeClose(err error) bool {
	return errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed)
}

func (a *App) monitorRelays() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-a.relayDone:
			return
		case <-ticker.C:
			a.tunnels.ReassignUnhealthyRelayTunnels(context.Background())
			a.tunnels.AssignAvailableRelayTunnels(context.Background())
		}
	}
}

func (a *App) Address() string {
	scheme := "http"
	if a.config.Server.TLS.Enabled {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, a.config.Server.Listen)
}
