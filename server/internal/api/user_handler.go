package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"gbi/server/internal/service"
)

type UserHandler struct{ svc *service.UserService }

func NewUserHandler(s *service.UserService) *UserHandler {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.NewUserHandler")
	defer func() {
		slog.Debug("方法离开", "fn", "api.NewUserHandler", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &UserHandler{svc: s}
}

func (h *UserHandler) List(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "UserHandler.List")
	defer func() {
		slog.Debug("方法离开", "fn", "UserHandler.List", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	keyword := c.Query("keyword")
	items, total, err := h.svc.List(page, size, keyword)
	if err != nil {
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, gin.H{"items": items, "total": total})
}

func (h *UserHandler) Create(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "UserHandler.Create")
	defer func() {
		slog.Debug("方法离开", "fn", "UserHandler.Create", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	var req service.UserCreate
	if err := c.ShouldBindJSON(&req); err != nil || req.Username == "" || len(req.Password) < 6 {
		Fail(c, http.StatusBadRequest, 400, "用户名必填且密码至少6位")
		return
	}
	dto, err := h.svc.Create(req)
	if err != nil {
		if errors.Is(err, service.ErrDuplicateUsername) {
			Fail(c, http.StatusBadRequest, 2002, err.Error())
			return
		}
		Fail(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	OK(c, dto)
}

func (h *UserHandler) Update(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "UserHandler.Update")
	defer func() {
		slog.Debug("方法离开", "fn", "UserHandler.Update", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req service.UserUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, 400, "请求参数无效")
		return
	}
	dto, err := h.svc.Update(id, req)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			Fail(c, http.StatusNotFound, 404, err.Error())
			return
		}
		Fail(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	OK(c, dto)
}

func (h *UserHandler) Delete(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "UserHandler.Delete")
	defer func() {
		slog.Debug("方法离开", "fn", "UserHandler.Delete", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(id); err != nil {
		if errors.Is(err, service.ErrBuiltInUser) {
			Fail(c, http.StatusBadRequest, 2003, err.Error())
			return
		}
		if errors.Is(err, service.ErrUserNotFound) {
			Fail(c, http.StatusNotFound, 404, err.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, nil)
}

func (h *UserHandler) ResetPassword(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "UserHandler.ResetPassword")
	defer func() {
		slog.Debug("方法离开", "fn", "UserHandler.ResetPassword", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Password) < 6 {
		Fail(c, http.StatusBadRequest, 400, "密码至少6位")
		return
	}
	if err := h.svc.ResetPassword(id, req.Password); err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			Fail(c, http.StatusNotFound, 404, err.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, nil)
}
