package app

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nrytex/nrynet/internal/config"
)

func TestMainTransportTogglesTLSWithoutRestart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	certFile, keyFile, _ := writeTLSPair(t)
	cfg := dualTransportConfig(t, certFile, keyFile)
	cfg.Server.PlainEnabled = false
	cfg.Server.TLS.Enabled = false

	application, _, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = application.Run() }()
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })

	waitPlainHealth(t, "http://"+cfg.Server.Listen)
	assertTLSHealthRejected(t, cfg.Server.Listen)

	status, err := application.TransportController().SetTLSEnabled(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if !status.TLS.Enabled || status.Certificate == nil {
		t.Fatalf("unexpected transport status after enable: %+v", status)
	}
	waitTLSHealth(t, "https://"+cfg.Server.Listen)
	waitPlainHealth(t, "http://"+cfg.Server.Listen)

	status, err = application.TransportController().SetTLSEnabled(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if status.TLS.Enabled {
		t.Fatalf("TLS remained enabled: %+v", status)
	}
	assertTLSHealthRejected(t, cfg.Server.Listen)
	waitPlainHealth(t, "http://"+cfg.Server.Listen)
}

func TestMainTransportReloadsCertificateWithoutRestart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	certFile, keyFile, _ := writeTLSPair(t)
	cfg := dualTransportConfig(t, certFile, keyFile)
	cfg.Server.PlainEnabled = false

	application, _, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = application.Run() }()
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })
	waitTLSHealth(t, "https://"+cfg.Server.Listen)

	first := servedCertificateDER(t, cfg.Server.Listen)
	newCertFile, newKeyFile, _ := writeTLSPair(t)
	if _, err := application.TransportController().ReloadCertificate(ctx, newCertFile, newKeyFile); err != nil {
		t.Fatal(err)
	}
	second := servedCertificateDER(t, cfg.Server.Listen)
	if string(first) == string(second) {
		t.Fatal("TLS certificate did not change after reload")
	}
	if want := certificateDERFromFile(t, newCertFile); string(second) != string(want) {
		t.Fatal("server did not present the reloaded certificate")
	}
}

func TestMainDataTransportAcceptsPlainAndTLSOnSamePort(t *testing.T) {
	certFile, keyFile, _ := writeTLSPair(t)
	store, err := newTLSStore(tlsConfig(true, certFile, keyFile))
	if err != nil {
		t.Fatal(err)
	}
	base, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener := newDynamicTLSListener(base, store)
	defer listener.Close()
	go acceptAndWriteOK(listener)

	assertDataGreeting(t, listener.Addr().String(), false)
	assertDataGreeting(t, listener.Addr().String(), true)
}

func TestControllerRejectsTLSEnableWithoutCertificate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	certFile, keyFile, _ := writeTLSPair(t)
	cfg := dualTransportConfig(t, certFile, keyFile)
	cfg.Server.PlainEnabled = false
	cfg.Server.TLS.Enabled = false
	cfg.Server.TLS.CertFile = ""
	cfg.Server.TLS.KeyFile = ""

	application, _, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })
	if _, err := application.TransportController().SetTLSEnabled(ctx, true); err == nil {
		t.Fatal("TLS enable without certificate was accepted")
	}
}

func TestTLSDisabledIgnoresMissingCertificateFiles(t *testing.T) {
	cfg := config.TLSConfig{
		Enabled:  false,
		CertFile: filepath.Join(t.TempDir(), "missing-fullchain.pem"),
		KeyFile:  filepath.Join(t.TempDir(), "missing-privkey.pem"),
	}
	store, err := newTLSStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if store.Enabled() || store.CertificateLoaded() {
		t.Fatalf("unexpected TLS store state: enabled=%v loaded=%v", store.Enabled(), store.CertificateLoaded())
	}
}

func TestQUICCertificateIgnoresTCPEnabledFlag(t *testing.T) {
	store, err := newTLSStore(config.TLSConfig{Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetCertificate(nil); err == nil {
		t.Fatal("TCP TLS certificate was available while TLS was disabled")
	}
	cert, err := store.GetQUICCertificate(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cert == nil || len(cert.Certificate) == 0 {
		t.Fatal("QUIC fallback certificate was not generated")
	}
}

func TestPlainHotDisableFromPlainRequestReturnsAndKeepsAppRunning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	certFile, keyFile, _ := writeTLSPair(t)
	cfg := dualTransportConfig(t, certFile, keyFile)
	cfg.Server.TLS.Enabled = false

	application, _, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	router := application.server.Handler.(*gin.Engine)
	router.GET("/disable-plain", func(c *gin.Context) {
		if _, err := application.TransportController().SetPlainEnabled(c.Request.Context(), false); err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.String(http.StatusOK, "ok")
	})
	runErr := make(chan error, 1)
	go func() { runErr <- application.Run() }()
	t.Cleanup(func() { _ = application.Shutdown(context.Background()) })
	waitPlainHealth(t, "http://"+cfg.Server.PlainListen)

	response, err := http.Get("http://" + cfg.Server.PlainListen + "/disable-plain")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("disable from plain returned status %d", response.StatusCode)
	}
	assertCanListen(t, "tcp", cfg.Server.PlainListen)
	assertCanListen(t, "tcp", cfg.Server.PlainDataListen)
	select {
	case err := <-runErr:
		t.Fatalf("app run returned after hot plain disable: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
}

func tlsConfig(enabled bool, certFile, keyFile string) config.TLSConfig {
	return config.TLSConfig{Enabled: enabled, CertFile: certFile, KeyFile: keyFile}
}

func assertTLSHealthRejected(t *testing.T, address string) {
	t.Helper()
	client := &http.Client{
		Timeout:   500 * time.Millisecond,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
	if response, err := client.Get("https://" + address + "/health"); err == nil {
		_ = response.Body.Close()
		t.Fatal("TLS health request succeeded while TLS was disabled")
	}
}

func servedCertificateDER(t *testing.T, address string) []byte {
	t.Helper()
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", address, &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		t.Fatal("server returned no certificate")
	}
	return certs[0].Raw
}

func certificateDERFromFile(t *testing.T, certFile string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(certFile))
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		t.Fatal("certificate file has no PEM block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return cert.Raw
}

func acceptAndWriteOK(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go func() {
			defer conn.Close()
			_, _ = conn.Write([]byte("ok"))
		}()
	}
}

func assertDataGreeting(t *testing.T, address string, useTLS bool) {
	t.Helper()
	var conn net.Conn
	var err error
	if useTLS {
		conn, err = tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", address, &tls.Config{InsecureSkipVerify: true})
	} else {
		conn, err = net.DialTimeout("tcp", address, time.Second)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if !useTLS {
		if _, err := conn.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	buffer := make([]byte, 2)
	if _, err := conn.Read(buffer); err != nil {
		t.Fatal(err)
	}
	if string(buffer) != "ok" {
		t.Fatalf("data greeting=%q want ok", buffer)
	}
}
