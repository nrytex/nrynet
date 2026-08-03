package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nrytex/nrynet/internal/storage"
)

type logHandler struct {
	store *storage.Store
}

func (h logHandler) list(c *gin.Context) {
	filter := eventFilter(c)
	events, err := h.store.ListEvents(c.Request.Context(), filter)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	total, err := h.store.CountEvents(c.Request.Context(), filter)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": events, "total": total, "limit": filter.Limit, "offset": filter.Offset})
}

func (h logHandler) download(c *gin.Context) {
	filter := eventFilter(c)
	filter.Limit = 0
	events, err := h.store.ListEvents(c.Request.Context(), filter)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.Header("Content-Disposition", `attachment; filename="nrynet-logs.jsonl"`)
	c.Header("Content-Type", "application/x-ndjson")
	encoder := json.NewEncoder(c.Writer)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			return
		}
	}
}

func (h logHandler) clear(c *gin.Context) {
	before := time.Now().UTC().Add(time.Nanosecond)
	if value := c.Query("before"); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			respondError(c, http.StatusBadRequest, "before must be RFC3339")
			return
		}
		before = parsed
	}
	count, err := h.store.ClearEvents(c.Request.Context(), before)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": count})
}

func eventFilter(c *gin.Context) storage.EventFilter {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	return storage.EventFilter{
		Level: c.Query("level"), Keyword: c.Query("keyword"),
		Limit: limit, Offset: (page - 1) * limit,
	}
}
