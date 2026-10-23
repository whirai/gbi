package service

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"

	"gbi/server/internal/datacore"
	"gbi/server/internal/logger"
	"gbi/server/internal/model"
)

type ImportService struct {
	db       *gorm.DB
	subjects *SubjectService
	tables   *TableService
}

func NewImportService(db *gorm.DB, subjects *SubjectService, tables *TableService) *ImportService {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.NewImportService")
	defer func() {
		slog.Debug("方法离开", "fn", "service.NewImportService", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &ImportService{db: db, subjects: subjects, tables: tables}
}

type ImportParams struct {
	Mode      string `json:"mode"` // create | append
	TableName string `json:"table_name"`
	Kind      string `json:"kind"`     // create 模式:DWD | DIM
	TableID   uint   `json:"table_id"` // append 模式目标表
}

type ImportResult struct {
	TableID  uint `json:"table_id"`
	Inserted int  `json:"inserted"`
}

func (s *ImportService) Run(subjectID uint, file io.Reader, p ImportParams) (res *ImportResult, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "ImportService.Run", "subject_id", subjectID, "mode", logger.Truncate(p.Mode, 64), "table_name", logger.Truncate(p.TableName, 64))
	defer func() {
		slog.Debug("方法离开", "fn", "ImportService.Run", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	headers, rows, err := datacore.ParseExcel(file)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("文件没有数据行")
	}
	for _, h := range headers {
		if err := datacore.ValidateIdentifier(h); err != nil {
			return nil, fmt.Errorf("表头 %q 不是合法列名(字母/数字/下划线,不能以数字开头)", h)
		}
	}
	slog.Debug("导入开始", "subject_id", subjectID, "mode", p.Mode, "table_name", p.TableName, "rows", len(rows), "cols", len(headers))
	switch p.Mode {
	case "create":
		return s.importCreate(subjectID, headers, rows, p)
	case "append":
		return s.importAppend(subjectID, headers, rows, p)
	default:
		return nil, errors.New("mode 必须是 create 或 append")
	}
}

func (s *ImportService) importCreate(subjectID uint, headers []string, rows [][]string, p ImportParams) (res *ImportResult, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "ImportService.importCreate", "subject_id", subjectID, "table_name", logger.Truncate(p.TableName, 64), "rows", len(rows))
	defer func() {
		slog.Debug("方法离开", "fn", "ImportService.importCreate", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	cols := make([]model.ColumnDef, len(headers))
	for i, h := range headers {
		vals := make([]string, len(rows))
		for r := range rows {
			vals[r] = rows[r][i]
		}
		cols[i] = model.ColumnDef{Name: h, Type: datacore.InferColumnType(vals)}
	}
	dto, err := s.tables.Create(subjectID, TableCreate{Name: p.TableName, Kind: p.Kind, Columns: cols})
	if err != nil {
		return nil, err
	}
	inserted, err := s.insertRows(subjectID, dto.ID, headers, rows)
	if err != nil {
		// 建表成功但灌数失败:回滚整表(物理+元数据)
		_ = s.tables.Delete(subjectID, dto.ID)
		return nil, err
	}
	return &ImportResult{TableID: dto.ID, Inserted: inserted}, nil
}

func (s *ImportService) importAppend(subjectID uint, headers []string, rows [][]string, p ImportParams) (res *ImportResult, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "ImportService.importAppend", "subject_id", subjectID, "table_id", p.TableID, "rows", len(rows))
	defer func() {
		slog.Debug("方法离开", "fn", "ImportService.importAppend", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	_, t, err := s.tables.Get(subjectID, p.TableID)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, c := range ParseColumns(t) {
		known[c.Name] = true
	}
	var missing []string
	for _, h := range headers {
		if !known[h] {
			missing = append(missing, h)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("以下列不在表中: %s", strings.Join(missing, ", "))
	}
	inserted, err := s.insertRows(subjectID, p.TableID, headers, rows)
	if err != nil {
		return nil, err
	}
	return &ImportResult{TableID: p.TableID, Inserted: inserted}, nil
}

// insertRows 事务灌数;行号报错以 Excel 行号计(表头为第 1 行)。
func (s *ImportService) insertRows(subjectID, tableID uint, headers []string, rows [][]string) (inserted int, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "ImportService.insertRows", "subject_id", subjectID, "table_id", tableID, "rows", len(rows), "cols", len(headers))
	defer func() {
		slog.Debug("方法离开", "fn", "ImportService.insertRows", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	sub, t, err := s.tables.Get(subjectID, tableID)
	if err != nil {
		return 0, err
	}
	typeByName := map[string]string{}
	for _, c := range ParseColumns(t) {
		typeByName[c.Name] = c.Type
	}
	quoted := make([]string, len(headers))
	for i, h := range headers {
		quoted[i] = `"` + h + `"`
	}
	insertSQL := fmt.Sprintf(`INSERT INTO "%s" (%s) VALUES (%s)`,
		t.Name, strings.Join(quoted, ","), strings.TrimSuffix(strings.Repeat("?,", len(headers)), ","))

	d, closeFn, err := datacore.OpenSubject(sub.DbFile, false)
	if err != nil {
		return 0, err
	}
	defer closeFn()

	tx := d.Begin()
	for ri, row := range rows {
		vals := make([]any, len(headers))
		for i, v := range row {
			cv, err := coerceValue(typeByName[headers[i]], v)
			if err != nil {
				tx.Rollback()
				return 0, fmt.Errorf("第 %d 行 %d 列(%s): %v", ri+2, i+1, headers[i], err)
			}
			vals[i] = cv
		}
		if err := tx.Exec(insertSQL, vals...).Error; err != nil {
			tx.Rollback()
			return 0, err
		}
	}
	if err := tx.Commit().Error; err != nil {
		return 0, err
	}
	slog.Debug("导入灌数完成", "table_id", tableID, "inserted", len(rows))
	return len(rows), nil
}
