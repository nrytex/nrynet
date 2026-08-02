package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nat-link/nat-link/internal/storage"
)

type overviewHandler struct {
	store     *storage.Store
	runtime   Runtime
	startedAt time.Time
}

func (h overviewHandler) get(c *gin.Context) {
	counts, err := h.store.Counts(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	since, _ := storage.RangeStart("today", time.Now())
	traffic, err := h.store.TrafficSummary(c.Request.Context(), since)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	connections := int64(0)
	if metrics, ok := h.runtime.(interface{ ActiveConnections() int64 }); ok {
		connections = metrics.ActiveConnections()
	}
	c.JSON(http.StatusOK, gin.H{
		"status": "running", "uptime_seconds": int(time.Since(h.startedAt).Seconds()),
		"online_clients": counts.OnlineClients, "total_clients": counts.TotalClients,
		"active_tunnels": counts.ActiveTunnels, "total_tunnels": counts.TotalTunnels,
		"connections": connections, "bandwidth_bps": 0,
		"today_upload": traffic.Upload, "today_download": traffic.Download,
	})
}
