package protocol

import (
	"encoding/json"

	"github.com/nrytex/nrynet/internal/model"
)

const (
	TypeHello          = "hello"
	TypeHeartbeat      = "heartbeat"
	TypeTunnelSnapshot = "tunnel_snapshot"
	TypeOpenConnection = "open_connection"
	TypeUDPPacket      = "udp_packet"
	TypeP2PConnect     = "p2p_connect"
	TypeError          = "error"
)

type ControlMessage struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id,omitempty"`
	TunnelID  string          `json:"tunnel_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

type HelloPayload struct {
	Name     string `json:"name"`
	DeviceID string `json:"device_id"`
	OS       string `json:"os"`
	Version  string `json:"version"`
}

type TunnelSnapshotPayload struct {
	Tunnels []model.Tunnel `json:"tunnels"`
}

type OpenConnectionPayload struct {
	LocalHost string `json:"local_host"`
	LocalPort int    `json:"local_port"`
}

type UDPPacketPayload struct {
	LocalHost string `json:"local_host,omitempty"`
	LocalPort int    `json:"local_port,omitempty"`
	Payload   []byte `json:"payload"`
}

type P2PConnectPayload struct {
	RendezvousAddress string `json:"rendezvous_address"`
	SessionID         string `json:"session_id"`
	SessionKey        string `json:"session_key"`
	PeerID            string `json:"peer_id"`
	WantsPeerID       string `json:"wants_peer_id"`
	LocalHost         string `json:"local_host"`
	LocalPort         int    `json:"local_port"`
}

type ErrorPayload struct {
	Message string `json:"message"`
}

type DataHandshake struct {
	Token     string `json:"token"`
	DeviceID  string `json:"device_id"`
	RequestID string `json:"request_id"`
}

// RelayHandshake is sent by a relay node before it forwards a visitor stream
// to the central broker. It is intentionally separate from agent credentials.
type RelayHandshake struct {
	Role        string `json:"role"`
	Token       string `json:"token"`
	NodeID      string `json:"node_id"`
	TunnelID    string `json:"tunnel_id"`
	VisitorAddr string `json:"visitor_addr"`
}
