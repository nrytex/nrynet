package api

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nrytex/nrynet/internal/storage"
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
	if err := h.validatePlaintextState(item.Key, text); err != nil {
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
	if text == "" && isOptionalAddressSetting(key) {
		return "", nil
	}
	if text == "" {
		return "", errors.New("setting value cannot be empty")
	}
	switch key {
	case "server.plain_enabled", "server.tls.enabled":
		if _, ok := value.(bool); !ok {
			return "", errors.New("setting value must be a boolean")
		}
	case "server.listen", "server.plain_listen", "server.data_listen", "server.plain_data_listen", "server.quic_listen",
		"server.rendezvous_listen", "server.http_listen", "server.public_data_address",
		"server.public_quic_address", "server.public_rendezvous_address":
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

func isOptionalAddressSetting(key string) bool {
	return key == "server.plain_listen" || key == "server.plain_data_listen"
}

func (h *settingsHandler) validatePlaintextState(key, text string) error {
	plainEnabled := currentBoolSetting(h.items["server.plain_enabled"])
	plainListen := currentTextSetting(h.items["server.plain_listen"])
	plainDataListen := currentTextSetting(h.items["server.plain_data_listen"])
	switch key {
	case "server.plain_enabled":
		plainEnabled = text == "true"
	case "server.plain_listen":
		plainListen = text
	case "server.plain_data_listen":
		plainDataListen = text
	default:
		return nil
	}
	if !plainEnabled {
		return nil
	}
	if plainListen == "" || plainDataListen == "" {
		return errors.New("server.plain_listen and server.plain_data_listen are required when server.plain_enabled is true")
	}
	if _, err := validateSetting("server.plain_listen", plainListen); err != nil {
		return err
	}
	if _, err := validateSetting("server.plain_data_listen", plainDataListen); err != nil {
		return err
	}
	return nil
}

func currentTextSetting(item SettingItem) string {
	return settingText(item.Value)
}

func currentBoolSetting(item SettingItem) bool {
	value, ok := item.Value.(bool)
	return ok && value
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
