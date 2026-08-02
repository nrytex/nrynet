package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	netx "github.com/nat-link/nat-link/internal/advanced"
	"github.com/nat-link/nat-link/internal/auth"
	"github.com/nat-link/nat-link/internal/storage"
)

type RouterOptions struct {
	Runtime       Runtime
	Settings      []SettingItem
	RelayRegistry *netx.RelayRegistry
	RelayToken    string
}

func NewRouter(store *storage.Store, authService *auth.Service, startedAt time.Time, runtimes ...Runtime) *gin.Engine {
	options := RouterOptions{}
	if len(runtimes) > 0 {
		options.Runtime = runtimes[0]
	}
	return NewRouterWithOptions(store, authService, startedAt, options)
}

func NewRouterWithOptions(store *storage.Store, authService *auth.Service, startedAt time.Time, options RouterOptions) *gin.Engine {
	runtime := options.Runtime
	if runtime == nil {
		runtime = unavailableRuntime{}
	}
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	authAPI := authHandler{service: authService}
	tokenAPI := tokenHandler{store: store, auth: authService, runtime: runtime}
	clientAPI := clientHandler{store: store, auth: authService, runtime: runtime}
	tunnelAPI := tunnelHandler{store: store, runtime: runtime}
	overviewAPI := overviewHandler{store: store, runtime: runtime, startedAt: startedAt}
	trafficAPI := trafficHandler{store: store}
	logAPI := logHandler{store: store}
	settingsAPI := newSettingsHandler(store, options.Settings)
	relayAPI := relayHandler{registry: options.RelayRegistry, token: options.RelayToken}
	router.GET("/health", func(c *gin.Context) {
		if err := store.Ping(c.Request.Context()); err != nil {
			respondError(c, http.StatusServiceUnavailable, "database unavailable")
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "running", "uptime_seconds": int(time.Since(startedAt).Seconds())})
	})
	router.POST("/api/auth/login", authAPI.login)
	router.POST("/api/v2/relays/register", relayAPI.requireRelaySecret, relayAPI.register)
	router.POST("/api/v2/relays/:id/heartbeat", relayAPI.requireRelaySecret, relayAPI.heartbeat)
	secured := router.Group("/api", requireSession(authService))
	secured.GET("/me", me)
	secured.GET("/auth/me", me)
	secured.POST("/auth/password", authAPI.changePassword)
	secured.GET("/tokens", tokenAPI.list)
	secured.POST("/tokens", tokenAPI.create)
	secured.PATCH("/tokens/:id", tokenAPI.update)
	secured.DELETE("/tokens/:id", tokenAPI.delete)
	secured.GET("/clients", clientAPI.list)
	secured.GET("/clients/:id", clientAPI.get)
	secured.PATCH("/clients/:id", clientAPI.update)
	secured.DELETE("/clients/:id", clientAPI.delete)
	secured.POST("/clients/:id/reset-token", clientAPI.resetToken)
	secured.GET("/tunnels", tunnelAPI.list)
	secured.POST("/tunnels", tunnelAPI.create)
	secured.PUT("/tunnels/:id", tunnelAPI.update)
	secured.DELETE("/tunnels/:id", tunnelAPI.delete)
	secured.POST("/tunnels/:id/start", tunnelAPI.start)
	secured.POST("/tunnels/:id/stop", tunnelAPI.stop)
	secured.GET("/overview", overviewAPI.get)
	secured.GET("/traffic/summary", trafficAPI.summary)
	secured.GET("/logs", logAPI.list)
	secured.GET("/logs/download", logAPI.download)
	secured.DELETE("/logs", logAPI.clear)
	secured.GET("/settings", settingsAPI.list)
	secured.PATCH("/settings/:key", settingsAPI.update)
	secured.GET("/v2/relays", relayAPI.list)
	secured.GET("/v2/relays/assignments", relayAPI.assignments)
	secured.POST("/v2/relays/assignments/:tunnel_id", relayAPI.assign)
	secured.PATCH("/v2/relays/assignments/:tunnel_id", relayAPI.reassign)
	return router
}
