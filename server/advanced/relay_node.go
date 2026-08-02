package advanced

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	netx "github.com/nat-link/nat-link/internal/advanced"
	"github.com/nat-link/nat-link/internal/config"
	"github.com/nat-link/nat-link/internal/model"
	serverTunnel "github.com/nat-link/nat-link/server/tunnel"
)

type RemoteRelayNode struct {
	Token            string
	BrokerAddress    string
	BrokerTLS        bool
	BrokerServerName string
	Client           *http.Client
	Registry         *netx.RelayRegistry
}

type relayBindRequest struct {
	TunnelID         string `json:"tunnel_id"`
	RemotePort       int    `json:"remote_port"`
	BrokerAddress    string `json:"broker_address"`
	BrokerTLS        bool   `json:"broker_tls"`
	BrokerServerName string `json:"broker_server_name,omitempty"`
}

type remoteRelayBinding struct {
	url, token string
	client     *http.Client
}

func (n *RemoteRelayNode) BindTunnel(tunnel model.Tunnel, assignment netx.TunnelAssignment, _ func(net.Conn)) (serverTunnel.RelayBinding, error) {
	if assignment.NodeID == "" {
		return nil, fmt.Errorf("relay assignment is missing node")
	}
	control, err := n.controlAddress(assignment.NodeID)
	if err != nil {
		return nil, err
	}
	request := relayBindRequest{TunnelID: tunnel.ID, RemotePort: tunnel.RemotePort, BrokerAddress: n.BrokerAddress, BrokerTLS: n.BrokerTLS, BrokerServerName: n.BrokerServerName}
	url := strings.TrimRight(control, "/") + "/bindings/" + tunnel.ID
	if err := n.call(http.MethodPost, url, request); err != nil {
		return nil, err
	}
	return &remoteRelayBinding{url: url, token: n.Token, client: n.httpClient()}, nil
}

func (n *RemoteRelayNode) controlAddress(nodeID string) (string, error) {
	if n.Registry == nil {
		return "", fmt.Errorf("relay registry is unavailable")
	}
	node, ok := n.Registry.Node(nodeID)
	if !ok || node.ControlAddr == "" {
		return "", fmt.Errorf("relay %s has no control address", nodeID)
	}
	return node.ControlAddr, nil
}

func (n *RemoteRelayNode) call(method, url string, payload any) error {
	if err := config.ValidateSecureHTTPURL(url); err != nil {
		return err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequest(method, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-NAT-Link-Relay-Token", n.Token)
	response, err := n.httpClient().Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		return fmt.Errorf("relay control returned %s", response.Status)
	}
	return nil
}

func (n *RemoteRelayNode) httpClient() *http.Client {
	if n.Client != nil {
		return n.Client
	}
	return &http.Client{Timeout: 5 * time.Second}
}

func (b *remoteRelayBinding) Address() string { return b.url }
func (b *remoteRelayBinding) Close() error {
	request, err := http.NewRequest(http.MethodDelete, b.url, nil)
	if err != nil {
		return err
	}
	request.Header.Set("X-NAT-Link-Relay-Token", b.token)
	response, err := b.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		return fmt.Errorf("relay unbind returned %s", response.Status)
	}
	return nil
}
