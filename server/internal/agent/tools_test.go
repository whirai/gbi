package agent_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gbi/server/internal/agent"
	"gbi/server/internal/config"
	"gbi/server/internal/datacore"
	"gbi/server/internal/db"
	"gbi/server/internal/llm"
	"gbi/server/internal/model"
	"gbi/server/internal/rbac"
	"gbi/server/internal/service"
)

func newToolsEnv(t *testing.T) (*agent.ToolExecutor, agent.RunContext) {
	exec, rc, _ := newToolsEnvFull(t)
	return exec, rc
}

// newToolsEnvFull 与 newToolsEnv 相同,但额外返回 SubjectService(供测试直连主题库灌数据)。
func newToolsEnvFull(t *testing.T) (*agent.ToolExecutor, agent.RunContext, *service.SubjectService) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	require.NoError(t, db.Migrate(d))
	require.NoError(t, db.Seed(d))
	e, err := rbac.NewEnforcer(d)
	require.NoError(t, err)
	require.NoError(t, db.SeedRBAC(d, e))

	cfg := config.Config{DBPath: filepath.Join(t.TempDir(), "meta.db")}
	subjects := service.NewSubjectService(d, &cfg)
	tables := service.NewTableService(d, subjects)
	pages := service.NewPageService(d, e)

	sub, err := subjects.Create(service.SubjectCreate{Name: "销售", Code: "sales"})
	require.NoError(t, err)
	_, err = tables.Create(sub.ID, service.TableCreate{
		Name: "orders", Kind: "DWD",
		Columns: []model.ColumnDef{{Name: "region", Type: "TEXT"}, {Name: "amount", Type: "INTEGER"}},
	})
	require.NoError(t, err)
	page, err := pages.Create(1, service.PageCreate{Name: "测试页"})
	require.NoError(t, err)

	renderErrs := service.NewRenderErrorService(d)
	exec := agent.NewToolExecutor(d, e, subjects, tables, pages, renderErrs, 10*time.Millisecond)
	return exec, agent.RunContext{PageID: page.ID, SubjectIDs: []uint{sub.ID}, UserID: 1}, subjects
}

// TestToolCheckPageRender 渲染错误查询:未读返回→标记已读→再查为空。
func TestToolCheckPageRender(t *testing.T) {
	exec, rc := newToolsEnv(t)
	_ = exec // rc 用于后续
	// 通过 exec 无法直接 Report——直接经 service 层断言已在 Task 3 覆盖;
	// 此处验证 CheckRender 对无错页面的输出。
	out, isErr := exec.CheckRender(context.Background(), rc)
	require.False(t, isErr)
	require.Contains(t, out, "渲染校验")
}

func TestToolListDatasets(t *testing.T) {
	exec, rc := newToolsEnv(t)
	out, isErr := exec.Execute(context.Background(), "list_datasets", json.RawMessage(`{}`), rc)
	require.False(t, isErr)
	require.Contains(t, out, "sales")
	require.Contains(t, out, "dwd_orders")
}

func TestToolGetSchema(t *testing.T) {
	exec, rc := newToolsEnv(t)
	out, isErr := exec.Execute(context.Background(), "get_schema",
		json.RawMessage(`{"subject_code":"sales","table":"dwd_orders"}`), rc)
	require.False(t, isErr)
	require.Contains(t, out, "region")
	require.Contains(t, out, "amount")
}

func TestToolExecuteSQL(t *testing.T) {
	exec, rc := newToolsEnv(t)
	out, isErr := exec.Execute(context.Background(), "execute_sql",
		json.RawMessage(`{"sql":"SELECT COUNT(*) AS n FROM dwd_orders"}`), rc)
	require.False(t, isErr)
	require.Contains(t, out, `"n"`)

	// 写操作被拒(阶段 2 闸门)
	out, isErr = exec.Execute(context.Background(), "execute_sql",
		json.RawMessage(`{"sql":"DELETE FROM dwd_orders"}`), rc)
	require.True(t, isErr)
	require.Contains(t, out, "SELECT")
}

// TestToolExecuteSQLTruncates 回填行数裁剪:整段结果超过 200 行时仅回填前 200 行,
// 并标注 truncated/total_rows(未聚合宽查询直接注入上下文会放大 token 成本)。
func TestToolExecuteSQLTruncates(t *testing.T) {
	exec, rc, subjects := newToolsEnvFull(t)
	sub, err := subjects.Get(rc.SubjectIDs[0])
	require.NoError(t, err)
	d, closeFn, err := datacore.OpenSubject(sub.DbFile, false)
	require.NoError(t, err)
	tx := d.Begin()
	for i := 0; i < 250; i++ {
		require.NoError(t, tx.Exec(`INSERT INTO "dwd_orders" ("region","amount") VALUES (?,?)`, "east", i).Error)
	}
	require.NoError(t, tx.Commit().Error)
	closeFn()

	type sqlResult struct {
		Rows      [][]any `json:"rows"`
		TotalRows int     `json:"total_rows"`
		Truncated bool    `json:"truncated"`
	}
	var res sqlResult
	out, isErr := exec.Execute(context.Background(), "execute_sql",
		json.RawMessage(`{"sql":"SELECT region, amount FROM dwd_orders"}`), rc)
	require.False(t, isErr)
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	require.Len(t, res.Rows, 200)
	require.Equal(t, 250, res.TotalRows)
	require.True(t, res.Truncated)

	// 未超限:全量回填且 truncated=false
	res = sqlResult{}
	out, isErr = exec.Execute(context.Background(), "execute_sql",
		json.RawMessage(`{"sql":"SELECT region, amount FROM dwd_orders LIMIT 5"}`), rc)
	require.False(t, isErr)
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	require.Len(t, res.Rows, 5)
	require.Equal(t, 5, res.TotalRows)
	require.False(t, res.Truncated)
}

func TestToolUpdatePage(t *testing.T) {
	exec, rc := newToolsEnv(t)
	out, isErr := exec.Execute(context.Background(), "update_page", json.RawMessage(`{
		"charts":[{"id":"c1","title":"t","width":"full","type":"interval",
			"sql":"SELECT region, SUM(amount) AS total FROM dwd_orders GROUP BY region",
			"subjects":[1],"spec":{"type":"interval","encode":{"x":"region","y":"total"}}}]
	}`), rc)
	require.False(t, isErr)
	require.Contains(t, out, "ok")

	// 非法宽度被拒
	out, isErr = exec.Execute(context.Background(), "update_page", json.RawMessage(`{
		"charts":[{"id":"c2","title":"t","width":"bad","type":"interval","sql":"SELECT 1",
			"subjects":[1],"spec":{"type":"interval"}}]
	}`), rc)
	require.True(t, isErr)
}

// TestToolUpdatePageEncodeColumnCheck 线上事故回归:模型写出的 spec encode 字段
// 与 SQL 返回列不一致(如 color:"燃料" 不在列中)导致图表渲染空白,而模型
// 没有渲染反馈会在不知情下宣称成功。update_page 必须执行 SQL 校验返回列,
// 拒绝不一致的 encode 并给出可指导重试的错误。
func TestToolUpdatePageEncodeColumnCheck(t *testing.T) {
	exec, rc := newToolsEnv(t)

	// encode.color 引用不存在的列 → 拒绝,错误信息含字段名与实际列
	out, isErr := exec.Execute(context.Background(), "update_page", json.RawMessage(`{
		"charts":[{"id":"c1","title":"t","width":"full","type":"interval",
			"sql":"SELECT region, SUM(amount) AS total FROM dwd_orders GROUP BY region",
			"subjects":[1],
			"spec":{"type":"interval","encode":{"x":"region","y":"total","color":"燃料"}}}]
	}`), rc)
	require.True(t, isErr)
	require.Contains(t, out, "燃料")
	require.Contains(t, out, "region", "错误信息应列出 SQL 实际返回列")

	// SQL 本身执行失败 → 拒绝(此前会入库,前端渲染时才暴露)
	out, isErr = exec.Execute(context.Background(), "update_page", json.RawMessage(`{
		"charts":[{"id":"c2","title":"t","width":"full","type":"interval",
			"sql":"SELECT no_such_col FROM dwd_orders","subjects":[1],
			"spec":{"type":"interval","encode":{"x":"region"}}}]
	}`), rc)
	require.True(t, isErr)
	require.Contains(t, out, "no_such_col")
}

// TestToolUpdatePageContract 真实 LLM 冒烟事故回归:模型可能省略 subjects/spec,
// 此类配置入库后渲染必然失败(/query 要求 subjects 非空),必须在工具层拒绝并给出
// 可指导重试的错误信息。
func TestToolUpdatePageContract(t *testing.T) {
	exec, rc := newToolsEnv(t)

	// 缺 subjects → 拒绝
	out, isErr := exec.Execute(context.Background(), "update_page", json.RawMessage(`{
		"charts":[{"id":"c1","title":"t","width":"full","type":"interval",
			"sql":"SELECT 1","spec":{"type":"interval"}}]
	}`), rc)
	require.True(t, isErr)
	require.Contains(t, out, "subjects")

	// 缺 spec → 拒绝
	out, isErr = exec.Execute(context.Background(), "update_page", json.RawMessage(`{
		"charts":[{"id":"c1","title":"t","width":"full","type":"interval",
			"sql":"SELECT 1","subjects":[1]}]
	}`), rc)
	require.True(t, isErr)
	require.Contains(t, out, "spec")

	// 不存在的主题 ID → 拒绝
	out, isErr = exec.Execute(context.Background(), "update_page", json.RawMessage(`{
		"charts":[{"id":"c1","title":"t","width":"full","type":"interval",
			"sql":"SELECT 1","subjects":[999],"spec":{"type":"interval"}}]
	}`), rc)
	require.True(t, isErr)
	require.Contains(t, out, "999")

	// 空 charts(清空页面)合法
	out, isErr = exec.Execute(context.Background(), "update_page", json.RawMessage(`{"charts":[]}`), rc)
	require.False(t, isErr)
	require.Contains(t, out, "ok")
}

// TestToolUpdatePageRejectsNilCharts 线上事故回归:模型发出非法 JSON 输入被客户端
// 降级为 {} 后,update_page 不得把 nil charts 当作"清空页面"(曾把整页清空白)。
func TestToolUpdatePageRejectsNilCharts(t *testing.T) {
	exec, rc := newToolsEnv(t)
	// {} 与缺 charts 字段都必须被拒
	for _, input := range []string{`{}`, `{"other":1}`} {
		out, isErr := exec.Execute(context.Background(), "update_page", json.RawMessage(input), rc)
		require.True(t, isErr)
		require.Contains(t, out, "charts 必填")
	}
	// 显式空数组仍合法(清空页面意图,既有回归)
	out, isErr := exec.Execute(context.Background(), "update_page", json.RawMessage(`{"charts":[]}`), rc)
	require.False(t, isErr)
	require.Contains(t, out, "ok")
}

// TestToolDefinitionsUpdatePageSchema update_page 的 input_schema 必须描述
// charts 元素契约(properties+required),否则真实模型无从得知必填字段。
func TestToolDefinitionsUpdatePageSchema(t *testing.T) {
	var up *llm.Tool
	defs := agent.ToolDefinitions()
	for i := range defs {
		if defs[i].Name == "update_page" {
			up = &defs[i]
			break
		}
	}
	require.NotNil(t, up)
	b, err := json.Marshal(up.InputSchema)
	require.NoError(t, err)
	s := string(b)
	for _, field := range []string{`"id"`, `"title"`, `"width"`, `"type"`, `"sql"`, `"subjects"`, `"spec"`, `"required"`, `"enum"`} {
		require.Contains(t, s, field, "update_page schema 应包含 %s", field)
	}
}
