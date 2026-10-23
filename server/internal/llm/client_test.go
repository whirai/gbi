package llm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"gbi/server/internal/llm"
)

// sseBody 返回一个最小合法的 Anthropic 风格 SSE 流。
// kind="text" → 文本块 + end_turn;kind="tool" → tool_use 块 + stop_reason=tool_use。
func sseBody(kind string) string {
	var b strings.Builder
	b.WriteString("event: message_start\n")
	b.WriteString(`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"fake","content":[],"stop_reason":null,"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n")
	if kind == "text" {
		b.WriteString("event: content_block_start\n")
		b.WriteString(`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n")
		b.WriteString("event: content_block_delta\n")
		b.WriteString(`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"已生成图表"}}` + "\n\n")
		b.WriteString("event: content_block_stop\n")
		b.WriteString(`data: {"type":"content_block_stop","index":0}` + "\n\n")
		b.WriteString("event: message_delta\n")
		b.WriteString(`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}` + "\n\n")
	} else {
		b.WriteString("event: content_block_start\n")
		b.WriteString(`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"execute_sql","input":{}}}` + "\n\n")
		b.WriteString("event: content_block_delta\n")
		b.WriteString(`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"sql\":\"SELECT 1\"}"}}` + "\n\n")
		b.WriteString("event: content_block_stop\n")
		b.WriteString(`data: {"type":"content_block_stop","index":0}` + "\n\n")
		b.WriteString("event: message_delta\n")
		b.WriteString(`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":1}}` + "\n\n")
	}
	b.WriteString("event: message_stop\n")
	b.WriteString(`data: {"type":"message_stop"}` + "\n\n")
	return b.String()
}

func newFakeServer(t *testing.T, kind string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/messages", r.URL.Path)
		require.NotEmpty(t, r.Header.Get("X-Api-Key"))
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody(kind))
	}))
}

func TestStreamMessageText(t *testing.T) {
	srv := newFakeServer(t, "text")
	defer srv.Close()

	c := llm.NewClient(srv.URL, "test-key", "fake-model", 16384)
	res, err := c.StreamMessage(context.Background(), llm.StreamRequest{
		System:   "sys",
		Messages: []llm.Message{{Role: "user", Text: "你好"}},
	})
	require.NoError(t, err)
	require.Equal(t, "已生成图表", res.Text)
	require.Equal(t, "end_turn", res.StopReason)
	require.Empty(t, res.ToolCalls)
}

func TestStreamMessageToolUse(t *testing.T) {
	srv := newFakeServer(t, "tool")
	defer srv.Close()

	c := llm.NewClient(srv.URL, "test-key", "fake-model", 16384)
	res, err := c.StreamMessage(context.Background(), llm.StreamRequest{
		System:   "sys",
		Messages: []llm.Message{{Role: "user", Text: "查一下"}},
		Tools:    []llm.Tool{{Name: "execute_sql", Description: "执行SQL", InputSchema: map[string]any{"type": "object"}}},
	})
	require.NoError(t, err)
	require.Equal(t, "tool_use", res.StopReason)
	require.Len(t, res.ToolCalls, 1)
	require.Equal(t, "execute_sql", res.ToolCalls[0].Name)
	require.Equal(t, "toolu_1", res.ToolCalls[0].ID)
	require.JSONEq(t, `{"sql":"SELECT 1"}`, string(res.ToolCalls[0].Input))
}

// TestStreamMessageEnablesThinking 思考是 flash 模型可靠产出工具调用的机制
// (禁思考后出现叙事循环事故)。请求必须携带 thinking:enabled + budget_tokens 8192。
func TestStreamMessageEnablesThinking(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody("text"))
	}))
	defer srv.Close()

	c := llm.NewClient(srv.URL, "test-key", "fake-model", 16384)
	_, err := c.StreamMessage(context.Background(), llm.StreamRequest{
		System:   "sys",
		Messages: []llm.Message{{Role: "user", Text: "hi"}},
	})
	require.NoError(t, err)

	var sent struct {
		Thinking struct {
			Type         string `json:"type"`
			BudgetTokens int64  `json:"budget_tokens"`
		} `json:"thinking"`
		MaxTokens int64 `json:"max_tokens"`
	}
	require.NoError(t, json.Unmarshal(gotBody, &sent))
	require.Equal(t, "enabled", sent.Thinking.Type)
	require.Equal(t, int64(8192), sent.Thinking.BudgetTokens)
	require.Equal(t, int64(16384), sent.MaxTokens)
}

// writeToolUseRaw 输出单个 tool_use 块,partial_json 为 rawInput 的 JSON 字符串形式
// (与真实线格式一致:片段是字符串,客户端拼接后再解析)。
func writeToolUseRaw(w http.ResponseWriter, id, name, rawInput string) {
	partial, _ := json.Marshal(rawInput)
	w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"fake\",\"content\":[],\"stop_reason\":null,\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"))
	w.Write([]byte("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"" + id + "\",\"name\":\"" + name + "\",\"input\":{}}}\n\n"))
	w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":" + string(partial) + "}}\n\n"))
	w.Write([]byte("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n"))
	w.Write([]byte("event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":1}}\n\n"))
	w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
}

// TestStreamMessageDegradesInvalidToolInput 模型偶发输出非法 JSON 的工具输入
// (长 spec 时更常见,线上 3 次会话崩溃事故)。客户端必须把非法输入降级为 {},
// 否则下一轮回放该工具调用时请求序列化直接报错终止。
func TestStreamMessageDegradesInvalidToolInput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeToolUseRaw(w, "toolu_9", "update_page", `{"charts":[{"id":"c1"}`)
	}))
	defer srv.Close()

	c := llm.NewClient(srv.URL, "test-key", "fake-model", 16384)
	res, err := c.StreamMessage(context.Background(), llm.StreamRequest{
		System:   "sys",
		Messages: []llm.Message{{Role: "user", Text: "更新页面"}},
	})
	require.NoError(t, err)
	require.Len(t, res.ToolCalls, 1)
	require.Equal(t, "update_page", res.ToolCalls[0].Name)
	require.JSONEq(t, `{}`, string(res.ToolCalls[0].Input), "非法输入应降级为空对象")
}

// TestReplayInvalidToolInputDoesNotCrash 回放携带非法输入的工具调用时,
// 请求必须能正常序列化发出(防御:任何构造路径都不能让 MarshalJSON 炸掉)。
func TestReplayInvalidToolInputDoesNotCrash(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody("text"))
	}))
	defer srv.Close()

	c := llm.NewClient(srv.URL, "test-key", "fake-model", 16384)
	_, err := c.StreamMessage(context.Background(), llm.StreamRequest{
		System: "sys",
		Messages: []llm.Message{
			{Role: "user", Text: "画图"},
			{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "toolu_1", Name: "update_page", Input: json.RawMessage(`{bad`)}}},
		},
	})
	require.NoError(t, err, "非法工具输入的回放不得导致序列化错误")

	var sent struct {
		Messages []struct {
			Role    string            `json:"role"`
			Content []json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(gotBody, &sent), "发往端点的请求体必须是合法 JSON")
	var replayed struct {
		Type  string          `json:"type"`
		Input json.RawMessage `json:"input"`
	}
	for _, m := range sent.Messages {
		for _, c := range m.Content {
			if json.Unmarshal(c, &replayed) == nil && replayed.Type == "tool_use" {
				require.JSONEq(t, `{}`, string(replayed.Input), "回放的非法输入应降级为空对象")
			}
		}
	}
}

// TestStreamMessageForwardsToolSchema 校验工具 input_schema 的 properties 与 required
// 都原样发到 LLM 端点(required 缺失会导致真实模型不知道必填字段,冒烟事故根因之一)。
func TestStreamMessageForwardsToolSchema(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody("text"))
	}))
	defer srv.Close()

	c := llm.NewClient(srv.URL, "test-key", "fake-model", 16384)
	_, err := c.StreamMessage(context.Background(), llm.StreamRequest{
		System:   "sys",
		Messages: []llm.Message{{Role: "user", Text: "hi"}},
		Tools: []llm.Tool{{
			Name:        "execute_sql",
			Description: "执行SQL",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"sql": map[string]any{"type": "string"}},
				"required":   []string{"sql"},
			},
		}},
	})
	require.NoError(t, err)

	var sent struct {
		Tools []struct {
			Name        string `json:"name"`
			InputSchema struct {
				Properties map[string]any `json:"properties"`
				Required   []string       `json:"required"`
			} `json:"input_schema"`
		} `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(gotBody, &sent))
	require.Len(t, sent.Tools, 1)
	require.Equal(t, "execute_sql", sent.Tools[0].Name)
	require.Contains(t, sent.Tools[0].InputSchema.Properties, "sql")
	require.Equal(t, []string{"sql"}, sent.Tools[0].InputSchema.Required)
}
