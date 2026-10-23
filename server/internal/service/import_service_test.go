package service_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"gbi/server/internal/config"
	"gbi/server/internal/service"
)

func newImportEnv(t *testing.T) (*service.ImportService, *service.TableService, *service.SubjectService) {
	d := newTestDB(t)
	dir := newTestDBDataDir(t)
	cfg := &config.Config{DBPath: dir + "/meta.db"}
	subjects := service.NewSubjectService(d, cfg)
	tables := service.NewTableService(d, subjects)
	return service.NewImportService(d, subjects, tables), tables, subjects
}

func sampleXlsx(t *testing.T) *bytes.Buffer {
	t.Helper()
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	rows := [][]any{
		{"month", "region", "amount"},
		{"2026-01", "华东", 100},
		{"2026-01", "华北", 200},
		{"2026-02", "华东", 150},
	}
	for r, row := range rows {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+1)
			require.NoError(t, f.SetCellValue(sheet, cell, v))
		}
	}
	buf, err := f.WriteToBuffer()
	require.NoError(t, err)
	return buf
}

func TestImportCreate(t *testing.T) {
	svc, tables, subjects := newImportEnv(t)
	sub, err := subjects.Create(service.SubjectCreate{Name: "导入主题", Code: "imp"})
	require.NoError(t, err)

	res, err := svc.Run(sub.ID, sampleXlsx(t), service.ImportParams{
		Mode: "create", TableName: "sales", Kind: "DWD",
	})
	require.NoError(t, err)
	require.Equal(t, 3, res.Inserted)

	list, err := tables.List(sub.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "dwd_sales", list[0].Name)
	// 推断类型:month→DATE? "2026-01" 不匹配 yyyy-mm-dd → TEXT;region→TEXT;amount→INTEGER
	types := map[string]string{}
	for _, c := range list[0].Columns {
		types[c.Name] = c.Type
	}
	require.Equal(t, "TEXT", types["month"])
	require.Equal(t, "TEXT", types["region"])
	require.Equal(t, "INTEGER", types["amount"])
}

func TestImportAppend(t *testing.T) {
	svc, tables, subjects := newImportEnv(t)
	sub, _ := subjects.Create(service.SubjectCreate{Name: "追加主题", Code: "app"})

	res, err := svc.Run(sub.ID, sampleXlsx(t), service.ImportParams{
		Mode: "create", TableName: "sales", Kind: "DWD",
	})
	require.NoError(t, err)

	res2, err := svc.Run(sub.ID, sampleXlsx(t), service.ImportParams{
		Mode: "append", TableID: res.TableID,
	})
	require.NoError(t, err)
	require.Equal(t, 3, res2.Inserted)
	require.Equal(t, res.TableID, res2.TableID)

	// 文件列不在表中 → 整体拒绝
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetCellValue(sheet, "A1", "month")
	_ = f.SetCellValue(sheet, "B1", "not_exist")
	_ = f.SetCellValue(sheet, "A2", "2026-03")
	_ = f.SetCellValue(sheet, "B2", "x")
	buf, _ := f.WriteToBuffer()
	_, err = svc.Run(sub.ID, buf, service.ImportParams{Mode: "append", TableID: res.TableID})
	require.Error(t, err)

	// 非法表头(含非法标识符)→ create 拒绝
	f2 := excelize.NewFile()
	sheet2 := f2.GetSheetName(0)
	_ = f2.SetCellValue(sheet2, "A1", "1bad")
	_ = f2.SetCellValue(sheet2, "A2", "v")
	buf2, _ := f2.WriteToBuffer()
	_, err = svc.Run(sub.ID, buf2, service.ImportParams{Mode: "create", TableName: "x", Kind: "DWD"})
	require.Error(t, err)
	_ = tables
}
