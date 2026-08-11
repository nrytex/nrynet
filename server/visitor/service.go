package visitor

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/nrytex/nrynet/internal/model"
	"github.com/nrytex/nrynet/internal/protocol"
	"github.com/nrytex/nrynet/internal/storage"
	clienthub "github.com/nrytex/nrynet/server/client"
)

const signalTimeout = 20 * time.Second

type Service struct {
	store      *storage.Store
	hub        *clienthub.Hub
	iceServers []string
	upgrader   websocket.Upgrader

	mu       sync.RWMutex
	sessions map[string]*session
}

type session struct {
	id       string
	clientID string
	tunnelID string
	answer   chan protocol.VisitorWebRTCSignalPayload
	done     chan struct{}
	once     sync.Once
}

func New(store *storage.Store, hub *clienthub.Hub, iceServers []string) *Service {
	service := &Service{
		store:      store,
		hub:        hub,
		iceServers: append([]string{}, iceServers...),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		},
		sessions: make(map[string]*session),
	}
	hub.SetVisitorWebRTCHandler(service.handleAgentSignal)
	return service
}

func (s *Service) Close() {
	s.mu.Lock()
	sessions := make([]*session, 0, len(s.sessions))
	for _, current := range s.sessions {
		sessions = append(sessions, current)
	}
	s.sessions = make(map[string]*session)
	s.mu.Unlock()
	for _, current := range sessions {
		current.stop()
	}
}

func (s *Service) ServePage(c *gin.Context) {
	tunnel, ok := s.loadTunnel(c)
	if !ok {
		return
	}
	scopeURL := fmt.Sprintf("/visitor/%s/%s/", c.Param("id"), c.Param("token"))
	page, err := renderPage(pageConfig{
		TunnelName: tunnel.Name,
		SignalURL:  signalURL(c.Request, tunnel.ID, tunnel.VisitorToken),
		ICEServers: s.iceServers,
		ScopeURL:   scopeURL,
		WorkerURL:  scopeURL + "sw.js",
	})
	if err != nil {
		c.String(http.StatusInternalServerError, "visitor page unavailable")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/html; charset=utf-8", page)
}

func (s *Service) RedirectTrailingPage(c *gin.Context) {
	c.Redirect(http.StatusPermanentRedirect, fmt.Sprintf("/visitor/%s/%s", c.Param("id"), c.Param("token")))
}

func (s *Service) ServeWorker(c *gin.Context) {
	if _, ok := s.loadTunnel(c); !ok {
		return
	}
	scopeURL := fmt.Sprintf("/visitor/%s/%s/", c.Param("id"), c.Param("token"))
	c.Header("Cache-Control", "no-store")
	c.Header("Service-Worker-Allowed", scopeURL)
	c.Data(http.StatusOK, "application/javascript; charset=utf-8", []byte(visitorServiceWorker))
}

func (s *Service) ServeSignal(c *gin.Context) {
	tunnel, ok := s.loadTunnel(c)
	if !ok {
		return
	}
	connection, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer connection.Close()

	current := &session{
		id:       uuid.NewString(),
		clientID: tunnel.ClientID,
		tunnelID: tunnel.ID,
		answer:   make(chan protocol.VisitorWebRTCSignalPayload, 1),
		done:     make(chan struct{}),
	}
	s.register(current)
	defer s.unregister(current.id)

	_ = connection.SetReadDeadline(time.Now().Add(signalTimeout))
	var offer protocol.VisitorWebRTCSignalPayload
	if err := connection.ReadJSON(&offer); err != nil || offer.Kind != "offer" || offer.SDP == "" {
		return
	}
	message, err := protocol.NewMessage(protocol.TypeVisitorWebRTC, current.id, tunnel.ID,
		protocol.VisitorWebRTCSignalPayload{
			Kind: "offer", SessionID: current.id, SDP: offer.SDP,
			LocalHost: tunnel.LocalHost, LocalPort: tunnel.LocalPort,
			ICEServers: s.iceServers,
		})
	if err != nil || s.hub.SendControl(tunnel.ClientID, message) != nil {
		_ = connection.WriteJSON(protocol.VisitorWebRTCSignalPayload{Kind: "error", Error: "Agent 当前不在线"})
		return
	}

	select {
	case answer := <-current.answer:
		_ = connection.SetWriteDeadline(time.Now().Add(signalTimeout))
		_ = connection.WriteJSON(answer)
	case <-current.done:
		return
	case <-time.After(signalTimeout):
		_ = connection.WriteJSON(protocol.VisitorWebRTCSignalPayload{Kind: "error", Error: "P2P 信令超时"})
	}
}

func (s *Service) handleAgentSignal(clientID string, message protocol.ControlMessage) {
	payload, err := protocol.DecodePayload[protocol.VisitorWebRTCSignalPayload](message)
	if err != nil || payload.SessionID == "" {
		return
	}
	s.mu.RLock()
	current := s.sessions[payload.SessionID]
	s.mu.RUnlock()
	if current == nil || current.clientID != clientID || current.tunnelID != message.TunnelID {
		return
	}
	select {
	case current.answer <- payload:
	default:
	}
	if payload.Kind == "answer" {
		_ = s.hub.SendTunnelPath(clientID, message.TunnelID, protocol.TunnelPathVisitorP2P)
	}
}

func (s *Service) loadTunnel(c *gin.Context) (model.Tunnel, bool) {
	tunnel, err := s.store.GetTunnel(c.Request.Context(), c.Param("id"))
	if err != nil || tunnel.Protocol != "visitor_webrtc" || tunnel.Status != "running" || !sameSecret(tunnel.VisitorToken, c.Param("token")) {
		c.Status(http.StatusNotFound)
		return model.Tunnel{}, false
	}
	return tunnel, true
}

func (s *Service) register(current *session) {
	s.mu.Lock()
	s.sessions[current.id] = current
	s.mu.Unlock()
}

func (s *Service) unregister(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

func (current *session) stop() {
	current.once.Do(func() { close(current.done) })
}

func sameSecret(expected, actual string) bool {
	if expected == "" || actual == "" || len(expected) != len(actual) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) == 1
}

func signalURL(request *http.Request, tunnelID, token string) string {
	scheme := "ws"
	if request.TLS != nil || request.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "wss"
	}
	return fmt.Sprintf("%s://%s/visitor/webrtc/%s/%s", scheme, request.Host, tunnelID, token)
}
