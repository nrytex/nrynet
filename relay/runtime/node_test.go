package runtime

import (
	"crypto/tls"
	"encoding/pem"
	"os"
	"testing"

	netx "github.com/nrytex/nrynet/internal/advanced"
)

func TestNodeDialsTLSBrokerWithHostnameValidation(t *testing.T) {
	certificate, err := netx.SelfSignedCertificate()
	if err != nil {
		t.Fatal(err)
	}
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS13})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				if tlsConn, ok := conn.(*tls.Conn); ok {
					_ = tlsConn.Handshake()
				}
				_ = conn.Close()
			}()
		}
	}()
	caFile := t.TempDir() + "/broker.pem"
	pemData := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]})
	if err := os.WriteFile(caFile, pemData, 0o600); err != nil {
		t.Fatal(err)
	}
	node, err := New(Config{ID: "relay", Address: "203.0.113.1", ControlAddress: "http://127.0.0.1:7100", BrokerAddress: listener.Addr().String(), Token: "secret", BrokerTLS: true, BrokerServerName: "localhost", BrokerCAFile: caFile})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := node.dialBroker(bindRequest{BrokerAddress: listener.Addr().String(), BrokerTLS: true, BrokerServerName: "localhost"})
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	_, err = node.dialBroker(bindRequest{BrokerAddress: listener.Addr().String(), BrokerTLS: true, BrokerServerName: "wrong.example"})
	if err == nil {
		t.Fatal("expected hostname validation failure")
	}
}

func TestNodeRejectsRemotePlaintextControlAndBroker(t *testing.T) {
	base := Config{
		ID: "relay", Address: "203.0.113.1", ControlAddress: "http://127.0.0.1:7100",
		BrokerAddress: "127.0.0.1:7001", Token: "secret",
	}
	remoteControl := base
	remoteControl.ControlAddress = "http://relay.example:7100"
	if _, err := New(remoteControl); err == nil {
		t.Fatal("remote plaintext control address was accepted")
	}
	remoteBroker := base
	remoteBroker.BrokerAddress = "broker.example:7001"
	if _, err := New(remoteBroker); err == nil {
		t.Fatal("remote plaintext broker was accepted")
	}
	remoteListener := base
	remoteListener.ControlListen = "0.0.0.0:7100"
	if _, err := New(remoteListener); err == nil {
		t.Fatal("remote plaintext control listener was accepted")
	}
}
