package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"gbi/server/internal/auth"
	"gbi/server/internal/service"
)

type QueryHandler struct {
	svc   *service.QueryService
	pages *service.PageService
}

func NewQueryHandler(s *service.QueryService, pages *service.PageService) *QueryHandler {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.NewQueryHandler")
	defer func() {
		slog.Debug("方法离开", "fn", "api.NewQueryHandler", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &QueryHandler{svc: s, pages: pages}
}

// Run 处理 /api/v1/query。SQL/主题错误属用户输入问题→400(未选主题 2014/SQL 错误 2015);主题不存在→404。
// 对象级授权:非管理者(普通用户)仅可查询其可见(published+已授权)页面图表引用的主题。
func (h *QueryHandler) Run(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "QueryHandler.Run")
	defer func() {
		slog.Debug("方法离开", "fn", "QueryHandler.Run", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	var req service.QueryRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.SQL == "" {
		Fail(c, http.StatusBadRequest, 400, "sql 必填")
		return
	}
	if !requireManager(c) {
		uid := c.GetUint(auth.CtxUserID)
		ok, err := h.pages.CanQuerySubjects(uid, req.SubjectIDs)
		if err != nil {
			Fail(c, http.StatusInternalServerError, 500, err.Error())
			return
		}
		if !ok {
			Fail(c, http.StatusForbidden, 403, "无权限访问该数据主题")
			return
		}
	}
	res, err := h.svc.Run(req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrSubjectNotFound):
			Fail(c, http.StatusNotFound, 404, err.Error())
		case errors.Is(err, service.ErrNoSubjectSelected):
			Fail(c, http.StatusBadRequest, 2014, err.Error())
		default:
			Fail(c, http.StatusBadRequest, 2015, err.Error()) // SQL 校验/执行错误,面向用户展示
		}
		return
	}
	OK(c, res)
}
