package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/nrytex/nrynet/internal/auth"
)

func requireSession(service *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			respondError(c, http.StatusUnauthorized, "authentication required")
			return
		}
		claims, err := service.VerifyJWT(strings.TrimPrefix(header, "Bearer "))
		if err != nil {
			respondError(c, http.StatusUnauthorized, err.Error())
			return
		}
		c.Set("claims", claims)
		c.Next()
	}
}
