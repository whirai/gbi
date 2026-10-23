package datacore_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"gbi/server/internal/datacore"
)

func buildXlsx(t *testing.T, rows [][]any) *bytes.Buffer {
	t.Helper()
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	for r, row := range rows {
		for c, v := range row {
			cell, err := excelize.CoordinatesToCellName(c+1, r+1)
			require.NoError(t, err)
			require.NoError(t, f.SetCellValue(sheet, cell, v))
		}
	}
	buf, err := f.WriteToBuffer()
	require.NoError(t, err)
	return buf
}

func TestParseExcel(t *testing.T) {
	buf := buildXlsx(t, [][]any{
		{"month", "region", "amount"},
		{"2026-01", "华东", 100},
		{"2026-01", "华北", 200},
		{"", "", ""}, // 全空行应被跳过
		{"2026-02", "华东"},
	})
	headers, rows, err := datacore.ParseExcel(buf)
	require.NoError(t, err)
	require.Equal(t, []string{"month", "region", "amount"}, headers)
	require.Len(t, rows, 3)
	require.Equal(t, []string{"2026-01", "华东", "100"}, rows[0])
	require.Equal(t, []string{"2026-02", "华东", ""}, rows[2]) // 缺列补空
}

func TestInferColumnType(t *testing.T) {
	require.Equal(t, "INTEGER", datacore.InferColumnType([]string{"1", "2", ""}))
	require.Equal(t, "REAL", datacore.InferColumnType([]string{"1.5", "2", ""}))
	require.Equal(t, "DATE", datacore.InferColumnType([]string{"2026-01-01", "2026-02-03 10:00"}))
	require.Equal(t, "TEXT", datacore.InferColumnType([]string{"华东", "1"}))
	require.Equal(t, "TEXT", datacore.InferColumnType([]string{"", ""})) // 全空
}
