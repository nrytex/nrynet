package config

import "testing"

func TestServerPlaintextIsLimitedToLoopback(t *testing.T) {
	local := ServerConfig{Listen: "127.0.0.1:7000", DataListen: "[::1]:7001"}
	if err := ValidateServerTransport(local); err != nil {
		t.Fatal(err)
	}
	remote := local
	remote.Listen = "0.0.0.0:7000"
	if err := ValidateServerTransport(remote); err == nil {
		t.Fatal("remote plaintext server was accepted")
	}
	remote.TLS = TLSConfig{Enabled: true, CertFile: "server.crt", KeyFile: "server.key"}
	if err := ValidateServerTransport(remote); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteClientAndControlURLsRequireTLS(t *testing.T) {
	if err := ValidateSecureWebSocketURL("ws://127.0.0.1:7000/agent/connect", "127.0.0.1:7001"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSecureWebSocketURL("ws://server.example:7000/agent/connect", "server.example:7001"); err == nil {
		t.Fatal("remote plaintext agent URL was accepted")
	}
	if err := ValidateSecureWebSocketURL("wss://server.example:7000/agent/connect", "server.example:7001"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSecureHTTPURL("http://relay.example:7100"); err == nil {
		t.Fatal("remote plaintext control URL was accepted")
	}
}
