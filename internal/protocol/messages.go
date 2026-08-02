package protocol

import (
	"encoding/json"

	"github.com/nat-link/nat-link/internal/model"
)

const (
	TypeHello          = "hello"
	TypeHeartbeat      = "heartbeat"
	TypeTunnelSnapshot = "tunnel_snapshot"
	TypeOpenConnection = "open_connection"
	TypeUDPPacket      = "udp_packet"
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

type ErrorPayload struct {
	Message string `json:"message"`
}

type DataHandshake struct {
	Token     string `json:"token"`
	DeviceID  string `json:"device_id"`
	RequestID string `json:"request_id"`
}
