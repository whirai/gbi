package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gbi/server/internal/llm"
)

// estTokens 粗略 token 估算(中文为主:字符数×0.6,保守)。
func estTokens(msg llm.Message) int {
	start := time.Now()
	slog.Debug("方法进入", "fn", "agent.estTokens")
	defer func() {
		slog.Debug("方法离开", "fn", "agent.estTokens", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return len([]rune(msg.Text)) * 6 / 10
}

// BuildHistory 按预算装配历史:最近 keep 条原文;更早部分以摘要消息代表。
// summary 为空则旧消息直接丢弃。预算超限时从最旧开始裁剪(摘要消息优先保留)。
func BuildHistory(msgs []llm.Message, summary string, keep, budgetTokens int) []llm.Message {
	start := time.Now()
	slog.Debug("方法进入", "fn", "agent.BuildHistory", "msgs", len(msgs), "keep", keep, "budget_tokens", budgetTokens)
	defer func() {
		slog.Debug("方法离开", "fn", "agent.BuildHistory", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	recent := msgs
	if len(msgs) > keep {
		recent = msgs[len(msgs)-keep:]
	}
	hasOlder := len(msgs) > len(recent)
	var out []llm.Message
	if hasOlder && summary != "" {
		out = append(out, llm.Message{Role: "user", Text: "[对话早期要点]\n" + summary})
	}
	out = append(out, recent...)
	// 预算裁剪:从最旧(摘要之后的原文)开始丢
	total := 0
	for _, m := range out {
		total += estTokens(m)
	}
	for total > budgetTokens && len(out) > 1 {
		idx := 0
		if hasOlder && summary != "" && len(out) > 0 && out[0].Text != "" && out[0].Role == "user" && strings.HasPrefix(out[0].Text, "[对话早期要点]") {
			idx = 1 // 保留摘要,丢最旧原文
			if len(out) <= 2 {
				break
			}
		}
		total -= estTokens(out[idx])
		out = append(out[:idx], out[idx+1:]...)
	}
	return out
}

// Summarizer 生成"对话早期要点"(无工具、短输出)。
type Summarizer struct {
	cli *llm.Client
}

func NewSummarizer(cli *llm.Client) *Summarizer {
	start := time.Now()
	slog.Debug("方法进入", "fn", "agent.NewSummarizer")
	defer func() {
		slog.Debug("方法离开", "fn", "agent.NewSummarizer", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &Summarizer{cli: cli}
}

// Summarize 在调用方请求上下文内生成摘要:客户端断开可取消,避免卡住请求 goroutine。
func (s *Summarizer) Summarize(ctx context.Context, msgs []llm.Message) (out string, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "Summarizer.Summarize", "msgs", len(msgs))
	defer func() {
		slog.Debug("方法离开", "fn", "Summarizer.Summarize", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	prompt := "请用中文在200字内概括以下对话的早期要点(用户的目标、已确认的数据与图表要求、进行中的任务):\n\n"
	for _, m := range msgs {
		prompt += fmt.Sprintf("[%s] %s\n", m.Role, m.Text)
	}
	res, err := s.cli.StreamMessage(ctx, llm.StreamRequest{
		System:   "你是对话摘要助手,只输出要点本身,不要客套。",
		Messages: []llm.Message{{Role: "user", Text: prompt}},
	})
	if err != nil {
		return "", err
	}
	return res.Text, nil
}
