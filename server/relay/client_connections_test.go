package relay

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/nrytex/nrynet/internal/model"
)

func TestBrokerDisconnectClientClosesActiveStream(t *testing.T) {
	store, authService, tokenValue, clientID := newBrokerStore(t)
	broker := NewBroker(authService, store, time.Second)
	listener := listenLocal(t)
	go func() { _ = broker.Run(listener) }()
	visitorPeer, visitorBroker := net.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- broker.Pair("revoked", visitorBroker,
			model.Tunnel{ID: "tun", ClientID: clientID}, nil)
	}()
	dataConn := dialData(t, listener.Addr().String())
	writeDataHandshake(t, dataConn, tokenValue, "device-1", "revoked")
	go func() { _, _ = visitorPeer.Write([]byte("ready")) }()
	assertRead(t, dataConn, "ready")

	broker.DisconnectClient(clientID)
	if err := dataConn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := dataConn.Read(make([]byte, 1)); err == nil {
		t.Fatal("agent data connection remained open")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	_ = dataConn.Close()
	_ = visitorPeer.Close()
}

func TestBrokerDisconnectClientCancelsPendingVisitor(t *testing.T) {
	store, authService, _, clientID := newBrokerStore(t)
	broker := NewBroker(authService, store, time.Second)
	visitorPeer, visitorBroker := net.Pipe()
	defer visitorPeer.Close()
	pending, err := broker.RegisterPending("pending-disconnect", visitorBroker,
		model.Tunnel{ID: "tun", ClientID: clientID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- broker.Wait("pending-disconnect", pending) }()

	broker.DisconnectClient(clientID)
	select {
	case err := <-done:
		if !errors.Is(err, errClientDisconnected) {
			t.Fatalf("pending visitor ended with %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pending visitor was not cancelled on client disconnect")
	}
	if _, err := visitorPeer.Write([]byte("closed")); err == nil {
		t.Fatal("pending visitor connection remained open")
	}
}

func TestBrokerRejectsWrongTokenForAuthenticatedQUICStream(t *testing.T) {
	store, authService, _, clientID := newBrokerStore(t)
	client, err := store.GetClient(context.Background(), clientID)
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := authService.CreateAgentToken(context.Background(), "other")
	if err != nil {
		t.Fatal(err)
	}
	broker := NewBroker(authService, store, time.Second)
	visitorPeer, visitorBroker := net.Pipe()
	defer visitorPeer.Close()
	pending, err := broker.RegisterPending("quic-token", visitorBroker,
		model.Tunnel{ID: "tun", ClientID: clientID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer visitorBroker.Close()
	if _, err := broker.claimAuthenticatedPending(other.ID, client.DeviceID, "quic-token"); err == nil {
		t.Fatal("wrong token claimed authenticated QUIC stream")
	}
	if _, err := broker.claimAuthenticatedPending(client.TokenID, client.DeviceID, "quic-token"); err != nil {
		t.Fatal(err)
	}
	if pending == nil {
		t.Fatal("pending registration was lost")
	}
}

func TestBrokerRejectsDisabledTokenForAuthenticatedQUICStream(t *testing.T) {
	store, authService, _, clientID := newBrokerStore(t)
	client, err := store.GetClient(context.Background(), clientID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetTokenDisabled(context.Background(), client.TokenID, true); err != nil {
		t.Fatal(err)
	}
	broker := NewBroker(authService, store, time.Second)
	visitorPeer, visitorBroker := net.Pipe()
	defer visitorPeer.Close()
	defer visitorBroker.Close()
	if _, err := broker.RegisterPending("disabled-quic", visitorBroker,
		model.Tunnel{ID: "tun", ClientID: clientID}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := broker.claimAuthenticatedPending(client.TokenID, client.DeviceID, "disabled-quic"); err == nil {
		t.Fatal("disabled token claimed authenticated QUIC stream")
	}
}
