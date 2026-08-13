package agent

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	netx "github.com/nrytex/nrynet/internal/advanced"
)

const controlDialTimeout = 10 * time.Second

func (a *Agent) dialControl(ctx context.Context) (controlConn, error) {
	if a.options.Config.Transport != "quic" || a.webSocketFallbackEnabled() {
		return a.dialWebSocketControl(ctx)
	}
	return a.dialControlWith(ctx, a.dialQUICControl, a.dialWebSocketControl)
}

func (a *Agent) dialControlWith(
	ctx context.Context,
	quicDial func(context.Context) (controlConn, error),
	webSocketDial func(context.Context) (controlConn, error),
) (controlConn, error) {
	logger := a.logger
	if logger == nil {
		logger = slog.Default()
	}
	quicCtx, cancel := context.WithTimeout(ctx, controlDialTimeout)
	conn, quicErr := quicDial(quicCtx)
	cancel()
	if quicErr == nil {
		a.disableWebSocketFallback()
		return conn, nil
	}
	if ctx.Err() != nil {
		return nil, fmt.Errorf("dial QUIC control: %w", quicErr)
	}
	a.enableWebSocketFallback()
	logger.Warn("QUIC control unavailable; using WebSocket control", "error", quicErr.Error())
	conn, webSocketErr := webSocketDial(ctx)
	if webSocketErr != nil {
		return nil, fmt.Errorf("QUIC control unavailable: %s; WebSocket control fallback failed: %w", quicErr.Error(), webSocketErr)
	}
	return conn, nil
}

func (a *Agent) webSocketFallbackEnabled() bool {
	a.controlMu.Lock()
	defer a.controlMu.Unlock()
	if a.webSocketFallback && !a.webSocketFallbackUntil.IsZero() && !time.Now().Before(a.webSocketFallbackUntil) {
		a.webSocketFallback = false
	}
	return a.webSocketFallback
}

func (a *Agent) enableWebSocketFallback() {
	a.controlMu.Lock()
	a.webSocketFallback = true
	a.webSocketFallbackUntil = time.Now().Add(30 * time.Second)
	a.controlMu.Unlock()
}

func (a *Agent) disableWebSocketFallback() {
	a.controlMu.Lock()
	a.webSocketFallback = false
	a.webSocketFallbackUntil = time.Time{}
	a.controlMu.Unlock()
}

func (a *Agent) markWebSocketFallback(conn controlConn, reason error) {
	if _, ok := conn.(*quicControl); !ok {
		return
	}
	a.enableWebSocketFallback()
	logger := a.logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Warn("QUIC control session ended; using WebSocket control on reconnect", "error", reason.Error())
}

func (a *Agent) dialWebSocketControl(ctx context.Context) (controlConn, error) {
	tlsConfig, err := a.webSocketTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("prepare WebSocket control TLS: %w", err)
	}
	dialer := websocket.Dialer{HandshakeTimeout: controlDialTimeout, TLSClientConfig: tlsConfig}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+a.options.Config.Token)
	header.Set("X-Nrynet-Device-ID", a.options.Config.DeviceID)
	header.Set("X-NAT-Link-Device-ID", a.options.Config.DeviceID)
	conn, _, err := dialer.DialContext(ctx, a.options.Config.ServerURL, header)
	if err != nil {
		return nil, fmt.Errorf("dial WebSocket control: %w", err)
	}
	logger := a.logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Debug("WebSocket control connected", "server_url", a.options.Config.ServerURL)
	return &websocketControl{conn: conn, agent: a}, nil
}

func (a *Agent) dialQUICControl(ctx context.Context) (controlConn, error) {
	tlsConfig, err := secureClientTLS(
		quicServerName(a.options.Config.QUICAddress), a.options.Config, netx.QUICALPN,
	)
	if err != nil {
		return nil, fmt.Errorf("prepare QUIC control TLS: %w", err)
	}
	session, err := netx.DialQUIC(ctx, a.options.Config.QUICAddress, tlsConfig, netx.AuthRequest{
		Token: a.options.Config.Token, DeviceID: a.options.Config.DeviceID, Role: "agent",
	})
	if err != nil {
		return nil, fmt.Errorf("dial QUIC control session: %w", err)
	}
	stream, err := session.OpenStream(ctx, netx.FrameControl)
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("open QUIC control stream: %w", err)
	}
	return &quicControl{session: session, stream: stream, agent: a}, nil
}

func quicServerName(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err != nil || host == "" {
		return "localhost"
	}
	return host
}

func (a *Agent) webSocketTLSConfig() (*tls.Config, error) {
	if !strings.HasPrefix(strings.ToLower(a.options.Config.ServerURL), "wss://") {
		return nil, nil
	}
	parsed, err := url.Parse(a.options.Config.ServerURL)
	if err != nil {
		return nil, err
	}
	return secureClientTLS(parsed.Hostname(), a.options.Config)
}
