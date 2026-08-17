package relay

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"

	"github.com/nrytex/nrynet/internal/protocol"
)

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

type initialHandshake struct {
	Role        string `json:"role"`
	Token       string `json:"token"`
	DeviceID    string `json:"device_id"`
	RequestID   string `json:"request_id"`
	NodeID      string `json:"node_id"`
	TunnelID    string `json:"tunnel_id"`
	VisitorAddr string `json:"visitor_addr"`
}

func readInitialHandshake(conn net.Conn) (net.Conn, initialHandshake, error) {
	reader := bufio.NewReaderSize(conn, 4096)
	line, err := reader.ReadSlice('\n')
	if err != nil {
		return nil, initialHandshake{}, err
	}
	var handshake initialHandshake
	if err := json.Unmarshal(line, &handshake); err != nil {
		return nil, initialHandshake{}, err
	}
	if handshake.Role == "relay_visitor" {
		return bufferedConn{Conn: conn, reader: reader}, handshake, nil
	}
	requestRequired := handshake.Role != protocol.DataRoleWorkConnection
	if handshake.Token == "" || handshake.DeviceID == "" || (requestRequired && handshake.RequestID == "") {
		return nil, initialHandshake{}, errors.New("invalid data handshake")
	}
	return bufferedConn{Conn: conn, reader: reader}, handshake, nil
}

func (c bufferedConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

func (c bufferedConn) readJSONLine(value any) error {
	line, err := c.reader.ReadSlice('\n')
	if err != nil {
		return err
	}
	return json.Unmarshal(line, value)
}
