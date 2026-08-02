package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nat-link/nat-link/internal/auth"
	"github.com/nat-link/nat-link/internal/model"
	"github.com/nat-link/nat-link/internal/storage"
)

type clientHandler struct {
	store   *storage.Store
	auth    *auth.Service
	runtime Runtime
}

type updateClientRequest struct {
	Name     string `json:"name"`
	Disabled *bool  `json:"disabled"`
}

type clientTraffic struct {
	Today model.TrafficSummary `json:"today"`
	Month model.TrafficSummary `json:"month"`
}

func (h clientHandler) list(c *gin.Context) {
	clients, err := h.store.ListClients(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": clients})
}

func (h clientHandler) get(c *gin.Context) {
	client, err := h.store.GetClient(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, http.StatusNotFound, "client not found")
		return
	}
	tunnels, err := h.store.ListClientTunnels(c.Request.Context(), client.ID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	traffic, err := h.clientTraffic(c, client.ID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	connectedAt, connected := h.runtime.ClientConnectedAt(client.ID)
	var connectedValue any
	connectedSeconds := int64(0)
	if connected {
		connectedValue = connectedAt
		connectedSeconds = max(0, int64(time.Since(connectedAt).Seconds()))
	}
	c.JSON(http.StatusOK, gin.H{
		"client": client, "tunnels": tunnels, "traffic": traffic,
		"connected_at": connectedValue, "connected_seconds": connectedSeconds,
	})
}

func (h clientHandler) clientTraffic(c *gin.Context, clientID string) (clientTraffic, error) {
	now := time.Now().UTC()
	todayStart, _ := storage.RangeStart("today", now)
	monthStart, _ := storage.RangeStart("month", now)
	today, err := h.store.TrafficForClient(c.Request.Context(), clientID, todayStart)
	if err != nil {
		return clientTraffic{}, err
	}
	month, err := h.store.TrafficForClient(c.Request.Context(), clientID, monthStart)
	return clientTraffic{Today: today, Month: month}, err
}

func (h clientHandler) update(c *gin.Context) {
	var request updateClientRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "invalid client update")
		return
	}
	if err := h.store.UpdateClient(c.Request.Context(), c.Param("id"), request.Name, request.Disabled); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if request.Disabled != nil && *request.Disabled {
		h.runtime.DisconnectClient(c.Param("id"))
	}
	c.Status(http.StatusNoContent)
}

func (h clientHandler) delete(c *gin.Context) {
	id := c.Param("id")
	client, err := h.store.GetClient(c.Request.Context(), id)
	if err != nil {
		respondError(c, http.StatusNotFound, "client not found")
		return
	}
	h.runtime.DisconnectClient(client.ID)
	tunnels, err := h.store.ListClientTunnels(c.Request.Context(), id)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	for _, tunnel := range tunnels {
		if err := h.runtime.StopTunnel(c.Request.Context(), tunnel.ID); err != nil {
			respondError(c, http.StatusConflict, err.Error())
			return
		}
	}
	if err := h.store.DeleteClient(c.Request.Context(), id); err != nil {
		respondError(c, http.StatusConflict, err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

func (h clientHandler) resetToken(c *gin.Context) {
	client, err := h.store.GetClient(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, http.StatusNotFound, "client not found")
		return
	}
	token, cleartext, err := h.auth.CreateAgentToken(c.Request.Context(), client.Name+" reset")
	if err == nil {
		err = h.store.UpdateClientToken(c.Request.Context(), client.ID, token.ID)
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	h.runtime.DisconnectClient(client.ID)
	c.JSON(http.StatusCreated, gin.H{"token": token, "value": cleartext})
}
