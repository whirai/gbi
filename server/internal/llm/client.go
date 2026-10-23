// Package llm 封装 Anthropic SDK 的流式 Messages 调用。
// 内部 Message/Tool 表示与 SDK 解耦,便于替换兼容端点(如 DeepSeek Anthropic 兼容接口)。
package llm

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
)

// ThinkingBudgetTokens 思考预算(thinking.budget_tokens):max_tokens 不得低于
// 此值(budget>max 在原生 Anthropic 端点会 400)。单一定义源——config 的
// GBI_LLM_MAX_TOKENS 钳制下限引用此常量,调整预算只需改这里。
const ThinkingBudgetTokens = 8192

// Message 一轮对话消息(内部表示,与 SDK 解耦)。
type Message struct {
	Role        string // user | assistant
	Text        string
	ToolCalls   []ToolCall   // assistant 消息携带(重放历史时用)
	ToolResults []ToolResult // user 消息携带(工具结果重放)
}

type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

type ToolResult struct {
	ToolUseID string
	Content   string
	IsError   bool
}

type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type StreamRequest struct {
	System   string
	Messages []Message
	Tools    []Tool
}

type TurnResult struct {
	Text       string
	ToolCalls  []ToolCall
	StopReason string
}

type Client struct {
	inner     anthropic.Client // SDK v1.x NewClient 返回值类型
	model     string
	maxTokens int64
}

func NewClient(baseURL, apiKey, model string, maxTokens int64) *Client {
	start := time.Now()
	// 安全红线:apiKey 绝不入日志(只记 model/max_tokens/base_url)。
	slog.Debug("方法进入", "fn", "llm.NewClient", "model", model, "max_tokens", maxTokens, "base_url", baseURL)
	defer func() {
		slog.Debug("方法离开", "fn", "llm.NewClient", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &Client{
		inner: anthropic.NewClient(
			option.WithAPIKey(apiKey),
			option.WithBaseURL(baseURL),
			// SDK 的 NewClient 会自动读取环境变量 ANTHROPIC_AUTH_TOKEN 并额外附加
			// Authorization: Bearer 头;当运行环境(如 Claude Code)设置了该变量时,
			// 目标端点可能优先使用这个无关的 Authorization 头导致 401。
			// 删除 Authorization 头,确保只发送 X-Api-Key(即 GBI_LLM_API_KEY)。
			option.WithHeaderDel("Authorization"),
		),
		model:     model,
		maxTokens: maxTokens,
	}
}

// toSDKMessages 把内部消息转为 SDK 参数。
func toSDKMessages(msgs []Message) []anthropic.MessageParam {
	start := time.Now()
	slog.Debug("方法进入", "fn", "llm.toSDKMessages", "msgs", len(msgs))
	defer func() {
		slog.Debug("方法离开", "fn", "llm.toSDKMessages", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	out := make([]anthropic.MessageParam, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case "user":
			blocks := []anthropic.ContentBlockParamUnion{}
			if m.Text != "" {
				blocks = append(blocks, anthropic.NewTextBlock(m.Text))
			}
			for _, tr := range m.ToolResults {
				blocks = append(blocks, anthropic.NewToolResultBlock(tr.ToolUseID, tr.Content, tr.IsError))
			}
			out = append(out, anthropic.NewUserMessage(blocks...))
		case "assistant":
			blocks := []anthropic.ContentBlockParamUnion{}
			if m.Text != "" {
				blocks = append(blocks, anthropic.NewTextBlock(m.Text))
			}
			for _, tc := range m.ToolCalls {
				input := tc.Input
				if !json.Valid(input) {
					// 防御:任何构造路径(含未来新来源)都不能让回放炸掉序列化
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, anthropic.NewToolUseBlock(tc.ID, input, tc.Name))
			}
			out = append(out, anthropic.NewAssistantMessage(blocks...))
		}
	}
	return out
}

// toSDKTools 把内部工具定义转为 SDK 参数。
func toSDKTools(tools []Tool) []anthropic.ToolUnionParam {
	start := time.Now()
	slog.Debug("方法进入", "fn", "llm.toSDKTools", "tools", len(tools))
	defer func() {
		slog.Debug("方法离开", "fn", "llm.toSDKTools", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	out := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		out = append(out, anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{
			Name:        t.Name,
			Description: anthropic.String(t.Description),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: t.InputSchema["properties"],
				// required 必须随 schema 下发,否则模型不知道哪些字段必填
				Required: schemaRequired(t.InputSchema["required"]),
			},
		}})
	}
	return out
}

// schemaRequired 兼容 []string 与 []any 两种写法。
func schemaRequired(v any) []string {
	start := time.Now()
	slog.Debug("方法进入", "fn", "llm.schemaRequired")
	defer func() {
		slog.Debug("方法离开", "fn", "llm.schemaRequired", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	switch r := v.(type) {
	case []string:
		return r
	case []any:
		out := make([]string, 0, len(r))
		for _, item := range r {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// StreamMessage 单轮流式调用:收集文本增量与 tool_use 块,返回本轮结果。
func (c *Client) StreamMessage(ctx context.Context, req StreamRequest) (res *TurnResult, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "Client.StreamMessage", "msgs", len(req.Messages), "tools", len(req.Tools))
	defer func() {
		slog.Debug("方法离开", "fn", "Client.StreamMessage", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	return c.StreamMessageWithText(ctx, req, nil)
}

// StreamMessageWithText 同 StreamMessage;onText 可选,逐段回传文本增量(用于 SSE 转发)。
func (c *Client) StreamMessageWithText(ctx context.Context, req StreamRequest, onText func(string)) (res *TurnResult, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "Client.StreamMessageWithText", "msgs", len(req.Messages), "tools", len(req.Tools))
	defer func() {
		slog.Debug("方法离开", "fn", "Client.StreamMessageWithText", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: c.maxTokens,
		System: []anthropic.TextBlockParam{
			{Text: req.System},
		},
		Messages: toSDKMessages(req.Messages),
		// 思考是 flash 模型可靠产出工具调用的机制(禁思考曾致叙事循环事故)。
		// DeepSeek 兼容层忽略 budget_tokens 实际值,但 SDK 必填;预算上调防止
		// 思考耗尽输出(既有 agent 层 max_tokens 续跑兜底保留)。
		Thinking: anthropic.ThinkingConfigParamUnion{OfEnabled: &anthropic.ThinkingConfigEnabledParam{
			BudgetTokens: ThinkingBudgetTokens,
			Type:         constant.Enabled("enabled"),
		}},
	}
	if len(req.Tools) > 0 {
		params.Tools = toSDKTools(req.Tools)
	}

	slog.Debug("LLM调用开始",
		"model", c.model,
		"msgs", len(req.Messages),
		"tools", len(req.Tools),
		"system_len", len([]rune(req.System)),
		"max_tokens", c.maxTokens,
		"budget_tokens", ThinkingBudgetTokens)

	stream := c.inner.Messages.NewStreaming(ctx, params)
	defer stream.Close()

	res = &TurnResult{}
	// 每个内容块的状态(tool_use 需要跨 delta 拼接 input JSON)
	type blockState struct {
		toolID   string
		toolName string
		inputBuf string
	}
	blocks := map[int64]*blockState{}

	for stream.Next() {
		event := stream.Current()

		// v1.x SDK 的 MessageStreamEventUnion 为扁平联合体:
		// Type 为判别字段,ContentBlock/Delta/Index 等按事件类型填充。
		switch event.Type {
		case "content_block_start":
			if event.ContentBlock.Type == "tool_use" {
				blocks[event.Index] = &blockState{toolID: event.ContentBlock.ID, toolName: event.ContentBlock.Name}
			}
		case "content_block_delta":
			switch event.Delta.Type {
			case "text_delta":
				res.Text += event.Delta.Text
				if onText != nil {
					onText(event.Delta.Text)
				}
			case "input_json_delta":
				if st, ok := blocks[event.Index]; ok {
					st.inputBuf += event.Delta.PartialJSON
				}
			}
		case "content_block_stop":
			if st, ok := blocks[event.Index]; ok {
				input := json.RawMessage(st.inputBuf)
				if !json.Valid(input) {
					// 模型偶发输出非法 JSON 的工具输入(长 spec 时更常见);
					// 原样回放会在下一轮请求序列化时炸掉整个会话(线上事故)。
					// 降级为空对象:工具执行报 schema 错误,模型可据此重试。
					slog.Warn("工具输入JSON无效", "tool", st.toolName, "input_len", len(st.inputBuf))
					input = json.RawMessage("{}")
				}
				res.ToolCalls = append(res.ToolCalls, ToolCall{
					ID:    st.toolID,
					Name:  st.toolName,
					Input: input,
				})
				delete(blocks, event.Index)
			}
		case "message_delta":
			res.StopReason = string(event.Delta.StopReason)
		}
	}
	if err := stream.Err(); err != nil {
		slog.Warn("LLM调用失败", "model", c.model, "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
		return nil, err
	}
	slog.Debug("LLM轮次",
		"model", c.model,
		"msgs", len(req.Messages),
		"tools", len(req.Tools),
		"text_len", len(res.Text),
		"tool_calls", len(res.ToolCalls),
		"stop_reason", res.StopReason,
		"elapsed_ms", time.Since(start).Milliseconds(),
		"text_preview", truncate(res.Text, 200))
	return res, nil
}

// truncate 截断日志预览(与 logger.Truncate 同语义)。
// 本包不能 import logger:logger→config→llm 会形成 import 环,故本地实现。
func truncate(s string, n int) string {
	start := time.Now()
	// 只记长度不记内容:本函数处理的正是"需要截断的文本"。
	slog.Debug("方法进入", "fn", "llm.truncate", "s_len", len(s), "n", n)
	defer func() {
		slog.Debug("方法离开", "fn", "llm.truncate", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…(截断)"
}
