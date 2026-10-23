package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"gbi/server/internal/auth"
	"gbi/server/internal/service"
)

type PortalHandler struct {
	pages *service.PageService
	menus *service.MenuService
}

func NewPortalHandler(pages *service.PageService, menus *service.MenuService) *PortalHandler {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.NewPortalHandler")
	defer func() {
		slog.Debug("方法离开", "fn", "api.NewPortalHandler", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &PortalHandler{pages: pages, menus: menus}
}

// Pages 当前用户可见的已发布页面。
func (h *PortalHandler) Pages(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PortalHandler.Pages")
	defer func() {
		slog.Debug("方法离开", "fn", "PortalHandler.Pages", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	uid := c.GetUint(auth.CtxUserID)
	dtos, err := h.pages.VisiblePages(uid)
	if err != nil {
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, dtos)
}

// Menus 当前用户可见菜单(项已过滤,无可见项的菜单不下发)。
func (h *PortalHandler) Menus(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PortalHandler.Menus")
	defer func() {
		slog.Debug("方法离开", "fn", "PortalHandler.Menus", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	uid := c.GetUint(auth.CtxUserID)
	menus, err := h.menus.VisibleMenus(uid)
	if err != nil {
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, menus)
}
