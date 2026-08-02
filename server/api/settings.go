package api

import (
	"net/http"
	"sync"

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
	if err := h.store.SetSetting(c.Request.Context(), "config."+item.Key, settingText(request.Value)); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	item.Value = request.Value
	h.items[item.Key] = item
	c.JSON(http.StatusOK, item)
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
