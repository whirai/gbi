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

type GroupHandler struct{ svc *service.GroupService }

func NewGroupHandler(s *service.GroupService) *GroupHandler {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.NewGroupHandler")
	defer func() {
		slog.Debug("方法离开", "fn", "api.NewGroupHandler", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &GroupHandler{svc: s}
}

func (h *GroupHandler) List(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "GroupHandler.List")
	defer func() {
		slog.Debug("方法离开", "fn", "GroupHandler.List", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	groups, err := h.svc.List()
	if err != nil {
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, groups)
}

func (h *GroupHandler) Create(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "GroupHandler.Create")
	defer func() {
		slog.Debug("方法离开", "fn", "GroupHandler.Create", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	var req struct {
		Name string `json:"name"`
		Desc string `json:"desc"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		Fail(c, http.StatusBadRequest, 400, "组名称必填")
		return
	}
	g, err := h.svc.Create(req.Name, req.Desc)
	if err != nil {
		if errors.Is(err, service.ErrDuplicateGroupName) {
			Fail(c, http.StatusBadRequest, 2004, err.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, g)
}

func (h *GroupHandler) Update(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "GroupHandler.Update")
	defer func() {
		slog.Debug("方法离开", "fn", "GroupHandler.Update", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
		Desc string `json:"desc"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		Fail(c, http.StatusBadRequest, 400, "组名称必填")
		return
	}
	g, err := h.svc.Update(id, req.Name, req.Desc)
	if err != nil {
		if errors.Is(err, service.ErrGroupNotFound) {
			Fail(c, http.StatusNotFound, 404, "用户组不存在")
			return
		}
		if errors.Is(err, service.ErrDuplicateGroupName) {
			Fail(c, http.StatusBadRequest, 2004, err.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, g)
}

func (h *GroupHandler) Delete(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "GroupHandler.Delete")
	defer func() {
		slog.Debug("方法离开", "fn", "GroupHandler.Delete", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(id); err != nil {
		if errors.Is(err, service.ErrGroupNotFound) {
			Fail(c, http.StatusNotFound, 404, err.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, nil)
}

func (h *GroupHandler) Members(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "GroupHandler.Members")
	defer func() {
		slog.Debug("方法离开", "fn", "GroupHandler.Members", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	id, ok := parseID(c)
	if !ok {
		return
	}
	members, err := h.svc.Members(id)
	if err != nil {
		if errors.Is(err, service.ErrGroupNotFound) {
			Fail(c, http.StatusNotFound, 404, err.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, members)
}

func (h *GroupHandler) AddMembers(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "GroupHandler.AddMembers")
	defer func() {
		slog.Debug("方法离开", "fn", "GroupHandler.AddMembers", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req struct {
		UserIDs []uint `json:"user_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.UserIDs) == 0 {
		Fail(c, http.StatusBadRequest, 400, "user_ids 必填")
		return
	}
	if err := h.svc.AddMembers(id, req.UserIDs); err != nil {
		if errors.Is(err, service.ErrInvalidUserID) || errors.Is(err, service.ErrMemberUserNotFound) {
			Fail(c, http.StatusBadRequest, 2005, err.Error())
			return
		}
		if errors.Is(err, service.ErrGroupNotFound) {
			Fail(c, http.StatusNotFound, 404, err.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, nil)
}

func (h *GroupHandler) RemoveMember(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "GroupHandler.RemoveMember")
	defer func() {
		slog.Debug("方法离开", "fn", "GroupHandler.RemoveMember", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	id, ok := parseID(c)
	if !ok {
		return
	}
	uid64, err := strconv.ParseUint(c.Param("uid"), 10, 64)
	if err != nil {
		Fail(c, http.StatusBadRequest, 400, "无效的用户ID")
		return
	}
	if err := h.svc.RemoveMember(id, uint(uid64)); err != nil {
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, nil)
}
