package service

import (
	"errors"
	"log/slog"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gbi/server/internal/logger"
	"gbi/server/internal/model"
)

type StudioService struct{ db *gorm.DB }

func NewStudioService(db *gorm.DB) *StudioService {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.NewStudioService")
	defer func() {
		slog.Debug("方法离开", "fn", "service.NewStudioService", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &StudioService{db: db}
}

// GetOrCreateConversation 每页一个会话。
// 仅 ErrRecordNotFound 走创建;并发首条消息时靠唯一索引 + OnConflict DoNothing 幂等,
// 冲突后重查,避免一个请求因唯一键冲突而 500。
func (s *StudioService) GetOrCreateConversation(pageID, userID uint) (res *model.Conversation, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "StudioService.GetOrCreateConversation", "page_id", pageID, "user_id", userID)
	defer func() {
		slog.Debug("方法离开", "fn", "StudioService.GetOrCreateConversation", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	slog.Debug("StudioService.GetOrCreateConversation", "page_id", pageID)
	var conv model.Conversation
	err = s.db.Where("page_id = ?", pageID).First(&conv).Error
	if err == nil {
		slog.Debug("StudioService.GetOrCreateConversation成功", "conv_id", conv.ID)
		return &conv, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		slog.Debug("StudioService.GetOrCreateConversation错误", "err", err)
		return nil, err
	}
	conv = model.Conversation{PageID: pageID, Title: "生成式对话", CreatedBy: userID}
	if err := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&conv).Error; err != nil {
		slog.Debug("StudioService.GetOrCreateConversation错误", "err", err)
		return nil, err
	}
	if conv.ID == 0 {
		// 冲突未插入(并发创建):重查拿对方已插入的行
		if err := s.db.Where("page_id = ?", pageID).First(&conv).Error; err != nil {
			slog.Debug("StudioService.GetOrCreateConversation错误", "err", err)
			return nil, err
		}
	}
	slog.Debug("StudioService.GetOrCreateConversation成功", "conv_id", conv.ID)
	return &conv, nil
}

func (s *StudioService) Messages(convID uint) (msgs []model.Message, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "StudioService.Messages", "conv_id", convID)
	defer func() {
		slog.Debug("方法离开", "fn", "StudioService.Messages", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	err = s.db.Where("conversation_id = ?", convID).Order("id").Find(&msgs).Error
	return msgs, err
}

func (s *StudioService) AppendMessage(convID uint, role, content string) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "StudioService.AppendMessage", "conv_id", convID, "role", logger.Truncate(role, 64))
	defer func() {
		slog.Debug("方法离开", "fn", "StudioService.AppendMessage", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	slog.Debug("StudioService.AppendMessage", "conv_id", convID, "role", role, "content_len", len([]rune(content)), "preview", logger.Truncate(content, 100))
	msg := &model.Message{ConversationID: convID, Role: role, Content: content}
	err = s.db.Create(msg).Error
	if err != nil {
		slog.Debug("StudioService.AppendMessage错误", "err", err)
		return err
	}
	slog.Debug("StudioService.AppendMessage成功", "id", msg.ID)
	return nil
}

// UpdateSummary 持久化滚动摘要(upto 为已摘要到的最后一条消息 ID)。
func (s *StudioService) UpdateSummary(convID uint, summary string, upto uint) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "StudioService.UpdateSummary", "conv_id", convID, "upto", upto)
	defer func() {
		slog.Debug("方法离开", "fn", "StudioService.UpdateSummary", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	return s.db.Model(&model.Conversation{}).Where("id = ?", convID).
		Updates(map[string]any{"summary": summary, "summary_upto_id": upto}).Error
}

// DeleteConversation 清空页面会话(仅按 page_id 定位,与 GetOrCreateConversation 对称;
// manager 门控已在 handler 层,页面配置不受影响)。
func (s *StudioService) DeleteConversation(pageID uint) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "StudioService.DeleteConversation", "page_id", pageID)
	defer func() {
		slog.Debug("方法离开", "fn", "StudioService.DeleteConversation", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	slog.Debug("StudioService.DeleteConversation", "page_id", pageID)
	var conv model.Conversation
	if err := s.db.Where("page_id = ?", pageID).First(&conv).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // 无会话=已清空
		}
		return err
	}
	// 消息与会话两条删除包进同一事务:避免「消息已清、会话行(schema 含摘要)残留」的半清空态。
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("conversation_id = ?", conv.ID).Delete(&model.Message{}).Error; err != nil {
			return err
		}
		return tx.Delete(&conv).Error
	}); err != nil {
		slog.Debug("StudioService.DeleteConversation错误", "err", err)
		return err
	}
	slog.Debug("StudioService.DeleteConversation成功", "conv_id", conv.ID)
	return nil
}
