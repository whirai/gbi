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

type ImportHandler struct{ svc *service.ImportService }

func NewImportHandler(s *service.ImportService) *ImportHandler {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.NewImportHandler")
	defer func() {
		slog.Debug("方法离开", "fn", "api.NewImportHandler", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &ImportHandler{svc: s}
}

// Run 处理 multipart 上传:file + mode + (table_name,kind | table_id)。
func (h *ImportHandler) Run(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "ImportHandler.Run")
	defer func() {
		slog.Debug("方法离开", "fn", "ImportHandler.Run", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	sid, ok := parseID(c)
	if !ok {
		return
	}
	// 上传上限 2GB(用户裁决:原 20MB 上限误伤大文件导入;gin 默认 32MB 内存缓冲,
	// 超出部分自动落临时文件,excelize 再整体载入内存——大文件导入由使用方自行评估资源)。
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<30)
	file, err := c.FormFile("file")
	if err != nil {
		Fail(c, http.StatusBadRequest, 400, "请上传文件(不超过 2GB)")
		return
	}
	mode := c.PostForm("mode")
	params := service.ImportParams{
		Mode:      mode,
		TableName: c.PostForm("table_name"),
		Kind:      c.PostForm("kind"),
	}
	if mode == "append" {
		tid, err := strconv.ParseUint(c.PostForm("table_id"), 10, 64)
		if err != nil {
			Fail(c, http.StatusBadRequest, 400, "append 模式需要 table_id")
			return
		}
		params.TableID = uint(tid)
	}
	if mode == "create" && (params.TableName == "" || params.Kind == "") {
		Fail(c, http.StatusBadRequest, 400, "create 模式需要 table_name 与 kind")
		return
	}
	f, err := file.Open()
	if err != nil {
		Fail(c, http.StatusBadRequest, 400, "文件打开失败")
		return
	}
	defer f.Close()

	res, err := h.svc.Run(sid, f, params)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrSubjectNotFound),
			errors.Is(err, service.ErrTableNotFound):
			Fail(c, http.StatusNotFound, 404, err.Error())
		case errors.Is(err, service.ErrDuplicateTableName),
			errors.Is(err, service.ErrInvalidTableKind):
			Fail(c, http.StatusBadRequest, 2013, err.Error())
		default:
			Fail(c, http.StatusBadRequest, 2017, err.Error()) // 解析/类型/列不匹配等用户侧错误
		}
		return
	}
	slog.Info("导入成功", "subject_id", sid, "table_id", res.TableID, "rows", res.Inserted)
	OK(c, res)
}
