package advanced

import (
	"bytes"
	"net"
	"testing"
)

func TestP2PFrameAuthenticatesDirectionSequenceAndPayload(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	frame, err := EncodeP2PFrame(key, P2PDirectionServerToAgent, 1, []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(frame, []byte("hello")) {
		t.Fatal("p2p frame exposed plaintext payload")
	}
	payload, sequence, err := DecodeP2PFrame(key, P2PDirectionServerToAgent, 0, frame)
	if err != nil || string(payload) != "hello" || sequence != 1 {
		t.Fatalf("payload=%q sequence=%d err=%v", payload, sequence, err)
	}
	if _, _, err := DecodeP2PFrame(key, P2PDirectionAgentToServer, 0, frame); err == nil {
		t.Fatal("accepted frame in the wrong direction")
	}
	if _, _, err := DecodeP2PFrame(key, P2PDirectionServerToAgent, 1, frame); err == nil {
		t.Fatal("accepted replayed frame")
	}
	frame[13] ^= 0xff
	if _, _, err := DecodeP2PFrame(key, P2PDirectionServerToAgent, 0, frame); err == nil {
		t.Fatal("accepted tampered frame")
	}
}

func TestIsExpectedUDPPeer(t *testing.T) {
	expected := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 7000}
	if !IsExpectedUDPPeer(&net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 7000}, expected) {
		t.Fatal("matching peer was rejected")
	}
	if IsExpectedUDPPeer(&net.UDPAddr{IP: net.ParseIP("127.0.0.2"), Port: 7000}, expected) {
		t.Fatal("unexpected peer was accepted")
	}
}
