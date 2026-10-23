package datacore

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// MaxRows 单次查询最大返回行数。
const MaxRows = 10000

// QueryTimeout 单次查询超时。
const QueryTimeout = 10 * time.Second

var (
	blockedKeywordsRe = regexp.MustCompile(`(?i)\b(PRAGMA|ATTACH|DETACH|INSERT|UPDATE|DELETE|DROP|ALTER|CREATE|VACUUM|REINDEX|REPLACE|BEGIN|COMMIT|ROLLBACK|SAVEPOINT|GRANT)\b`)
	limitRe           = regexp.MustCompile(`(?i)\bLIMIT\s+(-?\d+)(\s*,\s*(-?\d+))?`)
	selectStartRe     = regexp.MustCompile(`(?is)^\s*(SELECT|WITH)\b`)
)

// ValidateReadOnlySQL 仅允许单条 SELECT/WITH 语句。
// 允许末尾单个分号(分号前后可带空白,规格裁决:宽松);
// 其余任何位置的 ';' 一律拒绝(安全优先)。
// 已知限制:字符串字面量内的 ';' 或关键词也会被拒绝(安全优先)。
func ValidateReadOnlySQL(sqlText string) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "datacore.ValidateReadOnlySQL", "sql_len", len(sqlText))
	defer func() {
		slog.Debug("方法离开", "fn", "datacore.ValidateReadOnlySQL", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	s := strings.TrimSpace(sqlText)
	s = strings.TrimSuffix(s, ";")
	if s == "" {
		return fmt.Errorf("SQL 不能为空")
	}
	if strings.Contains(s, ";") {
		return fmt.Errorf("仅允许单条语句")
	}
	if !selectStartRe.MatchString(s) {
		return fmt.Errorf("仅允许 SELECT 查询")
	}
	if blockedKeywordsRe.MatchString(s) {
		return fmt.Errorf("SQL 含禁止关键词")
	}
	if len(limitRe.FindAllStringIndex(s, -1)) > 1 {
		return fmt.Errorf("仅允许一个 LIMIT 子句")
	}
	return nil
}

// CapLimit 保证 LIMIT ≤ MaxRows:缺失则追加;超限、非正数(如 LIMIT -1 = 不限制)一律就地收紧。
func CapLimit(sqlText string) string {
	t0 := time.Now()
	slog.Debug("方法进入", "fn", "datacore.CapLimit", "sql_len", len(sqlText))
	defer func() {
		slog.Debug("方法离开", "fn", "datacore.CapLimit", "elapsed_ms", time.Since(t0).Milliseconds())
	}()
	s := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(sqlText), ";"))
	m := limitRe.FindStringSubmatchIndex(s)
	if m == nil {
		return s + " LIMIT " + strconv.Itoa(MaxRows)
	}
	start, end := m[2], m[3] // 第一个数字
	if m[6] >= 0 {           // LIMIT a, b 形式:b 才是行数
		start, end = m[6], m[7]
	}
	n, err := strconv.Atoi(s[start:end])
	if err != nil || n <= 0 || n > MaxRows {
		// 非正数(LIMIT -1/0)语义为"不限制",必须替换而非另行追加,
		// 否则会生成 `LIMIT -1 LIMIT 10000` 之类的语法错误。
		return s[:start] + strconv.Itoa(MaxRows) + s[end:]
	}
	return s
}

// SubjectRef ATTACH 目标:Code 为 SQL 限定前缀,File 为库文件路径。
type SubjectRef struct {
	Code string
	File string
}

// ExecuteQuery 在内存库上 ATTACH 各主题库执行只读 SQL,返回列名与行。
func ExecuteQuery(subjects []SubjectRef, sqlText string) (cols []string, rows [][]any, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "datacore.ExecuteQuery", "subjects", len(subjects), "sql_len", len(sqlText))
	defer func() {
		slog.Debug("方法离开", "fn", "datacore.ExecuteQuery", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	if err := ValidateReadOnlySQL(sqlText); err != nil {
		slog.Debug("SQL被拒", "reason", err.Error(), "sql", sqlText)
		return nil, nil, err
	}
	before := sqlText
	sqlText = CapLimit(sqlText)
	if sqlText != before {
		slog.Debug("SQL已收紧LIMIT", "sql", sqlText)
	}
	codes := make([]string, 0, len(subjects))
	for _, s := range subjects {
		codes = append(codes, s.Code)
	}
	slog.Debug("SQL开始执行", "sql", sqlText, "subjects", codes)

	d, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, nil, err
	}
	sqlDB, err := d.DB()
	if err != nil {
		return nil, nil, err
	}
	defer sqlDB.Close()

	for _, s := range subjects {
		if err := ValidateIdentifier(s.Code); err != nil {
			return nil, nil, fmt.Errorf("主题 code 非法: %s", s.Code)
		}
		if err := d.Exec(`ATTACH DATABASE ? AS "`+s.Code+`"`, filepath.ToSlash(s.File)).Error; err != nil {
			slog.Warn("挂载主题失败", "code", s.Code, "file", s.File, "err", err)
			return nil, nil, fmt.Errorf("挂载主题 %s 失败: %w", s.Code, err)
		}
	}
	if err := d.Exec("PRAGMA query_only = ON").Error; err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), QueryTimeout)
	defer cancel()

	rs, err := sqlDB.QueryContext(ctx, sqlText)
	if err != nil {
		slog.Warn("SQL执行失败", "sql", sqlText, "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
		return nil, nil, err
	}
	defer rs.Close()
	colNames, res, err := ScanRows(rs)
	if err != nil {
		slog.Warn("SQL执行失败", "sql", sqlText, "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
		return nil, nil, err
	}
	slog.Debug("SQL执行", "sql", sqlText, "rows", len(res), "cols", len(colNames), "elapsed_ms", time.Since(start).Milliseconds(), "preview", previewRows(colNames, res, 2, 60))
	return colNames, res, nil
}

// previewRows 生成调试用结果预览:最多 n 行、每字段截断 m 个 rune,整体再封顶 600 rune。
func previewRows(cols []string, rows [][]any, n, m int) string {
	start := time.Now()
	slog.Debug("方法进入", "fn", "datacore.previewRows", "cols", len(cols), "rows", len(rows), "n", n, "m", m)
	defer func() {
		slog.Debug("方法离开", "fn", "datacore.previewRows", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if len(rows) == 0 {
		return "(无行)"
	}
	var sb strings.Builder
	for i := 0; i < n && i < len(rows); i++ {
		if i > 0 {
			sb.WriteString(" | ")
		}
		parts := make([]string, 0, len(cols))
		for j, c := range cols {
			v := fmt.Sprintf("%v", rows[i][j])
			if r := []rune(v); len(r) > m {
				v = string(r[:m]) + "…"
			}
			parts = append(parts, c+"="+v)
		}
		sb.WriteString(strings.Join(parts, ","))
	}
	out := sb.String()
	if r := []rune(out); len(r) > 600 {
		out = string(r[:600]) + "…"
	}
	if len(rows) > n {
		out += fmt.Sprintf(" (共%d行)", len(rows))
	}
	return out
}

// ScanRows 扫描任意结果集:列名 + 行值([]byte→string,time→字符串,上限 MaxRows)。
func ScanRows(rows *sql.Rows) (colNames []string, result [][]any, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "datacore.ScanRows")
	defer func() {
		slog.Debug("方法离开", "fn", "datacore.ScanRows", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	colNames, err = rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	result = make([][]any, 0, 256)
	for rows.Next() {
		raw := make([]any, len(colNames))
		ptrs := make([]any, len(raw))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, err
		}
		row := make([]any, len(raw))
		for i, v := range raw {
			switch x := v.(type) {
			case []byte:
				row[i] = string(x)
			case time.Time:
				row[i] = x.Format("2006-01-02 15:04:05")
			default:
				row[i] = v
			}
		}
		result = append(result, row)
		if len(result) >= MaxRows {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return colNames, result, nil
}
