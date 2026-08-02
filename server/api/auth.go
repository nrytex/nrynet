package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/nat-link/nat-link/internal/auth"
)

type authHandler struct {
	service *auth.Service
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
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

func (h authHandler) changePassword(c *gin.Context) {
	var request changePasswordRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "current_password and new_password are required")
		return
	}
	claimsValue, _ := c.Get("claims")
	claims, ok := claimsValue.(jwt.MapClaims)
	adminID, _ := claims["sub"].(string)
	if !ok || adminID == "" {
		respondError(c, http.StatusUnauthorized, "invalid session claims")
		return
	}
	if err := h.service.ChangePassword(c.Request.Context(), adminID, request.CurrentPassword, request.NewPassword); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}
