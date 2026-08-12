package client

import (
	"time"

	"github.com/gorilla/websocket"
)

func configureWebSocketKeepAlive(conn *websocket.Conn, timeout time.Duration) {
	refresh := func() error {
		return conn.SetReadDeadline(time.Now().Add(timeout))
	}
	conn.SetPingHandler(func(appData string) error {
		if err := refresh(); err != nil {
			return err
		}
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(5*time.Second))
	})
	conn.SetPongHandler(func(string) error { return refresh() })
}
