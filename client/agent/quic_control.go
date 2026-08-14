package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	netx "github.com/nrytex/nrynet/internal/advanced"
)

type quicControl struct {
	session *netx.QUICSession
	stream  *netx.QUICStream
	agent   *Agent
	mu      sync.Mutex
}

func (*quicControl) singleDataOpen() bool { return true }

func (c *quicControl) readJSON(value any) error {
	frame, err := netx.ReadFrame(c.stream)
	if err != nil {
		return fmt.Errorf("read QUIC control frame: %w", err)
	}
	if err := json.Unmarshal(frame.Payload, value); err != nil {
		return fmt.Errorf("decode QUIC control message: %w", err)
	}
	return nil
}

func (c *quicControl) writeJSON(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode QUIC control message: %w", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := netx.WriteFrame(c.stream, netx.Frame{Kind: netx.FrameControl, Payload: data}); err != nil {
		return fmt.Errorf("write QUIC control frame: %w", err)
	}
	return nil
}

func (c *quicControl) close() error {
	_ = c.stream.Close()
	return c.session.Close()
}

func (c *quicControl) openData(ctx context.Context, requestID string) (dataConn, error) {
	data, err, usedFallback := openQUICDataWithFallback(
		ctx,
		requestID,
		func(openCtx context.Context, id string) (dataConn, error) {
			return c.session.OpenStreamFrame(openCtx, netx.Frame{Kind: netx.FrameData, RequestID: id})
		},
		func(fallbackCtx context.Context) (dataConn, error) {
			if c.agent == nil || c.agent.options.Config.DataAddress == "" {
				return nil, fmt.Errorf("legacy data address is not configured")
			}
			return c.agent.dialLegacyData(fallbackCtx)
		},
	)
	if usedFallback && c.agent != nil && err != nil {
		c.agent.logger.Warn("QUIC data channel unavailable; using TCP relay", "request_id", requestID, "error", err)
	}
	if usedFallback && err == nil {
		return &dataChannel{dataConn: data, needsHandshake: true}, nil
	}
	return data, err
}
