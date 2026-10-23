package service

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"gbi/server/internal/model"
	"gbi/server/internal/rbac"
)

// ACLGrant 页面查看授权(含展示名)。
type ACLGrant struct {
	SubjectType string `json:"subject_type"` // user | group
	SubjectID   uint   `json:"subject_id"`
	SubjectName string `json:"subject_name"`
}

// ACLGrantInput 设置授权的入参。
type ACLGrantInput struct {
	SubjectType string `json:"subject_type"`
	SubjectID   uint   `json:"subject_id"`
}

// GetACL 读取页面的全部 view 策略并解析主体名。
func (s *PageService) GetACL(pageID uint) (grants []ACLGrant, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageService.GetACL", "page_id", pageID)
	defer func() {
		slog.Debug("方法离开", "fn", "PageService.GetACL", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	if _, err := s.Get(pageID); err != nil {
		return nil, err
	}
	policies, err := s.e.GetFilteredPolicy(1, rbac.PageSubject(pageID))
	if err != nil {
		return nil, err
	}
	grants = make([]ACLGrant, 0, len(policies))
	for _, p := range policies {
		// p: [sub, obj, act]
		if len(p) < 3 || p[2] != "view" {
			continue
		}
		sub := p[0]
		switch {
		case strings.HasPrefix(sub, "user:"):
			id, err := strconv.ParseUint(strings.TrimPrefix(sub, "user:"), 10, 64)
			if err != nil {
				continue
			}
			var u model.User
			if s.db.First(&u, uint(id)).Error != nil {
				continue // 孤儿策略跳过
			}
			grants = append(grants, ACLGrant{SubjectType: "user", SubjectID: uint(id), SubjectName: u.Name})
		case strings.HasPrefix(sub, "group:"):
			id, err := strconv.ParseUint(strings.TrimPrefix(sub, "group:"), 10, 64)
			if err != nil {
				continue
			}
			var g model.Group
			if s.db.First(&g, uint(id)).Error != nil {
				continue
			}
			grants = append(grants, ACLGrant{SubjectType: "group", SubjectID: uint(id), SubjectName: g.Name})
		}
	}
	return grants, nil
}

// SetACL 全量替换页面授权。
func (s *PageService) SetACL(pageID uint, grants []ACLGrantInput) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageService.SetACL", "page_id", pageID, "grants", len(grants))
	defer func() {
		slog.Debug("方法离开", "fn", "PageService.SetACL", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	slog.Debug("PageService.SetACL", "page_id", pageID, "grants", len(grants))
	if _, err := s.Get(pageID); err != nil {
		slog.Debug("PageService.SetACL错误", "err", err)
		return err
	}
	// 先校验全部对象存在
	subs := make([]string, 0, len(grants))
	for _, g := range grants {
		switch g.SubjectType {
		case "user":
			var u model.User
			if err := s.db.First(&u, g.SubjectID).Error; err != nil {
				err := fmt.Errorf("授权用户不存在: %d", g.SubjectID)
				slog.Debug("PageService.SetACL错误", "err", err)
				return err
			}
			subs = append(subs, rbac.UserSubject(g.SubjectID))
		case "group":
			var grp model.Group
			if err := s.db.First(&grp, g.SubjectID).Error; err != nil {
				err := fmt.Errorf("授权用户组不存在: %d", g.SubjectID)
				slog.Debug("PageService.SetACL错误", "err", err)
				return err
			}
			subs = append(subs, rbac.GroupSubject(g.SubjectID))
		default:
			err := fmt.Errorf("授权类型非法: %s", g.SubjectType)
			slog.Debug("PageService.SetACL错误", "err", err)
			return err
		}
	}
	obj := rbac.PageSubject(pageID)
	if _, err := s.e.RemoveFilteredPolicy(1, obj); err != nil {
		slog.Debug("PageService.SetACL错误", "err", err)
		return err
	}
	for _, sub := range subs {
		if _, err := s.e.AddPolicy(sub, obj, "view"); err != nil {
			slog.Debug("PageService.SetACL错误", "err", err)
			return err
		}
	}
	slog.Debug("PageService.SetACL成功")
	return nil
}
