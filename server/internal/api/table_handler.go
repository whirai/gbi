package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"gbi/server/internal/service"
)

type TableHandler struct {
	tables *service.TableService
	rows   *service.RowsService
}

func NewTableHandler(tables *service.TableService, rows *service.RowsService) *TableHandler {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.NewTableHandler")
	defer func() {
		slog.Debug("方法离开", "fn", "api.NewTableHandler", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &TableHandler{tables: tables, rows: rows}
}

func (h *TableHandler) List(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "TableHandler.List")
	defer func() {
		slog.Debug("方法离开", "fn", "TableHandler.List", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	sid, ok := parseID(c)
	if !ok {
		return
	}
	dtos, err := h.tables.List(sid)
	if err != nil {
		if errors.Is(err, service.ErrSubjectNotFound) {
			Fail(c, http.StatusNotFound, 404, err.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, dtos)
}

func (h *TableHandler) Create(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "TableHandler.Create")
	defer func() {
		slog.Debug("方法离开", "fn", "TableHandler.Create", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	sid, ok := parseID(c)
	if !ok {
		return
	}
	var req service.TableCreate
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" || len(req.Columns) == 0 {
		Fail(c, http.StatusBadRequest, 400, "表名与至少一列必填")
		return
	}
	dto, err := h.tables.Create(sid, req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrSubjectNotFound):
			Fail(c, http.StatusNotFound, 404, err.Error())
		case errors.Is(err, service.ErrInvalidTableKind),
			errors.Is(err, service.ErrDuplicateTableName):
			Fail(c, http.StatusBadRequest, 2013, err.Error())
		default:
			Fail(c, http.StatusBadRequest, 400, err.Error()) // 标识符/列类型等校验错误
		}
		return
	}
	OK(c, dto)
}

func (h *TableHandler) Delete(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "TableHandler.Delete")
	defer func() {
		slog.Debug("方法离开", "fn", "TableHandler.Delete", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	sid, ok := parseID(c)
	if !ok {
		return
	}
	tid, ok := parseParamID(c, "tid")
	if !ok {
		return
	}
	if err := h.tables.Delete(sid, tid); err != nil {
		if errors.Is(err, service.ErrTableNotFound) || errors.Is(err, service.ErrSubjectNotFound) {
			Fail(c, http.StatusNotFound, 404, err.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, nil)
}

func (h *TableHandler) ListRows(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "TableHandler.ListRows")
	defer func() {
		slog.Debug("方法离开", "fn", "TableHandler.ListRows", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	sid, ok := parseID(c)
	if !ok {
		return
	}
	tid, ok := parseParamID(c, "tid")
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	// filters 为 JSON 对象(列名 → 候选值数组),URL 编码后传入;空/缺省表示不过滤
	var filters map[string][]string
	if raw := c.Query("filters"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &filters); err != nil {
			Fail(c, http.StatusBadRequest, 400, "filters 参数格式错误")
			return
		}
	}
	res, err := h.rows.List(sid, tid, page, size, filters)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTableNotFound), errors.Is(err, service.ErrSubjectNotFound):
			Fail(c, http.StatusNotFound, 404, err.Error())
		case errors.Is(err, service.ErrInvalidColumn):
			Fail(c, http.StatusBadRequest, 400, err.Error())
		default:
			Fail(c, http.StatusInternalServerError, 500, err.Error())
		}
		return
	}
	OK(c, res)
}

// DistinctValues 返回某列的去重取值(筛选下拉选项);limit 由服务端默认 100。
func (h *TableHandler) DistinctValues(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "TableHandler.DistinctValues")
	defer func() {
		slog.Debug("方法离开", "fn", "TableHandler.DistinctValues", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	sid, ok := parseID(c)
	if !ok {
		return
	}
	tid, ok := parseParamID(c, "tid")
	if !ok {
		return
	}
	vals, err := h.rows.DistinctValues(sid, tid, c.Param("column"), 0)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTableNotFound), errors.Is(err, service.ErrSubjectNotFound):
			Fail(c, http.StatusNotFound, 404, err.Error())
		case errors.Is(err, service.ErrInvalidColumn):
			Fail(c, http.StatusBadRequest, 400, err.Error())
		default:
			Fail(c, http.StatusInternalServerError, 500, err.Error())
		}
		return
	}
	OK(c, vals)
}

func (h *TableHandler) UpdateCell(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "TableHandler.UpdateCell")
	defer func() {
		slog.Debug("方法离开", "fn", "TableHandler.UpdateCell", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	sid, ok := parseID(c)
	if !ok {
		return
	}
	tid, ok := parseParamID(c, "tid")
	if !ok {
		return
	}
	rowID, err := strconv.ParseInt(c.Param("rowid"), 10, 64)
	if err != nil {
		Fail(c, http.StatusBadRequest, 400, "无效的rowid")
		return
	}
	var req struct {
		Column string `json:"column"`
		Value  any    `json:"value"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Column == "" {
		Fail(c, http.StatusBadRequest, 400, "column 必填")
		return
	}
	if err := h.rows.UpdateCell(sid, tid, rowID, req.Column, req.Value); err != nil {
		switch {
		case errors.Is(err, service.ErrRowNotFound),
			errors.Is(err, service.ErrTableNotFound),
			errors.Is(err, service.ErrSubjectNotFound):
			Fail(c, http.StatusNotFound, 404, err.Error())
		case errors.Is(err, service.ErrInvalidColumn),
			errors.Is(err, service.ErrInvalidValue):
			Fail(c, http.StatusBadRequest, 2016, err.Error())
		default:
			Fail(c, http.StatusInternalServerError, 500, err.Error())
		}
		return
	}
	OK(c, nil)
}

func (h *TableHandler) DeleteRow(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "TableHandler.DeleteRow")
	defer func() {
		slog.Debug("方法离开", "fn", "TableHandler.DeleteRow", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	sid, ok := parseID(c)
	if !ok {
		return
	}
	tid, ok := parseParamID(c, "tid")
	if !ok {
		return
	}
	rowID, err := strconv.ParseInt(c.Param("rowid"), 10, 64)
	if err != nil {
		Fail(c, http.StatusBadRequest, 400, "无效的rowid")
		return
	}
	if err := h.rows.DeleteRow(sid, tid, rowID); err != nil {
		if errors.Is(err, service.ErrRowNotFound) || errors.Is(err, service.ErrTableNotFound) || errors.Is(err, service.ErrSubjectNotFound) {
			Fail(c, http.StatusNotFound, 404, err.Error())
			return
		}
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, nil)
}
