package service

import (
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"

	"gbi/server/internal/datacore"
	"gbi/server/internal/logger"
	"gbi/server/internal/model"
)

var (
	ErrTableNotFound      = errors.New("表不存在")
	ErrDuplicateTableName = errors.New("表名已存在")
	ErrInvalidTableKind   = errors.New("kind 必须是 DWD 或 DIM")
)

type TableService struct {
	db       *gorm.DB
	subjects *SubjectService
}

func NewTableService(db *gorm.DB, subjects *SubjectService) *TableService {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.NewTableService")
	defer func() {
		slog.Debug("方法离开", "fn", "service.NewTableService", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &TableService{db: db, subjects: subjects}
}

type TableCreate struct {
	Name    string            `json:"name"` // 基础名,不带 dwd_/dim_ 前缀
	Kind    string            `json:"kind"` // DWD | DIM
	Columns []model.ColumnDef `json:"columns"`
}

type TableDTO struct {
	ID        uint              `json:"id"`
	SubjectID uint              `json:"subject_id"`
	Name      string            `json:"name"`
	Kind      string            `json:"kind"`
	Columns   []model.ColumnDef `json:"columns"`
}

func tableDTO(t *model.SubjectTable) TableDTO {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.tableDTO", "table_id", t.ID)
	defer func() {
		slog.Debug("方法离开", "fn", "service.tableDTO", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	var cols []model.ColumnDef
	_ = json.Unmarshal([]byte(t.Columns), &cols)
	return TableDTO{ID: t.ID, SubjectID: t.SubjectID, Name: t.Name, Kind: t.Kind, Columns: cols}
}

// ParseColumns 反序列化表元数据中的列定义(供 rows/import 服务复用)。
func ParseColumns(t *model.SubjectTable) []model.ColumnDef {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.ParseColumns", "table_id", t.ID)
	defer func() {
		slog.Debug("方法离开", "fn", "service.ParseColumns", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	var cols []model.ColumnDef
	_ = json.Unmarshal([]byte(t.Columns), &cols)
	return cols
}

func (s *TableService) List(subjectID uint) (dtos []TableDTO, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "TableService.List", "subject_id", subjectID)
	defer func() {
		slog.Debug("方法离开", "fn", "TableService.List", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	if _, err := s.subjects.Get(subjectID); err != nil {
		return nil, err
	}
	var tables []model.SubjectTable
	if err := s.db.Where("subject_id = ?", subjectID).Order("id").Find(&tables).Error; err != nil {
		return nil, err
	}
	dtos = make([]TableDTO, 0, len(tables))
	for i := range tables {
		dtos = append(dtos, tableDTO(&tables[i]))
	}
	return dtos, nil
}

func (s *TableService) Get(subjectID, tableID uint) (sub *model.Subject, tbl *model.SubjectTable, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "TableService.Get", "subject_id", subjectID, "table_id", tableID)
	defer func() {
		slog.Debug("方法离开", "fn", "TableService.Get", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	sub, err = s.subjects.Get(subjectID)
	if err != nil {
		return nil, nil, err
	}
	var t model.SubjectTable
	if err := s.db.First(&t, tableID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrTableNotFound
		}
		return nil, nil, err
	}
	if t.SubjectID != subjectID {
		return nil, nil, ErrTableNotFound
	}
	return sub, &t, nil
}

func (s *TableService) Create(subjectID uint, in TableCreate) (res *TableDTO, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "TableService.Create", "subject_id", subjectID, "name", logger.Truncate(in.Name, 64), "kind", logger.Truncate(in.Kind, 64))
	defer func() {
		slog.Debug("方法离开", "fn", "TableService.Create", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	slog.Debug("TableService.Create", "subject_id", subjectID, "name", in.Name, "kind", in.Kind)
	sub, err := s.subjects.Get(subjectID)
	if err != nil {
		slog.Debug("TableService.Create错误", "err", err)
		return nil, err
	}
	var kind string
	switch in.Kind {
	case "DWD":
		kind = "DWD"
	case "DIM":
		kind = "DIM"
	default:
		slog.Debug("TableService.Create错误", "err", ErrInvalidTableKind)
		return nil, ErrInvalidTableKind
	}
	fullName := strings.ToLower(kind) + "_" + in.Name
	if err := datacore.ValidateIdentifier(fullName); err != nil {
		slog.Debug("TableService.Create错误", "err", err)
		return nil, err
	}
	for _, c := range in.Columns {
		if err := datacore.ValidateIdentifier(c.Name); err != nil {
			slog.Debug("TableService.Create错误", "err", err)
			return nil, err
		}
	}
	var n int64
	if err := s.db.Model(&model.SubjectTable{}).
		Where("subject_id = ? AND name = ?", subjectID, fullName).Count(&n).Error; err != nil {
		slog.Debug("TableService.Create错误", "err", err)
		return nil, err
	}
	if n > 0 {
		slog.Debug("TableService.Create错误", "err", ErrDuplicateTableName)
		return nil, ErrDuplicateTableName
	}
	d, closeFn, err := datacore.OpenSubject(sub.DbFile, false)
	if err != nil {
		slog.Debug("TableService.Create错误", "err", err)
		return nil, err
	}
	defer closeFn()
	if err := datacore.CreateTable(d, fullName, in.Columns); err != nil {
		slog.Debug("TableService.Create错误", "err", err)
		return nil, err
	}
	colsJSON, err := json.Marshal(in.Columns)
	if err != nil {
		slog.Debug("TableService.Create错误", "err", err)
		return nil, err
	}
	t := &model.SubjectTable{SubjectID: subjectID, Name: fullName, Kind: kind, Columns: string(colsJSON)}
	// 物理表建在主题库文件、元数据在主库:两个 SQLite 文件无法共享单个事务,
	// 采用补偿式一致性——元数据落库失败时回收已建物理表,不留「无元数据的孤儿表」。
	if err := s.db.Create(t).Error; err != nil {
		if derr := datacore.DropTable(d, fullName); derr != nil {
			slog.Warn("TableService.Create补偿失败", "table", fullName, "err", derr)
		}
		slog.Debug("TableService.Create错误", "err", err)
		return nil, err
	}
	dto := tableDTO(t)
	slog.Debug("TableService.Create成功", "id", dto.ID)
	return &dto, nil
}

func (s *TableService) Delete(subjectID, tableID uint) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "TableService.Delete", "subject_id", subjectID, "table_id", tableID)
	defer func() {
		slog.Debug("方法离开", "fn", "TableService.Delete", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	slog.Debug("TableService.Delete", "tid", tableID)
	sub, t, err := s.Get(subjectID, tableID)
	if err != nil {
		slog.Debug("TableService.Delete错误", "err", err)
		return err
	}
	d, closeFn, err := datacore.OpenSubject(sub.DbFile, false)
	if err != nil {
		slog.Debug("TableService.Delete错误", "err", err)
		return err
	}
	defer closeFn()
	// 物理表在主题库文件、元数据在主库,跨文件无法共享单个事务:元数据删除放入事务,
	// 事务内最后一步执行物理 DROP——DROP 失败则回滚,元数据行自动恢复,消除
	// 「物理表已删但元数据仍在」的幽灵表(残留的元数据会让 List 报表存在而取数报 no such table)。
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&model.SubjectTable{}, t.ID).Error; err != nil {
			return err
		}
		return datacore.DropTable(d, t.Name)
	})
	if err != nil {
		slog.Debug("TableService.Delete错误", "err", err)
		return err
	}
	slog.Debug("TableService.Delete成功")
	return nil
}
