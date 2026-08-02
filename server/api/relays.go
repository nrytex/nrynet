package api

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	netx "github.com/nat-link/nat-link/internal/advanced"
)

const relaySecretHeader = "X-NAT-Link-Relay-Token"

type relayHandler struct {
	registry *netx.RelayRegistry
	token    string
}

type relayHeartbeatRequest struct {
	Connections int `json:"connections"`
}

func (h relayHandler) requireRelaySecret(c *gin.Context) {
	expected := strings.TrimSpace(h.token)
	if expected == "" {
		respondError(c, http.StatusUnauthorized, "relay authentication is not configured")
		c.Abort()
		return
	}
	actual := strings.TrimSpace(c.GetHeader(relaySecretHeader))
	if subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) != 1 {
		respondError(c, http.StatusUnauthorized, "invalid relay token")
		c.Abort()
	}
}

func (h relayHandler) register(c *gin.Context) {
	registry, ok := h.activeRegistry(c)
	if !ok {
		return
	}
	var node netx.RelayNode
	if err := c.ShouldBindJSON(&node); err != nil {
		respondError(c, http.StatusBadRequest, "invalid relay registration")
		return
	}
	registered, err := registry.Register(node)
	respondValue(c, registered, err)
}

func (h relayHandler) heartbeat(c *gin.Context) {
	registry, ok := h.activeRegistry(c)
	if !ok {
		return
	}
	var request relayHeartbeatRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "invalid relay heartbeat")
		return
	}
	node, err := registry.Heartbeat(c.Param("id"), request.Connections)
	respondValue(c, node, err)
}

func (h relayHandler) list(c *gin.Context) {
	registry, ok := h.activeRegistry(c)
	if ok {
		c.JSON(http.StatusOK, gin.H{"nodes": registry.Nodes()})
	}
}

func (h relayHandler) assignments(c *gin.Context) {
	registry, ok := h.activeRegistry(c)
	if ok {
		c.JSON(http.StatusOK, gin.H{"assignments": registry.Assignments()})
	}
}

func (h relayHandler) assign(c *gin.Context) {
	registry, ok := h.activeRegistry(c)
	if !ok {
		return
	}
	assignment, err := registry.AssignTunnel(c.Param("tunnel_id"))
	respondValue(c, assignment, err)
}

func (h relayHandler) reassign(c *gin.Context) {
	registry, ok := h.activeRegistry(c)
	if !ok {
		return
	}
	assignment, err := registry.ReassignTunnel(c.Param("tunnel_id"))
	respondValue(c, assignment, err)
}

func (h relayHandler) activeRegistry(c *gin.Context) (*netx.RelayRegistry, bool) {
	if h.registry == nil {
		respondError(c, http.StatusServiceUnavailable, "relay registry is disabled")
		return nil, false
	}
	return h.registry, true
}

func respondValue(c *gin.Context, value any, err error) {
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	c.JSON(http.StatusOK, value)
}
