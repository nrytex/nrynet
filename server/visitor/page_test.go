package visitor

import (
	"strings"
	"testing"
)

func TestRenderPageBootstrapsWebProxy(t *testing.T) {
	page, err := renderPage(pageConfig{
		TunnelName: "local site", SignalURL: "/visitor/webrtc", ScopeURL: "/visitor/tunnel/token/", WorkerURL: "/visitor/tunnel/token/sw.js",
	})
	if err != nil {
		t.Fatal(err)
	}
	content := string(page)
	for _, want := range []string{
		"navigator.serviceWorker.register",
		"/visitor/tunnel/token/sw.js",
		"/visitor/tunnel/token/",
		"request_start",
		"request_end",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("visitor page is missing %q", want)
		}
	}
}

func TestVisitorServiceWorkerStreamsAndRewritesResponses(t *testing.T) {
	for _, want := range []string{"response_start", "response_chunk", "ReadableStream", "rewriteHTML"} {
		if !strings.Contains(visitorServiceWorker, want) {
			t.Fatalf("visitor service worker is missing %q", want)
		}
	}
}
