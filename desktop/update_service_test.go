package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/endpoint"
)

type fakeRunner struct {
	cfg       updater.Config
	initCalls int
	checks    int
}

func (f *fakeRunner) Init(cfg updater.Config) error {
	f.cfg = cfg
	f.initCalls++
	return nil
}

func (f *fakeRunner) CheckAndInstall(context.Context) error {
	f.checks++
	return nil
}

func TestDecodePublicKeyBase64(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := decodePublicKey(base64.StdEncoding.EncodeToString(pub))
	if err != nil {
		t.Fatal(err)
	}
	if string(key) != string(pub) {
		t.Fatalf("unexpected key: %v", key)
	}
}

func TestDecodePublicKeyRejectsMissingOrInvalid(t *testing.T) {
	if _, err := decodePublicKey(""); err == nil {
		t.Fatal("expected missing public key error")
	}
	if _, err := decodePublicKey("AQIDBA=="); err == nil {
		t.Fatal("expected invalid Ed25519 public key error")
	}
}

func TestUpdateServiceRequiresManifestURL(t *testing.T) {
	svc := NewUpdateService(&fakeRunner{})
	if _, err := svc.CheckAndInstall(AppConfig{}); err == nil {
		t.Fatal("expected missing manifest URL error")
	}
}

func TestUpdateServiceConfiguresEndpointOnce(t *testing.T) {
	runner := &fakeRunner{}
	svc := NewUpdateService(runner)
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cfg := AppConfig{
		UpdateManifestURL: "https://updates.example.com/stable.json",
		UpdatePublicKey:   base64.StdEncoding.EncodeToString(pub),
	}
	if _, err := svc.CheckAndInstall(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CheckAndInstall(cfg); err != nil {
		t.Fatal(err)
	}
	if runner.initCalls != 1 {
		t.Fatalf("init calls = %d", runner.initCalls)
	}
	changed := cfg
	changed.UpdateManifestURL = "https://updates.example.com/next.json"
	if _, err := svc.CheckAndInstall(changed); err == nil {
		t.Fatal("expected changed update settings to require restart")
	}
}

func TestEndpointUpdaterVerifiesSHA256AndEd25519(t *testing.T) {
	artifact := []byte("nat-link signed update")
	digest := sha256.Sum256(artifact)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signature := ed25519.Sign(priv, digest[:])
	server := signedUpdateServer(t, artifact, digest[:], signature)
	defer server.Close()
	provider, err := endpoint.New(endpoint.Config{URL: server.URL + "/manifest.json"})
	if err != nil {
		t.Fatal(err)
	}
	host := &fakeHost{}
	u := updater.New(host)
	err = u.Init(updater.Config{
		CurrentVersion: "0.0.1", Providers: []updater.Provider{provider},
		PublicKey: pub, Window: updater.WindowNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := u.CheckAndInstall(context.Background()); err != nil {
		t.Fatal(err)
	}
	if u.DownloadedPath() == "" {
		t.Fatal("expected verified update artifact to be staged")
	}
}

func TestEndpointUpdaterRejectsBadSignature(t *testing.T) {
	artifact := []byte("nat-link signed update")
	digest := sha256.Sum256(artifact)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signature := ed25519.Sign(priv, digest[:])
	signature[0] ^= 0xff
	server := signedUpdateServer(t, artifact, digest[:], signature)
	defer server.Close()
	provider, err := endpoint.New(endpoint.Config{URL: server.URL + "/manifest.json"})
	if err != nil {
		t.Fatal(err)
	}
	u := updater.New(&fakeHost{})
	err = u.Init(updater.Config{
		CurrentVersion: "0.0.1", Providers: []updater.Provider{provider},
		PublicKey: pub, Window: updater.WindowNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := u.CheckAndInstall(context.Background()); err == nil {
		t.Fatal("expected bad signature to be rejected")
	}
}

func TestEndpointUpdaterDoesNotInstallOlderVersion(t *testing.T) {
	artifact := []byte("older nat-link update")
	digest := sha256.Sum256(artifact)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var downloaded atomic.Bool
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, manifestJSONVersion(server.URL, "1.0.0", artifact, digest[:], ed25519.Sign(priv, digest[:])))
	})
	mux.HandleFunc("/nat-link.exe", func(http.ResponseWriter, *http.Request) { downloaded.Store(true) })
	provider, err := endpoint.New(endpoint.Config{URL: server.URL + "/manifest.json"})
	if err != nil {
		t.Fatal(err)
	}
	u := updater.New(&fakeHost{})
	if err := u.Init(updater.Config{CurrentVersion: "2.0.0", Providers: []updater.Provider{provider}, PublicKey: pub}); err != nil {
		t.Fatal(err)
	}
	if err := u.CheckAndInstall(context.Background()); err != nil {
		t.Fatal(err)
	}
	if downloaded.Load() || u.DownloadedPath() != "" {
		t.Fatal("older update artifact was downloaded")
	}
}

func signedUpdateServer(t *testing.T, artifact, digest, signature []byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, manifestJSON(server.URL, artifact, digest, signature))
	})
	mux.HandleFunc("/nat-link.exe", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(artifact)
	})
	return server
}

func manifestJSON(base string, artifact, digest, signature []byte) string {
	return manifestJSONVersion(base, "9.9.9", artifact, digest, signature)
}

func manifestJSONVersion(base, version string, artifact, digest, signature []byte) string {
	return fmt.Sprintf(`{
		"schemaVersion":1,"version":"%s","channel":"stable",
		"artifacts":[{
			"url":"%s/nat-link.exe","filename":"nat-link.exe",
			"size":%d,"platform":"%s","arch":"%s",
			"digestAlgo":"sha256","digest":"%s",
			"signatureAlgo":"ed25519","signature":"%s"
		}]}`,
		version, base, len(artifact), runtime.GOOS, runtime.GOARCH,
		base64.StdEncoding.EncodeToString(digest),
		base64.StdEncoding.EncodeToString(signature),
	)
}

type fakeHost struct {
	mu        sync.Mutex
	listeners map[string][]func(any)
}

func (f *fakeHost) Emit(name string, data ...any) bool {
	f.mu.Lock()
	cbs := append([]func(any){}, f.listeners[name]...)
	f.mu.Unlock()
	var payload any
	if len(data) == 1 {
		payload = data[0]
	}
	for _, cb := range cbs {
		cb(payload)
	}
	return false
}

func (f *fakeHost) OnEvent(name string, cb func(any)) func() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listeners == nil {
		f.listeners = map[string][]func(any){}
	}
	f.listeners[name] = append(f.listeners[name], cb)
	return func() {}
}

func (f *fakeHost) OpenWindow(updater.WindowOptions) updater.WindowHandle {
	return fakeWindow{}
}

func (f *fakeHost) Quit() {}

type fakeWindow struct{}

func (fakeWindow) EmitEvent(string, ...any) bool { return false }
func (fakeWindow) Show()                         {}
func (fakeWindow) Close()                        {}
