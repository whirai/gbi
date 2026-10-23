package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"gbi/server/internal/service"
)

type RoleHandler struct{ svc *service.RoleService }

func NewRoleHandler(s *service.RoleService) *RoleHandler {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.NewRoleHandler")
	defer func() {
		slog.Debug("方法离开", "fn", "api.NewRoleHandler", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &RoleHandler{svc: s}
}

func (h *RoleHandler) List(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "RoleHandler.List")
	defer func() {
		slog.Debug("方法离开", "fn", "RoleHandler.List", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	roles, err := h.svc.List()
	if err != nil {
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, roles)
}

func (h *RoleHandler) Rename(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "RoleHandler.Rename")
	defer func() {
		slog.Debug("方法离开", "fn", "RoleHandler.Rename", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		Fail(c, http.StatusBadRequest, 400, "角色名称必填")
		return
	}
	r, err := h.svc.Rename(id, req.Name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Fail(c, http.StatusNotFound, 404, "角色不存在")
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, r)
}
