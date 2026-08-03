package tlspin

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"testing"
	"time"

	netx "github.com/nrytex/nrynet/internal/advanced"
)

func TestVerifyConnectionAcceptsMatchingSelfSignedCertificate(t *testing.T) {
	certificate := testCertificate(t)
	state := tls.ConnectionState{PeerCertificates: []*x509.Certificate{certificate}}
	verify := VerifyConnection("localhost", FromCertificate(certificate), time.Now)
	if err := verify(state); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyConnectionRejectsMismatchedPinAndHostname(t *testing.T) {
	certificate := testCertificate(t)
	state := tls.ConnectionState{PeerCertificates: []*x509.Certificate{certificate}}
	wrongPin := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	if err := VerifyConnection("localhost", wrongPin, time.Now)(state); err == nil {
		t.Fatal("mismatched pin was accepted")
	}
	if err := VerifyConnection("other.example", FromCertificate(certificate), time.Now)(state); err == nil {
		t.Fatal("mismatched hostname was accepted")
	}
}

func testCertificate(t *testing.T) *x509.Certificate {
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
