package advanced

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"math/big"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const QUICALPN = "nrynet-quic-v1"

func ServerTLSConfig(cert tls.Certificate) *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
		NextProtos:   []string{QUICALPN},
	}
}

type DynamicTLSStore struct {
	enabled    atomic.Bool
	cert       atomic.Pointer[tls.Certificate]
	fallback   atomic.Pointer[tls.Certificate]
	fallbackMu sync.Mutex
}

func NewDynamicTLSStore() *DynamicTLSStore {
	return &DynamicTLSStore{}
}

func (s *DynamicTLSStore) Enabled() bool {
	if s == nil {
		return false
	}
	return s.enabled.Load()
}

func (s *DynamicTLSStore) CertificateLoaded() bool {
	return s != nil && s.cert.Load() != nil
}

func (s *DynamicTLSStore) LoadX509KeyPair(certFile, keyFile string) error {
	certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return fmt.Errorf("load TLS certificate: %w", err)
	}
	s.cert.Store(&certificate)
	return nil
}

func (s *DynamicTLSStore) SetEnabled(enabled bool) error {
	if !enabled {
		s.enabled.Store(false)
		return nil
	}
	if !s.CertificateLoaded() {
		return errors.New("TLS certificate is not loaded")
	}
	s.enabled.Store(true)
	return nil
}

func (s *DynamicTLSStore) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	if !s.Enabled() {
		return nil, errors.New("TLS is disabled")
	}
	certificate := s.cert.Load()
	if certificate == nil {
		return nil, errors.New("TLS certificate is not loaded")
	}
	return certificate, nil
}

func (s *DynamicTLSStore) GetQUICCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	certificate := s.cert.Load()
	if certificate != nil {
		return certificate, nil
	}
	return s.fallbackCertificate()
}

func (s *DynamicTLSStore) fallbackCertificate() (*tls.Certificate, error) {
	if certificate := s.fallback.Load(); certificate != nil {
		return certificate, nil
	}
	s.fallbackMu.Lock()
	defer s.fallbackMu.Unlock()
	if certificate := s.fallback.Load(); certificate != nil {
		return certificate, nil
	}
	certificate, err := SelfSignedCertificate()
	if err != nil {
		return nil, err
	}
	s.fallback.Store(&certificate)
	return &certificate, nil
}

func ServerDynamicTLSConfig(store *DynamicTLSStore) *tls.Config {
	return &tls.Config{
		GetCertificate: store.GetCertificate,
		MinVersion:     tls.VersionTLS13,
	}
}

func QUICServerDynamicTLSConfig(store *DynamicTLSStore) *tls.Config {
	config := ServerDynamicTLSConfig(store)
	config.GetCertificate = store.GetQUICCertificate
	config.NextProtos = []string{QUICALPN}
	return config
}

func ClientTLSConfig(serverName string, skipVerify bool) *tls.Config {
	return &tls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: skipVerify,
		MinVersion:         tls.VersionTLS13,
		NextProtos:         []string{QUICALPN},
	}
}

func SelfSignedCertificate() (tls.Certificate, error) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	certDER, err := x509.CreateCertificate(
		rand.Reader, &template, &template, privateKey.Public(), privateKey,
	)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  privateKey,
	}, nil
}
