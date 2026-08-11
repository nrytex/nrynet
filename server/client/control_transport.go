package client

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type ControlTransport interface {
	ReadJSON(value any) error
	WriteJSON(value any) error
	Close() error
	SetReadDeadline(time.Time) error
}

type websocketControl struct {
	ws *websocket.Conn
	mu sync.Mutex
}

func (c *websocketControl) ReadJSON(value any) error {
	return c.ws.ReadJSON(value)
}

func (c *websocketControl) WriteJSON(value any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	// A blocked browser/Agent must not pin every visitor goroutine that is
	// waiting to send an OpenConnection command. The deadline is per write and
	// is cleared after a successful frame so idle sessions remain long-lived.
	_ = c.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
	err := c.ws.WriteJSON(value)
	_ = c.ws.SetWriteDeadline(time.Time{})
	return err
}

func (c *websocketControl) Close() error {
	// Gorilla permits Close to run concurrently with reads and writes. Do not
	// wait behind a potentially blocked WriteJSON call when tearing down a
	// stale control session.
	return c.ws.Close()
}

func (c *websocketControl) SetReadDeadline(deadline time.Time) error {
	return c.ws.SetReadDeadline(deadline)
}
