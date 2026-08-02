package advanced

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"net"
)

const (
	P2PDirectionServerToAgent byte = 1
	P2PDirectionAgentToServer byte = 2
	p2pFrameHeaderSize             = 13
	p2pFrameTagSize                = 16
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
	aead, err := newP2PAEAD(key)
	if err != nil {
		return nil, err
	}
	ciphertext := aead.Seal(frame[:p2pFrameHeaderSize], p2pNonce(direction, sequence), payload, frame[:p2pFrameHeaderSize])
	return ciphertext, nil
}

func DecodeP2PFrame(key []byte, direction byte, lastSequence uint64, frame []byte) ([]byte, uint64, error) {
	if len(key) != 32 || len(frame) < p2pFrameHeaderSize+p2pFrameTagSize {
		return nil, 0, errors.New("invalid p2p frame")
	}
	if !bytes.Equal(frame[:4], p2pFrameMagic[:]) || frame[4] != direction {
		return nil, 0, errors.New("invalid p2p frame header")
	}
	sequence := binary.BigEndian.Uint64(frame[5:13])
	if sequence <= lastSequence {
		return nil, 0, errors.New("replayed p2p frame")
	}
	aead, err := newP2PAEAD(key)
	if err != nil {
		return nil, 0, err
	}
	payload, err := aead.Open(nil, p2pNonce(direction, sequence), frame[p2pFrameHeaderSize:], frame[:p2pFrameHeaderSize])
	if err != nil {
		return nil, 0, errors.New("invalid p2p frame authentication")
	}
	return payload, sequence, nil
}

func newP2PAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.New("p2p session key must be 32 bytes")
	}
	return cipher.NewGCM(block)
}

func p2pNonce(direction byte, sequence uint64) []byte {
	nonce := make([]byte, 12)
	nonce[0] = direction
	binary.BigEndian.PutUint64(nonce[4:], sequence)
	return nonce
}

func IsExpectedUDPPeer(actual net.Addr, expected *net.UDPAddr) bool {
	udp, ok := actual.(*net.UDPAddr)
	return ok && expected != nil && udp.Port == expected.Port && udp.Zone == expected.Zone && udp.IP.Equal(expected.IP)
}
