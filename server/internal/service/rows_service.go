package service

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"gbi/server/internal/datacore"
	"gbi/server/internal/logger"
	"gbi/server/internal/model"
)

var (
	ErrRowNotFound   = errors.New("行不存在")
	ErrInvalidColumn = errors.New("列不存在")
	ErrInvalidValue  = errors.New("值与列类型不匹配")
)

type RowsService struct {
	db     *gorm.DB
	tables *TableService
}

// NewRowsService 保持两参数签名(router 传入 d 与 tableSvc)。
func NewRowsService(db *gorm.DB, tables *TableService) *RowsService {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.NewRowsService")
	defer func() {
		slog.Debug("方法离开", "fn", "service.NewRowsService", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &RowsService{db: db, tables: tables}
}

type RowsPage struct {
	Columns []string         `json:"columns"`
	Total   int64            `json:"total"`
	Rows    []map[string]any `json:"rows"` // 每行含 "_rid"
}

// List 分页读取行;filters 为只读列筛选(列名 → 候选值集合,IN 语义,多列 AND)。
// 空值列表视为不过滤;未知列返回 ErrInvalidColumn。
func (s *RowsService) List(subjectID, tableID uint, page, size int, filters map[string][]string) (res *RowsPage, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "RowsService.List", "subject_id", subjectID, "table_id", tableID, "page", page, "size", size, "filter_cols", len(filters))
	defer func() {
		slog.Debug("方法离开", "fn", "RowsService.List", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	sub, t, err := s.tables.Get(subjectID, tableID)
	if err != nil {
		return nil, err
	}
	cols := ParseColumns(t)

	// 校验筛选列并构建参数化 WHERE(键排序保证 SQL 稳定,便于日志/排查)
	colSet := make(map[string]bool, len(cols))
	for _, c := range cols {
		colSet[c.Name] = true
	}
	keys := make([]string, 0, len(filters))
	for k := range filters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var where string
	var args []any
	for _, k := range keys {
		vals := filters[k]
		if len(vals) == 0 {
			continue
		}
		if !colSet[k] {
			return nil, ErrInvalidColumn
		}
		if where == "" {
			where = " WHERE "
		} else {
			where += " AND "
		}
		where += fmt.Sprintf(`"%s" IN (%s)`, k, strings.TrimSuffix(strings.Repeat("?,", len(vals)), ","))
		for _, v := range vals {
			args = append(args, v)
		}
	}

	d, closeFn, err := datacore.OpenSubject(sub.DbFile, true)
	if err != nil {
		return nil, err
	}
	defer closeFn()

	sqlDB, err := d.DB()
	if err != nil {
		return nil, err
	}

	// 注意:表名已经过标识符校验,直接用裸名;值一律参数绑定。
	var total int64
	if err := sqlDB.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM "%s"%s`, t.Name, where), args...).Scan(&total); err != nil {
		return nil, err
	}

	colList := make([]string, 0, len(cols))
	for _, c := range cols {
		colList = append(colList, `"`+c.Name+`"`)
	}
	query := fmt.Sprintf(`SELECT rowid AS "_rid", %s FROM "%s"%s ORDER BY rowid LIMIT %d OFFSET %d`,
		strings.Join(colList, ","), t.Name, where, size, (page-1)*size)

	rs, err := sqlDB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	names, scanned, err := datacore.ScanRows(rs)
	if err != nil {
		return nil, err
	}
	rowsOut := make([]map[string]any, 0, len(scanned))
	for _, r := range scanned {
		m := make(map[string]any, len(names))
		for i, n := range names {
			m[n] = r[i]
		}
		rowsOut = append(rowsOut, m)
	}
	slog.Debug("行列表", "subject_id", subjectID, "table_id", tableID, "page", page, "size", size, "filter_cols", len(filters), "total", total, "returned", len(rowsOut))
	return &RowsPage{Columns: colNamesOf(cols), Total: total, Rows: rowsOut}, nil
}

// defaultDistinctLimit 前端筛选下拉最多返回的不同值个数(高基数列仅取前 N 个)。
const defaultDistinctLimit = 100

// DistinctValues 返回某列的去重取值(升序,null 与空串排除),limit 钳制在 [1,500]。
func (s *RowsService) DistinctValues(subjectID, tableID uint, column string, limit int) (vals []string, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "RowsService.DistinctValues", "subject_id", subjectID, "table_id", tableID, "column", logger.Truncate(column, 64), "limit", limit)
	defer func() {
		slog.Debug("方法离开", "fn", "RowsService.DistinctValues", "elapsed_ms", time.Since(start).Milliseconds(), "count", len(vals), "err", err)
	}()
	if limit <= 0 {
		limit = defaultDistinctLimit
	}
	if limit > 500 {
		limit = 500
	}
	sub, t, err := s.tables.Get(subjectID, tableID)
	if err != nil {
		return nil, err
	}
	found := false
	for _, c := range ParseColumns(t) {
		if c.Name == column {
			found = true
			break
		}
	}
	if !found {
		return nil, ErrInvalidColumn
	}
	d, closeFn, err := datacore.OpenSubject(sub.DbFile, true)
	if err != nil {
		return nil, err
	}
	defer closeFn()
	sqlDB, err := d.DB()
	if err != nil {
		return nil, err
	}
	// 列名已经过 ParseColumns 校验;null/空串不作为候选值
	q := fmt.Sprintf(`SELECT DISTINCT "%s" FROM "%s" WHERE "%s" IS NOT NULL AND "%s" <> '' ORDER BY "%s" LIMIT %d`,
		column, t.Name, column, column, column, limit)
	rs, err := sqlDB.Query(q)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	_, scanned, err := datacore.ScanRows(rs)
	if err != nil {
		return nil, err
	}
	vals = make([]string, 0, len(scanned))
	for _, r := range scanned {
		vals = append(vals, fmt.Sprintf("%v", r[0]))
	}
	slog.Debug("列取值", "subject_id", subjectID, "table_id", tableID, "column", column, "count", len(vals))
	return vals, nil
}

func colNamesOf(cols []model.ColumnDef) []string {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.colNamesOf", "count", len(cols))
	defer func() {
		slog.Debug("方法离开", "fn", "service.colNamesOf", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	names := make([]string, 0, len(cols))
	for _, c := range cols {
		names = append(names, c.Name)
	}
	return names
}

// UpdateCell 更新单元格;value 来自 JSON(float64/string/nil),按列类型强制转换。
func (s *RowsService) UpdateCell(subjectID, tableID uint, rowID int64, column string, value any) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "RowsService.UpdateCell", "subject_id", subjectID, "table_id", tableID, "row_id", rowID, "column", logger.Truncate(column, 64))
	defer func() {
		slog.Debug("方法离开", "fn", "RowsService.UpdateCell", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	sub, t, err := s.tables.Get(subjectID, tableID)
	if err != nil {
		return err
	}
	var colType string
	found := false
	for _, c := range ParseColumns(t) {
		if c.Name == column {
			colType = c.Type
			found = true
			break
		}
	}
	if !found {
		return ErrInvalidColumn
	}
	coerced, err := coerceValue(colType, value)
	if err != nil {
		return ErrInvalidValue
	}
	d, closeFn, err := datacore.OpenSubject(sub.DbFile, false)
	if err != nil {
		return err
	}
	defer closeFn()
	res := d.Exec(fmt.Sprintf(`UPDATE "%s" SET "%s" = ? WHERE rowid = ?`, t.Name, column), coerced, rowID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrRowNotFound
	}
	slog.Debug("单元格更新", "subject_id", subjectID, "table_id", tableID, "row_id", rowID, "column", column)
	return nil
}

func (s *RowsService) DeleteRow(subjectID, tableID uint, rowID int64) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "RowsService.DeleteRow", "subject_id", subjectID, "table_id", tableID, "row_id", rowID)
	defer func() {
		slog.Debug("方法离开", "fn", "RowsService.DeleteRow", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	sub, t, err := s.tables.Get(subjectID, tableID)
	if err != nil {
		return err
	}
	d, closeFn, err := datacore.OpenSubject(sub.DbFile, false)
	if err != nil {
		return err
	}
	defer closeFn()
	res := d.Exec(fmt.Sprintf(`DELETE FROM "%s" WHERE rowid = ?`, t.Name), rowID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrRowNotFound
	}
	slog.Debug("删除行", "subject_id", subjectID, "table_id", tableID, "row_id", rowID)
	return nil
}

// coerceValue 把 JSON 值转为列类型对应的存储值;空字符串/nil → NULL。
func coerceValue(typ string, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	if s, ok := value.(string); ok && strings.TrimSpace(s) == "" {
		return nil, nil
	}
	switch typ {
	case "INTEGER":
		switch v := value.(type) {
		case float64:
			if v != math.Trunc(v) {
				return nil, fmt.Errorf("需要整数")
			}
			// float64 无法精确表示 MaxInt64(转换为 float64 时向上取整为 2^63),上界用 >=
			if v < math.MinInt64 || v >= math.MaxInt64 {
				return nil, fmt.Errorf("需要整数")
			}
			return int64(v), nil
		case string:
			n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("需要整数")
			}
			return n, nil
		default:
			return nil, fmt.Errorf("需要整数")
		}
	case "REAL":
		switch v := value.(type) {
		case float64:
			return v, nil
		case string:
			f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil {
				return nil, fmt.Errorf("需要数字")
			}
			return f, nil
		default:
			return nil, fmt.Errorf("需要数字")
		}
	default: // TEXT / DATE
		switch v := value.(type) {
		case string:
			return v, nil
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64), nil
		default:
			return nil, fmt.Errorf("需要字符串")
		}
	}
}
