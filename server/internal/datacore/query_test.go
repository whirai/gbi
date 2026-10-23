package datacore_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"gbi/server/internal/datacore"
	"gbi/server/internal/model"
)

func TestValidateReadOnlySQL(t *testing.T) {
	ok := []string{
		"SELECT * FROM sales.dwd_orders",
		"  select count(*) from t  ",
		"WITH x AS (SELECT 1 AS a) SELECT * FROM x",
		"SELECT * FROM t;", // 允许末尾单个分号
	}
	for _, s := range ok {
		require.NoError(t, datacore.ValidateReadOnlySQL(s), s)
	}
	require.NoError(t, datacore.ValidateReadOnlySQL("  SELECT 1;\n"))
	bad := []string{
		"",
		"DELETE FROM t",
		"SELECT * FROM t; DROP TABLE t",
		"INSERT INTO t VALUES (1)",
		"UPDATE t SET a=1",
		"PRAGMA table_info(t)",
		"SELECT * FROM t WHERE name = 'x'", // 通过(字面量无关键词)
	}
	// 注意:上一行 'x' 的用例应当通过,单独断言:
	require.NoError(t, datacore.ValidateReadOnlySQL("SELECT * FROM t WHERE name = 'x'"))
	for _, s := range bad[:6] {
		require.Error(t, datacore.ValidateReadOnlySQL(s), s)
	}
	// 多个 LIMIT 拒绝
	require.Error(t, datacore.ValidateReadOnlySQL("SELECT * FROM (SELECT 1) LIMIT 5 LIMIT 5"))
	// 写操作关键词(大小写混合)
	require.Error(t, datacore.ValidateReadOnlySQL("dElEtE FROM t"))
	require.Error(t, datacore.ValidateReadOnlySQL("SELECT * FROM t WHERE a IN (SELECT b FROM c); ATTACH 'x' AS y"))
}

func TestCapLimit(t *testing.T) {
	require.Equal(t, "SELECT * FROM t LIMIT 10000", datacore.CapLimit("SELECT * FROM t"))
	require.Equal(t, "SELECT * FROM t LIMIT 5", datacore.CapLimit("SELECT * FROM t LIMIT 5"))
	require.Equal(t, "SELECT * FROM t LIMIT 10000", datacore.CapLimit("SELECT * FROM t LIMIT 20000"))
	require.Equal(t, "SELECT * FROM t LIMIT 10000", datacore.CapLimit("SELECT * FROM t LIMIT 999999"))
	require.Equal(t, "SELECT * FROM t LIMIT 5 OFFSET 10", datacore.CapLimit("SELECT * FROM t LIMIT 5 OFFSET 10"))
	require.Equal(t, "SELECT * FROM t LIMIT 2, 10000", datacore.CapLimit("SELECT * FROM t LIMIT 2, 20000"))
	// 非正数就地替换:LIMIT -1(不限制)此前会因追加第二段 LIMIT 变成语法错误
	require.Equal(t, "SELECT * FROM t LIMIT 10000", datacore.CapLimit("SELECT * FROM t LIMIT -1"))
	require.Equal(t, "SELECT * FROM t LIMIT 10000", datacore.CapLimit("SELECT * FROM t LIMIT 0"))
	require.Equal(t, "SELECT * FROM t LIMIT 2, 10000", datacore.CapLimit("SELECT * FROM t LIMIT 2, -1"))
}

func TestExecuteQuery(t *testing.T) {
	// 造两个主题库
	dir := t.TempDir()
	f1 := filepath.Join(dir, "s1.db")
	d1, close1, err := datacore.OpenSubject(f1, false)
	require.NoError(t, err)
	require.NoError(t, datacore.CreateTable(d1, "dwd_orders", []model.ColumnDef{
		{Name: "region", Type: "TEXT"}, {Name: "amount", Type: "INTEGER"},
	}))
	require.NoError(t, d1.Exec(`INSERT INTO "dwd_orders" VALUES ('east', 10), ('west', 20)`).Error)
	close1()

	f2 := filepath.Join(dir, "s2.db")
	d2, close2, err := datacore.OpenSubject(f2, false)
	require.NoError(t, err)
	require.NoError(t, datacore.CreateTable(d2, "dim_region", []model.ColumnDef{
		{Name: "code", Type: "TEXT"}, {Name: "name", Type: "TEXT"},
	}))
	require.NoError(t, d2.Exec(`INSERT INTO "dim_region" VALUES ('east', '华东')`).Error)
	close2()

	refs := []datacore.SubjectRef{{Code: "s1", File: f1}, {Code: "s2", File: f2}}

	cols, rows, err := datacore.ExecuteQuery(refs, "SELECT region, amount FROM s1.dwd_orders ORDER BY amount")
	require.NoError(t, err)
	require.Equal(t, []string{"region", "amount"}, cols)
	require.Len(t, rows, 2)
	require.Equal(t, "east", rows[0][0])
	require.Equal(t, int64(10), rows[0][1])

	// 跨主题 JOIN
	_, rows, err = datacore.ExecuteQuery(refs,
		"SELECT o.region, r.name FROM s1.dwd_orders o JOIN s2.dim_region r ON r.code = o.region")
	require.NoError(t, err)
	require.Len(t, rows, 1)

	// 只读防线:写操作被拒
	_, _, err = datacore.ExecuteQuery(refs, "DELETE FROM s1.dwd_orders")
	require.Error(t, err)
	_, _, err = datacore.ExecuteQuery(refs, "PRAGMA s1.table_info(dwd_orders)")
	require.Error(t, err)

	// LIMIT 上限:插入 3 行后 LIMIT 999999 仍只受 MaxRows 约束(此处验证 CapLimit 生效不报错)
	_, rows, err = datacore.ExecuteQuery(refs, "SELECT * FROM s1.dwd_orders LIMIT 999999")
	require.NoError(t, err)
	require.Len(t, rows, 2)

	// LIMIT -1(SQLite 惯用"不限制"):此前被追加成 `LIMIT -1 LIMIT 10000` 语法错误,现应正常执行
	_, rows, err = datacore.ExecuteQuery(refs, "SELECT * FROM s1.dwd_orders LIMIT -1")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	fmt.Println("query engine ok")
}
