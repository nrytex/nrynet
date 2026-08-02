package runtime

import (
	"bufio"
	"bytes"
	"context"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	netx "github.com/nat-link/nat-link/internal/advanced"
	"github.com/nat-link/nat-link/internal/protocol"
)

type Config struct {
	ID, Address, ControlAddress, BindHost, BrokerAddress, Token string
	BrokerTLS                                                   bool
	BrokerServerName                                            string
	BrokerCAFile                                                string
	ControlTLS                                                  bool
	ControlCertFile                                             string
	ControlKeyFile                                              string
}

type Node struct {
	config      Config
	mu          sync.Mutex
	bindings    map[string]net.Listener
	connections int
}

type bindRequest struct {
	TunnelID         string `json:"tunnel_id"`
	RemotePort       int    `json:"remote_port"`
	BrokerAddress    string `json:"broker_address"`
	BrokerTLS        bool   `json:"broker_tls"`
	BrokerServerName string `json:"broker_server_name,omitempty"`
}

func New(config Config) (*Node, error) {
	if config.ID == "" || config.Address == "" || config.ControlAddress == "" || config.BrokerAddress == "" || config.Token == "" {
		return nil, errors.New("relay id, address, control address, broker and token are required")
	}
	if config.BindHost == "" {
		config.BindHost = "0.0.0.0"
	}
	if config.ControlTLS && (config.ControlCertFile == "" || config.ControlKeyFile == "") {
		return nil, errors.New("control TLS certificate and key are required")
	}
	return &Node{config: config, bindings: make(map[string]net.Listener)}, nil
}

func (n *Node) Register(client *http.Client, server string) error {
	return postJSON(client, server+"/api/v2/relays/register", n.config.Token, netx.RelayNode{ID: n.config.ID, Address: n.config.Address, ControlAddr: n.config.ControlAddress})
}

func (n *Node) Heartbeat(client *http.Client, server string) error {
	n.mu.Lock()
	connections := n.connections
	n.mu.Unlock()
	return postJSON(client, server+"/api/v2/relays/"+n.config.ID+"/heartbeat", n.config.Token, map[string]int{"connections": connections})
}

func (n *Node) ServeControl(listener net.Listener) error {
	if n.config.ControlTLS {
		certificate, err := tls.LoadX509KeyPair(n.config.ControlCertFile, n.config.ControlKeyFile)
		if err != nil {
			return err
		}
		listener = tls.NewListener(listener, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS13})
	}
	server := &http.Server{Handler: authenticate(n.config.Token, http.HandlerFunc(n.handleBinding)), ReadHeaderTimeout: 5 * time.Second}
	return server.Serve(listener)
}

func (n *Node) Close() {
	n.mu.Lock()
	listeners := n.bindings
	n.bindings = make(map[string]net.Listener)
	n.mu.Unlock()
	for _, listener := range listeners {
		_ = listener.Close()
	}
}

func authenticate(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actual := r.Header.Get("X-NAT-Link-Relay-Token")
		if subtle.ConstantTimeCompare([]byte(actual), []byte(token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (n *Node) handleBinding(w http.ResponseWriter, r *http.Request) {
	const prefix = "/bindings/"
	if len(r.URL.Path) <= len(prefix) || r.URL.Path[:len(prefix)] != prefix {
		http.NotFound(w, r)
		return
	}
	tunnelID := r.URL.Path[len(prefix):]
	switch r.Method {
	case http.MethodPost:
		var request bindRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.TunnelID != tunnelID || request.RemotePort < 1 {
			http.Error(w, "invalid binding", http.StatusBadRequest)
			return
		}
		if err := n.bind(request); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusCreated)
	case http.MethodDelete:
		n.unbind(tunnelID)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (n *Node) bind(request bindRequest) error {
	listener, err := net.Listen("tcp", net.JoinHostPort(n.config.BindHost, fmt.Sprint(request.RemotePort)))
	if err != nil {
		return err
	}
	n.mu.Lock()
	if _, exists := n.bindings[request.TunnelID]; exists {
		n.mu.Unlock()
		_ = listener.Close()
		return nil
	}
	n.bindings[request.TunnelID] = listener
	n.mu.Unlock()
	go n.accept(request, listener)
	return nil
}

func (n *Node) unbind(tunnelID string) {
	n.mu.Lock()
	listener := n.bindings[tunnelID]
	delete(n.bindings, tunnelID)
	n.mu.Unlock()
	if listener != nil {
		_ = listener.Close()
	}
}

func (n *Node) accept(request bindRequest, listener net.Listener) {
	for {
		visitor, err := listener.Accept()
		if err != nil {
			return
		}
		go n.forward(request, visitor)
	}
}

func (n *Node) forward(request bindRequest, visitor net.Conn) {
	defer visitor.Close()
	broker, err := n.dialBroker(request)
	if err != nil {
		return
	}
	defer broker.Close()
	writer := bufio.NewWriter(broker)
	handshake := protocol.RelayHandshake{
		Role: "relay_visitor", Token: n.config.Token, NodeID: n.config.ID,
		TunnelID: request.TunnelID, VisitorAddr: visitor.RemoteAddr().String(),
	}
	if err := json.NewEncoder(writer).Encode(handshake); err != nil {
		return
	}
	if err := writer.Flush(); err != nil {
		return
	}
	n.mu.Lock()
	n.connections++
	n.mu.Unlock()
	defer func() { n.mu.Lock(); n.connections--; n.mu.Unlock() }()
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(broker, visitor); done <- struct{}{} }()
	go func() { _, _ = io.Copy(visitor, broker); done <- struct{}{} }()
	<-done
	_ = broker.Close()
	_ = visitor.Close()
	<-done
}

func (n *Node) dialBroker(request bindRequest) (net.Conn, error) {
	if !request.BrokerTLS && !n.config.BrokerTLS {
		return net.DialTimeout("tcp", request.BrokerAddress, 5*time.Second)
	}
	serverName := request.BrokerServerName
	if serverName == "" {
		serverName = n.config.BrokerServerName
	}
	config, err := brokerTLSConfig(serverName, n.config.BrokerCAFile)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return tls.DialWithDialer(dialer, "tcp", request.BrokerAddress, config)
}

func brokerTLSConfig(serverName, caFile string) (*tls.Config, error) {
	config := &tls.Config{MinVersion: tls.VersionTLS13, ServerName: serverName}
	if caFile == "" {
		return config, nil
	}
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		return nil, errors.New("broker CA file has no certificates")
	}
	config.RootCAs = roots
	return config, nil
}

func postJSON(client *http.Client, url, token string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-NAT-Link-Relay-Token", token)
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		return fmt.Errorf("relay API returned %s", response.Status)
	}
	return nil
}
