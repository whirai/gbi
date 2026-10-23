package service

import (
	"log/slog"
	"time"

	"gorm.io/gorm"

	"gbi/server/internal/logger"
	"gbi/server/internal/model"
)

type RoleService struct{ db *gorm.DB }

func NewRoleService(db *gorm.DB) *RoleService {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.NewRoleService")
	defer func() {
		slog.Debug("方法离开", "fn", "service.NewRoleService", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &RoleService{db: db}
}

func (s *RoleService) List() (roles []model.Role, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "RoleService.List")
	defer func() {
		slog.Debug("方法离开", "fn", "RoleService.List", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	err = s.db.Order("id").Find(&roles).Error
	return roles, err
}

// Rename 仅允许改显示名;code 为系统契约,不可修改。
func (s *RoleService) Rename(id uint, name string) (res *model.Role, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "RoleService.Rename", "id", id, "name", logger.Truncate(name, 64))
	defer func() {
		slog.Debug("方法离开", "fn", "RoleService.Rename", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var r model.Role
	if err := s.db.First(&r, id).Error; err != nil {
		return nil, err
	}
	r.Name = name
	if err := s.db.Save(&r).Error; err != nil {
		return nil, err
	}
	return &r, nil
}
