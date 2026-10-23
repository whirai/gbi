package service_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gbi/server/internal/config"
	"gbi/server/internal/datacore"
	"gbi/server/internal/model"
	"gbi/server/internal/service"
)

func newRowsEnv(t *testing.T) (*service.RowsService, *service.TableService, uint, uint) {
	d := newTestDB(t)
	dir := newTestDBDataDir(t)
	cfg := &config.Config{DBPath: dir + "/meta.db"}
	subjects := service.NewSubjectService(d, cfg)
	tables := service.NewTableService(d, subjects)
	rows := service.NewRowsService(d, tables)

	sub, err := subjects.Create(service.SubjectCreate{Name: "行主题", Code: "rows"})
	require.NoError(t, err)
	dto, err := tables.Create(sub.ID, service.TableCreate{
		Name: "t", Kind: "DWD",
		Columns: []model.ColumnDef{
			{Name: "name", Type: "TEXT"}, {Name: "qty", Type: "INTEGER"}, {Name: "price", Type: "REAL"},
		},
	})
	require.NoError(t, err)

	// 直接写 3 行测试数据
	sd, closeFn, err := datacore.OpenSubject(sub.DbFile, false)
	require.NoError(t, err)
	require.NoError(t, sd.Exec(`INSERT INTO "dwd_t" ("name","qty","price") VALUES ('a',1,1.5),('b',2,2.5),('c',3,3.5)`).Error)
	closeFn()
	return rows, tables, sub.ID, dto.ID
}

func TestRowsListAndUpdateCell(t *testing.T) {
	rows, _, sid, tid := newRowsEnv(t)

	page, err := rows.List(sid, tid, 1, 20, nil)
	require.NoError(t, err)
	require.Equal(t, int64(3), page.Total)
	require.Equal(t, []string{"name", "qty", "price"}, page.Columns)
	require.Len(t, page.Rows, 3)
	require.Equal(t, "a", page.Rows[0]["name"])
	require.NotZero(t, page.Rows[0]["_rid"])

	rid := int64(page.Rows[0]["_rid"].(int64))

	// 合法更新:INTEGER 数字 / 数字字符串 / TEXT
	require.NoError(t, rows.UpdateCell(sid, tid, rid, "qty", float64(10)))
	require.NoError(t, rows.UpdateCell(sid, tid, rid, "qty", "12"))
	require.NoError(t, rows.UpdateCell(sid, tid, rid, "name", "a2"))
	// NULL
	require.NoError(t, rows.UpdateCell(sid, tid, rid, "name", nil))

	page, _ = rows.List(sid, tid, 1, 20, nil)
	require.Equal(t, float64(12), toFloat(page.Rows[0]["qty"]))
	require.Nil(t, page.Rows[0]["name"])

	// 非法列 / 非法值 / 不存在的行
	require.ErrorIs(t, rows.UpdateCell(sid, tid, rid, "nope", "x"), service.ErrInvalidColumn)
	require.ErrorIs(t, rows.UpdateCell(sid, tid, rid, "qty", "abc"), service.ErrInvalidValue)
	require.ErrorIs(t, rows.UpdateCell(sid, tid, rid, "qty", 1e30), service.ErrInvalidValue) // 超出 int64 范围
	require.ErrorIs(t, rows.UpdateCell(sid, tid, 999999, "qty", float64(1)), service.ErrRowNotFound)
}

func toFloat(v any) float64 {
	switch x := v.(type) {
	case int64:
		return float64(x)
	case float64:
		return x
	default:
		return -1
	}
}

func TestRowsPaginationAndDelete(t *testing.T) {
	rows, _, sid, tid := newRowsEnv(t)

	page, err := rows.List(sid, tid, 1, 2, nil)
	require.NoError(t, err)
	require.Len(t, page.Rows, 2)
	page, err = rows.List(sid, tid, 2, 2, nil)
	require.NoError(t, err)
	require.Len(t, page.Rows, 1)

	rid := int64(page.Rows[0]["_rid"].(int64))
	require.NoError(t, rows.DeleteRow(sid, tid, rid))
	require.ErrorIs(t, rows.DeleteRow(sid, tid, rid), service.ErrRowNotFound)

	page, _ = rows.List(sid, tid, 1, 20, nil)
	require.Equal(t, int64(2), page.Total)
}

// TestRowsDistinctValues 列取值集合:去重升序、排除 NULL、limit 截断、整数列字符串化、非法列拒绝。
func TestRowsDistinctValues(t *testing.T) {
	rows, _, sid, tid := newRowsEnv(t)

	vals, err := rows.DistinctValues(sid, tid, "name", 100)
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b", "c"}, vals)

	// limit 截断(升序前 N)
	vals, err = rows.DistinctValues(sid, tid, "name", 2)
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, vals)

	// NULL 不进入候选值(把 name='a' 置 NULL 后消失)
	page, err := rows.List(sid, tid, 1, 20, nil)
	require.NoError(t, err)
	rid := int64(page.Rows[0]["_rid"].(int64))
	require.NoError(t, rows.UpdateCell(sid, tid, rid, "name", nil))
	vals, err = rows.DistinctValues(sid, tid, "name", 100)
	require.NoError(t, err)
	require.Equal(t, []string{"b", "c"}, vals)

	// 整数列候选值以字符串返回
	vals, err = rows.DistinctValues(sid, tid, "qty", 100)
	require.NoError(t, err)
	require.Equal(t, []string{"1", "2", "3"}, vals)

	// 非法列
	_, err = rows.DistinctValues(sid, tid, "nope", 100)
	require.ErrorIs(t, err, service.ErrInvalidColumn)
}

// TestRowsListFilters 服务端列筛选:多值 IN、整数列(列亲和性)、组合 AND、
// 空值列表=不过滤、非法列拒绝、total 与分页同步按筛选结果计算。
func TestRowsListFilters(t *testing.T) {
	rows, _, sid, tid := newRowsEnv(t)

	page, err := rows.List(sid, tid, 1, 20, map[string][]string{"name": {"a", "c"}})
	require.NoError(t, err)
	require.Equal(t, int64(2), page.Total)
	require.Len(t, page.Rows, 2)
	names := []string{page.Rows[0]["name"].(string), page.Rows[1]["name"].(string)}
	require.ElementsMatch(t, []string{"a", "c"}, names)

	// 整数列用字符串值筛选(SQLite 列亲和性把文本转数字后比较)
	page, err = rows.List(sid, tid, 1, 20, map[string][]string{"qty": {"2"}})
	require.NoError(t, err)
	require.Equal(t, int64(1), page.Total)
	require.Equal(t, "b", page.Rows[0]["name"])

	// 组合筛选 AND
	page, err = rows.List(sid, tid, 1, 20, map[string][]string{"name": {"a", "b"}, "qty": {"1"}})
	require.NoError(t, err)
	require.Equal(t, int64(1), page.Total)
	require.Equal(t, "a", page.Rows[0]["name"])

	// 空值列表视为不过滤
	page, err = rows.List(sid, tid, 1, 20, map[string][]string{"name": {}})
	require.NoError(t, err)
	require.Equal(t, int64(3), page.Total)

	// 不存在的列 → ErrInvalidColumn
	_, err = rows.List(sid, tid, 1, 20, map[string][]string{"nope": {"x"}})
	require.ErrorIs(t, err, service.ErrInvalidColumn)

	// 分页叠加:total 是筛选后的总数,行按 OFFSET 取
	page, err = rows.List(sid, tid, 1, 1, map[string][]string{"name": {"a", "b", "c"}})
	require.NoError(t, err)
	require.Equal(t, int64(3), page.Total)
	require.Len(t, page.Rows, 1)
}
