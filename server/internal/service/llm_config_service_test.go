package service_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gbi/server/internal/model"
	"gbi/server/internal/service"
)

func newLLMEnv(t *testing.T) *service.LLMConfigService {
	t.Helper()
	return service.NewLLMConfigService(newTestDB(t))
}

func TestLLMProviderAndModelCRUD(t *testing.T) {
	s := newLLMEnv(t)

	p, err := s.CreateProvider(service.ProviderInput{Name: "p1", BaseURL: "https://x.example/anthropic", APIKey: "sk-1234567890abcd"})
	require.NoError(t, err)
	require.Equal(t, "sk-***abcd", p.APIKeyMasked)
	require.True(t, p.HasKey)

	// 名称重复
	_, err = s.CreateProvider(service.ProviderInput{Name: "p1", BaseURL: "https://y.example"})
	require.ErrorIs(t, err, service.ErrLLMDuplicateProviderName)

	// URL 非法
	_, err = s.CreateProvider(service.ProviderInput{Name: "p2", BaseURL: "ftp://x"})
	require.ErrorIs(t, err, service.ErrLLMBaseURLInvalid)

	m, err := s.CreateModel(p.ID, service.ModelInput{Name: "m1"}) // 缺省 384000/16384
	require.NoError(t, err)
	require.Equal(t, 384000, m.ContextWindow)
	require.Equal(t, 16384, m.MaxTokens)
	require.False(t, m.IsCurrent)

	// max_tokens 边界:必须 > 8192 且 < context_window
	_, err = s.CreateModel(p.ID, service.ModelInput{Name: "bad1", ContextWindow: 384000, MaxTokens: 8192})
	require.ErrorIs(t, err, service.ErrLLMMaxTokensInvalid)
	_, err = s.CreateModel(p.ID, service.ModelInput{Name: "bad2", ContextWindow: 384000, MaxTokens: 384000})
	require.ErrorIs(t, err, service.ErrLLMMaxTokensInvalid)
	// 同 provider 内模型名重复
	_, err = s.CreateModel(p.ID, service.ModelInput{Name: "m1"})
	require.ErrorIs(t, err, service.ErrLLMDuplicateModelName)

	m2, err := s.CreateModel(p.ID, service.ModelInput{Name: "m2"})
	require.NoError(t, err)
	_, err = s.UpdateModel(m2.ID, service.ModelUpdate{Name: "m2x", ContextWindow: 200000, MaxTokens: 16000})
	require.NoError(t, err)

	list, err := s.ListModels(p.ID)
	require.NoError(t, err)
	require.Len(t, list, 2)
	require.Equal(t, "m2x", list[1].Name)
}

func TestLLMCurrentModelUniqueAndDeleteGuards(t *testing.T) {
	s := newLLMEnv(t)
	p, err := s.CreateProvider(service.ProviderInput{Name: "p", BaseURL: "https://x.example"})
	require.NoError(t, err)
	m1, err := s.CreateModel(p.ID, service.ModelInput{Name: "m1"})
	require.NoError(t, err)
	m2, err := s.CreateModel(p.ID, service.ModelInput{Name: "m2"})
	require.NoError(t, err)

	_, _, err = s.Current()
	require.ErrorIs(t, err, service.ErrNoCurrentModel)

	require.NoError(t, s.SetCurrentModel(m1.ID))
	require.NoError(t, s.SetCurrentModel(m2.ID))

	cm, cp, err := s.Current()
	require.NoError(t, err)
	require.Equal(t, m2.ID, cm.ID)
	require.Equal(t, p.ID, cp.ID)

	// 仅一个 is_current
	list, err := s.ListModels(p.ID)
	require.NoError(t, err)
	curCount := 0
	for _, m := range list {
		if m.IsCurrent {
			curCount++
		}
	}
	require.Equal(t, 1, curCount)

	// 当前模型拒删;有模型 provider 拒删
	require.ErrorIs(t, s.DeleteModel(m2.ID), service.ErrCurrentModel)
	require.ErrorIs(t, s.DeleteProvider(p.ID), service.ErrProviderHasModels)
	// 非当前模型可删;再次设为当前幂等
	require.NoError(t, s.DeleteModel(m1.ID))
	require.NoError(t, s.SetCurrentModel(m2.ID))
}

func TestLLMSeedFromExplicitAndDefaults(t *testing.T) {
	s := newLLMEnv(t)

	// 显式迁移:max_tokens 过小 → 归一 16384;base_url 去尾斜杠
	require.NoError(t, s.Seed(service.SeedInput{
		ProviderName: "deepseek", BaseURL: "https://api.deepseek.com/anthropic/", APIKey: "sk-real-12345678",
		ModelName: "deepseek-v4-flash", ContextWindow: 384000, MaxTokens: 8192,
	}))
	m, p, err := s.Current()
	require.NoError(t, err)
	require.Equal(t, "https://api.deepseek.com/anthropic", p.BaseURL)
	require.Equal(t, "deepseek-v4-flash", m.Name)
	require.Equal(t, 16384, m.MaxTokens)
	require.Equal(t, "sk-real-12345678", p.APIKey)

	// 已有 provider → Seed 不再新建
	providers, err := s.ListProviders()
	require.NoError(t, err)
	require.Len(t, providers, 1)
	require.NoError(t, s.Seed(service.SeedInput{ProviderName: "other", BaseURL: "https://y.example"}))
	providers, _ = s.ListProviders()
	require.Len(t, providers, 1)
}

// Seed 走与 CRUD 相同的校验:非法组合必须拒绝,不能落库
func TestLLMSeedRejectsInvalidCombo(t *testing.T) {
	s := newLLMEnv(t)
	err := s.Seed(service.SeedInput{
		ProviderName: "deepseek", BaseURL: "https://api.deepseek.com/anthropic",
		ModelName: "m", ContextWindow: 8192, MaxTokens: 16384,
	})
	require.ErrorIs(t, err, service.ErrLLMMaxTokensInvalid)
	// 未落库:通过 ListProviders 断言
	list, lerr := s.ListProviders()
	require.NoError(t, lerr)
	require.Empty(t, list)
}

func TestLLMBudgetTokens(t *testing.T) {
	require.Equal(t, 384000-16384, service.BudgetTokens(&model.LLMModel{ContextWindow: 384000, MaxTokens: 16384}))
	require.Equal(t, 1, service.BudgetTokens(&model.LLMModel{ContextWindow: 8192, MaxTokens: 8192}))
}

// EnsureSeeded:已有 provider 且当前模型缺 key 时,不迁移 env key,usable=false
func TestLLMEnsureSeededDoesNotMigrateWhenUnusable(t *testing.T) {
	s := newLLMEnv(t)
	p, err := s.CreateProvider(service.ProviderInput{Name: "p", BaseURL: "https://x.example", APIKey: ""})
	require.NoError(t, err)
	m, err := s.CreateModel(p.ID, service.ModelInput{Name: "m"})
	require.NoError(t, err)
	require.NoError(t, s.SetCurrentModel(m.ID))

	usable, err := s.EnsureSeeded(service.SeedInput{ProviderName: "other", BaseURL: "https://y.example", APIKey: "sk-env-should-not-be-dropped", ModelName: "m2"})
	require.NoError(t, err)
	require.False(t, usable)
	// 未新建 provider,key 仍为空(env 值未被吞掉)
	list, err := s.ListProviders()
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.False(t, list[0].HasKey)
}

// EnsureSeeded:全新库 → 播种并 usable=true
func TestLLMEnsureSeededFresh(t *testing.T) {
	s := newLLMEnv(t)
	usable, err := s.EnsureSeeded(service.SeedInput{APIKey: "sk-1234567890abcd"})
	require.NoError(t, err)
	require.True(t, usable)
	list, err := s.ListProviders()
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.True(t, list[0].HasKey)
}
