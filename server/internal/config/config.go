package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DefaultJWTSecret 开发默认 JWT 密钥(公开已知)。生产部署必须显式设置
// GBI_JWT_SECRET,否则启动期由 main 打出醒目告警。
const DefaultJWTSecret = "gbi-dev-secret-change-me"

type Config struct {
	Addr      string // 监听地址:GBI_PORT[/GBI_HOST] 优先,否则 GBI_ADDR,默认 :8080
	DBPath    string
	JWTSecret string
	JWTTTL    time.Duration

	DataDirOverride string // GBI_DATA_DIR(空=跟随 DBPath 目录)

	StaticRoot string // GBI_STATIC_ROOT,默认 web/dist
	SPAMode    bool   // GBI_SPA_MODE,默认 true
	CORSOrigin string // GBI_CORS_ORIGIN,默认 *
	LogFile    string // GBI_LOG_FILE(日志落地由子项目 C 实现)
	LogLevel   string // GBI_LOG_LEVEL,默认 info(debug|info|warn|error,非法值由 logger 包回退)
	LogMaxSize int    // GBI_LOG_MAX_SIZE(MB),默认 10;仅 LogFile 非空时生效
	EnvFile    string // GBI_ENV_FILE,默认 .env

	SkillsDir string // GBI_SKILLS_DIR

	RenderCheckWaitMs int // GBI_RENDER_CHECK_WAIT(ms),默认 1500
	HistoryKeep       int // GBI_HISTORY_KEEP,默认 24
}

func Load() *Config {
	start := time.Now()
	// 本包不可 import logger(成环);日志只记标量/长度,敏感值(密钥/密码)绝不入日志。
	slog.Debug("方法进入", "fn", "config.Load")
	defer func() {
		slog.Debug("方法离开", "fn", "config.Load", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	// 先加载 .env(路径只从 OS 环境读,避免鸡生蛋;已存在的变量不覆盖)
	loadConfigEnv()

	// 监听地址:新键优先;GBI_ADDR 兼容(e2e 依赖)
	addr := ""
	if port := os.Getenv("GBI_PORT"); port != "" {
		host := os.Getenv("GBI_HOST")
		if host == "" {
			host = "0.0.0.0"
		}
		addr = host + ":" + port
	} else {
		addr = envOr("GBI_ADDR", ":8080")
	}

	return &Config{
		Addr:      addr,
		DBPath:    envOr("GBI_DB_PATH", "data/gbi.db"),
		JWTSecret: envOr("GBI_JWT_SECRET", DefaultJWTSecret),
		JWTTTL:    12 * time.Hour,

		DataDirOverride: envOr("GBI_DATA_DIR", ""),

		StaticRoot: envOr("GBI_STATIC_ROOT", "web/dist"),
		SPAMode:    envOrBool("GBI_SPA_MODE", true),
		CORSOrigin: envOr("GBI_CORS_ORIGIN", "*"),
		LogFile:    envOr("GBI_LOG_FILE", ""),
		LogLevel:   envOr("GBI_LOG_LEVEL", "info"),
		LogMaxSize: envOrInt("GBI_LOG_MAX_SIZE", 10),
		EnvFile:    envOr("GBI_ENV_FILE", ".env"),

		SkillsDir: envOr("GBI_SKILLS_DIR", "skills"),

		RenderCheckWaitMs: envOrInt("GBI_RENDER_CHECK_WAIT", 1500),
		HistoryKeep:       envOrInt("GBI_HISTORY_KEEP", 24),
	}
}

// DataDir 返回数据目录:GBI_DATA_DIR 设置时返回它;否则为元数据库所在目录。
func (c *Config) DataDir() string {
	start := time.Now()
	slog.Debug("方法进入", "fn", "Config.DataDir")
	defer func() {
		slog.Debug("方法离开", "fn", "Config.DataDir", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if c.DataDirOverride != "" {
		return c.DataDirOverride
	}
	return filepath.Dir(c.DBPath)
}

func envOr(key, def string) string {
	start := time.Now()
	// 只记键名;env 值与 def(可能含密钥类默认值)一律不入日志。
	slog.Debug("方法进入", "fn", "config.envOr", "key", key)
	defer func() {
		slog.Debug("方法离开", "fn", "config.envOr", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envOrBool(key string, def bool) bool {
	start := time.Now()
	slog.Debug("方法进入", "fn", "config.envOrBool", "key", key)
	defer func() {
		slog.Debug("方法离开", "fn", "config.envOrBool", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	}
	return def
}

func envOrInt(key string, def int) int {
	start := time.Now()
	slog.Debug("方法进入", "fn", "config.envOrInt", "key", key)
	defer func() {
		slog.Debug("方法离开", "fn", "config.envOrInt", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// loadEnvFile 解析 KEY=VALUE 行并设置到进程环境;已存在的变量不覆盖。
// 文件不存在时静默跳过(与 webserver.go 语义一致)。
func loadEnvFile(path string) {
	start := time.Now()
	// 只记路径;文件内 KEY/VALUE 原文一律不入日志。
	slog.Debug("方法进入", "fn", "config.loadEnvFile", "path", path)
	defer func() {
		slog.Debug("方法离开", "fn", "config.loadEnvFile", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if path == "" {
		path = ".env"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(parts[1], `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
}

// loadConfigEnv 加载配置源:GBI_ENV_FILE 显式设置时仅读取(缺也不创建,保 e2e 确定性);
// 未设置时用默认 .env,缺失则先生成模板(首次运行自举)。
func loadConfigEnv() {
	start := time.Now()
	slog.Debug("方法进入", "fn", "config.loadConfigEnv")
	defer func() {
		slog.Debug("方法离开", "fn", "config.loadConfigEnv", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if p := os.Getenv("GBI_ENV_FILE"); p != "" {
		loadEnvFile(p)
		return
	}
	ensureEnvTemplate(".env")
	loadEnvFile(".env")
}

// envTemplate 首次运行生成的 .env 模板:全部为注释或非敏感默认值,绝不含密钥。
const envTemplate = `# GBI 配置模板(由 gbi.exe 首次运行自动生成)
# 修改后重启服务生效。

GBI_PORT=8080
GBI_LOG_FILE=./logs/gbi.log
GBI_LOG_LEVEL=info

# 大模型配置已移至控制台「大模型设置」页面(Provider/模型/当前模型),不再通过 .env 配置

# 生产环境务必设置强随机密钥(JWT 签发):
# GBI_JWT_SECRET=

# GBI_STATIC_ROOT 指向磁盘目录时优先磁盘,否则使用内置前端(分发版默认)
`

// ensureEnvTemplate 默认 .env 缺失时生成模板;已存在绝不覆盖;写失败仅告警不阻塞启动。
func ensureEnvTemplate(path string) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "config.ensureEnvTemplate", "path", path)
	defer func() {
		slog.Debug("方法离开", "fn", "config.ensureEnvTemplate", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if _, err := os.Stat(path); err == nil {
		return
	}
	if err := os.WriteFile(path, []byte(envTemplate), 0o644); err != nil {
		slog.Warn("生成 .env 模板失败,继续以环境变量/默认值启动", "path", path, "err", err)
		return
	}
	slog.Info("已生成 .env 模板,请在控制台「大模型设置」配置大模型", "path", path)
}

// obsoleteLLMEnvKeys 子项目 I 起由控制台管理,迁移落库后从 env 文件清理。
var obsoleteLLMEnvKeys = map[string]bool{
	"GBI_LLM_BASE_URL":   true,
	"GBI_LLM_API_KEY":    true,
	"GBI_LLM_MODEL":      true,
	"GBI_LLM_MAX_TOKENS": true,
	"GBI_HISTORY_TOKENS": true,
}

// CleanupObsoleteLLMEnv 删除 env 文件中的过时 LLM 键(写前备份,备份权限 0600);
// 无匹配键/文件不存在时不写文件、不产生备份。返回被删除的键名。
func CleanupObsoleteLLMEnv(path string) (removed []string, err error) {
	start := time.Now()
	if path == "" {
		path = ".env"
	}
	slog.Debug("方法进入", "fn", "config.CleanupObsoleteLLMEnv", "path", path)
	defer func() {
		slog.Debug("方法离开", "fn", "config.CleanupObsoleteLLMEnv", "elapsed_ms", time.Since(start).Milliseconds(), "removed", len(removed), "err", err)
	}()
	b, rerr := os.ReadFile(path)
	if rerr != nil {
		if os.IsNotExist(rerr) {
			return nil, nil
		}
		return nil, rerr
	}
	lines := strings.Split(string(b), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		key := strings.TrimSpace(strings.SplitN(line, "=", 2)[0])
		if obsoleteLLMEnvKeys[key] {
			removed = append(removed, key)
			continue
		}
		kept = append(kept, line)
	}
	if len(removed) == 0 {
		return nil, nil
	}
	info, serr := os.Stat(path)
	mode := os.FileMode(0o644)
	if serr == nil {
		mode = info.Mode().Perm()
	}
	backup := fmt.Sprintf("%s.bak-%s", path, time.Now().Format("20060102-150405"))
	if werr := os.WriteFile(backup, b, 0o600); werr != nil {
		return nil, fmt.Errorf("备份 %s 失败: %w", path, werr)
	}
	if werr := os.WriteFile(path, []byte(strings.Join(kept, "\n")), mode); werr != nil {
		return nil, fmt.Errorf("重写 %s 失败: %w", path, werr)
	}
	slog.Info("已备份并清理过时 LLM 配置", "backup", backup, "keys", len(removed))
	return removed, nil
}

// EnsureRuntimeDirs 首次运行自举:创建库目录、数据目录(GBI_DATA_DIR 可独立)、日志目录(幂等)。
func EnsureRuntimeDirs(c *Config) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "config.EnsureRuntimeDirs", "db_path", c.DBPath, "data_dir", c.DataDir(), "log_file", c.LogFile)
	defer func() {
		slog.Debug("方法离开", "fn", "config.EnsureRuntimeDirs", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	dirs := make([]string, 0, 3)
	if d := filepath.Dir(c.DBPath); d != "." && d != "" {
		dirs = append(dirs, d)
	}
	if d := c.DataDir(); d != "." && d != "" {
		dirs = append(dirs, d)
	}
	if c.LogFile != "" {
		if d := filepath.Dir(c.LogFile); d != "." && d != "" {
			dirs = append(dirs, d)
		}
	}
	for _, d := range dirs {
		if merr := os.MkdirAll(d, 0o755); merr != nil {
			return fmt.Errorf("创建目录 %s 失败: %w", d, merr)
		}
	}
	return nil
}
