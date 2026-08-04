package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

func TestGitHubReleaseProviderChecksAndDownloadsWithoutAPI(t *testing.T) {
	artifact := []byte("desktop archive")
	digest := sha256.Sum256(artifact)
	server := newReleaseServer(t, "v2.0.0", artifact, hex.EncodeToString(digest[:]))
	provider := releaseProviderForServer(t, server)

	release, err := provider.Check(context.Background(), updater.CheckRequest{
		CurrentVersion: "1.9.0", Platform: "windows", Arch: "amd64",
	})
	if err != nil {
		t.Fatal(err)
	}
	if release == nil || release.Version != "2.0.0" || !bytes.Equal(release.Verification.Digest, digest[:]) {
		t.Fatalf("unexpected release: %+v", release)
	}
	var downloaded bytes.Buffer
	if err := provider.Download(context.Background(), release, &downloaded, func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(downloaded.Bytes(), artifact) {
		t.Fatalf("downloaded %q", downloaded.Bytes())
	}
}

func TestGitHubReleaseProviderReturnsNilWhenCurrent(t *testing.T) {
	server := newReleaseServer(t, "v2.0.0", []byte("archive"), fmt.Sprintf("%064x", 1))
	provider := releaseProviderForServer(t, server)
	release, err := provider.Check(context.Background(), updater.CheckRequest{
		CurrentVersion: "2.0.0", Platform: "windows", Arch: "amd64",
	})
	if err != nil || release != nil {
		t.Fatalf("release=%+v err=%v", release, err)
	}
}

func TestDesktopReleaseAssetSelection(t *testing.T) {
	tests := []struct {
		platform string
		arch     string
		want     string
	}{
		{platform: "windows", arch: "amd64", want: "nrynet-desktop-windows-amd64.zip"},
		{platform: "darwin", arch: "amd64", want: "nrynet-desktop-darwin-universal.tar.gz"},
		{platform: "darwin", arch: "arm64", want: "nrynet-desktop-darwin-universal.tar.gz"},
	}
	for _, test := range tests {
		filename, _, err := desktopReleaseAsset(test.platform, test.arch)
		if err != nil || filename != test.want {
			t.Fatalf("%s/%s filename=%q err=%v", test.platform, test.arch, filename, err)
		}
	}
}

func newReleaseServer(t *testing.T, tag string, artifact []byte, digest string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/nrytex/nrynet/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/nrytex/nrynet/releases/tag/"+tag, http.StatusFound)
	})
	mux.HandleFunc("/nrytex/nrynet/releases/tag/"+tag, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/nrytex/nrynet/releases/download/"+tag+"/SHA256SUMS", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, "%s  nrynet-desktop-windows-amd64.zip\n", digest)
	})
	mux.HandleFunc("/nrytex/nrynet/releases/download/"+tag+"/nrynet-desktop-windows-amd64.zip", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(artifact)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func releaseProviderForServer(t *testing.T, server *httptest.Server) *githubReleaseProvider {
	t.Helper()
	provider, err := newGitHubReleaseProvider("nrytex/nrynet", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	provider.baseURL = server.URL
	return provider
}
