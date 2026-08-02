package gateway

import (
	"bufio"
	"crypto/tls"
	"io"
	"net"
	"strings"
	"testing"
)

func TestSniffHTTPPreservesRequest(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	request := "GET /socket HTTP/1.1\r\nHost: Example.COM:8080\r\nConnection: Upgrade\r\n\r\n"
	go func() { _, _ = io.WriteString(client, request) }()
	wrapped, protocol, domain, err := sniffConnection(server)
	if err != nil {
		t.Fatal(err)
	}
	if protocol != "http" || domain != "example.com" {
		t.Fatalf("route=%s domain=%s", protocol, domain)
	}
	read := make([]byte, len(request))
	if _, err := io.ReadFull(wrapped, read); err != nil {
		t.Fatal(err)
	}
	if string(read) != request {
		t.Fatalf("request was changed: %q", read)
	}
}

func TestSniffTLSReadsServerName(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	result := make(chan error, 1)
	go func() {
		connection := tls.Client(client, &tls.Config{InsecureSkipVerify: true, ServerName: "secure.example"})
		result <- connection.Handshake()
	}()
	_, protocol, domain, err := sniffConnection(server)
	if err != nil {
		t.Fatal(err)
	}
	if protocol != "https" || domain != "secure.example" {
		t.Fatalf("route=%s domain=%s", protocol, domain)
	}
	_ = server.Close()
	<-result
}

func TestSniffRejectsMissingHost(t *testing.T) {
	request := bufio.NewReader(strings.NewReader("GET / HTTP/1.0\r\n\r\n"))
	if _, _, err := sniffHTTP(request); err == nil {
		t.Fatal("missing Host must be rejected")
	}
}
