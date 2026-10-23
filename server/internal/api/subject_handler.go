package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"gbi/server/internal/service"
)

type SubjectHandler struct{ svc *service.SubjectService }

func NewSubjectHandler(s *service.SubjectService) *SubjectHandler {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.NewSubjectHandler")
	defer func() {
		slog.Debug("方法离开", "fn", "api.NewSubjectHandler", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &SubjectHandler{svc: s}
}

func (h *SubjectHandler) List(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "SubjectHandler.List")
	defer func() {
		slog.Debug("方法离开", "fn", "SubjectHandler.List", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	subjects, err := h.svc.List()
	if err != nil {
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, subjects)
}

func (h *SubjectHandler) Create(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "SubjectHandler.Create")
	defer func() {
		slog.Debug("方法离开", "fn", "SubjectHandler.Create", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	var req service.SubjectCreate
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" || req.Code == "" {
		Fail(c, http.StatusBadRequest, 400, "主题名称与编码必填")
		return
	}
	sub, err := h.svc.Create(req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidSubjectCode):
			Fail(c, http.StatusBadRequest, 2011, err.Error())
		case errors.Is(err, service.ErrDuplicateSubjectCode):
			Fail(c, http.StatusBadRequest, 2012, err.Error())
		default:
			Fail(c, http.StatusInternalServerError, 500, err.Error())
		}
		return
	}
	slog.Info("主题已创建", "subject_id", sub.ID, "code", sub.Code)
	OK(c, sub)
}

func (h *SubjectHandler) Update(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "SubjectHandler.Update")
	defer func() {
		slog.Debug("方法离开", "fn", "SubjectHandler.Update", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req service.SubjectUpdate
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		Fail(c, http.StatusBadRequest, 400, "主题名称必填")
		return
	}
	sub, err := h.svc.Update(id, req)
	if err != nil {
		if errors.Is(err, service.ErrSubjectNotFound) {
			Fail(c, http.StatusNotFound, 404, err.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, sub)
}

func (h *SubjectHandler) Delete(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "SubjectHandler.Delete")
	defer func() {
		slog.Debug("方法离开", "fn", "SubjectHandler.Delete", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(id); err != nil {
		if errors.Is(err, service.ErrSubjectNotFound) {
			Fail(c, http.StatusNotFound, 404, err.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	slog.Info("主题已删除", "subject_id", id)
	OK(c, nil)
}
