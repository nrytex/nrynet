package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/nat-link/nat-link/internal/auth"
)

type authHandler struct {
	service *auth.Service
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h authHandler) login(c *gin.Context) {
	var request loginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "username and password are required")
		return
	}
	token, err := h.service.Login(c.Request.Context(), request.Username, request.Password)
	if err != nil {
		respondError(c, http.StatusUnauthorized, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "token_type": "Bearer"})
}

func me(c *gin.Context) {
	claims, _ := c.Get("claims")
	c.JSON(http.StatusOK, claims)
}
