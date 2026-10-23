package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"gbi/server/internal/auth"
	"gbi/server/internal/model"
	"gbi/server/internal/service"
)

type PageHandler struct{ svc *service.PageService }

func NewPageHandler(s *service.PageService) *PageHandler {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.NewPageHandler")
	defer func() {
		slog.Debug("方法离开", "fn", "api.NewPageHandler", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &PageHandler{svc: s}
}

// pageDetail PageDTO 字段 + config 对象。
func pageDetail(p *model.Page) gin.H {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.pageDetail", "page_id", p.ID)
	defer func() {
		slog.Debug("方法离开", "fn", "api.pageDetail", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return gin.H{
		"id": p.ID, "name": p.Name, "desc": p.Desc, "status": p.Status,
		"created_by": p.CreatedBy, "updated_at": p.UpdatedAt,
		"config": service.ParsePageConfig(p),
	}
}

// requireManager 仅 admin/developer 可管理页面(用于列表等无对象级判断的端点)。
func requireManager(c *gin.Context) bool {
	role := c.GetString(auth.CtxRoleCode)
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.requireManager", "role", role)
	defer func() {
		slog.Debug("方法离开", "fn", "api.requireManager", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return role == "admin" || role == "developer"
}

func (h *PageHandler) List(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageHandler.List")
	defer func() {
		slog.Debug("方法离开", "fn", "PageHandler.List", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if !requireManager(c) {
		Fail(c, http.StatusForbidden, 403, "无权限")
		return
	}
	dtos, err := h.svc.List()
	if err != nil {
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, dtos)
}

func (h *PageHandler) Create(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageHandler.Create")
	defer func() {
		slog.Debug("方法离开", "fn", "PageHandler.Create", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if !requireManager(c) {
		Fail(c, http.StatusForbidden, 403, "无权限")
		return
	}
	var req service.PageCreate
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		Fail(c, http.StatusBadRequest, 400, "页面名称必填")
		return
	}
	uid := c.GetUint(auth.CtxUserID)
	p, err := h.svc.Create(uid, req)
	if err != nil {
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	slog.Info("页面已创建", "page_id", p.ID, "name", p.Name)
	OK(c, pageDetail(p))
}

func (h *PageHandler) Get(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageHandler.Get")
	defer func() {
		slog.Debug("方法离开", "fn", "PageHandler.Get", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	id, ok := parseID(c)
	if !ok {
		return
	}
	p, err := h.svc.Get(id)
	if err != nil {
		if errors.Is(err, service.ErrPageNotFound) {
			Fail(c, http.StatusNotFound, 404, service.ErrPageNotFound.Error())
			return
		}
		// 非 404 的查询失败(连接/锁等)是服务端故障:固定文案,细节仅入日志。
		slog.Error("查询页面失败", "page_id", id, "err", err)
		Fail(c, http.StatusInternalServerError, 500, "服务器内部错误")
		return
	}
	uid := c.GetUint(auth.CtxUserID)
	if !requireManager(c) {
		// 普通用户:仅可见 published 且已授权;不可见一律 404(不泄露存在性)
		if p.Status != "published" {
			Fail(c, http.StatusNotFound, 404, service.ErrPageNotFound.Error())
			return
		}
		can, err := h.svc.CanView(uid, p)
		if err != nil || !can {
			Fail(c, http.StatusNotFound, 404, service.ErrPageNotFound.Error())
			return
		}
	}
	OK(c, pageDetail(p))
}

func (h *PageHandler) Update(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageHandler.Update")
	defer func() {
		slog.Debug("方法离开", "fn", "PageHandler.Update", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if !requireManager(c) {
		Fail(c, http.StatusForbidden, 403, "无权限")
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req service.PageUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, 400, "请求参数无效")
		return
	}
	uid := c.GetUint(auth.CtxUserID)
	p, err := h.svc.Update(uid, id, req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPageNotFound):
			Fail(c, http.StatusNotFound, 404, err.Error())
		case errors.Is(err, service.ErrInvalidPageConfig):
			Fail(c, http.StatusBadRequest, 2021, err.Error())
		default:
			Fail(c, http.StatusInternalServerError, 500, err.Error())
		}
		return
	}
	OK(c, pageDetail(p))
}

func (h *PageHandler) Delete(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageHandler.Delete")
	defer func() {
		slog.Debug("方法离开", "fn", "PageHandler.Delete", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if !requireManager(c) {
		Fail(c, http.StatusForbidden, 403, "无权限")
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(id); err != nil {
		if errors.Is(err, service.ErrPageNotFound) {
			Fail(c, http.StatusNotFound, 404, err.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, nil)
}

func (h *PageHandler) setStatus(c *gin.Context, status string) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageHandler.setStatus", "status", status)
	defer func() {
		slog.Debug("方法离开", "fn", "PageHandler.setStatus", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if !requireManager(c) {
		Fail(c, http.StatusForbidden, 403, "无权限")
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.svc.SetStatus(id, status); err != nil {
		if errors.Is(err, service.ErrPageNotFound) {
			Fail(c, http.StatusNotFound, 404, err.Error())
			return
		}
		Fail(c, http.StatusBadRequest, 2022, err.Error())
		return
	}
	if status == "published" {
		slog.Info("页面已发布", "page_id", id)
	}
	OK(c, nil)
}

func (h *PageHandler) Publish(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageHandler.Publish")
	defer func() {
		slog.Debug("方法离开", "fn", "PageHandler.Publish", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	h.setStatus(c, "published")
}

func (h *PageHandler) Unpublish(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageHandler.Unpublish")
	defer func() {
		slog.Debug("方法离开", "fn", "PageHandler.Unpublish", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	h.setStatus(c, "draft")
}

func (h *PageHandler) Versions(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageHandler.Versions")
	defer func() {
		slog.Debug("方法离开", "fn", "PageHandler.Versions", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if !requireManager(c) {
		Fail(c, http.StatusForbidden, 403, "无权限")
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	vs, err := h.svc.Versions(id)
	if err != nil {
		if errors.Is(err, service.ErrPageNotFound) {
			Fail(c, http.StatusNotFound, 404, err.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, vs)
}

func (h *PageHandler) Restore(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageHandler.Restore")
	defer func() {
		slog.Debug("方法离开", "fn", "PageHandler.Restore", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if !requireManager(c) {
		Fail(c, http.StatusForbidden, 403, "无权限")
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	vid, ok := parseParamID(c, "vid")
	if !ok {
		return
	}
	uid := c.GetUint(auth.CtxUserID)
	if err := h.svc.Restore(uid, id, vid); err != nil {
		if errors.Is(err, service.ErrPageNotFound) {
			Fail(c, http.StatusNotFound, 404, err.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, nil)
}

func (h *PageHandler) GetACL(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageHandler.GetACL")
	defer func() {
		slog.Debug("方法离开", "fn", "PageHandler.GetACL", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if c.GetString(auth.CtxRoleCode) != "admin" {
		Fail(c, http.StatusForbidden, 403, "无权限")
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	grants, err := h.svc.GetACL(id)
	if err != nil {
		if errors.Is(err, service.ErrPageNotFound) {
			Fail(c, http.StatusNotFound, 404, err.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, grants)
}

func (h *PageHandler) SetACL(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageHandler.SetACL")
	defer func() {
		slog.Debug("方法离开", "fn", "PageHandler.SetACL", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if c.GetString(auth.CtxRoleCode) != "admin" {
		Fail(c, http.StatusForbidden, 403, "无权限")
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req struct {
		Grants []service.ACLGrantInput `json:"grants"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, 400, "请求参数无效")
		return
	}
	if err := h.svc.SetACL(id, req.Grants); err != nil {
		if errors.Is(err, service.ErrPageNotFound) {
			Fail(c, http.StatusNotFound, 404, err.Error())
			return
		}
		Fail(c, http.StatusBadRequest, 2023, err.Error())
		return
	}
	slog.Info("页面授权已更新", "page_id", id, "grants", len(req.Grants))
	OK(c, nil)
}
