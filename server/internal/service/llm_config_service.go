package service

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"

	"gbi/server/internal/llm"
	"gbi/server/internal/model"
)

// 出厂默认值(全新安装;显式 env 由 main 覆盖后传入 Seed)。
const (
	DefaultProviderName  = "deepseek"
	DefaultBaseURL       = "https://api.deepseek.com/anthropic"
	DefaultModelName     = "deepseek-flash"
	DefaultContextWindow = 384000
	DefaultMaxTokens     = 16384
)

var (
	ErrLLMProviderNotFound      = errors.New("provider 不存在")
	ErrLLMModelNotFound         = errors.New("模型不存在")
	ErrProviderHasModels        = errors.New("该 provider 下仍有模型,请先删除模型")
	ErrCurrentModel             = errors.New("不能删除当前模型,请先切换当前模型")
	ErrNoCurrentModel           = errors.New("未配置当前模型")
	ErrLLMNameRequired          = errors.New("名称不能为空")
	ErrLLMBaseURLInvalid        = errors.New("base_url 须为 http/https 地址")
	ErrLLMModelNameRequired     = errors.New("模型名不能为空")
	ErrLLMContextInvalid        = errors.New("context_window 必须为正整数")
	ErrLLMDuplicateProviderName = errors.New("provider 名称已存在")
	ErrLLMDuplicateModelName    = errors.New("同一 provider 下模型名已存在")
	ErrLLMMaxTokensInvalid      = fmt.Errorf("max_tokens 须大于 %d(思考预算)且小于 context_window", llm.ThinkingBudgetTokens)
)

// LLMProviderDTO 列表项:密钥只回掩码。
type LLMProviderDTO struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	BaseURL      string `json:"base_url"`
	APIKeyMasked string `json:"api_key_masked"`
	HasKey       bool   `json:"has_key"`
	ModelCount   int64  `json:"model_count"`
}

type ProviderInput struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
}

// ProviderUpdate APIKey 为 nil 或空串时不修改。
type ProviderUpdate struct {
	Name    string  `json:"name"`
	BaseURL string  `json:"base_url"`
	APIKey  *string `json:"api_key"`
}

type ModelInput struct {
	Name          string `json:"name"`
	ContextWindow int    `json:"context_window"`
	MaxTokens     int    `json:"max_tokens"`
}

type ModelUpdate struct {
	Name          string `json:"name"`
	ContextWindow int    `json:"context_window"`
	MaxTokens     int    `json:"max_tokens"`
}

// SeedInput 首次播种参数(显式 env 或出厂默认)。
type SeedInput struct {
	ProviderName  string
	BaseURL       string
	APIKey        string
	ModelName     string
	ContextWindow int
	MaxTokens     int
}

type LLMConfigService struct {
	db *gorm.DB
}

func NewLLMConfigService(db *gorm.DB) *LLMConfigService {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.NewLLMConfigService")
	defer func() {
		slog.Debug("方法离开", "fn", "service.NewLLMConfigService", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &LLMConfigService{db: db}
}

// maskKey 只保留前 3 与后 4 位。
func maskKey(k string) string {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.maskKey", "len", len(k))
	defer func() {
		slog.Debug("方法离开", "fn", "service.maskKey", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if k == "" {
		return ""
	}
	if len(k) <= 8 {
		return "****"
	}
	return k[:3] + "***" + k[len(k)-4:]
}

func validateProviderNameURL(name, baseURL string) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.validateProviderNameURL")
	defer func() {
		slog.Debug("方法离开", "fn", "service.validateProviderNameURL", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	if strings.TrimSpace(name) == "" {
		return ErrLLMNameRequired
	}
	u, perr := url.Parse(strings.TrimSpace(baseURL))
	if perr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ErrLLMBaseURLInvalid
	}
	return nil
}

func validateModelFields(name string, cw, mt int) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.validateModelFields", "cw", cw, "mt", mt)
	defer func() {
		slog.Debug("方法离开", "fn", "service.validateModelFields", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	if strings.TrimSpace(name) == "" {
		return ErrLLMModelNameRequired
	}
	if cw <= 0 {
		return ErrLLMContextInvalid
	}
	if mt <= llm.ThinkingBudgetTokens || mt >= cw {
		return ErrLLMMaxTokensInvalid
	}
	return nil
}

func (s *LLMConfigService) ListProviders() (out []LLMProviderDTO, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "LLMConfigService.ListProviders")
	defer func() {
		slog.Debug("方法离开", "fn", "LLMConfigService.ListProviders", "elapsed_ms", time.Since(start).Milliseconds(), "count", len(out), "err", err)
	}()
	var ps []model.LLMProvider
	if err := s.db.Order("id").Find(&ps).Error; err != nil {
		return nil, err
	}
	out = make([]LLMProviderDTO, 0, len(ps))
	for _, p := range ps {
		var n int64
		if err := s.db.Model(&model.LLMModel{}).Where("provider_id = ?", p.ID).Count(&n).Error; err != nil {
			return nil, err
		}
		out = append(out, LLMProviderDTO{
			ID: p.ID, Name: p.Name, BaseURL: p.BaseURL,
			APIKeyMasked: maskKey(p.APIKey), HasKey: p.APIKey != "", ModelCount: n,
		})
	}
	return out, nil
}

func (s *LLMConfigService) providerDTO(p *model.LLMProvider) (dto LLMProviderDTO, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "LLMConfigService.providerDTO", "id", p.ID)
	defer func() {
		slog.Debug("方法离开", "fn", "LLMConfigService.providerDTO", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var n int64
	if err = s.db.Model(&model.LLMModel{}).Where("provider_id = ?", p.ID).Count(&n).Error; err != nil {
		return LLMProviderDTO{}, err
	}
	return LLMProviderDTO{
		ID: p.ID, Name: p.Name, BaseURL: p.BaseURL,
		APIKeyMasked: maskKey(p.APIKey), HasKey: p.APIKey != "", ModelCount: n,
	}, nil
}

func (s *LLMConfigService) CreateProvider(in ProviderInput) (res *LLMProviderDTO, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "LLMConfigService.CreateProvider", "name", in.Name, "has_key", in.APIKey != "")
	defer func() {
		slog.Debug("方法离开", "fn", "LLMConfigService.CreateProvider", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	in.Name = strings.TrimSpace(in.Name)
	in.BaseURL = strings.TrimSpace(in.BaseURL)
	in.APIKey = strings.TrimSpace(in.APIKey)
	if err := validateProviderNameURL(in.Name, in.BaseURL); err != nil {
		return nil, err
	}
	var existing model.LLMProvider
	if ferr := s.db.Where("name = ?", in.Name).First(&existing).Error; ferr == nil {
		return nil, ErrLLMDuplicateProviderName
	} else if !errors.Is(ferr, gorm.ErrRecordNotFound) {
		return nil, ferr
	}
	p := model.LLMProvider{Name: in.Name, BaseURL: strings.TrimRight(in.BaseURL, "/"), APIKey: in.APIKey}
	if err := s.db.Create(&p).Error; err != nil {
		return nil, err
	}
	dto, derr := s.providerDTO(&p)
	if derr != nil {
		return nil, derr
	}
	return &dto, nil
}

func (s *LLMConfigService) UpdateProvider(id uint, in ProviderUpdate) (res *LLMProviderDTO, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "LLMConfigService.UpdateProvider", "id", id)
	defer func() {
		slog.Debug("方法离开", "fn", "LLMConfigService.UpdateProvider", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var p model.LLMProvider
	if err := s.db.First(&p, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrLLMProviderNotFound
		}
		return nil, err
	}
	name := p.Name
	if strings.TrimSpace(in.Name) != "" {
		name = strings.TrimSpace(in.Name)
	}
	baseURL := p.BaseURL
	if strings.TrimSpace(in.BaseURL) != "" {
		baseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	}
	if err := validateProviderNameURL(name, baseURL); err != nil {
		return nil, err
	}
	if name != p.Name {
		var dup model.LLMProvider
		if ferr := s.db.Where("name = ? AND id <> ?", name, p.ID).First(&dup).Error; ferr == nil {
			return nil, ErrLLMDuplicateProviderName
		} else if !errors.Is(ferr, gorm.ErrRecordNotFound) {
			return nil, ferr
		}
	}
	p.Name = name
	p.BaseURL = baseURL
	if in.APIKey != nil {
		if k := strings.TrimSpace(*in.APIKey); k != "" {
			p.APIKey = k
		}
	}
	if err := s.db.Save(&p).Error; err != nil {
		return nil, err
	}
	dto, derr := s.providerDTO(&p)
	if derr != nil {
		return nil, derr
	}
	return &dto, nil
}

func (s *LLMConfigService) DeleteProvider(id uint) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "LLMConfigService.DeleteProvider", "id", id)
	defer func() {
		slog.Debug("方法离开", "fn", "LLMConfigService.DeleteProvider", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var p model.LLMProvider
	if err := s.db.First(&p, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrLLMProviderNotFound
		}
		return err
	}
	var n int64
	if err := s.db.Model(&model.LLMModel{}).Where("provider_id = ?", id).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return ErrProviderHasModels
	}
	return s.db.Delete(&p).Error
}

func (s *LLMConfigService) ListModels(providerID uint) (out []model.LLMModel, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "LLMConfigService.ListModels", "provider_id", providerID)
	defer func() {
		slog.Debug("方法离开", "fn", "LLMConfigService.ListModels", "elapsed_ms", time.Since(start).Milliseconds(), "count", len(out), "err", err)
	}()
	var p model.LLMProvider
	if err := s.db.First(&p, providerID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrLLMProviderNotFound
		}
		return nil, err
	}
	if err := s.db.Where("provider_id = ?", providerID).Order("id").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (s *LLMConfigService) CreateModel(providerID uint, in ModelInput) (res *model.LLMModel, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "LLMConfigService.CreateModel", "provider_id", providerID, "name", in.Name)
	defer func() {
		slog.Debug("方法离开", "fn", "LLMConfigService.CreateModel", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var p model.LLMProvider
	if err := s.db.First(&p, providerID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrLLMProviderNotFound
		}
		return nil, err
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.ContextWindow <= 0 {
		in.ContextWindow = DefaultContextWindow
	}
	if in.MaxTokens <= 0 {
		in.MaxTokens = DefaultMaxTokens
	}
	if err := validateModelFields(in.Name, in.ContextWindow, in.MaxTokens); err != nil {
		return nil, err
	}
	var existing model.LLMModel
	if ferr := s.db.Where("provider_id = ? AND name = ?", providerID, in.Name).First(&existing).Error; ferr == nil {
		return nil, ErrLLMDuplicateModelName
	} else if !errors.Is(ferr, gorm.ErrRecordNotFound) {
		return nil, ferr
	}
	m := model.LLMModel{ProviderID: providerID, Name: in.Name, ContextWindow: in.ContextWindow, MaxTokens: in.MaxTokens}
	if err := s.db.Create(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *LLMConfigService) UpdateModel(id uint, in ModelUpdate) (res *model.LLMModel, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "LLMConfigService.UpdateModel", "id", id)
	defer func() {
		slog.Debug("方法离开", "fn", "LLMConfigService.UpdateModel", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var m model.LLMModel
	if err := s.db.First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrLLMModelNotFound
		}
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = m.Name
	}
	if err := validateModelFields(name, in.ContextWindow, in.MaxTokens); err != nil {
		return nil, err
	}
	var dup model.LLMModel
	if ferr := s.db.Where("provider_id = ? AND name = ? AND id <> ?", m.ProviderID, name, m.ID).First(&dup).Error; ferr == nil {
		return nil, ErrLLMDuplicateModelName
	} else if !errors.Is(ferr, gorm.ErrRecordNotFound) {
		return nil, ferr
	}
	m.Name = name
	m.ContextWindow = in.ContextWindow
	m.MaxTokens = in.MaxTokens
	if err := s.db.Save(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *LLMConfigService) DeleteModel(id uint) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "LLMConfigService.DeleteModel", "id", id)
	defer func() {
		slog.Debug("方法离开", "fn", "LLMConfigService.DeleteModel", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var m model.LLMModel
	if err := s.db.First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrLLMModelNotFound
		}
		return err
	}
	if m.IsCurrent {
		return ErrCurrentModel
	}
	return s.db.Delete(&m).Error
}

func (s *LLMConfigService) SetCurrentModel(id uint) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "LLMConfigService.SetCurrentModel", "id", id)
	defer func() {
		slog.Debug("方法离开", "fn", "LLMConfigService.SetCurrentModel", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	return s.db.Transaction(func(tx *gorm.DB) error {
		var m model.LLMModel
		if ferr := tx.First(&m, id).Error; ferr != nil {
			if errors.Is(ferr, gorm.ErrRecordNotFound) {
				return ErrLLMModelNotFound
			}
			return ferr
		}
		if ferr := tx.Model(&model.LLMModel{}).Where("is_current = ?", true).Update("is_current", false).Error; ferr != nil {
			return ferr
		}
		return tx.Model(&model.LLMModel{}).Where("id = ?", id).Update("is_current", true).Error
	})
}

func (s *LLMConfigService) Current() (m *model.LLMModel, p *model.LLMProvider, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "LLMConfigService.Current")
	defer func() {
		slog.Debug("方法离开", "fn", "LLMConfigService.Current", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	m = &model.LLMModel{}
	if ferr := s.db.Where("is_current = ?", true).First(m).Error; ferr != nil {
		if errors.Is(ferr, gorm.ErrRecordNotFound) {
			return nil, nil, ErrNoCurrentModel
		}
		return nil, nil, ferr
	}
	p = &model.LLMProvider{}
	if ferr := s.db.First(p, m.ProviderID).Error; ferr != nil {
		return nil, nil, ferr
	}
	return m, p, nil
}

// BudgetTokens 历史预算:上下文窗口扣除输出上限后剩余的输入额度(下限 1)。
func BudgetTokens(m *model.LLMModel) int {
	start := time.Now()
	b := m.ContextWindow - m.MaxTokens
	slog.Debug("方法进入", "fn", "service.BudgetTokens", "context_window", m.ContextWindow, "max_tokens", m.MaxTokens)
	defer func() {
		slog.Debug("方法离开", "fn", "service.BudgetTokens", "elapsed_ms", time.Since(start).Milliseconds(), "budget", b)
	}()
	if b < 1 {
		return 1
	}
	return b
}

// Seed 首次播种:有显式 env 值时由 main 传入;其余字段以出厂默认补齐。
// 库中已有 provider 时只补当前模型,不再新建。
func (s *LLMConfigService) Seed(in SeedInput) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "LLMConfigService.Seed", "provider", in.ProviderName, "model", in.ModelName, "has_key", in.APIKey != "")
	defer func() {
		slog.Debug("方法离开", "fn", "LLMConfigService.Seed", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var count int64
	if err := s.db.Model(&model.LLMProvider{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return s.ensureCurrent()
	}
	in.BaseURL = strings.TrimSpace(in.BaseURL)
	in.APIKey = strings.TrimSpace(in.APIKey)
	in.ModelName = strings.TrimSpace(in.ModelName)
	if in.ProviderName == "" {
		in.ProviderName = DefaultProviderName
	}
	if in.BaseURL == "" {
		in.BaseURL = DefaultBaseURL
	}
	if in.ModelName == "" {
		in.ModelName = DefaultModelName
	}
	if in.ContextWindow <= 0 {
		in.ContextWindow = DefaultContextWindow
	}
	if in.MaxTokens <= 0 {
		in.MaxTokens = DefaultMaxTokens
	}
	if in.MaxTokens <= llm.ThinkingBudgetTokens {
		slog.Warn("迁移的 max_tokens 不大于思考预算,归一为默认值", "old", in.MaxTokens, "new", DefaultMaxTokens)
		in.MaxTokens = DefaultMaxTokens
	}
	if err := validateProviderNameURL(in.ProviderName, in.BaseURL); err != nil {
		return err
	}
	if err := validateModelFields(in.ModelName, in.ContextWindow, in.MaxTokens); err != nil {
		return err
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		p := model.LLMProvider{Name: in.ProviderName, BaseURL: strings.TrimRight(in.BaseURL, "/"), APIKey: in.APIKey}
		if ferr := tx.Create(&p).Error; ferr != nil {
			return ferr
		}
		m := model.LLMModel{ProviderID: p.ID, Name: in.ModelName, ContextWindow: in.ContextWindow, MaxTokens: in.MaxTokens, IsCurrent: true}
		return tx.Create(&m).Error
	})
	if err == nil {
		slog.Info("LLM 配置已播种", "provider", in.ProviderName, "model", in.ModelName, "context_window", in.ContextWindow, "max_tokens", in.MaxTokens, "has_key", in.APIKey != "")
	}
	return err
}

// EnsureSeeded 播种(幂等),并返回"当前模型是否可用(存在且 API Key 非空)"。
// main 只在可用时清理 .env 过时键,防止把从未落库的密钥删掉。
func (s *LLMConfigService) EnsureSeeded(in SeedInput) (usable bool, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "LLMConfigService.EnsureSeeded")
	defer func() {
		slog.Debug("方法离开", "fn", "LLMConfigService.EnsureSeeded", "elapsed_ms", time.Since(start).Milliseconds(), "usable", usable, "err", err)
	}()
	if err := s.Seed(in); err != nil {
		return false, err
	}
	_, p, cerr := s.Current()
	if cerr != nil {
		return false, nil
	}
	return p.APIKey != "", nil
}

// ensureCurrent 有模型但无当前模型时,取最早模型设为当前(防止 studio 静默不可用)。
func (s *LLMConfigService) ensureCurrent() (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "LLMConfigService.ensureCurrent")
	defer func() {
		slog.Debug("方法离开", "fn", "LLMConfigService.ensureCurrent", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var cur model.LLMModel
	ferr := s.db.Where("is_current = ?", true).First(&cur).Error
	if ferr == nil {
		return nil
	}
	if !errors.Is(ferr, gorm.ErrRecordNotFound) {
		return ferr
	}
	var first model.LLMModel
	if ferr := s.db.Order("id").First(&first).Error; ferr != nil {
		if errors.Is(ferr, gorm.ErrRecordNotFound) {
			return nil // 无模型:等待后续创建
		}
		return ferr
	}
	slog.Warn("未设置当前模型,自动取最早模型", "model_id", first.ID, "name", first.Name)
	return s.SetCurrentModel(first.ID)
}
