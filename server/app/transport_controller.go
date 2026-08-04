package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	netx "github.com/nrytex/nrynet/internal/advanced"
	"github.com/nrytex/nrynet/internal/config"
	"github.com/nrytex/nrynet/server/api"
	"github.com/nrytex/nrynet/server/certbothelper"
)

type TransportController struct {
	app                *App
	tlsStore           *netx.DynamicTLSStore
	handler            http.Handler
	mu                 sync.Mutex
	certbot            certbothelper.Options
	lastJob            time.Time
	certTime           time.Time
	pendingCertificate *api.CertificateStatus
	pendingSince       time.Time
}

func newTLSStore(cfg config.TLSConfig) (*netx.DynamicTLSStore, error) {
	store := netx.NewDynamicTLSStore()
	if cfg.Enabled {
		if err := store.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile); err != nil {
			return nil, err
		}
	} else if tlsPairExists(cfg.CertFile, cfg.KeyFile) {
		_ = store.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	}
	if err := store.SetEnabled(cfg.Enabled); err != nil {
		return nil, err
	}
	return store, nil
}

func tlsPairExists(certFile, keyFile string) bool {
	if certFile == "" || keyFile == "" {
		return false
	}
	if _, err := os.Stat(certFile); err != nil {
		return false
	}
	if _, err := os.Stat(keyFile); err != nil {
		return false
	}
	return true
}

func newTransportController(app *App, tlsStore *netx.DynamicTLSStore, handler http.Handler) *TransportController {
	return &TransportController{app: app, tlsStore: tlsStore, handler: handler}
}

func (c *TransportController) bind(app *App, handler http.Handler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.app = app
	c.handler = handler
	c.certbot = certbothelper.OptionsForInstallDir(installDirFromDatabase(app.config.Server.Database))
	if info, err := os.Stat(app.config.Server.TLS.CertFile); err == nil {
		c.certTime = info.ModTime()
	}
	if value, err := app.store.GetSetting(context.Background(), "config.server.tls.certbot_applied_at"); err == nil {
		c.lastJob, _ = time.Parse(time.RFC3339Nano, value)
	}
}

func (c *TransportController) Status(context.Context) (api.TransportStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.statusLocked(), nil
}

func (c *TransportController) CurrentCertificatePin() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.app == nil || !c.tlsStore.Enabled() {
		return ""
	}
	pin, err := serverCertificatePin(c.app.config.Server)
	if err != nil {
		return ""
	}
	return pin
}

func (c *TransportController) SetTLSEnabled(_ context.Context, enabled bool) (api.TransportStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	previous := c.tlsStore.Enabled()
	if err := c.tlsStore.SetEnabled(enabled); err != nil {
		return c.statusLocked(), err
	}
	c.app.config.Server.TLS.Enabled = enabled
	if err := c.persistSetting("server.tls.enabled", boolText(enabled)); err != nil {
		_ = c.tlsStore.SetEnabled(previous)
		c.app.config.Server.TLS.Enabled = previous
		return c.statusLocked(), err
	}
	return c.statusLocked(), nil
}

func (c *TransportController) ReloadCertificate(_ context.Context, certFile, keyFile string) (api.TransportStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.tlsStore.LoadX509KeyPair(certFile, keyFile); err != nil {
		return c.statusLocked(), err
	}
	c.app.config.Server.TLS.CertFile = certFile
	c.app.config.Server.TLS.KeyFile = keyFile
	return c.statusLocked(), nil
}

func (c *TransportController) SetPlainEnabled(ctx context.Context, enabled bool) (api.TransportStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	previous := c.app.config.Server.PlainEnabled
	var status api.TransportStatus
	var err error
	if enabled {
		status, err = c.startPlainLocked()
	} else {
		status, err = c.stopPlainLocked(ctx)
	}
	if err != nil {
		return status, err
	}
	if err := c.persistSetting("server.plain_enabled", boolText(enabled)); err != nil {
		if previous != enabled {
			if previous {
				_, _ = c.startPlainLocked()
			} else {
				_, _ = c.stopPlainLocked(context.Background())
			}
		}
		return c.statusLocked(), err
	}
	return c.statusLocked(), nil
}

func (c *TransportController) startPlainLocked() (api.TransportStatus, error) {
	if c.app.plain != nil || c.app.plainData != nil {
		c.app.config.Server.PlainEnabled = true
		return c.statusLocked(), nil
	}
	if !plaintextPairEnabled(config.ServerConfig{
		PlainEnabled: true, PlainListen: c.app.config.Server.PlainListen,
		PlainDataListen: c.app.config.Server.PlainDataListen,
	}) {
		return c.statusLocked(), errors.New("plaintext control and data addresses are required")
	}
	plainData, err := listenPlainData(enabledPlainConfig(c.app.config.Server))
	if err != nil {
		return c.statusLocked(), err
	}
	plainServer, plainCtrl, err := listenPlainControl(enabledPlainConfig(c.app.config.Server), c.handler)
	if err != nil {
		_ = plainData.Close()
		return c.statusLocked(), err
	}
	c.app.plainData = plainData
	c.app.plain = plainServer
	c.app.plainCtrl = plainCtrl
	c.app.config.Server.PlainEnabled = true
	go func() { _ = c.app.broker.Run(plainData) }()
	go func() { _ = plainServer.Serve(plainCtrl) }()
	return c.statusLocked(), nil
}

func (c *TransportController) stopPlainLocked(ctx context.Context) (api.TransportStatus, error) {
	plainServer := c.app.plain
	if plainServer != nil {
		plainServer.SetKeepAlivesEnabled(false)
	}
	ctrlErr := closeOptionalListener(c.app.plainCtrl)
	dataErr := closeOptionalListener(c.app.plainData)
	c.app.plain = nil
	c.app.plainCtrl = nil
	c.app.plainData = nil
	c.app.config.Server.PlainEnabled = false
	if plainServer != nil {
		go func() { _ = plainServer.Shutdown(ctx) }()
	}
	return c.statusLocked(), errors.Join(ctrlErr, dataErr)
}

func (c *TransportController) statusLocked() api.TransportStatus {
	if c.app == nil {
		return api.TransportStatus{}
	}
	control := listenerAddress(c.app.control, c.app.config.Server.Listen)
	data := listenerAddress(c.app.data, c.app.config.Server.DataListen)
	plainControl := listenerAddress(c.app.plainCtrl, c.app.config.Server.PlainListen)
	plainData := listenerAddress(c.app.plainData, c.app.config.Server.PlainDataListen)
	publicControl := publicAddress(control, c.app.config.Server.PublicDataAddress, "")
	publicData := publicAddress(data, c.app.config.Server.PublicDataAddress, "")
	tlsControl := publicAddress(control, c.app.config.Server.PublicDataAddress, c.app.config.Server.TLS.Domain)
	tlsData := publicAddress(data, c.app.config.Server.PublicDataAddress, c.app.config.Server.TLS.Domain)
	compatControl := publicAddress(plainControl, c.app.config.Server.PublicDataAddress, "")
	compatData := publicAddress(plainData, c.app.config.Server.PublicDataAddress, "")
	certbot := c.certbotStatusLocked()
	return api.TransportStatus{
		Plain: api.TransportEndpoint{
			Enabled: true, Listen: control, DataListen: data,
			ControlURL: "http://" + publicControl, WebSocketURL: "ws://" + publicControl + "/agent/connect",
			DataAddress: publicData,
		},
		CompatibilityPlain: api.TransportEndpoint{
			Enabled: c.app.config.Server.PlainEnabled && (c.app.plain != nil || c.app.plainData != nil),
			Listen:  plainControl, DataListen: plainData, ControlURL: "http://" + compatControl,
			WebSocketURL: "ws://" + compatControl + "/agent/connect", DataAddress: compatData,
		},
		TLS: api.TransportEndpoint{
			Enabled: c.tlsStore.Enabled(), Listen: control, DataListen: data,
			ControlURL: "https://" + tlsControl, WebSocketURL: "wss://" + tlsControl + "/agent/connect",
			DataAddress: tlsData,
		},
		Certbot:      certbot,
		Certificate:  c.certificateStatusLocked(),
		Capabilities: transportCapabilities(certbot),
	}
}

func transportCapabilities(certbot api.CertbotStatus) api.TransportCapabilities {
	return api.TransportCapabilities{
		CertbotAvailable: certbot.Available, CertbotMessage: certbot.Message, HotReload: true,
	}
}

func enabledPlainConfig(cfg config.ServerConfig) config.ServerConfig {
	cfg.PlainEnabled = true
	return cfg
}

func listenerAddress(listener net.Listener, fallback string) string {
	if listener == nil {
		return fallback
	}
	return listener.Addr().String()
}
