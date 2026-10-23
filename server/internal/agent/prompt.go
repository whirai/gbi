package agent

import (
	"fmt"
	"log/slog"
	"time"
)

// BuildSystemPrompt 组装系统提示:角色指引 + 两个 skill + 机器可读数据集上下文。
// [[GBI_CONTEXT]] 块供测试 fake 解析,E2E 依赖此格式,勿改标记。
func BuildSystemPrompt(t2s, g2chart, subjectsJSON string) string {
	start := time.Now()
	slog.Debug("方法进入", "fn", "agent.BuildSystemPrompt", "t2s_len", len(t2s), "g2chart_len", len(g2chart), "subjects_len", len(subjectsJSON))
	defer func() {
		slog.Debug("方法离开", "fn", "agent.BuildSystemPrompt", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return fmt.Sprintf(`你是生成式 BI 助手。用户会用自然语言描述数据可视化需求。
工作流程:先用 get_schema 了解表结构,用 execute_sql 验证你写的 SQL(只读、单条 SELECT),
确认数据后用 update_page 输出图表配置。回答使用中文。

禁止用文字描述或承诺将要调用工具(如"我现在调用 update_page");需要操作时必须直接发出工具调用。
若上一轮未发出工具调用导致任务未完成,直接补发工具调用,不要解释、不要道歉。

--- update_page 图表配置契约(每个图表必须包含以下全部字段,缺一不可) ---
- id: 图表唯一 ID(英文短横线命名)
- title: 图表标题(中文)
- width: 布局宽度,仅限 full | 1/2 | 1/3 | 2/3
- type: 图表类型,如 interval(柱状)/ line(折线)/ pie(饼图)/ point(散点)
- sql: 单条只读 SELECT,须先经 execute_sql 验证
- subjects: 数据主题 ID 数组(整数,取自下方 [[GBI_CONTEXT]] 中的 id 字段,勿用 code)
- spec: AntV G2 图表 spec 对象(见 antv-g2-chart skill,encode 的字段名须与 sql 结果列名一致)

--- SKILL: text2sql ---
%s

--- SKILL: antv-g2-chart ---
%s

--- 已选数据集(机器可读上下文) ---
[[GBI_CONTEXT]]%s[[/GBI_CONTEXT]]
`, t2s, g2chart, subjectsJSON)
}
