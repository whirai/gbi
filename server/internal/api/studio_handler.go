package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"gbi/server/internal/agent"
	"gbi/server/internal/auth"
	"gbi/server/internal/llm"
	"gbi/server/internal/logger"
	"gbi/server/internal/model"
	"gbi/server/internal/service"
)

type StudioHandler struct {
	studio      *service.StudioService
	pages       *service.PageService
	exec        *agent.ToolExecutor // 用于生成数据集上下文(list_datasets 输出)
	skillsT2S   string
	skillsG2    string
	llmCfg      *service.LLMConfigService // 每请求解析当前模型(切换即时生效)
	historyKeep int                       // GBI_HISTORY_KEEP(≤0 时 Chat 内默认 24)
}

func NewStudioHandler(studio *service.StudioService, pages *service.PageService, exec *agent.ToolExecutor, skillsT2S, skillsG2 string, llmCfg *service.LLMConfigService, historyKeep int) *StudioHandler {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.NewStudioHandler")
	defer func() {
		slog.Debug("方法离开", "fn", "api.NewStudioHandler", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &StudioHandler{studio: studio, pages: pages, exec: exec, skillsT2S: skillsT2S, skillsG2: skillsG2, llmCfg: llmCfg, historyKeep: historyKeep}
}

// Chat SSE:POST /api/v1/studio/chat {page_id, subject_ids, message}
func (h *StudioHandler) Chat(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "StudioHandler.Chat")
	defer func() {
		slog.Debug("方法离开", "fn", "StudioHandler.Chat", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if !requireManager(c) {
		Fail(c, http.StatusForbidden, 403, "无权限")
		return
	}
	var req struct {
		PageID     uint   `json:"page_id"`
		SubjectIDs []uint `json:"subject_ids"`
		Message    string `json:"message"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.PageID == 0 || req.Message == "" || len(req.SubjectIDs) == 0 {
		Fail(c, http.StatusBadRequest, 400, "page_id/subject_ids/message 必填")
		return
	}
	if _, err := h.pages.Get(req.PageID); err != nil {
		Fail(c, http.StatusNotFound, 404, err.Error())
		return
	}
	// 当前模型每请求解析:控制台切换即时生效;无模型/无 key 时明确报错
	mdl, prov, cerr := h.llmCfg.Current()
	if cerr != nil {
		if errors.Is(cerr, service.ErrNoCurrentModel) {
			Fail(c, http.StatusBadRequest, 400, "请先在控制台「大模型设置」配置当前模型")
		} else {
			slog.Warn("当前模型解析失败", "err", cerr)
			Fail(c, http.StatusInternalServerError, 500, "当前模型解析失败")
		}
		return
	}
	if prov.APIKey == "" {
		Fail(c, http.StatusBadRequest, 400, "当前模型未配置 API Key,请在控制台「大模型设置」配置")
		return
	}
	cli := llm.NewClient(prov.BaseURL, prov.APIKey, mdl.Name, int64(mdl.MaxTokens))
	runner := agent.NewRunner(cli, h.exec)
	summarizer := agent.NewSummarizer(cli)
	slog.Debug("当前模型解析", "provider", prov.Name, "model", mdl.Name, "context_window", mdl.ContextWindow, "max_tokens", mdl.MaxTokens, "has_key", true)
	uid := c.GetUint(auth.CtxUserID)
	started := time.Now()
	slog.Debug("对话开始", "page_id", req.PageID, "uid", uid, "subjects", req.SubjectIDs,
		"msg_len", len([]rune(req.Message)), "msg_preview", logger.Truncate(req.Message, 80))

	conv, err := h.studio.GetOrCreateConversation(req.PageID, uid)
	if err != nil {
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	if err := h.studio.AppendMessage(conv.ID, "user", req.Message); err != nil {
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}

	// 历史装配:最近 keep 条原文 + 更早滚动摘要(超出部分才摘要)。
	msgs, err := h.studio.Messages(conv.ID)
	if err != nil {
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	keep := h.historyKeep
	if keep <= 0 {
		keep = 24
	}
	summary := ""
	if len(msgs) > keep && summarizer != nil {
		older := msgs[:len(msgs)-keep]
		if conv.Summary == "" || conv.SummaryUptoID < older[len(older)-1].ID {
			s, err := summarizer.Summarize(c.Request.Context(), toLLMMessages(older))
			if err == nil {
				summary = s
				_ = h.studio.UpdateSummary(conv.ID, summary, older[len(older)-1].ID)
				slog.Debug("历史摘要完成", "summary_len", len([]rune(s)), "upto_id", older[len(older)-1].ID)
			} else {
				slog.Warn("历史摘要生成失败,丢弃旧消息", "err", err)
			}
		} else {
			summary = conv.Summary
		}
	}
	budget := service.BudgetTokens(mdl)
	history := agent.BuildHistory(toLLMMessages(msgs), summary, keep, budget)
	slog.Debug("历史装配", "total_msgs", len(msgs), "kept", keep, "summary_len", len([]rune(summary)), "used", len(history), "budget", budget)

	// SSE 头
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	// 显式 close:done 事件后由服务端确定性收尾流。keep-alive 经 vite 代理转发时
	// 会与 chunked 尾块产生竞态,客户端偶发 net::ERR_INCOMPLETE_CHUNKED_ENCODING。
	c.Writer.Header().Set("Connection", "close")
	flusher := c.Writer.(http.Flusher)
	send := func(event, data string) {
		fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, data)
		flusher.Flush()
	}

	// 组装 system prompt(含数据集上下文):复用 ToolExecutor 的 list_datasets 输出
	runCtx := agent.RunContext{PageID: req.PageID, SubjectIDs: req.SubjectIDs, UserID: uid}
	subjectsJSON, isErr := h.exec.Execute(c.Request.Context(), "list_datasets", json.RawMessage(`{}`), runCtx)
	if isErr {
		send("error", mustJSON(map[string]string{"message": subjectsJSON}))
		send("done", "{}")
		return
	}
	system := agent.BuildSystemPrompt(h.skillsT2S, h.skillsG2, subjectsJSON)
	slog.Debug("系统提示组装", "prompt_len", len([]rune(system)), "t2s_len", len([]rune(h.skillsT2S)),
		"g2_len", len([]rune(h.skillsG2)), "subjects_len", len(subjectsJSON))

	onEvent := func(ev agent.AgentEvent) {
		send(ev.Type, ev.Data)
	}

	// 终态文本由 Run 返回值携带(不再经共享 Runner 字段,避免并发请求写-写竞态)。
	finalText, runErr := runner.Run(c.Request.Context(), agent.RunRequest{
		System:  system,
		History: history,
		Context: runCtx,
	}, onEvent)

	if runErr != nil {
		slog.Warn("对话失败", "page_id", req.PageID, "elapsed_ms", time.Since(started).Milliseconds(), "err", runErr)
		send("error", mustJSON(map[string]string{"message": runErr.Error()}))
	}
	if finalText != "" {
		_ = h.studio.AppendMessage(conv.ID, "assistant", finalText)
	}
	slog.Debug("对话结束", "page_id", req.PageID, "final_text_len", len([]rune(finalText)),
		"elapsed_ms", time.Since(started).Milliseconds())
	send("done", "{}")
}

// Conversation GET /api/v1/studio/conversations/:page_id
func (h *StudioHandler) Conversation(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "StudioHandler.Conversation")
	defer func() {
		slog.Debug("方法离开", "fn", "StudioHandler.Conversation", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if !requireManager(c) {
		Fail(c, http.StatusForbidden, 403, "无权限")
		return
	}
	pageID, ok := parseParamID(c, "page_id")
	if !ok {
		return
	}
	// 页面存在性前置校验:GetOrCreateConversation 会对任意 page_id 造行,
	// 不校验可直接制造悬挂会话(与 Delete 同一防线)。
	if _, err := h.pages.Get(pageID); err != nil {
		Fail(c, http.StatusNotFound, 404, "页面不存在")
		return
	}
	uid := c.GetUint(auth.CtxUserID)
	conv, err := h.studio.GetOrCreateConversation(pageID, uid)
	if err != nil {
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	msgs, err := h.studio.Messages(conv.ID)
	if err != nil {
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, gin.H{"conversation": conv, "messages": msgs})
}

// Delete DELETE /api/v1/studio/conversations/:page_id 清空会话(新对话)。
func (h *StudioHandler) Delete(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "StudioHandler.Delete")
	defer func() {
		slog.Debug("方法离开", "fn", "StudioHandler.Delete", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if !requireManager(c) {
		Fail(c, http.StatusForbidden, 403, "无权限")
		return
	}
	pageID, ok := parseParamID(c, "page_id")
	if !ok {
		return
	}
	if _, err := h.pages.Get(pageID); err != nil {
		Fail(c, http.StatusNotFound, 404, "页面不存在")
		return
	}
	if err := h.studio.DeleteConversation(pageID); err != nil {
		Fail(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	OK(c, gin.H{"deleted": true})
}

func mustJSON(v any) string {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.mustJSON")
	defer func() {
		slog.Debug("方法离开", "fn", "api.mustJSON", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	b, _ := json.Marshal(v)
	return string(b)
}

// toLLMMessages 把会话消息转为 LLM 文本消息(user/assistant 文本)。
func toLLMMessages(msgs []model.Message) []llm.Message {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.toLLMMessages", "n", len(msgs))
	defer func() {
		slog.Debug("方法离开", "fn", "api.toLLMMessages", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	out := make([]llm.Message, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, llm.Message{Role: m.Role, Text: m.Content})
	}
	return out
}
