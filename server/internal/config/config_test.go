package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"gbi/server/internal/config"
)

// writeEnv 在临时目录写 .env 内容。
func writeEnv(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o644))
	return filepath.Join(dir, ".env")
}

// resetGBIEnv 清除 GBI_* 变量,隔离 loadEnvFile 对进程环境的全局修改(os.Setenv),
// 保证每个测试从干净环境开始。t.Setenv 无法恢复非本测试设置的变量,故须显式 Unsetenv。
func resetGBIEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"GBI_ADDR", "GBI_PORT", "GBI_HOST", "GBI_DB_PATH", "GBI_JWT_SECRET",
		"GBI_DATA_DIR", "GBI_STATIC_ROOT", "GBI_SPA_MODE", "GBI_CORS_ORIGIN",
		"GBI_LOG_FILE", "GBI_LOG_LEVEL", "GBI_LOG_MAX_SIZE", "GBI_ENV_FILE",
		"GBI_LLM_BASE_URL", "GBI_LLM_API_KEY", "GBI_LLM_MODEL", "GBI_LLM_MAX_TOKENS", "GBI_SKILLS_DIR",
	}
	for _, k := range keys {
		old, existed := os.LookupEnv(k)
		require.NoError(t, os.Unsetenv(k))
		t.Cleanup(func() {
			if existed {
				_ = os.Setenv(k, old)
			}
		})
	}
}

func TestLoadEnvFileSetsMissingKeys(t *testing.T) {
	resetGBIEnv(t)
	p := writeEnv(t, "# 注释\nGBI_PORT=9090\nGBI_SPA_MODE=false\nGBI_CORS_ORIGIN=https://x.com\nGBI_STATIC_ROOT=dist2\nGBI_DATA_DIR=/tmp/xyz\n")
	t.Setenv("GBI_ENV_FILE", p)
	cfg := config.Load()

	require.Equal(t, "0.0.0.0:9090", cfg.Addr)
	require.False(t, cfg.SPAMode)
	require.Equal(t, "https://x.com", cfg.CORSOrigin)
	require.Equal(t, "dist2", cfg.StaticRoot)
	require.Equal(t, "/tmp/xyz", cfg.DataDirOverride)
}

func TestEnvFileDoesNotOverrideOSEnv(t *testing.T) {
	resetGBIEnv(t)
	p := writeEnv(t, "GBI_PORT=9090\n")
	t.Setenv("GBI_ENV_FILE", p)
	t.Setenv("GBI_PORT", "7070")
	cfg := config.Load()
	require.Equal(t, "0.0.0.0:7070", cfg.Addr)
}

func TestDefaults(t *testing.T) {
	resetGBIEnv(t)
	t.Setenv("GBI_ENV_FILE", filepath.Join(t.TempDir(), "missing.env")) // 不存在→静默跳过
	cfg := config.Load()
	require.Equal(t, ":8080", cfg.Addr)
	require.True(t, cfg.SPAMode)
	require.Equal(t, "*", cfg.CORSOrigin)
	require.Equal(t, "web/dist", cfg.StaticRoot)
	require.Equal(t, "", cfg.DataDirOverride)
	// 未配置时回退公开默认密钥;main 依赖该常量做启动告警,二者必须一致
	require.Equal(t, config.DefaultJWTSecret, cfg.JWTSecret)
}

func TestGBIAddrCompatibility(t *testing.T) {
	// e2e 依赖 GBI_ADDR;仅当 GBI_PORT 未设置时生效
	resetGBIEnv(t)
	t.Setenv("GBI_ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))
	t.Setenv("GBI_ADDR", ":9999")
	cfg := config.Load()
	require.Equal(t, ":9999", cfg.Addr)

	// GBI_PORT 设置时优先
	t.Setenv("GBI_PORT", "8081")
	cfg = config.Load()
	require.Equal(t, "0.0.0.0:8081", cfg.Addr)
}

func TestDataDirOverride(t *testing.T) {
	resetGBIEnv(t)
	t.Setenv("GBI_ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))
	// 未设置 GBI_DATA_DIR:跟随 DBPath 目录
	cfg := config.Load()
	require.Equal(t, filepath.Dir(cfg.DBPath), cfg.DataDir())

	// 设置后:返回指定目录
	dir := t.TempDir()
	t.Setenv("GBI_DATA_DIR", dir)
	cfg = config.Load()
	require.Equal(t, dir, cfg.DataDir())
}

func TestLogDefaults(t *testing.T) {
	resetGBIEnv(t)
	t.Setenv("GBI_ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))
	cfg := config.Load()
	require.Equal(t, "info", cfg.LogLevel)
	require.Equal(t, 10, cfg.LogMaxSize)
}

func TestLogEnvOverrides(t *testing.T) {
	resetGBIEnv(t)
	p := writeEnv(t, "GBI_LOG_LEVEL=debug\nGBI_LOG_MAX_SIZE=50\nGBI_LOG_FILE=./gbi.log\n")
	t.Setenv("GBI_ENV_FILE", p)
	cfg := config.Load()
	require.Equal(t, "debug", cfg.LogLevel)
	require.Equal(t, 50, cfg.LogMaxSize)
	require.Equal(t, "./gbi.log", cfg.LogFile)

	// 非法整数回退默认
	os.Unsetenv("GBI_LOG_MAX_SIZE") // 上一段经 loadEnvFile 写入进程环境,loadEnvFile 不覆盖已存在变量,须先清除
	p2 := writeEnv(t, "GBI_LOG_MAX_SIZE=abc\n")
	t.Setenv("GBI_ENV_FILE", p2)
	cfg = config.Load()
	require.Equal(t, 10, cfg.LogMaxSize)
}

// TestEnsureEnvTemplateGenerated 默认路径(未设置 GBI_ENV_FILE)缺 .env → 生成模板;
// 已存在绝不覆盖;模板不含任何密钥值。
func TestEnsureEnvTemplateGenerated(t *testing.T) {
	resetGBIEnv(t)
	dir := t.TempDir()
	t.Chdir(dir) // Go 1.24+:测试内切换 CWD,模板相对默认路径写入

	cfg := config.Load()
	envPath := filepath.Join(dir, ".env")
	require.FileExists(t, envPath)
	b, err := os.ReadFile(envPath)
	require.NoError(t, err)
	require.NotContains(t, string(b), "GBI_LLM_", "模板不再包含 LLM 配置键")
	require.Contains(t, string(b), "大模型设置", "模板应指引到控制台配置")
	require.Contains(t, string(b), "GBI_PORT=8080")
	require.Equal(t, "0.0.0.0:8080", cfg.Addr)

	// 已存在的 .env 不被模板覆盖
	require.NoError(t, os.WriteFile(envPath, []byte("GBI_PORT=9091\n"), 0o644))
	_ = config.Load()
	b2, err := os.ReadFile(envPath)
	require.NoError(t, err)
	require.Equal(t, "GBI_PORT=9091\n", string(b2))
}

// TestEnvTemplateNotGeneratedForExplicitPath 显式 GBI_ENV_FILE 缺失时不生成(e2e 确定性)。
func TestEnvTemplateNotGeneratedForExplicitPath(t *testing.T) {
	resetGBIEnv(t)
	p := filepath.Join(t.TempDir(), "no-such-env-file")
	t.Setenv("GBI_ENV_FILE", p)
	_ = config.Load()
	require.NoFileExists(t, p)
}

// TestCleanupObsoleteLLMEnv 删除过时 LLM 键并备份;其它键保留;无匹配不写文件。
func TestCleanupObsoleteLLMEnv(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	orig := "GBI_PORT=8080\nGBI_LLM_API_KEY=sk-secret\nGBI_LLM_BASE_URL=https://x\nGBI_HISTORY_TOKENS=90000\nGBI_LOG_LEVEL=info\n"
	require.NoError(t, os.WriteFile(p, []byte(orig), 0o644))

	removed, err := config.CleanupObsoleteLLMEnv(p)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"GBI_LLM_API_KEY", "GBI_LLM_BASE_URL", "GBI_HISTORY_TOKENS"}, removed)

	b, err := os.ReadFile(p)
	require.NoError(t, err)
	require.NotContains(t, string(b), "GBI_LLM_")
	require.NotContains(t, string(b), "GBI_HISTORY_TOKENS")
	require.Contains(t, string(b), "GBI_PORT=8080")
	require.Contains(t, string(b), "GBI_LOG_LEVEL=info")

	// 备份存在且含原始内容(含密钥)
	matches, err := filepath.Glob(p + ".bak-*")
	require.NoError(t, err)
	require.Len(t, matches, 1)
	bb, err := os.ReadFile(matches[0])
	require.NoError(t, err)
	require.Equal(t, orig, string(bb))

	// 幂等:再跑一次不产生新备份
	removed, err = config.CleanupObsoleteLLMEnv(p)
	require.NoError(t, err)
	require.Empty(t, removed)
	matches, _ = filepath.Glob(p + ".bak-*")
	require.Len(t, matches, 1)

	// 文件不存在 → 静默
	removed, err = config.CleanupObsoleteLLMEnv(filepath.Join(dir, "nope.env"))
	require.NoError(t, err)
	require.Empty(t, removed)
}

// TestCleanupNoMatchKeepsFile 无目标键时:内容逐字节不变且不产生备份。
func TestCleanupNoMatchKeepsFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	orig := "GBI_PORT=8080\n# GBI_LLM_API_KEY=commented\nGBI_LOG_LEVEL=info\n"
	require.NoError(t, os.WriteFile(p, []byte(orig), 0o644))

	removed, err := config.CleanupObsoleteLLMEnv(p)
	require.NoError(t, err)
	require.Empty(t, removed)

	b, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, orig, string(b))
	matches, err := filepath.Glob(p + ".bak-*")
	require.NoError(t, err)
	require.Empty(t, matches)
}

// TestEnsureRuntimeDirs 建库目录/数据目录/日志目录,幂等;失败路径返回错误。
func TestEnsureRuntimeDirs(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		DBPath:          filepath.Join(dir, "data", "gbi.db"),
		DataDirOverride: filepath.Join(dir, "subjects"),
		LogFile:         filepath.Join(dir, "logs", "gbi.log"),
	}
	require.NoError(t, config.EnsureRuntimeDirs(cfg))
	require.DirExists(t, filepath.Join(dir, "data"))
	require.DirExists(t, filepath.Join(dir, "subjects"))
	require.DirExists(t, filepath.Join(dir, "logs"))
	require.NoError(t, config.EnsureRuntimeDirs(cfg)) // 幂等

	// 失败:父路径是文件 → MkdirAll 必然失败
	file := filepath.Join(dir, "afile")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))
	bad := &config.Config{DBPath: filepath.Join(file, "gbi.db")}
	require.Error(t, config.EnsureRuntimeDirs(bad))
}
