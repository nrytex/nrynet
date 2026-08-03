package config

import "testing"

func TestServerTransportAllowsPlaintextOrTLS(t *testing.T) {
	local := ServerConfig{Listen: "127.0.0.1:7000", DataListen: "[::1]:7001"}
	if err := ValidateServerTransport(local); err != nil {
		t.Fatal(err)
	}
	remote := local
	remote.Listen = "0.0.0.0:7000"
	if err := ValidateServerTransport(remote); err != nil {
		t.Fatal(err)
	}
	remote.TLS = TLSConfig{Enabled: true}
	if err := ValidateServerTransport(remote); err == nil {
		t.Fatal("TLS server without certificate was accepted")
	}
	remote.TLS = TLSConfig{Enabled: true, CertFile: "server.crt", KeyFile: "server.key"}
	if err := ValidateServerTransport(remote); err != nil {
		t.Fatal(err)
	}
	remote.PlainListen = "0.0.0.0:7004"
	if err := ValidateServerTransport(remote); err != nil {
		t.Fatal(err)
	}
	remote.PlainDataListen = "0.0.0.0:7005"
	if err := ValidateServerTransport(remote); err != nil {
		t.Fatal(err)
	}
}

func TestClientWebSocketURLsAllowWSAndWSS(t *testing.T) {
	if err := ValidateSecureWebSocketURL("ws://server.example:7000/agent/connect", "server.example:7001"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSecureWebSocketURL("wss://server.example:7000/agent/connect", "server.example:7001"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSecureWebSocketURL("http://server.example:7000/agent/connect", "server.example:7001"); err == nil {
		t.Fatal("non-websocket URL was accepted")
	}
	if err := ValidateSecureHTTPURL("https://relay.example:7100"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSecureHTTPURL("http://relay.example:7100"); err == nil {
		t.Fatal("remote plaintext control URL was accepted")
	}
}

func TestPlainWebSocketIgnoresTLSVerificationBypass(t *testing.T) {
	cfg := ClientConfig{
		ServerURL: "ws://server.example:7004/agent/connect", DataAddress: "server.example:7005",
		Transport: "websocket", InsecureSkipVerify: true,
	}
	if err := ValidateTLSVerification(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.ServerURL = "wss://server.example:7000/agent/connect"
	cfg.DataAddress = "server.example:7001"
	if err := ValidateTLSVerification(cfg); err == nil {
		t.Fatal("remote wss TLS verification bypass was accepted")
	}
}
