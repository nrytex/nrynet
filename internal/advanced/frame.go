package advanced

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	FrameAuth    = "auth"
	FrameControl = "control"
	FrameData    = "data"
	FrameClose   = "close"

	maxFrameSize = 4 << 20
)

type Frame struct {
	Kind      string `json:"kind"`
	RequestID string `json:"request_id,omitempty"`
	TunnelID  string `json:"tunnel_id,omitempty"`
	Payload   []byte `json:"payload,omitempty"`
}

type AuthRequest struct {
	Token    string `json:"token"`
	DeviceID string `json:"device_id"`
	Role     string `json:"role,omitempty"`
}

func WriteFrame(writer io.Writer, frame Frame) error {
	data, err := json.Marshal(frame)
	if err != nil {
		return fmt.Errorf("marshal frame: %w", err)
	}
	if len(data) > maxFrameSize {
		return fmt.Errorf("frame too large: %d", len(data))
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(data)))
	if _, err := writer.Write(header[:]); err != nil {
		return fmt.Errorf("write frame header: %w", err)
	}
	if _, err := writer.Write(data); err != nil {
		return fmt.Errorf("write frame body: %w", err)
	}
	return nil
}

func ReadFrame(reader io.Reader) (Frame, error) {
	var header [4]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return Frame{}, fmt.Errorf("read frame header: %w", err)
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || size > maxFrameSize {
		return Frame{}, fmt.Errorf("invalid frame size: %d", size)
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(reader, data); err != nil {
		return Frame{}, fmt.Errorf("read frame body: %w", err)
	}
	var frame Frame
	if err := json.Unmarshal(data, &frame); err != nil {
		return Frame{}, fmt.Errorf("decode frame: %w", err)
	}
	if frame.Kind == "" {
		return Frame{}, errors.New("frame kind is required")
	}
	return frame, nil
}

func EncodeAuth(request AuthRequest) ([]byte, error) {
	if request.Token == "" || request.DeviceID == "" {
		return nil, errors.New("token and device_id are required")
	}
	return json.Marshal(request)
}

func DecodeAuth(frame Frame) (AuthRequest, error) {
	if frame.Kind != FrameAuth {
		return AuthRequest{}, fmt.Errorf("expected auth frame, got %q", frame.Kind)
	}
	var request AuthRequest
	if err := json.Unmarshal(frame.Payload, &request); err != nil {
		return AuthRequest{}, fmt.Errorf("decode auth request: %w", err)
	}
	if request.Token == "" || request.DeviceID == "" {
		return AuthRequest{}, errors.New("invalid auth request")
	}
	return request, nil
}
