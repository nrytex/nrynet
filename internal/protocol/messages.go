package protocol

import (
	"encoding/json"

	"github.com/nrytex/nrynet/internal/model"
)

const (
	TypeHello              = "hello"
	TypeHeartbeat          = "heartbeat"
	TypeTunnelSnapshot     = "tunnel_snapshot"
	TypeTunnelPath         = "tunnel_path"
	TypeOpenConnection     = "open_connection"
	TypeUDPPacket          = "udp_packet"
	TypeP2PConnect         = "p2p_connect"
	TypeVisitorWebRTC      = "visitor_webrtc"
	TypeConnectionFailed   = "connection_failed"
	TypeError              = "error"
	P2PModeStream          = "stream"
	TunnelPathP2P          = "p2p"
	TunnelPathRelay        = "relay"
	TunnelPathVisitorP2P   = "visitor_p2p"
	TunnelPathVisitorRelay = "visitor_relay"
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

type TunnelPathPayload struct {
	Path string `json:"path"`
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
	Mode              string `json:"mode,omitempty"`
	RequestID         string `json:"request_id,omitempty"`
	TunnelID          string `json:"tunnel_id,omitempty"`
}

type VisitorWebRTCSignalPayload struct {
	Kind       string   `json:"kind"`
	SessionID  string   `json:"session_id"`
	SDP        string   `json:"sdp,omitempty"`
	LocalHost  string   `json:"local_host,omitempty"`
	LocalPort  int      `json:"local_port,omitempty"`
	ICEServers []string `json:"ice_servers,omitempty"`
	Error      string   `json:"error,omitempty"`
}

type VisitorWebRTCDataMessage struct {
	Kind    string              `json:"kind"`
	ID      string              `json:"id"`
	Method  string              `json:"method,omitempty"`
	Path    string              `json:"path,omitempty"`
	Headers map[string][]string `json:"headers,omitempty"`
	Body    string              `json:"body,omitempty"`
	Status  int                 `json:"status,omitempty"`
	Error   string              `json:"error,omitempty"`
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
