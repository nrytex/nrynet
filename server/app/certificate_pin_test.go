package app

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	netx "github.com/nrytex/nrynet/internal/advanced"
	"github.com/nrytex/nrynet/internal/config"
	"github.com/nrytex/nrynet/internal/tlspin"
)

func TestServerCertificatePinLoadsSelfSignedCertificate(t *testing.T) {
	pair, err := netx.SelfSignedCertificate()
	if err != nil {
		t.Fatal(err)
	}
	path := writeCertificate(t, pair.Certificate[0])
	certificate, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	pin, err := serverCertificatePin(config.ServerConfig{TLS: config.TLSConfig{Enabled: true, CertFile: path}})
	if err != nil || pin != tlspin.FromCertificate(certificate) {
		t.Fatalf("certificate pin=%q err=%v", pin, err)
	}
	if pin, err := serverCertificatePin(config.ServerConfig{}); err != nil || pin != "" {
		t.Fatalf("disabled TLS pin=%q err=%v", pin, err)
	}
}

func TestServerCertificatePinSkipsCASignedCertificate(t *testing.T) {
	root, err := netx.SelfSignedCertificate()
	if err != nil {
		t.Fatal(err)
	}
	rootCertificate, err := x509.ParseCertificate(root.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(2), NotBefore: time.Now().Add(-time.Minute),
		NotAfter: time.Now().Add(time.Hour), DNSNames: []string{"server.example"},
	}
	certificate, err := x509.CreateCertificate(rand.Reader, &template, rootCertificate, publicKey, root.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	pin, err := serverCertificatePin(config.ServerConfig{TLS: config.TLSConfig{
		Enabled: true, CertFile: writeCertificate(t, certificate),
	}})
	if err != nil || pin != "" {
		t.Fatalf("CA-signed certificate pin=%q err=%v", pin, err)
	}
}

func writeCertificate(t *testing.T, certificate []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "server.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
