package app

import (
	"fmt"

	"github.com/nrytex/nrynet/internal/config"
	"github.com/nrytex/nrynet/internal/tlspin"
)

func serverCertificatePin(cfg config.ServerConfig) (string, error) {
	if !cfg.TLS.Enabled {
		return "", nil
	}
	pin, err := tlspin.FromSelfSignedPEMFile(cfg.TLS.CertFile)
	if err != nil {
		return "", fmt.Errorf("load TLS certificate pin: %w", err)
	}
	return pin, nil
}
