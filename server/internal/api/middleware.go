package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"gbi/server/internal/config"
)

// corsMiddleware 全 API 响应加 CORS 头;OPTIONS 预检直接 200。
// 头集与 webapp 包静态响应一致(与 webserver.go 相同)。
func corsMiddleware(cfg *config.Config) gin.HandlerFunc {
	origin := cfg.CORSOrigin
	return func(c *gin.Context) {
		start := time.Now()
		slog.Debug("方法进入", "fn", "api.corsMiddleware")
		defer func() {
			slog.Debug("方法离开", "fn", "api.corsMiddleware", "elapsed_ms", time.Since(start).Milliseconds())
		}()
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusOK)
			return
		}
		c.Next()
	}
}
