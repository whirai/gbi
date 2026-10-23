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

type DeptHandler struct{ svc *service.DeptService }

func NewDeptHandler(s *service.DeptService) *DeptHandler {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.NewDeptHandler")
	defer func() {
		slog.Debug("方法离开", "fn", "api.NewDeptHandler", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &DeptHandler{svc: s}
}

func (h *DeptHandler) Tree(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "DeptHandler.Tree")
	defer func() {
		slog.Debug("方法离开", "fn", "DeptHandler.Tree", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	tree, err := h.svc.Tree()
	if err != nil {
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, tree)
}

func (h *DeptHandler) Create(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "DeptHandler.Create")
	defer func() {
		slog.Debug("方法离开", "fn", "DeptHandler.Create", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	var req struct {
		Name     string `json:"name"`
		ParentID *uint  `json:"parent_id"`
		Sort     int    `json:"sort"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		Fail(c, http.StatusBadRequest, 400, "部门名称必填")
		return
	}
	d, err := h.svc.Create(req.Name, req.ParentID, req.Sort)
	if err != nil {
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, d)
}

func (h *DeptHandler) Update(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "DeptHandler.Update")
	defer func() {
		slog.Debug("方法离开", "fn", "DeptHandler.Update", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
		Sort int    `json:"sort"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		Fail(c, http.StatusBadRequest, 400, "部门名称必填")
		return
	}
	d, err := h.svc.Update(id, req.Name, req.Sort)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Fail(c, http.StatusNotFound, 404, "部门不存在")
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, d)
}

func (h *DeptHandler) Delete(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "DeptHandler.Delete")
	defer func() {
		slog.Debug("方法离开", "fn", "DeptHandler.Delete", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(id); err != nil {
		if errors.Is(err, service.ErrDeptHasChildren) || errors.Is(err, service.ErrDeptHasUsers) {
			Fail(c, http.StatusBadRequest, 2001, err.Error())
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			Fail(c, http.StatusNotFound, 404, "部门不存在")
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, nil)
}
