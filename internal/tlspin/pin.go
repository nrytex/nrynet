package tlspin

import (
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"time"
)

func FromCertificate(certificate *x509.Certificate) string {
	sum := sha256.Sum256(certificate.RawSubjectPublicKeyInfo)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func FromSelfSignedPEMFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", errors.New("TLS certificate file has no PEM certificate")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("parse TLS certificate: %w", err)
	}
	if err := certificate.CheckSignature(
		certificate.SignatureAlgorithm, certificate.RawTBSCertificate, certificate.Signature,
	); err != nil {
		return "", nil
	}
	return FromCertificate(certificate), nil
}

func VerifyConnection(expectedPin string, now func() time.Time) func(tls.ConnectionState) error {
	return func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return errors.New("server did not provide a TLS certificate")
		}
		certificate := state.PeerCertificates[0]
		currentTime := time.Now()
		if now != nil {
			currentTime = now()
		}
		if currentTime.Before(certificate.NotBefore) || currentTime.After(certificate.NotAfter) {
			return errors.New("server TLS certificate is expired or not yet valid")
		}
		actual := FromCertificate(certificate)
		if subtle.ConstantTimeCompare([]byte(actual), []byte(expectedPin)) != 1 {
			return errors.New("server TLS certificate pin does not match the Agent Token")
		}
		return nil
	}
}
