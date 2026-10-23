package main

import (
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gbi/server/internal/api"
	"gbi/server/internal/assets"
	"gbi/server/internal/config"
	"gbi/server/internal/db"
	"gbi/server/internal/logger"
	"gbi/server/internal/rbac"
	"gbi/server/internal/service"
	"gbi/server/internal/webapp"
)

func main() {
	start := time.Now()
	// 进入行早于 logger.Init(需先 Load 配置),默认 handler 下不落盘,属预期。
	slog.Debug("方法进入", "fn", "gbi.main")
	defer func() {
		slog.Debug("方法离开", "fn", "gbi.main", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	cfg := config.Load()

	// 首次运行自举:目录可能尚不存在(logger.Init 依赖日志目录)
	if err := config.EnsureRuntimeDirs(cfg); err != nil {
		log.Fatal(err)
	}

	logger.Init(cfg)
	if cfg.LogLevel == "" || !strings.EqualFold(strings.TrimSpace(cfg.LogLevel), "debug") {
		gin.SetMode(gin.ReleaseMode)
	}

	// 安全告警(不 fail-fast,兼容 e2e/开发默认值):默认密钥公开可知,任何人都能伪造令牌
	if cfg.JWTSecret == config.DefaultJWTSecret {
		slog.Warn("GBI_JWT_SECRET 未配置,正在使用公开的默认密钥!生产环境务必设置强随机密钥,否则任意用户可伪造管理员令牌")
	}

	// 静态源解析:磁盘目录存在优先,否则内置(分发版);SPA 模式要求解析源含 index.html
	staticFS, staticSource := assets.Resolve(cfg.StaticRoot, assets.DistFS)
	if cfg.SPAMode {
		if _, err := fs.Stat(staticFS, "index.html"); err != nil {
			log.Fatalf("SPA 模式要求前端产物:磁盘 %q 与内置均无 index.html。请用 scripts/build-release.sh 构建分发版,或 cd web && npm run build 并设置 GBI_STATIC_ROOT", cfg.StaticRoot)
		}
	}

	d, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	// 版本化升级:版本章检测 →(既有库)备份 → AutoMigrate → 步骤 → 完整性校验
	if err := db.Upgrade(d, cfg.DBPath, db.SchemaSteps); err != nil {
		log.Fatal(err)
	}
	if err := db.Seed(d); err != nil {
		log.Fatal(err)
	}
	e, err := rbac.NewEnforcer(d)
	if err != nil {
		log.Fatal(err)
	}
	if err := db.SeedRBAC(d, e); err != nil {
		log.Fatal(err)
	}

	// LLM 配置:库中无 provider 时播种(显式 env 迁入;否则出厂默认),落库成功后再清理 .env 过时键
	lc := service.NewLLMConfigService(d)
	seed := service.SeedInput{
		ProviderName: service.DefaultProviderName, BaseURL: service.DefaultBaseURL,
		ModelName: service.DefaultModelName, ContextWindow: service.DefaultContextWindow, MaxTokens: service.DefaultMaxTokens,
	}
	explicit := os.Getenv("GBI_LLM_BASE_URL") != "" || os.Getenv("GBI_LLM_API_KEY") != "" ||
		os.Getenv("GBI_LLM_MODEL") != "" || os.Getenv("GBI_LLM_MAX_TOKENS") != ""
	if explicit {
		if v := os.Getenv("GBI_LLM_BASE_URL"); v != "" {
			seed.BaseURL = v
		}
		if v := os.Getenv("GBI_LLM_API_KEY"); v != "" {
			seed.APIKey = v
		}
		if v := os.Getenv("GBI_LLM_MODEL"); v != "" {
			seed.ModelName = v
		}
		if v := os.Getenv("GBI_LLM_MAX_TOKENS"); v != "" {
			if n, cerr := strconv.Atoi(v); cerr == nil && n > 0 {
				seed.MaxTokens = n
			}
		}
		slog.Info("检测到 .env/环境变量中的 LLM 配置,将迁入数据库", "model", seed.ModelName, "base_url", seed.BaseURL, "has_key", seed.APIKey != "")
	}
	usable, lerr := lc.EnsureSeeded(seed)
	if lerr != nil {
		log.Fatalf("LLM 配置播种失败: %v", lerr)
	}
	if usable {
		if removed, cerr := config.CleanupObsoleteLLMEnv(cfg.EnvFile); cerr != nil {
			log.Fatalf("清理 .env 过时 LLM 配置失败: %v", cerr)
		} else if len(removed) > 0 {
			slog.Info("已从 .env 清理过时 LLM 配置(原文件已备份)", "keys", strings.Join(removed, ","), "file", cfg.EnvFile)
		}
	} else if explicit {
		slog.Warn("检测到 .env LLM 配置,但数据库中的当前模型尚不可用(缺 API Key 或无模型):本次不迁移、不清理 .env;请在控制台「大模型设置」配置后再处理")
	}
	// 启动可观测性:当前模型不可用时明确告警
	if _, _, cerr := lc.Current(); cerr != nil || !usable {
		slog.Warn("当前模型未配置或缺少 API Key,studio 对话不可用(请在控制台「大模型设置」配置)")
	}

	r := api.NewRouter(d, e, cfg)
	webHandler, status := webapp.New(cfg, staticFS, staticSource)
	api.RegisterDebugRoutes(r, []byte(cfg.JWTSecret), func() map[string]any {
		return status.JSON(cfg)
	})

	// 顶层分发:/api 与 /debug 走 gin,其余走静态托管
	mux := http.NewServeMux()
	mux.Handle("/api/", r)
	mux.Handle("/debug/", r)
	mux.Handle("/", webHandler)

	server := &http.Server{
		Addr:         cfg.Addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		// WriteTimeout 不设(0):studio SSE 流式响应可长达分钟级,整体写截止会截断长流
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	go webapp.Monitor(status, cfg.StaticRoot)

	slog.Info("GBI server 启动", "addr", cfg.Addr, "db", cfg.DBPath, "static", cfg.StaticRoot, "spa", cfg.SPAMode)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
