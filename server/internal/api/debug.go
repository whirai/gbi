package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"gbi/server/internal/auth"
)

// RegisterDebugRoutes 注册运维端点(装配层调用,状态数据以闭包注入)。
// /debug/status 仅 admin 可读,不暴露敏感配置。
func RegisterDebugRoutes(r *gin.Engine, jwtSecret []byte, status func() map[string]any) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.RegisterDebugRoutes")
	defer func() {
		slog.Debug("方法离开", "fn", "api.RegisterDebugRoutes", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	r.GET("/debug/status", auth.Middleware(jwtSecret), func(c *gin.Context) {
		if c.GetString(auth.CtxRoleCode) != "admin" {
			Fail(c, http.StatusForbidden, 403, "无权限")
			return
		}
		c.JSON(http.StatusOK, status())
	})
}
