package service_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gbi/server/internal/config"
	"gbi/server/internal/datacore"
	"gbi/server/internal/model"
	"gbi/server/internal/service"
)

func newTableEnv(t *testing.T) (*service.TableService, *service.SubjectService) {
	d := newTestDB(t)
	dir := newTestDBDataDir(t)
	cfg := &config.Config{DBPath: dir + "/meta.db"}
	subjects := service.NewSubjectService(d, cfg)
	return service.NewTableService(d, subjects), subjects
}

func TestTableCreateListDelete(t *testing.T) {
	svc, subjects := newTableEnv(t)
	sub, err := subjects.Create(service.SubjectCreate{Name: "主题", Code: "demo"})
	require.NoError(t, err)

	dto, err := svc.Create(sub.ID, service.TableCreate{
		Name: "orders", Kind: "DWD",
		Columns: []model.ColumnDef{{Name: "month", Type: "TEXT"}, {Name: "amount", Type: "INTEGER"}},
	})
	require.NoError(t, err)
	require.Equal(t, "dwd_orders", dto.Name)
	require.Equal(t, "DWD", dto.Kind)
	require.Len(t, dto.Columns, 2)

	// 物理表真实存在且可写
	d, closeFn, err := datacore.OpenSubject(sub.DbFile, false)
	require.NoError(t, err)
	require.NoError(t, d.Exec(`INSERT INTO "dwd_orders" ("month","amount") VALUES (?,?)`, "2026-01", 5).Error)
	closeFn()

	// DIM 前缀
	dto2, err := svc.Create(sub.ID, service.TableCreate{
		Name: "region", Kind: "DIM",
		Columns: []model.ColumnDef{{Name: "code", Type: "TEXT"}},
	})
	require.NoError(t, err)
	require.Equal(t, "dim_region", dto2.Name)

	// 重复表名
	_, err = svc.Create(sub.ID, service.TableCreate{
		Name: "orders", Kind: "DWD",
		Columns: []model.ColumnDef{{Name: "x", Type: "TEXT"}},
	})
	require.ErrorIs(t, err, service.ErrDuplicateTableName)

	// 非法 kind / 非法列名
	_, err = svc.Create(sub.ID, service.TableCreate{Name: "a", Kind: "ODS", Columns: []model.ColumnDef{{Name: "x", Type: "TEXT"}}})
	require.ErrorIs(t, err, service.ErrInvalidTableKind)
	_, err = svc.Create(sub.ID, service.TableCreate{Name: "b", Kind: "DWD", Columns: []model.ColumnDef{{Name: "1x", Type: "TEXT"}}})
	require.Error(t, err)

	list, err := svc.List(sub.ID)
	require.NoError(t, err)
	require.Len(t, list, 2)

	require.NoError(t, svc.Delete(sub.ID, dto.ID))
	list, _ = svc.List(sub.ID)
	require.Len(t, list, 1)
	err = svc.Delete(sub.ID, dto.ID) // 重复删除
	require.ErrorIs(t, err, service.ErrTableNotFound)
}

func TestTableGetCrossSubjectDenied(t *testing.T) {
	svc, subjects := newTableEnv(t)
	s1, _ := subjects.Create(service.SubjectCreate{Name: "A", Code: "a"})
	s2, _ := subjects.Create(service.SubjectCreate{Name: "B", Code: "b"})
	dto, err := svc.Create(s1.ID, service.TableCreate{Name: "t", Kind: "DWD", Columns: []model.ColumnDef{{Name: "x", Type: "TEXT"}}})
	require.NoError(t, err)
	_, _, err = svc.Get(s2.ID, dto.ID) // 跨主题访问
	require.ErrorIs(t, err, service.ErrTableNotFound)
}
