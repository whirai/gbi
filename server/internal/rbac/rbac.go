package rbac

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/casbin/casbin/v2"
	casbinmodel "github.com/casbin/casbin/v2/model"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"gbi/server/internal/auth"
	"gbi/server/internal/logger"
)

const modelText = `
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && keyMatch(r.obj, p.obj) && regexMatch(r.act, p.act)
`

func NewEnforcer(d *gorm.DB) (e *casbin.Enforcer, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "rbac.NewEnforcer")
	defer func() {
		slog.Debug("方法离开", "fn", "rbac.NewEnforcer", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	m, err := casbinmodel.NewModelFromString(modelText)
	if err != nil {
		return nil, err
	}
	adapter, err := gormadapter.NewAdapterByDB(d)
	if err != nil {
		return nil, err
	}
	return casbin.NewEnforcer(m, adapter)
}

func UserSubject(id uint) string {
	start := time.Now()
	slog.Debug("方法进入", "fn", "rbac.UserSubject", "id", id)
	defer func() {
		slog.Debug("方法离开", "fn", "rbac.UserSubject", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return fmt.Sprintf("user:%d", id)
}

func RoleSubject(code string) string {
	start := time.Now()
	slog.Debug("方法进入", "fn", "rbac.RoleSubject", "code", logger.Truncate(code, 32))
	defer func() {
		slog.Debug("方法离开", "fn", "rbac.RoleSubject", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return "role:" + code
}

func GroupSubject(id uint) string {
	start := time.Now()
	slog.Debug("方法进入", "fn", "rbac.GroupSubject", "id", id)
	defer func() {
		slog.Debug("方法离开", "fn", "rbac.GroupSubject", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return fmt.Sprintf("group:%d", id)
}

func PageSubject(id uint) string {
	start := time.Now()
	slog.Debug("方法进入", "fn", "rbac.PageSubject", "id", id)
	defer func() {
		slog.Debug("方法离开", "fn", "rbac.PageSubject", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return fmt.Sprintf("page:%d", id)
}

// Middleware 依赖 auth.Middleware 先行注入 userID。
func Middleware(e *casbin.Enforcer) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetUint(auth.CtxUserID)
		start := time.Now()
		slog.Debug("方法进入", "fn", "rbac.Middleware", "uid", uid)
		defer func() {
			slog.Debug("方法离开", "fn", "rbac.Middleware", "elapsed_ms", time.Since(start).Milliseconds())
		}()
		ok, err := e.Enforce(UserSubject(uid), c.Request.URL.Path, c.Request.Method)
		if err != nil || !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "message": "无权限"})
			return
		}
		c.Next()
	}
}
