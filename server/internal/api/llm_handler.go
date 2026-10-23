package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"gbi/server/internal/service"
)

type LLMHandler struct {
	cfg *service.LLMConfigService
}

func NewLLMHandler(cfg *service.LLMConfigService) *LLMHandler {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.NewLLMHandler")
	defer func() {
		slog.Debug("方法离开", "fn", "api.NewLLMHandler", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &LLMHandler{cfg: cfg}
}

// bindOK 解析 JSON;失败已响应 400 并返回 false。
func bindOK(c *gin.Context, out any) (ok bool) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.bindOK")
	defer func() {
		slog.Debug("方法离开", "fn", "api.bindOK", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if err := c.ShouldBindJSON(out); err != nil {
		Fail(c, http.StatusBadRequest, 400, "参数格式错误")
		return false
	}
	return true
}

// failLLM 统一错误映射;返回 true 表示已响应。
func failLLM(c *gin.Context, err error) (responded bool) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.failLLM")
	defer func() {
		slog.Debug("方法离开", "fn", "api.failLLM", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	switch {
	case err == nil:
		return false
	case errors.Is(err, service.ErrLLMProviderNotFound), errors.Is(err, service.ErrLLMModelNotFound):
		Fail(c, http.StatusNotFound, 404, err.Error())
	case errors.Is(err, service.ErrProviderHasModels), errors.Is(err, service.ErrCurrentModel),
		errors.Is(err, service.ErrLLMNameRequired), errors.Is(err, service.ErrLLMBaseURLInvalid),
		errors.Is(err, service.ErrLLMModelNameRequired), errors.Is(err, service.ErrLLMContextInvalid),
		errors.Is(err, service.ErrLLMMaxTokensInvalid), errors.Is(err, service.ErrLLMDuplicateProviderName),
		errors.Is(err, service.ErrLLMDuplicateModelName):
		Fail(c, http.StatusBadRequest, 400, err.Error())
	default:
		Fail(c, http.StatusInternalServerError, 500, err.Error())
	}
	return true
}

func (h *LLMHandler) ListProviders(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "LLMHandler.ListProviders")
	defer func() {
		slog.Debug("方法离开", "fn", "LLMHandler.ListProviders", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	list, err := h.cfg.ListProviders()
	if failLLM(c, err) {
		return
	}
	OK(c, list)
}

func (h *LLMHandler) CreateProvider(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "LLMHandler.CreateProvider")
	defer func() {
		slog.Debug("方法离开", "fn", "LLMHandler.CreateProvider", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	var in service.ProviderInput
	if !bindOK(c, &in) {
		return
	}
	res, err := h.cfg.CreateProvider(in)
	if failLLM(c, err) {
		return
	}
	OK(c, res)
}

func (h *LLMHandler) UpdateProvider(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "LLMHandler.UpdateProvider")
	defer func() {
		slog.Debug("方法离开", "fn", "LLMHandler.UpdateProvider", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	id, ok := parseParamID(c, "id")
	if !ok {
		return
	}
	var in service.ProviderUpdate
	if !bindOK(c, &in) {
		return
	}
	res, err := h.cfg.UpdateProvider(id, in)
	if failLLM(c, err) {
		return
	}
	OK(c, res)
}

func (h *LLMHandler) DeleteProvider(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "LLMHandler.DeleteProvider")
	defer func() {
		slog.Debug("方法离开", "fn", "LLMHandler.DeleteProvider", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	id, ok := parseParamID(c, "id")
	if !ok {
		return
	}
	if failLLM(c, h.cfg.DeleteProvider(id)) {
		return
	}
	OK(c, nil)
}

func (h *LLMHandler) ListModels(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "LLMHandler.ListModels")
	defer func() {
		slog.Debug("方法离开", "fn", "LLMHandler.ListModels", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	id, ok := parseParamID(c, "id")
	if !ok {
		return
	}
	res, err := h.cfg.ListModels(id)
	if failLLM(c, err) {
		return
	}
	OK(c, res)
}

func (h *LLMHandler) CreateModel(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "LLMHandler.CreateModel")
	defer func() {
		slog.Debug("方法离开", "fn", "LLMHandler.CreateModel", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	id, ok := parseParamID(c, "id")
	if !ok {
		return
	}
	var in service.ModelInput
	if !bindOK(c, &in) {
		return
	}
	res, err := h.cfg.CreateModel(id, in)
	if failLLM(c, err) {
		return
	}
	OK(c, res)
}

func (h *LLMHandler) UpdateModel(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "LLMHandler.UpdateModel")
	defer func() {
		slog.Debug("方法离开", "fn", "LLMHandler.UpdateModel", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	id, ok := parseParamID(c, "id")
	if !ok {
		return
	}
	var in service.ModelUpdate
	if !bindOK(c, &in) {
		return
	}
	res, err := h.cfg.UpdateModel(id, in)
	if failLLM(c, err) {
		return
	}
	OK(c, res)
}

func (h *LLMHandler) DeleteModel(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "LLMHandler.DeleteModel")
	defer func() {
		slog.Debug("方法离开", "fn", "LLMHandler.DeleteModel", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	id, ok := parseParamID(c, "id")
	if !ok {
		return
	}
	if failLLM(c, h.cfg.DeleteModel(id)) {
		return
	}
	OK(c, nil)
}

func (h *LLMHandler) SetCurrentModel(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "LLMHandler.SetCurrentModel")
	defer func() {
		slog.Debug("方法离开", "fn", "LLMHandler.SetCurrentModel", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	id, ok := parseParamID(c, "id")
	if !ok {
		return
	}
	if failLLM(c, h.cfg.SetCurrentModel(id)) {
		return
	}
	OK(c, nil)
}

// Current 返回当前 provider(手工 map,绝不含 key 原文)与模型。
func (h *LLMHandler) Current(c *gin.Context) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "LLMHandler.Current")
	defer func() {
		slog.Debug("方法离开", "fn", "LLMHandler.Current", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	m, p, err := h.cfg.Current()
	if errors.Is(err, service.ErrNoCurrentModel) {
		OK(c, gin.H{"provider": nil, "model": nil})
		return
	}
	if failLLM(c, err) {
		return
	}
	OK(c, gin.H{
		"provider": gin.H{"id": p.ID, "name": p.Name, "base_url": p.BaseURL, "has_key": p.APIKey != ""},
		"model":    m,
	})
}
