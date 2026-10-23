// middleware.go
package auth

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	CtxUserID   = "userID"
	CtxUsername = "username"
	CtxRoleCode = "roleCode"
)

func Middleware(secret []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		// 安全红线:secret 与 Authorization/token 原文绝不入日志。
		slog.Debug("方法进入", "fn", "auth.Middleware", "secret_set", len(secret) > 0)
		defer func() {
			slog.Debug("方法离开", "fn", "auth.Middleware", "elapsed_ms", time.Since(start).Milliseconds())
		}()
		h := c.GetHeader("Authorization")
		token := strings.TrimPrefix(h, "Bearer ")
		if h == "" || token == h {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
			return
		}
		claims, err := Parse(secret, token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "登录已过期"})
			return
		}
		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxUsername, claims.Username)
		c.Set(CtxRoleCode, claims.RoleCode)
		c.Next()
	}
}
