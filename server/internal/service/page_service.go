package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/casbin/casbin/v2"
	"gorm.io/gorm"

	"gbi/server/internal/logger"
	"gbi/server/internal/model"
	"gbi/server/internal/rbac"
)

var (
	ErrPageNotFound      = errors.New("页面不存在")
	ErrInvalidPageConfig = errors.New("页面配置非法")
	ErrInvalidPageStatus = errors.New("页面状态非法")
)

type PageService struct {
	db *gorm.DB
	e  *casbin.Enforcer
}

func NewPageService(db *gorm.DB, e *casbin.Enforcer) *PageService {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.NewPageService")
	defer func() {
		slog.Debug("方法离开", "fn", "service.NewPageService", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &PageService{db: db, e: e}
}

type PageCreate struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
}

// PageUpdate 指针字段 = 局部更新;Config 非 nil 时校验并(若变化)记版本。
type PageUpdate struct {
	Name   *string           `json:"name"`
	Desc   *string           `json:"desc"`
	Config *model.PageConfig `json:"config"`
}

type PageDTO struct {
	ID        uint      `json:"id"`
	Name      string    `json:"name"`
	Desc      string    `json:"desc"`
	Status    string    `json:"status"`
	CreatedBy uint      `json:"created_by"`
	UpdatedAt time.Time `json:"updated_at"`
}

func pageDTO(p *model.Page) PageDTO {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.pageDTO", "page_id", p.ID)
	defer func() {
		slog.Debug("方法离开", "fn", "service.pageDTO", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return PageDTO{ID: p.ID, Name: p.Name, Desc: p.Desc, Status: p.Status, CreatedBy: p.CreatedBy, UpdatedAt: p.UpdatedAt}
}

func (s *PageService) List() (dtos []PageDTO, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageService.List")
	defer func() {
		slog.Debug("方法离开", "fn", "PageService.List", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var pages []model.Page
	if err := s.db.Order("id").Find(&pages).Error; err != nil {
		return nil, err
	}
	dtos = make([]PageDTO, 0, len(pages))
	for i := range pages {
		dtos = append(dtos, pageDTO(&pages[i]))
	}
	return dtos, nil
}

func (s *PageService) Get(id uint) (res *model.Page, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageService.Get", "id", id)
	defer func() {
		slog.Debug("方法离开", "fn", "PageService.Get", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var p model.Page
	if err := s.db.First(&p, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPageNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (s *PageService) Create(userID uint, in PageCreate) (res *model.Page, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageService.Create", "user_id", userID, "name", logger.Truncate(in.Name, 64))
	defer func() {
		slog.Debug("方法离开", "fn", "PageService.Create", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	slog.Debug("PageService.Create", "name", in.Name)
	cfgStr, err := model.MarshalPageConfig(model.DefaultPageConfig())
	if err != nil {
		slog.Debug("PageService.Create错误", "err", err)
		return nil, err
	}
	p := &model.Page{Name: in.Name, Desc: in.Desc, Config: cfgStr, Status: "draft", CreatedBy: userID}
	if err := s.db.Create(p).Error; err != nil {
		slog.Debug("PageService.Create错误", "err", err)
		return nil, err
	}
	slog.Debug("PageService.Create成功", "id", p.ID)
	return p, nil
}

func (s *PageService) Update(userID uint, id uint, in PageUpdate) (res *model.Page, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageService.Update", "user_id", userID, "id", id)
	defer func() {
		slog.Debug("方法离开", "fn", "PageService.Update", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	slog.Debug("PageService.Update", "id", id)
	p, err := s.Get(id)
	if err != nil {
		slog.Debug("PageService.Update错误", "err", err)
		return nil, err
	}
	if in.Name != nil {
		p.Name = *in.Name
	}
	if in.Desc != nil {
		p.Desc = *in.Desc
	}
	if in.Config != nil {
		cfgStr, err := model.MarshalPageConfig(*in.Config)
		if err != nil {
			slog.Debug("PageService.Update错误", "err", err)
			return nil, fmt.Errorf("%w: %v", ErrInvalidPageConfig, err)
		}
		if cfgStr != p.Config {
			p.Config = cfgStr
			if err := s.saveVersion(p, userID); err != nil {
				slog.Debug("PageService.Update错误", "err", err)
				return nil, err
			}
		}
	}
	if err := s.db.Save(p).Error; err != nil {
		slog.Debug("PageService.Update错误", "err", err)
		return nil, err
	}
	slog.Debug("PageService.Update成功")
	return p, nil
}

// saveVersion 以新 config 记一个版本(version = max+1)。调用方负责在 Save 前调用。
func (s *PageService) saveVersion(p *model.Page, userID uint) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageService.saveVersion", "page_id", p.ID, "user_id", userID)
	defer func() {
		slog.Debug("方法离开", "fn", "PageService.saveVersion", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var max struct {
		V int
	}
	if err := s.db.Model(&model.PageVersion{}).
		Select("COALESCE(MAX(version),0) AS v").
		Where("page_id = ?", p.ID).Scan(&max.V).Error; err != nil {
		return err
	}
	v := model.PageVersion{PageID: p.ID, Version: max.V + 1, Config: p.Config, SavedBy: userID}
	return s.db.Create(&v).Error
}

func (s *PageService) Delete(id uint) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageService.Delete", "id", id)
	defer func() {
		slog.Debug("方法离开", "fn", "PageService.Delete", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	if _, err := s.Get(id); err != nil {
		return err
	}
	// DB 侧四条级联删除 + 页面行同事务,失败整体回滚(不留半删页面)。
	// casbin 策略无法参与 DB 事务,置于提交成功之后:顺序理由——先保证 DB 一致,
	// 再清 ACL;若反序(casbin 先清而 DB 失败),页面仍在但所有人失去授权,
	// 且无法通过重试恢复;现顺序下 casbin 清理失败只会残留不再指向可访问对象的策略。
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&model.PageVersion{}, "page_id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Delete(&model.MenuItem{}, "page_id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Delete(&model.PageRenderError{}, "page_id = ?", id).Error; err != nil {
			return err
		}
		return tx.Delete(&model.Page{}, id).Error
	}); err != nil {
		return err
	}
	_, err = s.e.RemoveFilteredPolicy(1, rbac.PageSubject(id))
	return err
}

func (s *PageService) SetStatus(id uint, status string) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageService.SetStatus", "id", id, "status", logger.Truncate(status, 64))
	defer func() {
		slog.Debug("方法离开", "fn", "PageService.SetStatus", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	slog.Debug("PageService.SetStatus", "id", id, "status", status)
	if status != "draft" && status != "published" {
		slog.Debug("PageService.SetStatus错误", "err", ErrInvalidPageStatus)
		return ErrInvalidPageStatus
	}
	p, err := s.Get(id)
	if err != nil {
		slog.Debug("PageService.SetStatus错误", "err", err)
		return err
	}
	err = s.db.Model(p).Update("status", status).Error
	if err != nil {
		slog.Debug("PageService.SetStatus错误", "err", err)
		return err
	}
	slog.Debug("PageService.SetStatus成功")
	return nil
}

func (s *PageService) IsAdmin(userID uint) bool {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageService.IsAdmin", "user_id", userID)
	defer func() {
		slog.Debug("方法离开", "fn", "PageService.IsAdmin", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	var u model.User
	if err := s.db.First(&u, userID).Error; err != nil {
		return false
	}
	var r model.Role
	if err := s.db.First(&r, u.RoleID).Error; err != nil {
		return false
	}
	return r.Code == "admin"
}

// CanView admin 全量;作者可见(含草稿);其余按 casbin 策略。
func (s *PageService) CanView(userID uint, p *model.Page) (ok bool, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageService.CanView", "user_id", userID, "page_id", p.ID)
	defer func() {
		slog.Debug("方法离开", "fn", "PageService.CanView", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	if s.IsAdmin(userID) {
		return true, nil
	}
	if p.CreatedBy == userID {
		return true, nil
	}
	return s.e.Enforce(rbac.UserSubject(userID), rbac.PageSubject(p.ID), "view")
}

// VisiblePages 仅 published 且 CanView。
func (s *PageService) VisiblePages(userID uint) (dtos []PageDTO, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageService.VisiblePages", "user_id", userID)
	defer func() {
		slog.Debug("方法离开", "fn", "PageService.VisiblePages", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var pages []model.Page
	if err := s.db.Where("status = ?", "published").Order("id").Find(&pages).Error; err != nil {
		return nil, err
	}
	dtos = make([]PageDTO, 0, len(pages))
	for i := range pages {
		ok, err := s.CanView(userID, &pages[i])
		if err != nil {
			return nil, err
		}
		if ok {
			dtos = append(dtos, pageDTO(&pages[i]))
		}
	}
	return dtos, nil
}

// CanQuerySubjects 门户图表取数授权:admin 全量放行;其余用户要求
// 每个主题都被其可见(published 且 CanView)页面的图表引用。
// 对应 /api/v1/query 的对象级校验(防止任意用户枚举主题 ID 取数)。
func (s *PageService) CanQuerySubjects(userID uint, subjectIDs []uint) (ok bool, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageService.CanQuerySubjects", "user_id", userID, "subjects", len(subjectIDs))
	defer func() {
		slog.Debug("方法离开", "fn", "PageService.CanQuerySubjects", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	if s.IsAdmin(userID) {
		return true, nil
	}
	allowed := make(map[uint]bool)
	var pages []model.Page
	if err := s.db.Where("status = ?", "published").Order("id").Find(&pages).Error; err != nil {
		return false, err
	}
	for i := range pages {
		ok, err := s.CanView(userID, &pages[i])
		if err != nil {
			return false, err
		}
		if !ok {
			continue
		}
		var cfg model.PageConfig
		if err := json.Unmarshal([]byte(pages[i].Config), &cfg); err != nil {
			continue // 配置损坏的页面渲染侧同样会失败,跳过
		}
		for _, ch := range cfg.Charts {
			for _, sid := range ch.Subjects {
				allowed[sid] = true
			}
		}
	}
	for _, sid := range subjectIDs {
		if !allowed[sid] {
			return false, nil
		}
	}
	return true, nil
}

// Versions 按 version 降序。页面不存在(已删除)时返回空列表,
// 供删除级联断言使用;其余 DB 错误照常返回。
func (s *PageService) Versions(pageID uint) (vs []model.PageVersion, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageService.Versions", "page_id", pageID)
	defer func() {
		slog.Debug("方法离开", "fn", "PageService.Versions", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	if _, err := s.Get(pageID); err != nil && !errors.Is(err, ErrPageNotFound) {
		return nil, err
	}
	err = s.db.Where("page_id = ?", pageID).Order("version DESC").Find(&vs).Error
	return vs, err
}

// Restore 把指定版本的 config 设为当前,并记新版本。
func (s *PageService) Restore(userID uint, pageID, versionID uint) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "PageService.Restore", "user_id", userID, "page_id", pageID, "version_id", versionID)
	defer func() {
		slog.Debug("方法离开", "fn", "PageService.Restore", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	slog.Debug("PageService.Restore", "id", pageID, "vid", versionID)
	p, err := s.Get(pageID)
	if err != nil {
		slog.Debug("PageService.Restore错误", "err", err)
		return err
	}
	var v model.PageVersion
	if err := s.db.First(&v, versionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Debug("PageService.Restore错误", "err", ErrPageNotFound)
			return ErrPageNotFound
		}
		// 基础设施故障(连接/锁)不得伪装成 404,原样上抛交由 handler 记 500。
		slog.Debug("PageService.Restore错误", "err", err)
		return err
	}
	if v.PageID != pageID {
		slog.Debug("PageService.Restore错误", "err", ErrPageNotFound)
		return ErrPageNotFound
	}
	p.Config = v.Config
	if err := s.saveVersion(p, userID); err != nil {
		slog.Debug("PageService.Restore错误", "err", err)
		return err
	}
	err = s.db.Save(p).Error
	if err != nil {
		slog.Debug("PageService.Restore错误", "err", err)
		return err
	}
	slog.Debug("PageService.Restore成功")
	return nil
}

// ParsePageConfig 供 handler 反序列化响应用。
func ParsePageConfig(p *model.Page) model.PageConfig {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.ParsePageConfig", "page_id", p.ID)
	defer func() {
		slog.Debug("方法离开", "fn", "service.ParsePageConfig", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	var cfg model.PageConfig
	if err := json.Unmarshal([]byte(p.Config), &cfg); err != nil {
		return model.DefaultPageConfig()
	}
	return cfg
}
