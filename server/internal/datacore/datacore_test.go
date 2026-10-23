package datacore_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"gbi/server/internal/datacore"
	"gbi/server/internal/model"
)

func TestValidateIdentifier(t *testing.T) {
	require.NoError(t, datacore.ValidateIdentifier("dwd_orders"))
	require.NoError(t, datacore.ValidateIdentifier("_x1"))
	require.Error(t, datacore.ValidateIdentifier(""))
	require.Error(t, datacore.ValidateIdentifier("1abc"))
	require.Error(t, datacore.ValidateIdentifier("a-b"))
	require.Error(t, datacore.ValidateIdentifier(`a"b`))
	require.Error(t, datacore.ValidateIdentifier(string(make([]rune, 65))))
}

func TestOpenSubjectAndReadWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subject-1.db")

	d, closeFn, err := datacore.OpenSubject(path, false)
	require.NoError(t, err)
	cols := []model.ColumnDef{{Name: "name", Type: "TEXT"}, {Name: "qty", Type: "INTEGER"}}
	require.NoError(t, datacore.CreateTable(d, "dwd_t", cols))
	require.NoError(t, d.Exec(`INSERT INTO "dwd_t" ("name","qty") VALUES (?,?)`, "a", 1).Error)
	closeFn()

	// 只读打开:查询可以,写入必须失败
	ro, closeRO, err := datacore.OpenSubject(path, true)
	require.NoError(t, err)
	defer closeRO()
	var n int64
	// 注意:Table 参数不能自带双引号——GORM 会再用反引号包一层,导致标识符变成带引号的字面名。
	require.NoError(t, ro.Table("dwd_t").Count(&n).Error)
	require.Equal(t, int64(1), n)
	require.Error(t, ro.Exec(`INSERT INTO "dwd_t" ("name","qty") VALUES (?,?)`, "b", 2).Error)

	// DropTable
	d2, closeFn2, err := datacore.OpenSubject(path, false)
	require.NoError(t, err)
	defer closeFn2()
	require.NoError(t, datacore.DropTable(d2, "dwd_t"))
}

func TestCreateTableRejectsBadDefs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subject-2.db")
	d, closeFn, err := datacore.OpenSubject(path, false)
	require.NoError(t, err)
	defer closeFn()

	require.Error(t, datacore.CreateTable(d, "bad-name", []model.ColumnDef{{Name: "a", Type: "TEXT"}}))
	require.Error(t, datacore.CreateTable(d, "ok_name", []model.ColumnDef{{Name: "1bad", Type: "TEXT"}}))
	require.Error(t, datacore.CreateTable(d, "ok_name", []model.ColumnDef{{Name: "a", Type: "VARCHAR"}}))
	require.Error(t, datacore.CreateTable(d, "ok_name", nil))
}
