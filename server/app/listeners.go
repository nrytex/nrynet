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

	"github.com/nrytex/nrynet/internal/config"
)

func listenData(address string, tlsConfig config.TLSConfig) (net.Listener, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("listen for data connections: %w", err)
	}
	if !tlsConfig.Enabled {
		return listener, nil
	}
	certificate, err := tls.LoadX509KeyPair(tlsConfig.CertFile, tlsConfig.KeyFile)
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("load data TLS certificate: %w", err)
	}
	return tls.NewListener(listener, &tls.Config{
		Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS13,
	}), nil
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

func listenPlainControl(address string, handler http.Handler) (*http.Server, net.Listener, error) {
	if strings.TrimSpace(address) == "" {
		return nil, nil, nil
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, nil, fmt.Errorf("listen for plaintext control connections: %w", err)
	}
	server := &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
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

func normalizePlaintextPair(cfg *config.ServerConfig) {
	if plaintextPairEnabled(*cfg) {
		return
	}
	cfg.PlainListen = ""
	cfg.PlainDataListen = ""
}

func plaintextPairEnabled(cfg config.ServerConfig) bool {
	return strings.TrimSpace(cfg.PlainListen) != "" && strings.TrimSpace(cfg.PlainDataListen) != ""
}
