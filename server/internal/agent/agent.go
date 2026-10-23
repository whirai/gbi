package agent

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"gbi/server/internal/llm"
	"gbi/server/internal/logger"
)

// AgentEvent 推送给前端的事件。
type AgentEvent struct {
	Type string // message_delta | tool_call | config_updated | done | error
	Data string // JSON 字符串
}

type RunRequest struct {
	System  string
	History []llm.Message // 历史 + 本轮 user 消息
	Context RunContext
}

type Runner struct {
	llm  *llm.Client
	exec *ToolExecutor
}

func NewRunner(c *llm.Client, exec *ToolExecutor) *Runner {
	start := time.Now()
	slog.Debug("方法进入", "fn", "agent.NewRunner")
	defer func() {
		slog.Debug("方法离开", "fn", "agent.NewRunner", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &Runner{llm: c, exec: exec}
}

const maxToolRounds = 12

var errMaxRounds = errors.New("工具轮次超出上限")

// Run 工具循环:调用 LLM → 若有 tool_use 则执行工具并回填 tool_result → 直到 end_turn。
// onEvent 按请求注入(SSE 转发)。返回终态文本(供调用方持久化 assistant 消息);
// 不写入任何共享状态,Runner 可被并发请求复用。
func (r *Runner) Run(ctx context.Context, req RunRequest, onEvent func(AgentEvent)) (text string, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "Runner.Run", "page_id", req.Context.PageID, "msgs", len(req.History))
	defer func() {
		slog.Debug("方法离开", "fn", "Runner.Run", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	msgs := append([]llm.Message{}, req.History...)
	for round := 0; round < maxToolRounds; round++ {
		slog.Debug("Agent轮次开始", "round", round+1, "msgs", len(msgs), "msg_chars", totalMsgChars(msgs))
		res, err := r.llm.StreamMessageWithText(ctx, llm.StreamRequest{
			System:   req.System,
			Messages: msgs,
			Tools:    ToolDefinitions(),
		}, func(delta string) {
			b, _ := json.Marshal(map[string]string{"text": delta})
			onEvent(AgentEvent{Type: "message_delta", Data: string(b)})
		})
		if err != nil {
			return "", err
		}
		slog.Debug("Agent轮次收到", "round", round+1, "stop_reason", res.StopReason, "tool_calls", len(res.ToolCalls), "text_len", len([]rune(res.Text)))

		if len(res.ToolCalls) == 0 {
			// 思考耗尽输出预算时 stop=max_tokens 且无任何可见输出;
			// 回填"继续"消息让模型续写,而不是静默结束(线上"不回复"事故)。
			if res.StopReason == "max_tokens" && round+1 < maxToolRounds {
				slog.Debug("空轮次续跑", "round", round+1, "text_len", len([]rune(res.Text)))
				if res.Text != "" {
					msgs = append(msgs, llm.Message{Role: "assistant", Text: res.Text})
				}
				msgs = append(msgs, llm.Message{Role: "user", Text: "请继续完成上面的回复"})
				continue
			}
			// 终态:无工具调用
			onEvent(AgentEvent{Type: "done", Data: "{}"})
			return res.Text, nil
		}

		// 回填 assistant(含 tool_use)+ user(tool_result),进入下一轮
		msgs = append(msgs, llm.Message{Role: "assistant", Text: res.Text, ToolCalls: res.ToolCalls})
		results := make([]llm.ToolResult, 0, len(res.ToolCalls))
		didUpdate := false
		for _, tc := range res.ToolCalls {
			evData, _ := json.Marshal(map[string]any{"name": tc.Name, "input": json.RawMessage(tc.Input)})
			onEvent(AgentEvent{Type: "tool_call", Data: string(evData)})
			slog.Debug("工具调用", "tool", tc.Name, "input", string(tc.Input))

			out, isErr := r.exec.Execute(ctx, tc.Name, tc.Input, req.Context)
			slog.Debug("工具结果", "tool", tc.Name, "is_error", isErr, "output_len", len(out), "output_preview", logger.Truncate(out, 300))
			results = append(results, llm.ToolResult{ToolUseID: tc.ID, Content: out, IsError: isErr})

			if tc.Name == "update_page" && !isErr {
				didUpdate = true
				evData, _ := json.Marshal(map[string]any{"page_id": req.Context.PageID})
				onEvent(AgentEvent{Type: "config_updated", Data: string(evData)})
				slog.Info("页面配置已更新", "page_id", req.Context.PageID)
			}
		}
		msgs = append(msgs, llm.Message{Role: "user", ToolResults: results})

		if didUpdate {
			// 自动核验:update_page 成功后强制查询渲染结果并注入下一轮,
			// 让模型看到真实渲染错误(不依赖模型自觉调用 check_page_render)。
			out, _ := r.exec.CheckRender(ctx, req.Context)
			slog.Debug("渲染核验注入", "page_id", req.Context.PageID, "check_len", len([]rune(out)), "check_preview", logger.Truncate(out, 300))
			msgs = append(msgs, llm.Message{Role: "user", Text: out})
		}
	}
	slog.Warn("工具轮次超出上限", "rounds", maxToolRounds, "page_id", req.Context.PageID)
	return "", errMaxRounds
}

// totalMsgChars 统计消息文本/工具输入/工具结果的字符数(rune),用于日志观察上下文规模。
func totalMsgChars(msgs []llm.Message) int {
	start := time.Now()
	slog.Debug("方法进入", "fn", "agent.totalMsgChars", "msgs", len(msgs))
	defer func() {
		slog.Debug("方法离开", "fn", "agent.totalMsgChars", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	n := 0
	for _, m := range msgs {
		n += len([]rune(m.Text))
		for _, tc := range m.ToolCalls {
			n += len(tc.Input)
		}
		for _, tr := range m.ToolResults {
			n += len([]rune(tr.Content))
		}
	}
	return n
}
