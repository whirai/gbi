package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"gbi/server/internal/service"
)

type MenuHandler struct{ svc *service.MenuService }

func NewMenuHandler(s *service.MenuService) *MenuHandler {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.NewMenuHandler")
	defer func() {
		slog.Debug("方法离开", "fn", "api.NewMenuHandler", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &MenuHandler{svc: s}
}

func (h *MenuHandler) List(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "MenuHandler.List")
	defer func() {
		slog.Debug("方法离开", "fn", "MenuHandler.List", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if !requireManager(c) {
		Fail(c, http.StatusForbidden, 403, "无权限")
		return
	}
	menus, err := h.svc.ListMenus()
	if err != nil {
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, menus)
}

func (h *MenuHandler) Create(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "MenuHandler.Create")
	defer func() {
		slog.Debug("方法离开", "fn", "MenuHandler.Create", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if !requireManager(c) {
		Fail(c, http.StatusForbidden, 403, "无权限")
		return
	}
	var req service.MenuCreate
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		Fail(c, http.StatusBadRequest, 400, "菜单名称必填")
		return
	}
	m, err := h.svc.CreateMenu(req)
	if err != nil {
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	slog.Info("菜单已创建", "menu_id", m.ID, "name", m.Name)
	OK(c, m)
}

func (h *MenuHandler) Update(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "MenuHandler.Update")
	defer func() {
		slog.Debug("方法离开", "fn", "MenuHandler.Update", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if !requireManager(c) {
		Fail(c, http.StatusForbidden, 403, "无权限")
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req service.MenuUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, 400, "请求参数无效")
		return
	}
	// 显式传空 name 会静默清空菜单名(模型无非空约束),与 Create 的校验对齐拒绝。
	if req.Name != nil && *req.Name == "" {
		Fail(c, http.StatusBadRequest, 400, "菜单名称不能为空")
		return
	}
	m, err := h.svc.UpdateMenu(id, req)
	if err != nil {
		if errors.Is(err, service.ErrMenuNotFound) {
			Fail(c, http.StatusNotFound, 404, err.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, m)
}

func (h *MenuHandler) Delete(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "MenuHandler.Delete")
	defer func() {
		slog.Debug("方法离开", "fn", "MenuHandler.Delete", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if !requireManager(c) {
		Fail(c, http.StatusForbidden, 403, "无权限")
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteMenu(id); err != nil {
		if errors.Is(err, service.ErrMenuNotFound) {
			Fail(c, http.StatusNotFound, 404, err.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, nil)
}

func (h *MenuHandler) ListItems(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "MenuHandler.ListItems")
	defer func() {
		slog.Debug("方法离开", "fn", "MenuHandler.ListItems", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if !requireManager(c) {
		Fail(c, http.StatusForbidden, 403, "无权限")
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	items, err := h.svc.ListItems(id)
	if err != nil {
		if errors.Is(err, service.ErrMenuNotFound) {
			Fail(c, http.StatusNotFound, 404, err.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, items)
}

func (h *MenuHandler) CreateItem(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "MenuHandler.CreateItem")
	defer func() {
		slog.Debug("方法离开", "fn", "MenuHandler.CreateItem", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if !requireManager(c) {
		Fail(c, http.StatusForbidden, 403, "无权限")
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req service.MenuItemCreate
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" || req.PageID == 0 {
		Fail(c, http.StatusBadRequest, 400, "菜单项名称与页面必填")
		return
	}
	item, err := h.svc.CreateItem(id, req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrMenuNotFound), errors.Is(err, service.ErrPageNotFound):
			Fail(c, http.StatusNotFound, 404, err.Error())
		default:
			Fail(c, http.StatusInternalServerError, 500, err.Error())
		}
		return
	}
	OK(c, item)
}

func (h *MenuHandler) UpdateItem(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "MenuHandler.UpdateItem")
	defer func() {
		slog.Debug("方法离开", "fn", "MenuHandler.UpdateItem", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if !requireManager(c) {
		Fail(c, http.StatusForbidden, 403, "无权限")
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	iid, ok := parseParamID(c, "iid")
	if !ok {
		return
	}
	var req service.MenuItemUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, 400, "请求参数无效")
		return
	}
	// 与 CreateItem 一致:空名称直接 400,不落库。
	if req.Name != nil && *req.Name == "" {
		Fail(c, http.StatusBadRequest, 400, "菜单项名称不能为空")
		return
	}
	item, err := h.svc.UpdateItem(id, iid, req)
	if err != nil {
		if errors.Is(err, service.ErrMenuItemNotFound) || errors.Is(err, service.ErrMenuNotFound) || errors.Is(err, service.ErrPageNotFound) {
			Fail(c, http.StatusNotFound, 404, err.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, item)
}

func (h *MenuHandler) DeleteItem(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "MenuHandler.DeleteItem")
	defer func() {
		slog.Debug("方法离开", "fn", "MenuHandler.DeleteItem", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if !requireManager(c) {
		Fail(c, http.StatusForbidden, 403, "无权限")
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	iid, ok := parseParamID(c, "iid")
	if !ok {
		return
	}
	if err := h.svc.DeleteItem(id, iid); err != nil {
		if errors.Is(err, service.ErrMenuItemNotFound) || errors.Is(err, service.ErrMenuNotFound) {
			Fail(c, http.StatusNotFound, 404, err.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, nil)
}
