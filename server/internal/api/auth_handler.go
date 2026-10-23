// auth_handler.go
package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"gbi/server/internal/auth"
	"gbi/server/internal/service"
)

type AuthHandler struct{ svc *service.AuthService }

func NewAuthHandler(s *service.AuthService) *AuthHandler {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.NewAuthHandler")
	defer func() {
		slog.Debug("方法离开", "fn", "api.NewAuthHandler", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &AuthHandler{svc: s}
}

func (h *AuthHandler) Login(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "AuthHandler.Login")
	defer func() {
		slog.Debug("方法离开", "fn", "AuthHandler.Login", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, 400, "请求参数无效")
		return
	}
	res, err := h.svc.Login(req.Username, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrUserDisabled) {
			Fail(c, http.StatusForbidden, 1002, err.Error())
			return
		}
		// 对外统一固定文案,真实错误仅落服务端日志(避免回传内部实现细节)
		slog.Error("登录失败", "username", req.Username, "err", err)
		Fail(c, http.StatusUnauthorized, 1001, "用户名或密码错误")
		return
	}
	slog.Info("登录成功", "username", req.Username)
	OK(c, res)
}

func (h *AuthHandler) Me(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "AuthHandler.Me")
	defer func() {
		slog.Debug("方法离开", "fn", "AuthHandler.Me", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	uid := c.GetUint(auth.CtxUserID)
	res, err := h.svc.Me(uid)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Fail(c, http.StatusNotFound, 404, "用户不存在")
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, res)
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "AuthHandler.ChangePassword")
	defer func() {
		slog.Debug("方法离开", "fn", "AuthHandler.ChangePassword", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	uid := c.GetUint(auth.CtxUserID)
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, 400, "请求参数无效")
		return
	}
	if utf8.RuneCountInString(req.NewPassword) < 6 {
		Fail(c, http.StatusBadRequest, 400, "新密码至少6位")
		return
	}
	if err := h.svc.ChangePassword(uid, req.OldPassword, req.NewPassword); err != nil {
		if errors.Is(err, service.ErrInvalidCredential) {
			// 用 400 而非 401:本请求已通过 JWT 鉴权,避免 web client 对 401 强制登出
			Fail(c, http.StatusBadRequest, 1001, "原密码错误")
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, nil)
}
