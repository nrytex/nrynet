package protocol

import (
	"encoding/json"
	"testing"
)

func TestControlMessageRoundTrip(t *testing.T) {
	message, err := NewMessage(TypeHello, "req-1", "tun-1", HelloPayload{
		Name: "office", DeviceID: "dev-1", OS: "linux", Version: "1.0.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ControlMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	payload, err := DecodePayload[HelloPayload](decoded)
	if err != nil {
		t.Fatal(err)
	}
	if payload.DeviceID != "dev-1" || decoded.TunnelID != "tun-1" {
		t.Fatalf("unexpected message: %#v %#v", decoded, payload)
	}
}

func TestDataHandshakeJSONLine(t *testing.T) {
	data, err := json.Marshal(DataHandshake{
		Token: "tok", DeviceID: "dev", RequestID: "req",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := string(data) + "\n"
	want := "{\"token\":\"tok\",\"device_id\":\"dev\",\"request_id\":\"req\"}\n"
	if got != want {
		t.Fatalf("handshake mismatch\nwant %q\n got %q", want, got)
	}
}
