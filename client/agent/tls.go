package agent

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"os"

	"github.com/nrytex/nrynet/internal/config"
)

func secureClientTLS(serverName string, cfg config.ClientConfig, protocols ...string) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		ServerName: serverName, MinVersion: tls.VersionTLS13,
		InsecureSkipVerify: cfg.InsecureSkipVerify, NextProtos: protocols,
	}
	if cfg.CAFile == "" {
		return tlsConfig, nil
	}
	pem, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return nil, err
	}
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	if !roots.AppendCertsFromPEM(pem) {
		return nil, errors.New("client CA file has no certificates")
	}
	tlsConfig.RootCAs = roots
	return tlsConfig, nil
}

func tlsServerName(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return address
	}
	return host
}
