package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	netx "github.com/nrytex/nrynet/internal/advanced"
	"github.com/nrytex/nrynet/internal/config"
)

func listenControl(address string, tlsStore *netx.DynamicTLSStore) (net.Listener, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("listen for control connections: %w", err)
	}
	return newDynamicTLSListener(listener, tlsStore), nil
}

func listenData(address string, tlsStore *netx.DynamicTLSStore) (net.Listener, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("listen for data connections: %w", err)
	}
	return newDynamicTLSListener(listener, tlsStore), nil
}

func listenPlainData(cfg config.ServerConfig) (net.Listener, error) {
	if !plaintextPairEnabled(cfg) {
		return nil, nil
	}
	listener, err := net.Listen("tcp", cfg.PlainDataListen)
	if err != nil {
		return nil, fmt.Errorf("listen for plaintext data connections: %w", err)
	}
	return listener, nil
}

func listenPlainControl(cfg config.ServerConfig, handler http.Handler) (*http.Server, net.Listener, error) {
	if !plaintextPairEnabled(cfg) {
		return nil, nil, nil
	}
	listener, err := net.Listen("tcp", cfg.PlainListen)
	if err != nil {
		return nil, nil, fmt.Errorf("listen for plaintext control connections: %w", err)
	}
	server := &http.Server{Addr: cfg.PlainListen, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	return server, listener, nil
}

func closeOptionalListener(listener net.Listener) error {
	if listener == nil {
		return nil
	}
	if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}
	return nil
}

func shutdownOptionalServer(ctx context.Context, server *http.Server) error {
	if server == nil {
		return nil
	}
	return server.Shutdown(ctx)
}

func publicDataHostname(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err == nil {
		return host
	}
	return address
}

func plaintextPairEnabled(cfg config.ServerConfig) bool {
	return cfg.PlainEnabled && strings.TrimSpace(cfg.PlainListen) != "" && strings.TrimSpace(cfg.PlainDataListen) != ""
}
