package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/nrytex/nrynet/internal/protocol"
)

const (
	visitorDataChannelMaxBufferedBytes = 64 * 1024 * 1024
	visitorDataChannelLowThreshold     = 16 * 1024 * 1024
	visitorDataChannelPollInterval     = 100 * time.Millisecond
	visitorDataChannelSendTimeout      = 30 * time.Second
)

func (s *visitorDataSession) configureChannel(channel *webrtc.DataChannel) {
	channel.SetBufferedAmountLowThreshold(visitorDataChannelLowThreshold)
	channel.OnBufferedAmountLow(func() {
		select {
		case s.bufferedAmountLow <- struct{}{}:
		default:
		}
	})
}

func (s *visitorDataSession) sendFrame(channel *webrtc.DataChannel, message protocol.VisitorWebRTCDataMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if len(data) > visitorMaxMessageBytes {
		return fmt.Errorf("visitor response frame is too large")
	}

	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if err := s.waitForSendCapacity(channel); err != nil {
		return err
	}
	return channel.SendText(string(data))
}

func (s *visitorDataSession) waitForSendCapacity(channel *webrtc.DataChannel) error {
	deadline := time.NewTimer(visitorDataChannelSendTimeout)
	defer deadline.Stop()
	poll := time.NewTicker(visitorDataChannelPollInterval)
	defer poll.Stop()

	for {
		if err := s.sendContextError(); err != nil {
			return err
		}
		if channel.ReadyState() != webrtc.DataChannelStateOpen {
			return io.ErrClosedPipe
		}
		if channel.BufferedAmount() <= visitorDataChannelMaxBufferedBytes {
			return nil
		}

		select {
		case <-s.bufferedAmountLow:
		case <-poll.C:
		case <-deadline.C:
			if s.agent != nil && s.agent.logger != nil {
				s.agent.logger.Warn("visitor WebRTC send buffer stayed full", "buffered_bytes", channel.BufferedAmount())
			}
			s.close()
			_ = channel.Close()
			return fmt.Errorf("visitor data channel send buffer stayed full")
		case <-s.ctx.Done():
			return s.ctx.Err()
		}
	}
}

func (s *visitorDataSession) sendContextError() error {
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	default:
		return nil
	}
}
