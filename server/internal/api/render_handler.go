package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"gbi/server/internal/auth"
	"gbi/server/internal/logger"
	"gbi/server/internal/service"
)

type RenderErrorHandler struct {
	svc   *service.RenderErrorService
	pages *service.PageService
}

func NewRenderErrorHandler(svc *service.RenderErrorService, pages *service.PageService) *RenderErrorHandler {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.NewRenderErrorHandler")
	defer func() {
		slog.Debug("方法离开", "fn", "api.NewRenderErrorHandler", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &RenderErrorHandler{svc: svc, pages: pages}
}

// Report POST /api/v1/pages/:id/render-errors 前端渲染错误上报(静默降级:失败不影响渲染)。
func (h *RenderErrorHandler) Report(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "RenderErrorHandler.Report")
	defer func() {
		slog.Debug("方法离开", "fn", "RenderErrorHandler.Report", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	pageID, ok := parseParamID(c, "id")
	if !ok {
		return
	}
	uid := c.GetUint(auth.CtxUserID)
	p, err := h.pages.Get(pageID)
	if err != nil {
		Fail(c, http.StatusNotFound, 404, "页面不存在")
		return
	}
	okView, err := h.pages.CanView(uid, p)
	if err != nil {
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	if !okView {
		Fail(c, http.StatusForbidden, 403, "无权限")
		return
	}
	var req struct {
		ChartID string `json:"chart_id"`
		Message string `json:"message"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Message == "" {
		Fail(c, http.StatusBadRequest, 400, "chart_id/message 必填")
		return
	}
	slog.Debug("渲染错误上报", "page_id", pageID, "chart_id", req.ChartID, "msg_preview", logger.Truncate(req.Message, 120))
	if err := h.svc.Report(pageID, req.ChartID, req.Message); err != nil {
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, gin.H{"reported": true})
}
