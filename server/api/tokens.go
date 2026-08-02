package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/nat-link/nat-link/internal/auth"
	"github.com/nat-link/nat-link/internal/storage"
)

type tokenHandler struct {
	store *storage.Store
	auth  *auth.Service
}

type createTokenRequest struct {
	Name string `json:"name" binding:"required"`
}

type updateTokenRequest struct {
	Disabled *bool `json:"disabled" binding:"required"`
}

func (h tokenHandler) list(c *gin.Context) {
	tokens, err := h.store.ListTokens(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": tokens})
}

func (h tokenHandler) create(c *gin.Context) {
	var request createTokenRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "token name is required")
		return
	}
	token, cleartext, err := h.auth.CreateAgentToken(c.Request.Context(), request.Name)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusCreated, gin.H{"token": token, "value": cleartext})
}

func (h tokenHandler) update(c *gin.Context) {
	var request updateTokenRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Disabled == nil {
		respondError(c, http.StatusBadRequest, "disabled is required")
		return
	}
	if err := h.store.SetTokenDisabled(c.Request.Context(), c.Param("id"), *request.Disabled); err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

func (h tokenHandler) delete(c *gin.Context) {
	if err := h.store.DeleteToken(c.Request.Context(), c.Param("id")); err != nil {
		respondError(c, http.StatusConflict, err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}
