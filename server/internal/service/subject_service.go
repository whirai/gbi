package service

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"gorm.io/gorm"

	"gbi/server/internal/config"
	"gbi/server/internal/datacore"
	"gbi/server/internal/logger"
	"gbi/server/internal/model"
)

var (
	ErrSubjectNotFound      = errors.New("数据主题不存在")
	ErrDuplicateSubjectCode = errors.New("主题编码已存在")
	ErrInvalidSubjectCode   = errors.New("主题编码须以小写字母开头,仅含小写字母/数字/下划线,且不超32字符")
)

var subjectCodeRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

type SubjectService struct {
	db  *gorm.DB
	cfg *config.Config
}

func NewSubjectService(db *gorm.DB, cfg *config.Config) *SubjectService {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.NewSubjectService")
	defer func() {
		slog.Debug("方法离开", "fn", "service.NewSubjectService", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &SubjectService{db: db, cfg: cfg}
}

type SubjectCreate struct {
	Name string `json:"name"`
	Code string `json:"code"`
	Desc string `json:"desc"`
}

type SubjectUpdate struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
}

func (s *SubjectService) List() (subjects []model.Subject, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "SubjectService.List")
	defer func() {
		slog.Debug("方法离开", "fn", "SubjectService.List", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	err = s.db.Order("id").Find(&subjects).Error
	return subjects, err
}

func (s *SubjectService) Get(id uint) (res *model.Subject, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "SubjectService.Get", "id", id)
	defer func() {
		slog.Debug("方法离开", "fn", "SubjectService.Get", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var sub model.Subject
	if err := s.db.First(&sub, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSubjectNotFound
		}
		return nil, err
	}
	return &sub, nil
}

func (s *SubjectService) Create(in SubjectCreate) (res *model.Subject, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "SubjectService.Create", "name", logger.Truncate(in.Name, 64), "code", logger.Truncate(in.Code, 64))
	defer func() {
		slog.Debug("方法离开", "fn", "SubjectService.Create", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	slog.Debug("SubjectService.Create", "name", in.Name, "code", in.Code)
	if len(in.Code) > 32 || !subjectCodeRe.MatchString(in.Code) {
		slog.Debug("SubjectService.Create错误", "err", ErrInvalidSubjectCode)
		return nil, ErrInvalidSubjectCode
	}
	var n int64
	if err := s.db.Model(&model.Subject{}).Where("code = ?", in.Code).Count(&n).Error; err != nil {
		slog.Debug("SubjectService.Create错误", "err", err)
		return nil, err
	}
	if n > 0 {
		slog.Debug("SubjectService.Create错误", "err", ErrDuplicateSubjectCode)
		return nil, ErrDuplicateSubjectCode
	}
	sub := &model.Subject{Name: in.Name, Code: in.Code, Desc: in.Desc}
	if err := s.db.Create(sub).Error; err != nil {
		slog.Debug("SubjectService.Create错误", "err", err)
		return nil, err
	}
	sub.DbFile = filepath.Join(s.cfg.DataDir(), fmt.Sprintf("subject-%d.db", sub.ID))
	if err := s.db.Model(sub).Update("db_file", sub.DbFile).Error; err != nil {
		slog.Debug("SubjectService.Create错误", "err", err)
		return nil, err
	}
	// 物化空库文件,同时验证目录可写。
	_, closeFn, err := datacore.OpenSubject(sub.DbFile, false)
	if err != nil {
		// 补偿:物化失败 → 删除已写入的主题行,避免残留指向不存在库文件的记录。
		if derr := s.db.Delete(&model.Subject{}, sub.ID).Error; derr != nil {
			slog.Warn("SubjectService.Create补偿失败", "id", sub.ID, "err", derr)
		}
		slog.Debug("SubjectService.Create错误", "err", err)
		return nil, err
	}
	closeFn()
	slog.Debug("SubjectService.Create成功", "id", sub.ID)
	return sub, nil
}

func (s *SubjectService) Update(id uint, in SubjectUpdate) (res *model.Subject, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "SubjectService.Update", "id", id, "name", logger.Truncate(in.Name, 64))
	defer func() {
		slog.Debug("方法离开", "fn", "SubjectService.Update", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	sub, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	sub.Name = in.Name
	sub.Desc = in.Desc
	if err := s.db.Save(sub).Error; err != nil {
		return nil, err
	}
	return sub, nil
}

// Delete 删除主题:表元数据 → 主题记录 → 库文件。
func (s *SubjectService) Delete(id uint) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "SubjectService.Delete", "id", id)
	defer func() {
		slog.Debug("方法离开", "fn", "SubjectService.Delete", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	slog.Debug("SubjectService.Delete", "id", id)
	sub, err := s.Get(id)
	if err != nil {
		slog.Debug("SubjectService.Delete错误", "err", err)
		return err
	}
	if err := s.db.Delete(&model.SubjectTable{}, "subject_id = ?", id).Error; err != nil {
		slog.Debug("SubjectService.Delete错误", "err", err)
		return err
	}
	if err := s.db.Delete(&model.Subject{}, id).Error; err != nil {
		slog.Debug("SubjectService.Delete错误", "err", err)
		return err
	}
	if sub.DbFile != "" {
		_ = os.Remove(sub.DbFile)
	}
	slog.Debug("SubjectService.Delete成功")
	return nil
}
