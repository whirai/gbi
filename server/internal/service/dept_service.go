package service

import (
	"errors"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"gbi/server/internal/logger"
	"gbi/server/internal/model"
)

var (
	ErrDeptHasChildren = errors.New("请先删除子部门")
	ErrDeptHasUsers    = errors.New("部门下存在用户,无法删除")
)

type DeptService struct{ db *gorm.DB }

func NewDeptService(db *gorm.DB) *DeptService {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.NewDeptService")
	defer func() {
		slog.Debug("方法离开", "fn", "service.NewDeptService", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &DeptService{db: db}
}

type DeptNode struct {
	model.Department
	Children []DeptNode `json:"children"`
}

func (s *DeptService) Tree() (nodes []DeptNode, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "DeptService.Tree")
	defer func() {
		slog.Debug("方法离开", "fn", "DeptService.Tree", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var all []model.Department
	if err := s.db.Order("sort, id").Find(&all).Error; err != nil {
		return nil, err
	}
	return buildTree(all, nil), nil
}

func buildTree(all []model.Department, parent *uint) []DeptNode {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.buildTree", "count", len(all))
	defer func() {
		slog.Debug("方法离开", "fn", "service.buildTree", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	nodes := []DeptNode{}
	for _, d := range all {
		if ptrEq(d.ParentID, parent) {
			nodes = append(nodes, DeptNode{
				Department: d,
				Children:   buildTree(all, &d.ID),
			})
		}
	}
	return nodes
}

func ptrEq(a, b *uint) bool {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.ptrEq")
	defer func() {
		slog.Debug("方法离开", "fn", "service.ptrEq", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func (s *DeptService) Create(name string, parentID *uint, sort int) (res *model.Department, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "DeptService.Create", "name", logger.Truncate(name, 64), "sort", sort)
	defer func() {
		slog.Debug("方法离开", "fn", "DeptService.Create", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	d := &model.Department{Name: name, ParentID: parentID, Sort: sort}
	if err := s.db.Create(d).Error; err != nil {
		return nil, err
	}
	return d, nil
}

func (s *DeptService) Update(id uint, name string, sort int) (res *model.Department, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "DeptService.Update", "id", id, "name", logger.Truncate(name, 64), "sort", sort)
	defer func() {
		slog.Debug("方法离开", "fn", "DeptService.Update", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var d model.Department
	if err := s.db.First(&d, id).Error; err != nil {
		return nil, err
	}
	d.Name = name
	d.Sort = sort
	if err := s.db.Save(&d).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *DeptService) Delete(id uint) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "DeptService.Delete", "id", id)
	defer func() {
		slog.Debug("方法离开", "fn", "DeptService.Delete", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var n int64
	if err := s.db.Model(&model.Department{}).Where("parent_id = ?", id).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return ErrDeptHasChildren
	}
	if err := s.db.Model(&model.User{}).Where("dept_id = ?", id).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return ErrDeptHasUsers
	}
	res := s.db.Delete(&model.Department{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
