package integration

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nrytex/nrynet/internal/storage"
	clienthub "github.com/nrytex/nrynet/server/client"
	"github.com/nrytex/nrynet/server/relay"
	serverTunnel "github.com/nrytex/nrynet/server/tunnel"
)

func TestTCPTunnelServes256ConcurrentConnections(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store, authService := testServices(t)
	token := createToken(t, ctx, authService)
	hub := clienthub.NewHub(store, authService, 30*time.Second)
	control := httptest.NewServer(controlRouter(hub))
	defer control.Close()
	dataListener := listenTCP(t)
	broker := relay.NewBroker(authService, store, 5*time.Second)
	runBroker(t, dataListener, broker)
	echo := startEcho(t)
	defer echo.Close()
	agent := newAgent(t, control.URL, dataListener.Addr().String(), token)
	runAgent(t, ctx, cancel, agent)
	client := waitForClient(t, store, hub, "integration-device")
	remotePort := reservePort(t)
	manager := serverTunnel.NewManager(store, hub, broker)
	defer manager.Close()
	tunnel := createTunnel(t, store, client.ID, echo.Addr().(*net.TCPAddr).Port, remotePort)
	if err := manager.StartTunnel(ctx, tunnel.ID); err != nil {
		t.Fatal(err)
	}

	const connections = 256
	errs := make(chan error, connections)
	var wg sync.WaitGroup
	wg.Add(connections)
	for index := 0; index < connections; index++ {
		go func(index int) {
			defer wg.Done()
			payload := []byte(fmt.Sprintf("request-%03d", index))
			for attempt := 0; attempt < 20; attempt++ {
				visitor, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprint(remotePort)), 5*time.Second)
				if err != nil {
					time.Sleep(50 * time.Millisecond)
					continue
				}
				_, writeErr := visitor.Write(payload)
				response := make([]byte, len(payload))
				_, readErr := io.ReadFull(visitor, response)
				_ = visitor.Close()
				if writeErr == nil && readErr == nil && string(response) == string(payload) {
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
			errs <- fmt.Errorf("request %d did not complete", index)
		}(index)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	assertNoConnectionFailures(t, store)
	current, err := store.GetClient(context.Background(), client.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != "online" || hub.OnlineCount() != 1 {
		t.Fatalf("client lost control session during concurrent relay: client=%+v online=%d", current, hub.OnlineCount())
	}
}

func assertNoConnectionFailures(t *testing.T, store *storage.Store) {
	t.Helper()
	events, err := store.ListEvents(context.Background(), storage.EventFilter{Limit: 0})
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Event == "connection.failed" {
			t.Fatalf("high-concurrency relay recorded connection failure: %s", event.Message)
		}
	}
}
