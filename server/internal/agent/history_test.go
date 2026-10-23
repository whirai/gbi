package agent_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gbi/server/internal/agent"
	"gbi/server/internal/llm"
)

func msgs(n int) []llm.Message {
	out := make([]llm.Message, n)
	for i := range out {
		out[i] = llm.Message{Role: "user", Text: "消息内容消息内容消息内容消息内容消息内容"}
	}
	return out
}

func TestBuildHistoryKeepsRecent(t *testing.T) {
	// 24 条以内:不截断、不插摘要
	in := msgs(10)
	out := agent.BuildHistory(in, "", 24, 90000)
	require.Len(t, out, 10)
}

func TestBuildHistoryTruncatesAndPrependsSummary(t *testing.T) {
	in := msgs(40)
	out := agent.BuildHistory(in, "早期要点:用户要求三张占比图", 24, 90000)
	require.Len(t, out, 25) // 1 摘要 + 24 原文
	require.Contains(t, out[0].Text, "早期要点")
	require.Contains(t, out[0].Text, "用户要求三张占比图")
	require.Equal(t, in[16].Text, out[1].Text) // 最早保留的是第 17 条
}

func TestBuildHistoryTokenBudgetTrimsOldest(t *testing.T) {
	in := msgs(24)
	// 预算极小:只容 2 条
	out := agent.BuildHistory(in, "", 24, 10)
	require.LessOrEqual(t, len(out), 2)
	require.Greater(t, len(out), 0)
	// 保留的一定是最新的
	require.Equal(t, in[len(in)-1].Text, out[len(out)-1].Text)
}

func TestSummarizerFailureDegrades(t *testing.T) {
	// 摘要依赖 LLM;失败时调用方(handler)降级为 summary="",BuildHistory 正常出最近 24 条
	out := agent.BuildHistory(msgs(40), "", 24, 90000)
	require.Len(t, out, 24)
}
