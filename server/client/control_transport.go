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
	return c.ws.WriteJSON(value)
}

func (c *websocketControl) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ws.Close()
}

func (c *websocketControl) SetReadDeadline(deadline time.Time) error {
	return c.ws.SetReadDeadline(deadline)
}
