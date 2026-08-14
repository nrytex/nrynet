package agent

import (
	"time"

	"github.com/gorilla/websocket"
)

const minimumWebSocketLivenessTimeout = 15 * time.Second

func websocketLivenessTimeout(heartbeatInterval time.Duration) time.Duration {
	if heartbeatInterval <= 0 {
		heartbeatInterval = defaultHeartbeatInterval
	}
	// Allow one delayed heartbeat/pong while still detecting a half-open
	// control socket without waiting for the next reconnect backoff.
	timeout := heartbeatInterval * 2
	if timeout < minimumWebSocketLivenessTimeout {
		return minimumWebSocketLivenessTimeout
	}
	return timeout
}

func configureWebSocketLiveness(conn *websocket.Conn, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = websocketLivenessTimeout(0)
	}
	refresh := func() error {
		return conn.SetReadDeadline(time.Now().Add(timeout))
	}
	if err := refresh(); err != nil {
		return err
	}
	conn.SetPongHandler(func(string) error { return refresh() })
	return nil
}
