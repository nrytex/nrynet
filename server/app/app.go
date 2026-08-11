package app

import (
	"context"
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
	"github.com/nrytex/nrynet/server/visitor"
)

type App struct {
	config    config.Config
	store     *storage.Store
	server    *http.Server
	control   net.Listener
	plain     *http.Server
	plainCtrl net.Listener
	data      net.Listener
	plainData net.Listener
	web       net.Listener
	broker    *relay.Broker
	gateway   *gateway.Gateway
	tunnels   *tunnel.Manager
	visitor   *visitor.Service
	quic      *serveradvanced.QUICControlServer
	rdv       *serveradvanced.RendezvousService
	relayDone chan struct{}
	transport *TransportController
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
	tunnelManager.SetP2PEnabled(cfg.Server.P2PEnabled)
	registry := netx.NewRelayRegistry(cfg.Server.HeartbeatTimeout)
	relayToken := cfg.Server.RelayAPIToken
	binder := &serveradvanced.RemoteRelayNode{Token: relayToken, BrokerAddress: cfg.Server.PublicDataAddress, BrokerTLS: cfg.Server.TLS.Enabled, BrokerServerName: publicDataHostname(cfg.Server.PublicDataAddress), Registry: registry}
	tunnelManager.SetRelayRegistry(registry, binder)
	broker.SetRelayVisitorHandler(relayToken, tunnelManager.RouteRelayVisitor)
	visitorService := visitor.New(store, hub, cfg.Server.WebRTCICEServers)
	tlsStore, err := newTLSStore(cfg.Server.TLS)
	if err != nil {
		store.Close()
		return nil, auth.BootstrapResult{}, err
	}
	dataListener, err := listenData(cfg.Server.DataListen, tlsStore)
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
	transport := newTransportController(nil, tlsStore, nil)
	router := api.NewRouterWithOptions(store, authService, time.Now(), api.RouterOptions{
		Runtime: tunnelManager, Settings: safeSettings(cfg), SettingApplier: tunnelManager,
		RelayRegistry: registry, RelayToken: relayToken, CertificatePinProvider: transport.CurrentCertificatePin,
		Transport: transport,
	})
	router.GET("/agent/connect", hub.Handle)
	router.GET("/visitor/:id/:token", visitorService.ServePage)
	router.GET("/visitor/webrtc/:id/:token", visitorService.ServeSignal)
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
	}
	controlListener, err := listenControl(cfg.Server.Listen, tlsStore)
	if err != nil {
		_ = dataListener.Close()
		_ = closeOptionalListener(plainDataListener)
		_ = webListener.Close()
		store.Close()
		return nil, auth.BootstrapResult{}, err
	}
	plainServer, plainControlListener, err := listenPlainControl(cfg.Server, router)
	if err != nil {
		_ = controlListener.Close()
		_ = dataListener.Close()
		_ = closeOptionalListener(plainDataListener)
		_ = webListener.Close()
		store.Close()
		return nil, auth.BootstrapResult{}, err
	}
	quicServer, err := listenQUIC(cfg.Server, tlsStore, authService, hub, broker)
	if err != nil {
		_ = controlListener.Close()
		_ = dataListener.Close()
		_ = closeOptionalListener(plainDataListener)
		_ = closeOptionalListener(plainControlListener)
		_ = webListener.Close()
		store.Close()
		return nil, auth.BootstrapResult{}, err
	}
	rdv, err := serveradvanced.ListenRendezvous(cfg.Server.RendezvousListen)
	if err != nil {
		_ = controlListener.Close()
		_ = dataListener.Close()
		_ = closeOptionalListener(plainDataListener)
		_ = closeOptionalListener(plainControlListener)
		_ = webListener.Close()
		_ = quicServer.Close()
		store.Close()
		return nil, auth.BootstrapResult{}, err
	}
	app := &App{config: cfg, store: store, server: server, control: controlListener, plain: plainServer, plainCtrl: plainControlListener,
		data: dataListener, plainData: plainDataListener, web: webListener,
		broker: broker, gateway: webGateway, tunnels: tunnelManager, quic: quicServer, rdv: rdv, relayDone: make(chan struct{})}
	app.visitor = visitorService
	transport.bind(app, router)
	app.transport = transport
	return app, bootstrap, nil
}

func listenQUIC(
	cfg config.ServerConfig,
	tlsStore *netx.DynamicTLSStore,
	authService *auth.Service,
	hub *clienthub.Hub,
	broker *relay.Broker,
) (*serveradvanced.QUICControlServer, error) {
	return serveradvanced.ListenQUIC(cfg.QUICListen, netx.QUICServerDynamicTLSConfig(tlsStore), authService, hub, broker)
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
	go a.transport.monitorCertificates(a.relayDone)
	go reportServeError(errCh, "control server", func() error { return a.server.Serve(a.control) })
	select {
	case err := <-errCh:
		return err
	case <-a.relayDone:
		return nil
	}
}

func (a *App) Shutdown(ctx context.Context) error {
	select {
	case <-a.relayDone:
	default:
		close(a.relayDone)
	}
	serverErr := a.server.Shutdown(ctx)
	controlErr := closeOptionalListener(a.control)
	plainErr := shutdownOptionalServer(ctx, a.plain)
	plainCtrlErr := closeOptionalListener(a.plainCtrl)
	dataErr := a.data.Close()
	plainDataErr := closeOptionalListener(a.plainData)
	webErr := a.web.Close()
	quicErr := a.quic.Close()
	rdvErr := a.rdv.Close()
	tunnelErr := a.tunnels.Close()
	a.visitor.Close()
	storeErr := a.store.Close()
	return errors.Join(serverErr, controlErr, plainErr, plainCtrlErr, dataErr, plainDataErr, webErr, quicErr, rdvErr, tunnelErr, storeErr)
}

func reportServeError(errCh chan<- error, name string, serve func() error) {
	if err := serve(); err != nil && !isNormalServeClose(err) {
		errCh <- fmt.Errorf("%s: %w", name, err)
	}
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

func (a *App) TransportController() *TransportController {
	return a.transport
}
