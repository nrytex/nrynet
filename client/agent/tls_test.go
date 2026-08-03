package agent

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	netx "github.com/nrytex/nrynet/internal/advanced"
	"github.com/nrytex/nrynet/internal/agenttoken"
	"github.com/nrytex/nrynet/internal/config"
	"github.com/nrytex/nrynet/internal/tlspin"
)

func TestSecureClientTLSUsesCertificatePinFromToken(t *testing.T) {
	certificate := clientTestCertificate(t)
	token, err := agenttoken.WithCertificatePin("id.secret", tlspin.FromCertificate(certificate))
	if err != nil {
		t.Fatal(err)
	}
	tlsConfig, err := secureClientTLS("localhost", config.ClientConfig{Token: token})
	if err != nil {
		t.Fatal(err)
	}
	if !tlsConfig.InsecureSkipVerify || tlsConfig.VerifyConnection == nil {
		t.Fatal("pinned token did not install custom TLS verification")
	}
	state := tls.ConnectionState{PeerCertificates: []*x509.Certificate{certificate}}
	if err := tlsConfig.VerifyConnection(state); err != nil {
		t.Fatal(err)
	}
}

func TestSecureClientTLSRejectsCertificateNotPinnedByToken(t *testing.T) {
	trusted := clientTestCertificate(t)
	presented := clientTestCertificate(t)
	token, err := agenttoken.WithCertificatePin("id.secret", tlspin.FromCertificate(trusted))
	if err != nil {
		t.Fatal(err)
	}
	tlsConfig, err := secureClientTLS("localhost", config.ClientConfig{Token: token})
	if err != nil {
		t.Fatal(err)
	}
	state := tls.ConnectionState{PeerCertificates: []*x509.Certificate{presented}}
	if err := tlsConfig.VerifyConnection(state); err == nil {
		t.Fatal("certificate not pinned by token was accepted")
	}
}

func TestSecureClientTLSKeepsCAValidationWhenCAFileIsConfigured(t *testing.T) {
	certificate := clientTestCertificate(t)
	path := filepath.Join(t.TempDir(), "ca.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	token, err := agenttoken.WithCertificatePin("id.secret", tlspin.FromCertificate(certificate))
	if err != nil {
		t.Fatal(err)
	}
	tlsConfig, err := secureClientTLS("localhost", config.ClientConfig{Token: token, CAFile: path})
	if err != nil {
		t.Fatal(err)
	}
	if tlsConfig.InsecureSkipVerify || tlsConfig.RootCAs == nil || tlsConfig.VerifyConnection == nil {
		t.Fatal("CA validation was disabled by the token certificate pin")
	}
}

func clientTestCertificate(t *testing.T) *x509.Certificate {
	t.Helper()
	pair, err := netx.SelfSignedCertificate()
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	return certificate
}
