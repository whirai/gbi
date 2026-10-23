package agent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"gbi/server/internal/agent"
	"gbi/server/internal/llm"
)

// writeToolUse/writeText 输出最小合法的 Anthropic 风格 SSE 流(与 llm 客户端解析对应)。
func writeToolUse(w http.ResponseWriter, id, name, inputJSON string) {
	partial, _ := json.Marshal(inputJSON)
	w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"fake\",\"content\":[],\"stop_reason\":null,\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"))
	w.Write([]byte("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"" + id + "\",\"name\":\"" + name + "\",\"input\":{}}}\n\n"))
	w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":" + string(partial) + "}}\n\n"))
	w.Write([]byte("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n"))
	w.Write([]byte("event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":1}}\n\n"))
	w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
}

func writeText(w http.ResponseWriter, text string) {
	delta, _ := json.Marshal(text)
	w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"fake\",\"content\":[],\"stop_reason\":null,\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"))
	w.Write([]byte("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n"))
	w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":" + string(delta) + "}}\n\n"))
	w.Write([]byte("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n"))
	w.Write([]byte("event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\n"))
	w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
}

func TestRunnerFullLoop(t *testing.T) {
	// newToolsEnv 定义在 tools_test.go(同属 agent_test 包),直接复用
	exec, rc := newToolsEnv(t)
	subjectID := rc.SubjectIDs[0]

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string            `json:"role"`
				Content []json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		toolResults := 0
		for _, m := range body.Messages {
			for _, c := range m.Content {
				var blk struct {
					Type string `json:"type"`
				}
				_ = json.Unmarshal(c, &blk)
				if blk.Type == "tool_result" {
					toolResults++
				}
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		switch toolResults {
		case 0:
			writeToolUse(w, "toolu_1", "get_schema", `{"subject_code":"sales"}`)
		case 1:
			writeToolUse(w, "toolu_2", "execute_sql", `{"sql":"SELECT COUNT(*) AS n FROM dwd_orders"}`)
		case 2:
			writeToolUse(w, "toolu_3", "update_page",
				`{"charts":[{"id":"c1","title":"t","width":"full","type":"interval","sql":"SELECT region, SUM(amount) AS total FROM dwd_orders GROUP BY region","subjects":[`+
					itoa(subjectID)+`],"spec":{"type":"interval","encode":{"x":"region","y":"total"}}}]}`)
		default:
			writeText(w, "已为您生成图表")
		}
	}))
	defer srv.Close()

	cli := llm.NewClient(srv.URL, "k", "fake", 16384)
	runner := agent.NewRunner(cli, exec)

	var events []agent.AgentEvent
	finalText, err := runner.Run(context.Background(), agent.RunRequest{
		System:  "sys",
		History: []llm.Message{{Role: "user", Text: "画个图"}},
		Context: rc,
	}, func(ev agent.AgentEvent) { events = append(events, ev) })
	require.NoError(t, err)

	// 事件断言:tool_call ×3、config_updated ×1、done、文本增量
	var types []string
	var textGot bool
	for _, ev := range events {
		types = append(types, ev.Type)
		if ev.Type == "message_delta" {
			textGot = true
		}
	}
	require.Contains(t, types, "tool_call")
	require.Contains(t, types, "config_updated") // config_updated 仅在 update_page 成功时发出,证明页面已更新
	require.Contains(t, types, "done")
	require.True(t, textGot)
	require.Equal(t, "已为您生成图表", finalText)
}

// writeMaxTokensEmpty 输出一个"思考耗尽预算"的轮次:stop_reason=max_tokens,
// 无文本、无工具调用(线上"不回复"事故的模型侧表现)。
func writeMaxTokensEmpty(w http.ResponseWriter) {
	w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"fake\",\"content\":[],\"stop_reason\":null,\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"))
	w.Write([]byte("event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"max_tokens\"},\"usage\":{\"output_tokens\":1}}\n\n"))
	w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
}

// TestRunnerContinuesOnMaxTokens 模型因 max_tokens 空手而归时(无文本无工具),
// runner 必须回填"继续"消息再跑一轮,而不是静默结束(线上"不回复"事故)。
func TestRunnerContinuesOnMaxTokens(t *testing.T) {
	exec, rc := newToolsEnv(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "text/event-stream")
		switch calls {
		case 1:
			writeMaxTokensEmpty(w)
		case 2:
			var body struct {
				Messages []struct {
					Role    string            `json:"role"`
					Content []json.RawMessage `json:"content"`
				} `json:"messages"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			// 第二轮必须带上了续写提示
			var texts []string
			for _, m := range body.Messages {
				for _, c := range m.Content {
					var blk struct {
						Type string `json:"type"`
						Text string `json:"text"`
					}
					if json.Unmarshal(c, &blk) == nil && blk.Type == "text" {
						texts = append(texts, blk.Text)
					}
				}
			}
			require.NotEmpty(t, texts, "第二轮应有 user 续写消息")
			require.Contains(t, texts[len(texts)-1], "继续")
			writeText(w, "图表已更新完成")
		default:
			writeText(w, "图表已更新完成")
		}
	}))
	defer srv.Close()

	cli := llm.NewClient(srv.URL, "k", "fake", 16384)
	runner := agent.NewRunner(cli, exec)

	var events []agent.AgentEvent
	finalText, err := runner.Run(context.Background(), agent.RunRequest{
		System:  "sys",
		History: []llm.Message{{Role: "user", Text: "调整图表"}},
		Context: rc,
	}, func(ev agent.AgentEvent) { events = append(events, ev) })
	require.NoError(t, err)
	require.Equal(t, 2, calls, "max_tokens 空轮次后应再跑一轮")
	require.Equal(t, "图表已更新完成", finalText)

	var types []string
	for _, ev := range events {
		types = append(types, ev.Type)
	}
	require.Contains(t, types, "done")
}

func itoa(u uint) string { return strconv.FormatUint(uint64(u), 10) }

// TestRunnerAutoCheckAfterUpdatePage update_page 成功后,runner 必须把
// [渲染校验结果] 消息注入下一轮(不依赖模型发起 check)。
func TestRunnerAutoCheckAfterUpdatePage(t *testing.T) {
	exec, rc := newToolsEnv(t)
	subjectID := rc.SubjectIDs[0]
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string            `json:"role"`
				Content []json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		toolResults := 0
		checkSeen := false
		for _, m := range body.Messages {
			for _, c := range m.Content {
				var blk struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}
				_ = json.Unmarshal(c, &blk)
				if blk.Type == "tool_result" {
					toolResults++
				}
				if strings.Contains(blk.Text, "[渲染校验结果]") {
					checkSeen = true
				}
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		switch toolResults {
		case 0:
			writeToolUse(w, "toolu_1", "update_page",
				`{"charts":[{"id":"c1","title":"t","width":"full","type":"interval","sql":"SELECT region, SUM(amount) AS total FROM dwd_orders GROUP BY region","subjects":[`+itoa(subjectID)+`],"spec":{"type":"interval","encode":{"x":"region","y":"total"}}}]}`)
		case 1:
			require.True(t, checkSeen, "update_page 成功后下一轮必须携带渲染校验结果")
			writeText(w, "已生成图表")
		default:
			writeText(w, "已生成图表")
		}
	}))
	defer srv.Close()

	cli := llm.NewClient(srv.URL, "k", "fake", 16384)
	runner := agent.NewRunner(cli, exec)
	finalText, err := runner.Run(context.Background(), agent.RunRequest{
		System:  "sys",
		History: []llm.Message{{Role: "user", Text: "画个图"}},
		Context: rc,
	}, func(ev agent.AgentEvent) {})
	require.NoError(t, err)
	require.Equal(t, "已生成图表", finalText)
}

func TestBuildSystemPrompt(t *testing.T) {
	s := agent.BuildSystemPrompt("T2S", "G2", `[{"id":1,"code":"sales"}]`)
	require.Contains(t, s, "T2S")
	require.Contains(t, s, "G2")
	require.Contains(t, s, "[[GBI_CONTEXT]]")
	require.Contains(t, s, `"code":"sales"`)
}

// TestBuildSystemPromptAntiNarration 提示词必须禁止"用文字描述将调用工具"的叙事
// (线上事故:模型反复说"我现在调用 update_page"而不发工具调用)。
func TestBuildSystemPromptAntiNarration(t *testing.T) {
	s := agent.BuildSystemPrompt("T2S", "G2", `[]`)
	require.Contains(t, s, "禁止用文字描述")
	require.Contains(t, s, "直接发出工具调用")
}

// TestBuildSystemPromptChartContract 系统提示必须显式说明 update_page 的图表配置契约
// (真实 LLM 冒烟事故:模型不知 subjects/spec 必填,生成不可渲染的配置)。
func TestBuildSystemPromptChartContract(t *testing.T) {
	s := agent.BuildSystemPrompt("T2S", "G2", `[{"id":1,"code":"sales"}]`)
	for _, kw := range []string{"update_page", "subjects", "spec", "width", "full", "1/2"} {
		require.Contains(t, s, kw, "系统提示应包含契约关键词 %s", kw)
	}
}
