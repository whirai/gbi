package api

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"gbi/server/internal/service"
)

type BrandingHandler struct {
	svc *service.BrandingService
}

func NewBrandingHandler(svc *service.BrandingService) *BrandingHandler {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.NewBrandingHandler")
	defer func() {
		slog.Debug("方法离开", "fn", "api.NewBrandingHandler", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &BrandingHandler{svc: svc}
}

// failBranding 统一错误映射;返回 true 表示已响应。
func failBranding(c *gin.Context, err error) (responded bool) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.failBranding")
	defer func() {
		slog.Debug("方法离开", "fn", "api.failBranding", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	switch {
	case err == nil:
		return false
	case errors.Is(err, service.ErrBrandImageNotFound):
		Fail(c, http.StatusNotFound, 404, err.Error())
	case errors.Is(err, service.ErrBrandNameInvalid), errors.Is(err, service.ErrBrandTitleInvalid),
		errors.Is(err, service.ErrBrandKindInvalid), errors.Is(err, service.ErrBrandImageType),
		errors.Is(err, service.ErrBrandImageTooLarge):
		Fail(c, http.StatusBadRequest, 400, err.Error())
	default:
		Fail(c, http.StatusInternalServerError, 500, err.Error())
	}
	return true
}

// Get 公共只读:品牌 JSON(免鉴权)。
func (h *BrandingHandler) Get(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "BrandingHandler.Get")
	defer func() {
		slog.Debug("方法离开", "fn", "BrandingHandler.Get", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	dto, err := h.svc.Get()
	if failBranding(c, err) {
		return
	}
	OK(c, dto)
}

// Image 公共只读:品牌图片(kind=logo|favicon)。
func (h *BrandingHandler) Image(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "BrandingHandler.Image", "kind", c.Param("kind"))
	defer func() {
		slog.Debug("方法离开", "fn", "BrandingHandler.Image", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	data, contentType, err := h.svc.ImageBytes(c.Param("kind"))
	if failBranding(c, err) {
		return
	}
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Data(http.StatusOK, contentType, data)
}

// Update 修改系统名与浏览器标题(仅 admin)。
func (h *BrandingHandler) Update(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "BrandingHandler.Update")
	defer func() {
		slog.Debug("方法离开", "fn", "BrandingHandler.Update", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	var in struct {
		SystemName   string `json:"system_name"`
		BrowserTitle string `json:"browser_title"`
	}
	if !bindOK(c, &in) {
		return
	}
	dto, err := h.svc.UpdateNames(in.SystemName, in.BrowserTitle)
	if failBranding(c, err) {
		return
	}
	OK(c, dto)
}

// Upload 上传 logo/favicon(仅 admin):multipart 字段 file,PNG/ICO,≤512KB。
func (h *BrandingHandler) Upload(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "BrandingHandler.Upload", "kind", c.Param("kind"))
	defer func() {
		slog.Debug("方法离开", "fn", "BrandingHandler.Upload", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	fh, err := c.FormFile("file")
	if err != nil {
		Fail(c, http.StatusBadRequest, 400, "缺少 file 文件")
		return
	}
	if fh.Size > service.MaxBrandImageBytes {
		Fail(c, http.StatusBadRequest, 400, "图片不能超过 512KB")
		return
	}
	f, err := fh.Open()
	if err != nil {
		Fail(c, http.StatusBadRequest, 400, "文件读取失败")
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, int64(service.MaxBrandImageBytes)+1))
	if err != nil {
		Fail(c, http.StatusBadRequest, 400, "文件读取失败")
		return
	}
	dto, err := h.svc.SaveImage(c.Param("kind"), data)
	if failBranding(c, err) {
		return
	}
	OK(c, dto)
}

// DeleteImage 删除单个品牌图片(仅 admin,幂等)。
func (h *BrandingHandler) DeleteImage(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "BrandingHandler.DeleteImage", "kind", c.Param("kind"))
	defer func() {
		slog.Debug("方法离开", "fn", "BrandingHandler.DeleteImage", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if failBranding(c, h.svc.DeleteImage(c.Param("kind"))) {
		return
	}
	OK(c, nil)
}

// Reset 恢复默认(名称清空、删除两张图;仅 admin)。
func (h *BrandingHandler) Reset(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "BrandingHandler.Reset")
	defer func() {
		slog.Debug("方法离开", "fn", "BrandingHandler.Reset", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if failBranding(c, h.svc.Reset()) {
		return
	}
	OK(c, nil)
}
