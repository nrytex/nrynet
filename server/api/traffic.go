package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nrytex/nrynet/internal/storage"
)

type trafficHandler struct {
	store *storage.Store
}

func (h trafficHandler) summary(c *gin.Context) {
	since, err := storage.RangeStart(c.DefaultQuery("range", "today"), time.Now())
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	summary, err := h.store.TrafficSummary(c.Request.Context(), since)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	clients, err := h.store.TrafficByClient(c.Request.Context(), since)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	tunnels, err := h.store.TrafficByTunnel(c.Request.Context(), since)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"summary": summary, "clients": clients, "tunnels": tunnels, "since": since})
}
