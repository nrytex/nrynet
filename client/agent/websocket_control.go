package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type websocketControl struct {
	conn  *websocket.Conn
	agent *Agent
	mu    sync.Mutex
}

func (*websocketControl) supportsWorkConnections() bool { return true }

func (c *websocketControl) readJSON(value any) error {
	if err := c.conn.ReadJSON(value); err != nil {
		return fmt.Errorf("read WebSocket control message: %w", err)
	}
	return nil
}

func (c *websocketControl) writeJSON(value any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(controlDialTimeout))
	err := c.conn.WriteJSON(value)
	_ = c.conn.SetWriteDeadline(time.Time{})
	if err != nil {
		return fmt.Errorf("write WebSocket control message: %w", err)
	}
	return nil
}

func (c *websocketControl) close() error {
	return c.conn.Close()
}

func (c *websocketControl) ping() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteControl(websocket.PingMessage, []byte("nrynet"), time.Now().Add(5*time.Second))
}

func (c *websocketControl) openData(ctx context.Context, _ string) (dataConn, error) {
	data, err := c.agent.dialLegacyData(ctx)
	if err != nil {
		return nil, err
	}
	// WebSocket control always pairs with the TCP broker, including QUIC
	// sessions that were downgraded after the control path failed.
	return &dataChannel{dataConn: data, needsHandshake: true}, nil
}
