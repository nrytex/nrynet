package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

func ValidateServerTransport(cfg ServerConfig) error {
	if cfg.TLS.Enabled {
		if strings.TrimSpace(cfg.TLS.CertFile) == "" || strings.TrimSpace(cfg.TLS.KeyFile) == "" {
			return errors.New("server TLS certificate and key are required")
		}
		return nil
	}
	for name, address := range map[string]string{
		"server.listen":      cfg.Listen,
		"server.data_listen": cfg.DataListen,
	} {
		if !IsLoopbackAddress(address) {
			return fmt.Errorf("%s may use plaintext only on loopback; configure server.tls for remote access", name)
		}
	}
	return nil
}

func ValidateSecureWebSocketURL(rawURL, dataAddress string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return errors.New("client.server_url must be a valid WebSocket URL")
	}
	if parsed.Scheme == "wss" {
		return nil
	}
	if parsed.Scheme != "ws" || !isLoopbackHost(parsed.Hostname()) || !IsLoopbackAddress(dataAddress) {
		return errors.New("remote agent connections require wss and TLS data transport")
	}
	return nil
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
