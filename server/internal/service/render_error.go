package service

import (
	"log/slog"
	"time"

	"gorm.io/gorm"

	"gbi/server/internal/logger"
	"gbi/server/internal/model"
)

type RenderErrorService struct {
	db *gorm.DB
}

func NewRenderErrorService(db *gorm.DB) *RenderErrorService {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.NewRenderErrorService")
	defer func() {
		slog.Debug("方法离开", "fn", "service.NewRenderErrorService", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &RenderErrorService{db: db}
}

// 上报字段长度约束:chart_id 与模型 tag 对齐(≤64),message 截断防止存储放大。
const (
	maxChartIDRunes = 64
	maxMessageRunes = 2000
)

func truncateRunes(s string, n int) string {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.truncateRunes", "len", len(s), "n", n)
	defer func() {
		slog.Debug("方法离开", "fn", "service.truncateRunes", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n])
}

func (s *RenderErrorService) Report(pageID uint, chartID, message string) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "RenderErrorService.Report", "page_id", pageID, "chart_id", logger.Truncate(chartID, 64))
	defer func() {
		slog.Debug("方法离开", "fn", "RenderErrorService.Report", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	// 截断而非拒绝:上报是静默降级通道,截断保证记录仍可用,不因超长丢失整条错误。
	chartID = truncateRunes(chartID, maxChartIDRunes)
	message = truncateRunes(message, maxMessageRunes)
	return s.db.Create(&model.PageRenderError{PageID: pageID, ChartID: chartID, Message: message}).Error
}

func (s *RenderErrorService) Unseen(pageID uint) (list []model.PageRenderError, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "RenderErrorService.Unseen", "page_id", pageID)
	defer func() {
		slog.Debug("方法离开", "fn", "RenderErrorService.Unseen", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	err = s.db.Where("page_id = ? AND seen = ?", pageID, false).Order("id").Find(&list).Error
	return list, err
}

func (s *RenderErrorService) MarkSeen(ids []uint) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "RenderErrorService.MarkSeen", "count", len(ids))
	defer func() {
		slog.Debug("方法离开", "fn", "RenderErrorService.MarkSeen", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	if len(ids) == 0 {
		return nil
	}
	return s.db.Model(&model.PageRenderError{}).Where("id IN ?", ids).Update("seen", true).Error
}
