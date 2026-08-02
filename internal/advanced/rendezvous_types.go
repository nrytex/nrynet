package advanced

import (
	"net"
	"strconv"
)

const (
	PacketRegister = "register"
	PacketObserved = "observed"
	PacketPeer     = "peer"
	PacketPunch    = "punch"
	PacketPunchAck = "punch_ack"
)

type Endpoint struct {
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

type RendezvousPacket struct {
	Type        string   `json:"type"`
	SessionID   string   `json:"session_id,omitempty"`
	PeerID      string   `json:"peer_id,omitempty"`
	WantsPeerID string   `json:"wants_peer_id,omitempty"`
	Endpoint    Endpoint `json:"endpoint,omitempty"`
	Relay       bool     `json:"relay,omitempty"`
}

func endpointFromAddr(addr net.Addr) Endpoint {
	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok {
		return Endpoint{}
	}
	return Endpoint{IP: udpAddr.IP.String(), Port: udpAddr.Port}
}

func (e Endpoint) UDPAddr() (*net.UDPAddr, error) {
	return net.ResolveUDPAddr("udp", net.JoinHostPort(e.IP, strconv.Itoa(e.Port)))
}
