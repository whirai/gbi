package datacore

import (
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}([ T]\d{2}:\d{2}(:\d{2})?)?$`)

// ParseExcel 读取第一个工作表:首行表头;全空行跳过;数据行补齐到表头长度。
func ParseExcel(r io.Reader) (headers []string, rows [][]string, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "datacore.ParseExcel")
	defer func() {
		slog.Debug("方法离开", "fn", "datacore.ParseExcel", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, nil, fmt.Errorf("Excel 解析失败: %w", err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)
	if sheet == "" {
		return nil, nil, fmt.Errorf("工作簿没有工作表")
	}
	all, err := f.GetRows(sheet)
	if err != nil {
		return nil, nil, err
	}
	first := 0
	for first < len(all) && isEmptyRow(all[first]) {
		first++
	}
	if first >= len(all) {
		return nil, nil, fmt.Errorf("文件为空")
	}
	headers = append([]string(nil), all[first]...)
	for len(headers) > 0 && strings.TrimSpace(headers[len(headers)-1]) == "" {
		headers = headers[:len(headers)-1]
	}
	if len(headers) == 0 {
		return nil, nil, fmt.Errorf("表头不能为空")
	}
	for i := range headers {
		headers[i] = strings.TrimSpace(headers[i])
	}
	rows = make([][]string, 0, len(all))
	for _, raw := range all[first+1:] {
		if isEmptyRow(raw) {
			continue
		}
		row := make([]string, len(headers))
		for i := range row {
			if i < len(raw) {
				row[i] = strings.TrimSpace(raw[i])
			}
		}
		rows = append(rows, row)
	}
	return headers, rows, nil
}

func isEmptyRow(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

// InferColumnType 推断列类型:INTEGER → REAL → DATE → TEXT;空值不影响推断,全空为 TEXT。
func InferColumnType(values []string) string {
	start := time.Now()
	slog.Debug("方法进入", "fn", "datacore.InferColumnType", "values", len(values))
	defer func() {
		slog.Debug("方法离开", "fn", "datacore.InferColumnType", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	intOK, realOK, dateOK, has := true, true, true, false
	for _, v := range values {
		if v == "" {
			continue
		}
		has = true
		if intOK {
			if _, err := strconv.ParseInt(v, 10, 64); err != nil {
				intOK = false
			}
		}
		if realOK {
			if _, err := strconv.ParseFloat(v, 64); err != nil {
				realOK = false
			}
		}
		if dateOK && !dateRe.MatchString(v) {
			dateOK = false
		}
	}
	if !has {
		return "TEXT"
	}
	if intOK {
		return "INTEGER"
	}
	if realOK {
		return "REAL"
	}
	if dateOK {
		return "DATE"
	}
	return "TEXT"
}
