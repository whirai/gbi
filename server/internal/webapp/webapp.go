// Package webapp 提供生产形态的静态托管:前端构建产物、SPA fallback、
// 健康检查与调试状态(纯 net/http,不依赖 gin)。/api、/debug 由装配层分流给 gin。
// 静态内容统一经 fs.FS 提供:磁盘目录(开发/热替换)或内嵌资源(分发版)由装配层解析后注入。
package webapp

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gbi/server/internal/config"
	"gbi/server/internal/logger"
)

// Status 服务器状态(请求计数由 handler 原子累加,JSON 供 /debug/status 与 /health 复用)。
type Status struct {
	// 64 位原子访问字段置前,保证 32 位平台上的 8 字节对齐
	TotalRequests  int64
	StaticRequests int64
	StartTime      time.Time
	// Source 静态资源来源(disk|embedded),New 时确定、此后只读,Monitor 并发读取安全
	Source string

	fsys fs.FS // 静态资源源,JSON() 统计文件数用

	mu            sync.Mutex // 保护 LastError/LastErrorTime(Monitor 写、JSON 读)
	LastError     string
	LastErrorTime time.Time
}

type handler struct {
	cfg    *config.Config
	st     *Status
	fsys   fs.FS
	source string // disk | embedded
}

// New 返回静态/健康检查 handler 与共享状态。staticFS/source 由装配层(main)解析注入;
// 启动校验(SPA 目录/内置 index.html 缺失报错)在装配层执行。
func New(cfg *config.Config, staticFS fs.FS, source string) (http.Handler, *Status) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "webapp.New", "addr", cfg.Addr, "static_root", logger.Truncate(cfg.StaticRoot, 128), "spa", cfg.SPAMode, "source", source)
	defer func() {
		slog.Debug("方法离开", "fn", "webapp.New", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	st := &Status{StartTime: time.Now(), Source: source, fsys: staticFS}
	h := &handler{cfg: cfg, st: st, fsys: staticFS, source: source}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.health)
	mux.HandleFunc("/", h.static)
	return mux, st
}

// JSON 组装 /debug/status 的响应(不含任何敏感配置)。
func (s *Status) JSON(cfg *config.Config) map[string]any {
	start := time.Now()
	slog.Debug("方法进入", "fn", "Status.JSON", "static_root", logger.Truncate(cfg.StaticRoot, 128))
	defer func() {
		slog.Debug("方法离开", "fn", "Status.JSON", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	staticOK := s.Source == "embedded" || dirExists(cfg.StaticRoot)
	staticPath := cfg.StaticRoot
	if s.Source == "embedded" {
		staticPath = "embedded"
	}
	out := map[string]any{
		"server": map[string]any{
			"start_time":      s.StartTime.Format(time.RFC3339),
			"uptime":          time.Since(s.StartTime).String(),
			"total_requests":  atomic.LoadInt64(&s.TotalRequests),
			"static_requests": atomic.LoadInt64(&s.StaticRequests),
			"goroutines":      runtime.NumGoroutine(),
		},
		"static": map[string]any{
			"path":     staticPath,
			"exists":   staticOK,
			"source":   s.Source,
			"spa_mode": cfg.SPAMode,
			"file_count": func() int {
				entries, err := fs.ReadDir(s.fsys, ".")
				if err != nil {
					return 0
				}
				return len(entries)
			}(),
		},
		"config": map[string]any{
			"addr":        cfg.Addr,
			"static_root": cfg.StaticRoot,
			"cors_origin": cfg.CORSOrigin,
			"data_dir":    cfg.DataDir(),
		},
	}
	s.mu.Lock()
	lastError, lastErrorTime := s.LastError, s.LastErrorTime
	s.mu.Unlock()
	if lastError != "" {
		out["last_error"] = map[string]any{
			"message": lastError,
			"time":    lastErrorTime.Format(time.RFC3339),
		}
	}
	return out
}

// health 增强版健康检查:磁盘源目录丢失 → 503 degraded(内置源恒 ok)。
func (h *handler) health(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	// unexported 类型方法加包前缀,避免与其它包的 handler 重名
	slog.Debug("方法进入", "fn", "webapp.handler.health")
	defer func() {
		slog.Debug("方法离开", "fn", "webapp.handler.health", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	staticOK := h.source == "embedded" || dirExists(h.cfg.StaticRoot)
	staticPath := h.cfg.StaticRoot
	if h.source == "embedded" {
		staticPath = "embedded"
	}
	status, code := "healthy", http.StatusOK
	if !staticOK {
		status, code = "degraded", http.StatusServiceUnavailable
	}
	setCORS(w, h.cfg.CORSOrigin)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":         status,
		"time":           time.Now().Format(time.RFC3339),
		"uptime":         time.Since(h.st.StartTime).String(),
		"static_ok":      staticOK,
		"static_path":    staticPath,
		"static_source":  h.source,
		"total_requests": atomic.LoadInt64(&h.st.TotalRequests),
		"goroutines":     runtime.NumGoroutine(),
	})
}

// static 静态文件 + SPA fallback(磁盘/内置同一路径:fs.FS)。
func (h *handler) static(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "webapp.handler.static", "path", logger.Truncate(r.URL.Path, 128))
	defer func() {
		slog.Debug("方法离开", "fn", "webapp.handler.static", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	atomic.AddInt64(&h.st.TotalRequests, 1)
	setCORS(w, h.cfg.CORSOrigin)

	// URL 路径 → fs 路径:fs.FS 恒以 / 分隔;Clean 先归并 .. 片段,ValidPath 再拒绝非法名
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name != "" && fs.ValidPath(name) {
		if info, err := fs.Stat(h.fsys, name); err == nil && !info.IsDir() {
			if f, err := h.fsys.Open(name); err == nil {
				defer f.Close()
				atomic.AddInt64(&h.st.StaticRequests, 1)
				setCacheHeader(w, r.URL.Path)
				// 磁盘(os.File)与 embed 文件均实现 io.Seeker;万一不实现则整体读入兜底
				rs, _ := f.(io.ReadSeeker)
				if rs == nil {
					b, _ := fs.ReadFile(h.fsys, name)
					rs = bytes.NewReader(b)
				}
				http.ServeContent(w, r, name, info.ModTime(), rs)
				return
			}
		}
	}
	// 未命中/目录:SPA 模式回落 index.html,否则 404
	if h.cfg.SPAMode {
		if b, err := fs.ReadFile(h.fsys, "index.html"); err == nil {
			w.Header().Set("Cache-Control", "no-cache, must-revalidate")
			http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(b))
			return
		}
	}
	http.NotFound(w, r)
}

// setCacheHeader 静态资源长缓存;index.html 与其余不缓存。
func setCacheHeader(w http.ResponseWriter, path string) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "webapp.setCacheHeader", "path", logger.Truncate(path, 128))
	defer func() {
		slog.Debug("方法离开", "fn", "webapp.setCacheHeader", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	if strings.Contains(path, "/assets/") || strings.Contains(path, "/static/") ||
		strings.HasSuffix(path, ".css") || strings.HasSuffix(path, ".js") ||
		strings.HasSuffix(path, ".woff") || strings.HasSuffix(path, ".woff2") ||
		strings.HasSuffix(path, ".ttf") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")
}

// setCORS 与 api 包 corsMiddleware 使用同一头集(与 webserver.go 一致)。
func setCORS(w http.ResponseWriter, origin string) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "webapp.setCORS", "origin", logger.Truncate(origin, 64))
	defer func() {
		slog.Debug("方法离开", "fn", "webapp.setCORS", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Max-Age", "86400")
}

func dirExists(path string) bool {
	start := time.Now()
	slog.Debug("方法进入", "fn", "webapp.dirExists", "path", logger.Truncate(path, 128))
	defer func() {
		slog.Debug("方法离开", "fn", "webapp.dirExists", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// Monitor 后台监控:仅磁盘源检查目录丢失(内置源不会丢)+ goroutine 超阈告警(30s 周期)。
func Monitor(st *Status, staticRoot string) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "webapp.Monitor", "static_root", logger.Truncate(staticRoot, 128), "source", st.Source)
	defer func() {
		// 常驻循环,离开行仅在 ticker 退出(当前实现不会)时打印
		slog.Debug("方法离开", "fn", "webapp.Monitor", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if st.Source == "disk" && !dirExists(staticRoot) {
			st.mu.Lock()
			st.LastError = "静态文件目录丢失"
			st.LastErrorTime = time.Now()
			st.mu.Unlock()
			slog.Warn("静态文件目录丢失", "static_root", staticRoot)
		}
		if n := runtime.NumGoroutine(); n > 1000 {
			slog.Warn("goroutine 数量过多", "count", n)
		}
	}
}
