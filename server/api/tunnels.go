package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/nrytex/nrynet/internal/model"
	"github.com/nrytex/nrynet/internal/storage"
)

type tunnelHandler struct {
	store   *storage.Store
	runtime Runtime
}

func (h tunnelHandler) list(c *gin.Context) {
	tunnels, err := h.store.ListTunnels(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": tunnels})
}

func (h tunnelHandler) create(c *gin.Context) {
	var tunnel model.Tunnel
	if err := c.ShouldBindJSON(&tunnel); err != nil {
		respondError(c, http.StatusBadRequest, "invalid tunnel")
		return
	}
	requestedStatus := tunnel.Status
	created, err := h.store.CreateTunnel(c.Request.Context(), tunnel)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if requestedStatus == "running" {
		if err := h.runtime.StartTunnel(c.Request.Context(), created.ID); err != nil {
			respondError(c, http.StatusConflict, err.Error())
			return
		}
		created, _ = h.store.GetTunnel(c.Request.Context(), created.ID)
	}
	_ = h.store.RecordEvent(c.Request.Context(), "info", "tunnel.created", "Tunnel created", map[string]any{
		"tunnel_id": created.ID, "name": created.Name, "protocol": created.Protocol,
	})
	c.JSON(http.StatusCreated, created)
}

func (h tunnelHandler) update(c *gin.Context) {
	current, err := h.store.GetTunnel(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, http.StatusNotFound, "tunnel not found")
		return
	}
	var request model.Tunnel
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "invalid tunnel")
		return
	}
	request.ID = current.ID
	if current.Status == "running" {
		_ = h.runtime.StopTunnel(c.Request.Context(), current.ID)
	}
	updated, err := h.store.UpdateTunnel(c.Request.Context(), request)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if current.Status == "running" {
		err = h.runtime.StartTunnel(c.Request.Context(), updated.ID)
	}
	_ = h.runtime.SyncClient(c.Request.Context(), updated.ClientID)
	if err != nil {
		respondError(c, http.StatusConflict, err.Error())
		return
	}
	_ = h.store.RecordEvent(c.Request.Context(), "info", "tunnel.updated", "Tunnel updated", map[string]any{
		"tunnel_id": updated.ID, "name": updated.Name,
	})
	c.JSON(http.StatusOK, updated)
}

func (h tunnelHandler) start(c *gin.Context) {
	if err := h.runtime.StartTunnel(c.Request.Context(), c.Param("id")); err != nil {
		respondError(c, http.StatusConflict, err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

func (h tunnelHandler) stop(c *gin.Context) {
	if err := h.runtime.StopTunnel(c.Request.Context(), c.Param("id")); err != nil {
		respondError(c, http.StatusConflict, err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

func (h tunnelHandler) delete(c *gin.Context) {
	id := c.Param("id")
	_ = h.runtime.StopTunnel(c.Request.Context(), id)
	if err := h.store.DeleteTunnel(c.Request.Context(), id); err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	_ = h.store.RecordEvent(c.Request.Context(), "info", "tunnel.deleted", "Tunnel deleted",
		map[string]any{"tunnel_id": id})
	c.Status(http.StatusNoContent)
}
