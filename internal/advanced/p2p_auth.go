package advanced

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"net"
)

const (
	P2PDirectionServerToAgent byte = 1
	P2PDirectionAgentToServer byte = 2
	p2pFrameHeaderSize             = 13
	p2pFrameTagSize                = sha256.Size
	maxUDPDatagramSize             = 65507
)

var p2pFrameMagic = [4]byte{'N', 'L', 'P', '2'}

func EncodeP2PFrame(key []byte, direction byte, sequence uint64, payload []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, errors.New("p2p session key must be 32 bytes")
	}
	if len(payload) > maxUDPDatagramSize-p2pFrameHeaderSize-p2pFrameTagSize {
		return nil, errors.New("p2p datagram exceeds UDP payload limit")
	}
	frame := make([]byte, p2pFrameHeaderSize+len(payload)+p2pFrameTagSize)
	copy(frame[:4], p2pFrameMagic[:])
	frame[4] = direction
	binary.BigEndian.PutUint64(frame[5:13], sequence)
	copy(frame[p2pFrameHeaderSize:], payload)
	tagStart := len(frame) - p2pFrameTagSize
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(frame[:tagStart])
	copy(frame[tagStart:], mac.Sum(nil))
	return frame, nil
}

func DecodeP2PFrame(key []byte, direction byte, lastSequence uint64, frame []byte) ([]byte, uint64, error) {
	if len(key) != 32 || len(frame) < p2pFrameHeaderSize+p2pFrameTagSize {
		return nil, 0, errors.New("invalid p2p frame")
	}
	if !hmac.Equal(frame[:4], p2pFrameMagic[:]) || frame[4] != direction {
		return nil, 0, errors.New("invalid p2p frame header")
	}
	sequence := binary.BigEndian.Uint64(frame[5:13])
	if sequence <= lastSequence {
		return nil, 0, errors.New("replayed p2p frame")
	}
	tagStart := len(frame) - p2pFrameTagSize
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(frame[:tagStart])
	if !hmac.Equal(frame[tagStart:], mac.Sum(nil)) {
		return nil, 0, errors.New("invalid p2p frame authentication")
	}
	return append([]byte(nil), frame[p2pFrameHeaderSize:tagStart]...), sequence, nil
}

func IsExpectedUDPPeer(actual net.Addr, expected *net.UDPAddr) bool {
	udp, ok := actual.(*net.UDPAddr)
	return ok && expected != nil && udp.Port == expected.Port && udp.Zone == expected.Zone && udp.IP.Equal(expected.IP)
}
