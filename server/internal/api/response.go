package api

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func OK(c *gin.Context, data any) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.OK")
	defer func() {
		slog.Debug("方法离开", "fn", "api.OK", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": data})
}

func Fail(c *gin.Context, httpStatus, code int, msg string) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.Fail", "code", code)
	defer func() {
		slog.Debug("方法离开", "fn", "api.Fail", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	c.JSON(httpStatus, gin.H{"code": code, "message": msg})
}

// parseID 解析路径参数 :id;失败时已写 400 响应,返回 false。
func parseID(c *gin.Context) (uint, bool) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.parseID")
	defer func() {
		slog.Debug("方法离开", "fn", "api.parseID", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	id64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, http.StatusBadRequest, 400, "无效的ID")
		return 0, false
	}
	return uint(id64), true
}

// parseParamID 解析指定名称的路径参数(如 :tid/:rowid);失败时已写 400 响应。
func parseParamID(c *gin.Context, name string) (uint, bool) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.parseParamID", "name", name)
	defer func() {
		slog.Debug("方法离开", "fn", "api.parseParamID", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	id64, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil {
		Fail(c, http.StatusBadRequest, 400, "无效的"+name)
		return 0, false
	}
	return uint(id64), true
}
