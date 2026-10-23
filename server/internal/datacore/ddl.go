package datacore

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"

	"gbi/server/internal/logger"
	"gbi/server/internal/model"
)

var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ValidateIdentifier 校验表名/列名:1-64 个字母/数字/下划线,字母或下划线开头。
func ValidateIdentifier(s string) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "datacore.ValidateIdentifier", "ident", logger.Truncate(s, 64))
	defer func() {
		slog.Debug("方法离开", "fn", "datacore.ValidateIdentifier", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	if s == "" || len(s) > 64 {
		return fmt.Errorf("标识符长度须为 1-64")
	}
	if !identRe.MatchString(s) {
		return fmt.Errorf("标识符仅允许字母/数字/下划线,且不能以数字开头")
	}
	return nil
}

var columnTypes = map[string]string{
	"TEXT": "TEXT", "INTEGER": "INTEGER", "REAL": "REAL", "DATE": "DATE",
}

// CreateTable 建物理表;表名/列名先校验,DDL 中全部加双引号。
func CreateTable(d *gorm.DB, table string, cols []model.ColumnDef) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "datacore.CreateTable", "table", logger.Truncate(table, 64), "cols", len(cols))
	defer func() {
		slog.Debug("方法离开", "fn", "datacore.CreateTable", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	if err := ValidateIdentifier(table); err != nil {
		return err
	}
	if len(cols) == 0 {
		return fmt.Errorf("至少需要一列")
	}
	parts := make([]string, 0, len(cols))
	for _, c := range cols {
		if err := ValidateIdentifier(c.Name); err != nil {
			return err
		}
		typ, ok := columnTypes[c.Type]
		if !ok {
			return fmt.Errorf("不支持的列类型: %s", c.Type)
		}
		parts = append(parts, fmt.Sprintf(`"%s" %s`, c.Name, typ))
	}
	return d.Exec(fmt.Sprintf(`CREATE TABLE "%s" (%s)`, table, strings.Join(parts, ", "))).Error
}

// DropTable 删除物理表(IF EXISTS)。
func DropTable(d *gorm.DB, table string) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "datacore.DropTable", "table", logger.Truncate(table, 64))
	defer func() {
		slog.Debug("方法离开", "fn", "datacore.DropTable", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	if err := ValidateIdentifier(table); err != nil {
		return err
	}
	return d.Exec(fmt.Sprintf(`DROP TABLE IF EXISTS "%s"`, table)).Error
}
