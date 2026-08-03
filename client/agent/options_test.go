package agent

import (
	"testing"

	"github.com/nrytex/nrynet/internal/config"
)

func TestNewOptionsNormalizesClientIdentity(t *testing.T) {
	options := NewOptions(config.Config{Client: config.ClientConfig{
		ServerURL:   "ws://127.0.0.1/agent/connect",
		DataAddress: "127.0.0.1:7001",
		Token:       "token",
	}}, "test")
	if options.Config.Name == "" {
		t.Fatal("expected default name")
	}
	if options.Config.DeviceID == "" {
		t.Fatal("expected default device id")
	}
	if err := options.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestOptionsRejectRemotePlaintextTransport(t *testing.T) {
	options := NewOptions(config.Config{Client: config.ClientConfig{
		ServerURL: "ws://server.example:7000/agent/connect", DataAddress: "server.example:7001",
		Token: "token", DeviceID: "device",
	}}, "test")
	if err := options.Validate(); err == nil {
		t.Fatal("remote plaintext client transport was accepted")
	}
	options.Config.ServerURL = "wss://server.example:7000/agent/connect"
	if err := options.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestOptionsRejectRemoteCertificateVerificationBypass(t *testing.T) {
	options := NewOptions(config.Config{Client: config.ClientConfig{
		ServerURL: "wss://server.example:7000/agent/connect", DataAddress: "server.example:7001",
		Token: "token", DeviceID: "device", InsecureSkipVerify: true,
	}}, "test")
	if err := options.Validate(); err == nil {
		t.Fatal("remote TLS verification bypass was accepted")
	}

	options.Config.ServerURL = "wss://127.0.0.1:7000/agent/connect"
	options.Config.DataAddress = "127.0.0.1:7001"
	if err := options.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestOptionsRejectRemoteQUICCertificateVerificationBypass(t *testing.T) {
	options := NewOptions(config.Config{Client: config.ClientConfig{
		ServerURL: "wss://server.example:7000/agent/connect", Transport: "quic",
		QUICAddress: "server.example:7002", Token: "token", DeviceID: "device", InsecureSkipVerify: true,
	}}, "test")
	if err := options.Validate(); err == nil {
		t.Fatal("remote QUIC verification bypass was accepted")
	}
}
