package integration

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	clientagent "github.com/nrytex/nrynet/client/agent"
	"github.com/nrytex/nrynet/internal/config"
	clienthub "github.com/nrytex/nrynet/server/client"
	"github.com/nrytex/nrynet/server/gateway"
	"github.com/nrytex/nrynet/server/relay"
	serverTunnel "github.com/nrytex/nrynet/server/tunnel"
)

func TestHTTPTunnelSurvivesConcurrentStaticRequests(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store, authService := testServices(t)
	cleartext := createToken(t, ctx, authService)
	// Keep the heartbeat window independent from the request burst. The agent
	// heartbeat interval is 15s, so a 2s test timeout would model an invalid
	// production configuration and make the race detector look like a relay
	// failure.
	hub := clienthub.NewHub(store, authService, 45*time.Second)
	control := httptest.NewServer(controlRouter(hub))
	defer control.Close()
	dataListener := listenTCP(t)
	broker := relay.NewBroker(authService, store, 5*time.Second)
	runBroker(t, dataListener, broker)
	agent := newHTTPStabilityAgent(t, control.URL, dataListener.Addr().String(), cleartext)
	runAgent(t, ctx, cancel, agent)
	client := waitForClient(t, store, hub, "http-stability-device")

	body := strings.Repeat("image-payload-", 4096)
	local := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "image/svg+xml")
		_, _ = io.WriteString(writer, body)
	}))
	defer local.Close()
	tunnel := createDomainTunnel(t, store, client.ID, "http", "assets.test", serverPort(t, local.URL))
	manager := serverTunnel.NewManager(store, hub, broker)
	defer manager.Close()
	if err := manager.StartTunnel(ctx, tunnel.ID); err != nil {
		t.Fatal(err)
	}

	listener := listenTCP(t)
	defer listener.Close()
	webGateway := gateway.New(store, manager)
	go func() { _ = webGateway.Run(listener) }()
	address := listener.Addr().String()
	transport := &http.Transport{
		MaxConnsPerHost:     100,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "tcp", address)
		},
	}
	defer transport.CloseIdleConnections()
	clientHTTP := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	const requests = 100
	errs := make(chan error, requests)
	var wait sync.WaitGroup
	for index := 0; index < requests; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			request, err := http.NewRequestWithContext(ctx, http.MethodGet,
				"http://assets.test/image-"+strconv.Itoa(index), nil)
			if err != nil {
				errs <- err
				return
			}
			response, err := clientHTTP.Do(request)
			if err != nil {
				errs <- err
				return
			}
			defer response.Body.Close()
			data, err := io.ReadAll(response.Body)
			if err != nil {
				errs <- err
				return
			}
			if response.StatusCode != http.StatusOK || string(data) != body {
				errs <- &unexpectedHTTPResponse{status: response.StatusCode, size: len(data)}
			}
		}(index)
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent static request failed: %v", err)
	}
	current, err := store.GetClient(context.Background(), client.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != "online" || hub.OnlineCount() != 1 {
		t.Fatalf("client lost control session during static requests: client=%+v online=%d", current, hub.OnlineCount())
	}
}

type unexpectedHTTPResponse struct {
	status int
	size   int
}

func (e *unexpectedHTTPResponse) Error() string {
	return fmt.Sprintf("unexpected HTTP response: status=%d size=%d", e.status, e.size)
}

func newHTTPStabilityAgent(t *testing.T, serverURL, dataAddress, token string) *clientagent.Agent {
	t.Helper()
	clientConfig := config.ClientConfig{
		ServerURL:   strings.Replace(serverURL, "http://", "ws://", 1) + "/agent/connect",
		DataAddress: dataAddress, Token: token, Name: "http-stability-agent", DeviceID: "http-stability-device",
	}
	client, err := clientagent.New(clientagent.NewOptions(config.Config{Client: clientConfig}, "test"), slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	return client
}
