package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nat-link/nat-link/internal/auth"
	"github.com/nat-link/nat-link/internal/storage"
)

func NewRouter(store *storage.Store, authService *auth.Service, startedAt time.Time) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	authAPI := authHandler{service: authService}
	tokenAPI := tokenHandler{store: store, auth: authService}
	router.GET("/health", func(c *gin.Context) {
		if err := store.Ping(c.Request.Context()); err != nil {
			respondError(c, http.StatusServiceUnavailable, "database unavailable")
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "running", "uptime_seconds": int(time.Since(startedAt).Seconds())})
	})
	router.POST("/api/auth/login", authAPI.login)
	secured := router.Group("/api", requireSession(authService))
	secured.GET("/me", me)
	secured.GET("/tokens", tokenAPI.list)
	secured.POST("/tokens", tokenAPI.create)
	secured.PATCH("/tokens/:id", tokenAPI.update)
	secured.DELETE("/tokens/:id", tokenAPI.delete)
	return router
}
