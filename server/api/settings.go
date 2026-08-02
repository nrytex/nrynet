package api

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nat-link/nat-link/internal/storage"
)

type SettingItem struct {
	Key         string `json:"key"`
	Value       any    `json:"value"`
	Description string `json:"description,omitempty"`
	Mutable     bool   `json:"mutable"`
}

type settingsHandler struct {
	store *storage.Store
	mu    sync.RWMutex
	items map[string]SettingItem
	order []string
}

func (h *settingsHandler) list(c *gin.Context) {
	h.mu.RLock()
	items := make([]SettingItem, 0, len(h.items))
	for _, key := range h.order {
		items = append(items, h.items[key])
	}
	h.mu.RUnlock()
	c.JSON(http.StatusOK, gin.H{"items": items})
}

type updateSettingRequest struct {
	Value any `json:"value" binding:"required"`
}

func (h *settingsHandler) update(c *gin.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	item, exists := h.items[c.Param("key")]
	if !exists || !item.Mutable {
		respondError(c, http.StatusBadRequest, "setting is not mutable")
		return
	}
	var request updateSettingRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "value is required")
		return
	}
	text, err := validateSetting(item.Key, request.Value)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.store.SetSetting(c.Request.Context(), "config."+item.Key, text); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	item.Value = request.Value
	h.items[item.Key] = item
	c.JSON(http.StatusOK, item)
}

func validateSetting(key string, value any) (string, error) {
	text := settingText(value)
	if text == "" {
		return "", errors.New("setting value cannot be empty")
	}
	switch key {
	case "server.listen", "server.data_listen", "server.quic_listen",
		"server.rendezvous_listen", "server.http_listen":
		if _, _, err := net.SplitHostPort(text); err != nil {
			return "", fmt.Errorf("setting must be a host:port address: %w", err)
		}
	case "server.heartbeat_timeout":
		duration, err := time.ParseDuration(text)
		if err != nil || duration <= 0 {
			return "", errors.New("heartbeat timeout must be a positive duration")
		}
	}
	return text, nil
}

func settingText(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func newSettingsHandler(store *storage.Store, items []SettingItem) *settingsHandler {
	indexed := make(map[string]SettingItem, len(items))
	order := make([]string, 0, len(items))
	for _, item := range items {
		indexed[item.Key] = item
		order = append(order, item.Key)
	}
	return &settingsHandler{store: store, items: indexed, order: order}
}
