package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/casbin/casbin/v2"
	"gorm.io/gorm"

	"gbi/server/internal/datacore"
	"gbi/server/internal/llm"
	"gbi/server/internal/logger"
	"gbi/server/internal/model"
	"gbi/server/internal/service"
)

// RunContext 一次 Run 的上下文(页面/已选主题/用户)。
type RunContext struct {
	PageID     uint
	SubjectIDs []uint
	UserID     uint
}

type ToolExecutor struct {
	db         *gorm.DB
	e          *casbin.Enforcer
	subjects   *service.SubjectService
	tables     *service.TableService
	pages      *service.PageService
	renderErrs *service.RenderErrorService
	checkWait  time.Duration
}

func NewToolExecutor(db *gorm.DB, e *casbin.Enforcer, subjects *service.SubjectService, tables *service.TableService, pages *service.PageService, renderErrs *service.RenderErrorService, checkWait time.Duration) *ToolExecutor {
	start := time.Now()
	slog.Debug("方法进入", "fn", "agent.NewToolExecutor", "check_wait_ms", checkWait.Milliseconds())
	defer func() {
		slog.Debug("方法离开", "fn", "agent.NewToolExecutor", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &ToolExecutor{db: db, e: e, subjects: subjects, tables: tables, pages: pages, renderErrs: renderErrs, checkWait: checkWait}
}

// ToolDefinitions 供 LLM 调用的工具清单(schema 与 Execute 的入参对应)。
func ToolDefinitions() []llm.Tool {
	start := time.Now()
	slog.Debug("方法进入", "fn", "agent.ToolDefinitions")
	defer func() {
		slog.Debug("方法离开", "fn", "agent.ToolDefinitions", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return []llm.Tool{
		{Name: "list_datasets", Description: "列出当前已选数据主题及其表", InputSchema: map[string]any{"type": "object", "properties": map[string]any{}}},
		{Name: "get_schema", Description: "查看某主题某表的列定义与样例行", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"subject_code": map[string]any{"type": "string"},
				"table":        map[string]any{"type": "string"},
			},
			"required": []string{"subject_code"},
		}},
		{Name: "execute_sql", Description: "只读执行单条 SELECT(自动限 10000 行;回填展示最多 200 行,超出时 truncated=true 并带 total_rows),用于验证 SQL", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"sql": map[string]any{"type": "string"}},
			"required":   []string{"sql"},
		}},
		{Name: "update_page", Description: "用给定的图表配置全量更新当前页面(charts 为页面全部图表)", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"charts": map[string]any{
					"type":        "array",
					"description": "页面全部图表配置;传空数组表示清空页面",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":       map[string]any{"type": "string", "description": "图表唯一 ID(英文)"},
							"title":    map[string]any{"type": "string", "description": "图表标题"},
							"width":    map[string]any{"type": "string", "enum": []string{"full", "1/2", "1/3", "2/3"}, "description": "布局宽度"},
							"type":     map[string]any{"type": "string", "description": "图表类型,如 interval/line/pie/point"},
							"sql":      map[string]any{"type": "string", "description": "图表数据 SQL(单条只读 SELECT,须先经 execute_sql 验证)"},
							"subjects": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "图表使用的数据主题 ID 数组(取自已选数据集的 id,勿用 code)"},
							"spec":     map[string]any{"type": "object", "description": "AntV G2 图表 spec 对象(type/encode/transform 等,encode 字段须与 sql 结果列名一致)"},
						},
						"required": []string{"id", "title", "width", "type", "sql", "subjects", "spec"},
					},
				},
			},
			"required": []string{"charts"},
		}},
		{Name: "check_page_render", Description: "查看当前页面最近一次渲染的报错(按图表列出;update_page 后会自动执行,也可手动调用)", InputSchema: map[string]any{"type": "object", "properties": map[string]any{}}},
	}
}

func (x *ToolExecutor) subjectsInContext(rc RunContext) (subs []model.Subject, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "ToolExecutor.subjectsInContext", "subjects", len(rc.SubjectIDs))
	defer func() {
		slog.Debug("方法离开", "fn", "ToolExecutor.subjectsInContext", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	subs = make([]model.Subject, 0, len(rc.SubjectIDs))
	for _, id := range rc.SubjectIDs {
		s, err := x.subjects.Get(id)
		if err != nil {
			return nil, err
		}
		subs = append(subs, *s)
	}
	return subs, nil
}

func (x *ToolExecutor) refsForSubjects(ids []uint) (refs []datacore.SubjectRef, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "ToolExecutor.refsForSubjects", "ids", len(ids))
	defer func() {
		slog.Debug("方法离开", "fn", "ToolExecutor.refsForSubjects", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	refs = make([]datacore.SubjectRef, 0, len(ids))
	for _, id := range ids {
		s, err := x.subjects.Get(id)
		if err != nil {
			return nil, err
		}
		refs = append(refs, datacore.SubjectRef{Code: s.Code, File: s.DbFile})
	}
	return refs, nil
}

func (x *ToolExecutor) toolRefs(rc RunContext) (refs []datacore.SubjectRef, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "ToolExecutor.toolRefs", "subjects", len(rc.SubjectIDs))
	defer func() {
		slog.Debug("方法离开", "fn", "ToolExecutor.toolRefs", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	return x.refsForSubjects(rc.SubjectIDs)
}

// Execute 执行单个工具,返回文本结果与是否为错误。
func (x *ToolExecutor) Execute(ctx context.Context, name string, input json.RawMessage, rc RunContext) (out string, isErr bool) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "ToolExecutor.Execute", "tool", name)
	defer func() {
		slog.Debug("方法离开", "fn", "ToolExecutor.Execute", "elapsed_ms", time.Since(start).Milliseconds(), "is_error", isErr)
	}()
	switch name {
	case "list_datasets":
		return x.listDatasets(rc)
	case "get_schema":
		var in struct {
			SubjectCode string `json:"subject_code"`
			Table       string `json:"table"`
		}
		if err := json.Unmarshal(input, &in); err != nil {
			return "入参解析失败: " + err.Error(), true
		}
		return x.getSchema(rc, in.SubjectCode, in.Table)
	case "execute_sql":
		var in struct {
			SQL string `json:"sql"`
		}
		if err := json.Unmarshal(input, &in); err != nil {
			return "入参解析失败: " + err.Error(), true
		}
		return x.executeSQL(rc, in.SQL)
	case "update_page":
		var in struct {
			Charts []model.ChartConfig `json:"charts"`
		}
		if err := json.Unmarshal(input, &in); err != nil {
			return "入参解析失败: " + err.Error(), true
		}
		return x.updatePage(rc, in.Charts)
	case "check_page_render":
		return x.CheckRender(ctx, rc)
	default:
		return "未知工具: " + name, true
	}
}

func (x *ToolExecutor) listDatasets(rc RunContext) (out string, isErr bool) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "ToolExecutor.listDatasets", "subjects", len(rc.SubjectIDs))
	defer func() {
		slog.Debug("方法离开", "fn", "ToolExecutor.listDatasets", "elapsed_ms", time.Since(start).Milliseconds(), "is_error", isErr)
	}()
	subs, err := x.subjectsInContext(rc)
	if err != nil {
		return err.Error(), true
	}
	type tableInfo struct {
		Name    string            `json:"name"`
		Kind    string            `json:"kind"`
		Columns []model.ColumnDef `json:"columns"`
	}
	type subjectInfo struct {
		ID     uint        `json:"id"`
		Code   string      `json:"code"`
		Name   string      `json:"name"`
		Tables []tableInfo `json:"tables"`
	}
	infos := make([]subjectInfo, 0, len(subs))
	for _, s := range subs {
		tables, err := x.tables.List(s.ID)
		if err != nil {
			return err.Error(), true
		}
		ts := make([]tableInfo, 0, len(tables))
		for _, t := range tables {
			ts = append(ts, tableInfo{Name: t.Name, Kind: t.Kind, Columns: t.Columns})
		}
		infos = append(infos, subjectInfo{ID: s.ID, Code: s.Code, Name: s.Name, Tables: ts})
	}
	b, _ := json.Marshal(infos)
	return string(b), false
}

func (x *ToolExecutor) getSchema(rc RunContext, subjectCode, table string) (out string, isErr bool) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "ToolExecutor.getSchema", "subject_code", logger.Truncate(subjectCode, 64), "table", logger.Truncate(table, 64))
	defer func() {
		slog.Debug("方法离开", "fn", "ToolExecutor.getSchema", "elapsed_ms", time.Since(start).Milliseconds(), "is_error", isErr)
	}()
	subs, err := x.subjectsInContext(rc)
	if err != nil {
		return err.Error(), true
	}
	var target *model.Subject
	for i := range subs {
		if subs[i].Code == subjectCode {
			target = &subs[i]
			break
		}
	}
	if target == nil {
		return "主题不在已选数据集: " + subjectCode, true
	}
	tables, err := x.tables.List(target.ID)
	if err != nil {
		return err.Error(), true
	}
	var sb strings.Builder
	for _, t := range tables {
		if table != "" && t.Name != table {
			continue
		}
		fmt.Fprintf(&sb, "表 %s (%s)\n", t.Name, t.Kind)
		for _, c := range t.Columns {
			fmt.Fprintf(&sb, "  - %s %s %s\n", c.Name, c.Type, c.Comment)
		}
		// 3 行样例
		refs := []datacore.SubjectRef{{Code: target.Code, File: target.DbFile}}
		cols, rows, err := datacore.ExecuteQuery(refs, "SELECT * FROM "+t.Name+" LIMIT 3")
		if err == nil {
			fmt.Fprintf(&sb, "  样例(%s)\n", strings.Join(cols, ","))
			for _, r := range rows {
				fmt.Fprintf(&sb, "  %v\n", r)
			}
		}
	}
	if sb.Len() == 0 {
		return "未找到表: " + table, true
	}
	return sb.String(), false
}

// modelRowsCap 回填给模型的最大行数:ExecuteQuery 上限为 10000 行,
// 未聚合宽查询整段入库会污染上下文并放大 token 成本,展示层面再截断(规格:≤200 行)。
const modelRowsCap = 200

func (x *ToolExecutor) executeSQL(rc RunContext, sql string) (out string, isErr bool) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "ToolExecutor.executeSQL", "sql_len", len(sql))
	defer func() {
		slog.Debug("方法离开", "fn", "ToolExecutor.executeSQL", "elapsed_ms", time.Since(start).Milliseconds(), "is_error", isErr)
	}()
	refs, err := x.toolRefs(rc)
	if err != nil {
		return err.Error(), true
	}
	cols, rows, err := datacore.ExecuteQuery(refs, sql)
	if err != nil {
		return err.Error(), true
	}
	total := len(rows)
	truncated := total > modelRowsCap
	if truncated {
		rows = rows[:modelRowsCap]
	}
	payload := map[string]any{"columns": cols, "rows": rows, "total_rows": total, "truncated": truncated}
	b, _ := json.Marshal(payload)
	return string(b), false
}

func (x *ToolExecutor) updatePage(rc RunContext, charts []model.ChartConfig) (out string, isErr bool) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "ToolExecutor.updatePage", "page_id", rc.PageID, "charts", len(charts))
	defer func() {
		slog.Debug("方法离开", "fn", "ToolExecutor.updatePage", "elapsed_ms", time.Since(start).Milliseconds(), "is_error", isErr)
	}()
	if charts == nil {
		return "charts 必填(数组;传空数组清空页面)", true
	}
	// 契约校验:真实 LLM 可能省略 subjects/spec,此类配置入库后渲染必然失败
	// (/query 要求 subjects 非空、ChartView 需要 spec)。在工具层拒绝并返回
	// 可指导模型重试的错误信息,而不是保存不可渲染的配置。
	for i, ch := range charts {
		if len(ch.Subjects) == 0 {
			return fmt.Sprintf("charts[%d] 缺少 subjects:必须为数据主题 ID 数组(取自已选数据集的 id)", i), true
		}
		if len(ch.Spec) == 0 {
			return fmt.Sprintf("charts[%d] 缺少 spec:必须为 AntV G2 图表 spec 对象(type/encode 等)", i), true
		}
		for _, sid := range ch.Subjects {
			if _, err := x.subjects.Get(sid); err != nil {
				return fmt.Sprintf("charts[%d].subjects 含不存在的数据主题 ID: %d(只能用已选数据集的 id)", i, sid), true
			}
		}
		// 渲染前校验:执行该图表的 SQL 并核对 spec.encode 字段名与返回列一致。
		// 线上事故:encode 引用不存在的列(如 color:"燃料" 不在列中)导致图表
		// 渲染空白,而模型没有渲染反馈,会在不知情下宣称成功。
		refs, err := x.refsForSubjects(ch.Subjects)
		if err != nil {
			return err.Error(), true
		}
		cols, _, err := datacore.ExecuteQuery(refs, ch.SQL)
		if err != nil {
			return fmt.Sprintf("charts[%d](%s) 的 SQL 执行失败: %v", i, ch.Title, err), true
		}
		colSet := make(map[string]bool, len(cols))
		for _, c := range cols {
			colSet[c] = true
		}
		var missing []string
		if enc, ok := ch.Spec["encode"].(map[string]any); ok {
			for field, v := range enc {
				if name, ok := v.(string); ok && !colSet[name] {
					missing = append(missing, fmt.Sprintf("encode.%s=%q", field, name))
				}
			}
		}
		if len(missing) > 0 {
			return fmt.Sprintf("charts[%d](%s) 的 spec 字段不在 SQL 返回列中: %s(SQL 返回列: %s)",
				i, ch.Title, strings.Join(missing, ", "), strings.Join(cols, ", ")), true
		}
	}
	cfg := model.PageConfig{Version: 1, Layout: "fluid-grid", Charts: charts}
	_, err := x.pages.Update(rc.UserID, rc.PageID, service.PageUpdate{Config: &cfg})
	if err != nil {
		return err.Error(), true
	}
	return "ok: 页面配置已更新", false
}

// CheckRender 查询当前页面未读渲染错误(两次间隔查询,等前端异步渲染上报)。
// 返回人类可读文本;查询失败不中断调用方。
func (x *ToolExecutor) CheckRender(ctx context.Context, rc RunContext) (out string, isErr bool) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "ToolExecutor.CheckRender", "page_id", rc.PageID)
	defer func() {
		slog.Debug("方法离开", "fn", "ToolExecutor.CheckRender", "elapsed_ms", time.Since(start).Milliseconds(), "is_error", isErr)
	}()
	slog.Debug("渲染核验查询", "page_id", rc.PageID, "wait_ms", x.checkWait.Milliseconds())
	if x.renderErrs == nil {
		return "[渲染校验结果] 渲染校验不可用", false
	}
	var all []model.PageRenderError
	for i := 0; i < 2; i++ {
		select {
		case <-ctx.Done():
			return "[渲染校验结果] 渲染校验不可用: 请求已取消", false
		case <-time.After(x.checkWait):
		}
		unseen, err := x.renderErrs.Unseen(rc.PageID)
		if err != nil {
			return "[渲染校验结果] 渲染校验不可用: " + err.Error(), false
		}
		all = append(all, unseen...)
		if len(unseen) > 0 {
			break
		}
	}
	if len(all) == 0 {
		slog.Debug("渲染核验结果", "page_id", rc.PageID, "errors", 0)
		return "[渲染校验结果] 暂无报错", false
	}
	ids := make([]uint, 0, len(all))
	var sb strings.Builder
	sb.WriteString("[渲染校验结果]\n")
	for _, e := range all {
		ids = append(ids, e.ID)
		fmt.Fprintf(&sb, "- 图表 %s: %s\n", e.ChartID, e.Message)
	}
	_ = x.renderErrs.MarkSeen(ids)
	slog.Warn("渲染核验发现渲染错误", "page_id", rc.PageID, "count", len(all), "first_preview", logger.Truncate(all[0].Message, 160))
	return sb.String(), false
}
