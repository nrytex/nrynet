package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

func ValidateServerTransport(cfg ServerConfig) error {
	if strings.TrimSpace(cfg.Listen) == "" || strings.TrimSpace(cfg.DataListen) == "" {
		return errors.New("server.listen and server.data_listen are required")
	}
	if cfg.TLS.Enabled {
		if strings.TrimSpace(cfg.TLS.CertFile) == "" || strings.TrimSpace(cfg.TLS.KeyFile) == "" {
			return errors.New("server TLS certificate and key are required")
		}
	}
	if err := validatePlaintextListeners(cfg); err != nil {
		return err
	}
	if cfg.TLS.Enabled || hasPlaintextListeners(cfg) {
		return nil
	}
	return errors.New("server must expose TLS listeners or plaintext listeners")
}

func validatePlaintextListeners(cfg ServerConfig) error {
	if !cfg.PlainEnabled {
		return nil
	}
	if strings.TrimSpace(cfg.PlainListen) == "" || strings.TrimSpace(cfg.PlainDataListen) == "" {
		return errors.New("server.plain_listen and server.plain_data_listen are required when server.plain_enabled is true")
	}
	if _, _, err := net.SplitHostPort(cfg.PlainListen); err != nil {
		return fmt.Errorf("server.plain_listen must be a host:port address: %w", err)
	}
	if _, _, err := net.SplitHostPort(cfg.PlainDataListen); err != nil {
		return fmt.Errorf("server.plain_data_listen must be a host:port address: %w", err)
	}
	return nil
}

func ValidateSecureWebSocketURL(rawURL, dataAddress string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return errors.New("client.server_url must be a valid WebSocket URL")
	}
	if parsed.Scheme == "ws" || parsed.Scheme == "wss" {
		return nil
	}
	return errors.New("client.server_url must use ws or wss")
}

func ValidateSecureHTTPURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return errors.New("URL is invalid")
	}
	if parsed.Scheme == "https" || (parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
		return nil
	}
	return errors.New("remote control URLs require HTTPS")
}

func ValidateTLSVerification(cfg ClientConfig) error {
	if !cfg.InsecureSkipVerify {
		return nil
	}
	if cfg.Transport == "quic" && IsLoopbackAddress(cfg.QUICAddress) {
		return nil
	}
	parsed, err := url.Parse(cfg.ServerURL)
	if cfg.Transport != "quic" && err == nil && parsed.Scheme == "ws" {
		return nil
	}
	if cfg.Transport != "quic" && err == nil && isLoopbackHost(parsed.Hostname()) && IsLoopbackAddress(cfg.DataAddress) {
		return nil
	}
	return errors.New("client.insecure_skip_verify may be enabled only for loopback development")
}

func IsLoopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	return err == nil && isLoopbackHost(host)
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(host, "[]")
	return strings.EqualFold(host, "localhost") || net.ParseIP(host).IsLoopback()
}

func hasPlaintextListeners(cfg ServerConfig) bool {
	return strings.TrimSpace(cfg.Listen) != "" && strings.TrimSpace(cfg.DataListen) != ""
}
