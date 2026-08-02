package advanced

import (
	"errors"
	"testing"
	"time"
)

func TestRelayRegistryReassignsAfterFailure(t *testing.T) {
	registry := NewRelayRegistry(time.Second)
	mustRegisterRelay(t, registry, "relay-a", "127.0.0.1:9001")
	mustRegisterRelay(t, registry, "relay-b", "127.0.0.1:9002")
	first, err := registry.AssignTunnel("tun-1")
	if err != nil {
		t.Fatal(err)
	}
	registry.MarkUnhealthy(first.NodeID)
	next, err := registry.ReassignTunnel("tun-1")
	if err != nil {
		t.Fatal(err)
	}
	if next.NodeID == first.NodeID {
		t.Fatalf("expected reassignment away from failed node: %#v", next)
	}
	nodes := registry.Nodes()
	if len(nodes) != 2 || !nodes[0].Healthy {
		t.Fatalf("unexpected relay health ordering: %#v", nodes)
	}
}

func TestRelayRegistryReturnsNoHealthyRelay(t *testing.T) {
	registry := NewRelayRegistry(time.Nanosecond)
	mustRegisterRelay(t, registry, "relay-a", "127.0.0.1:9001")
	time.Sleep(time.Millisecond)
	_, err := registry.AssignTunnel("tun-1")
	if !errors.Is(err, ErrNoHealthyRelay) {
		t.Fatalf("expected no healthy relay, got %v", err)
	}
}

func TestRelayRegistryRejectsRemotePlaintextControl(t *testing.T) {
	registry := NewRelayRegistry(time.Second)
	_, err := registry.Register(RelayNode{
		ID: "relay", Address: "203.0.113.10", ControlAddr: "http://relay.example:7100",
	})
	if err == nil {
		t.Fatal("remote plaintext relay control address was accepted")
	}
	if _, err := registry.Register(RelayNode{
		ID: "relay", Address: "203.0.113.10", ControlAddr: "https://relay.example:7100",
	}); err != nil {
		t.Fatal(err)
	}
}

func mustRegisterRelay(t *testing.T, registry *RelayRegistry, id, address string) {
	t.Helper()
	if _, err := registry.Register(RelayNode{ID: id, Address: address}); err != nil {
		t.Fatal(err)
	}
}
