// Package logger 基于标准库 log/slog 的分级日志:控制台文本 + 可选文件 JSON(按大小轮转)。
// Init 后 slog.SetDefault,全库直接用 slog.Debug/Info/Warn/Error。
package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"gbi/server/internal/auth"
	"gbi/server/internal/config"
)

// level 全局级别(LevelVar 便于测试与未来动态调级)。
var level = &slog.LevelVar{}

// ParseLevel 解析级别字符串;非法/空回退 Info。
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Init 构建全局 slog:控制台文本(stderr)+ 文件 JSON(GBI_LOG_FILE 非空时,轮转)。
// 所有失败路径内部降级,不返回错误(文件打不开→warn 仅控制台)。
func Init(cfg *config.Config) {
	lv := ParseLevel(cfg.LogLevel)
	if cfg.LogLevel != "" && lv == slog.LevelInfo && !strings.EqualFold(strings.TrimSpace(cfg.LogLevel), "info") {
		// 非法值:以 info 生效,先以默认 logger 告警一次
		slog.Warn("GBI_LOG_LEVEL 非法,回退 info", "value", cfg.LogLevel)
	}
	level.Set(lv)

	opts := &slog.HandlerOptions{Level: level}
	handlers := []slog.Handler{slog.NewTextHandler(os.Stderr, opts)}
	if cfg.LogFile != "" {
		w, err := NewRotateWriter(cfg.LogFile, int64(cfg.LogMaxSize)*1024*1024)
		if err != nil {
			slog.Warn("日志文件不可用,仅控制台输出", "file", cfg.LogFile, "err", err)
		} else {
			handlers = append(handlers, slog.NewJSONHandler(w, opts))
		}
	}
	slog.SetDefault(slog.New(newMultiHandler(handlers)))
}

// multiHandler 把每条记录扇出到多个 handler(文本+JSON)。
type multiHandler struct{ hs []slog.Handler }

func newMultiHandler(hs []slog.Handler) slog.Handler {
	if len(hs) == 1 {
		return hs[0]
	}
	return &multiHandler{hs: hs}
}

func (m *multiHandler) Enabled(ctx context.Context, l slog.Level) bool {
	for _, h := range m.hs {
		if h.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m.hs {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	hs := make([]slog.Handler, len(m.hs))
	for i, h := range m.hs {
		hs[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{hs: hs}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	hs := make([]slog.Handler, len(m.hs))
	for i, h := range m.hs {
		hs[i] = h.WithGroup(name)
	}
	return &multiHandler{hs: hs}
}

// RotateWriter 按大小轮转的写入器:达到阈值时把当前文件重命名为
// <name去后缀>-<yyyyMMdd-HHmmss>.log 后新建。并发安全。
type RotateWriter struct {
	mu      sync.Mutex
	path    string
	maxSize int64
	size    int64
	f       *os.File
	dropped int64 // 写盘失败的丢弃计数(仅用于监控,不暴露接口)
}

func NewRotateWriter(path string, maxSize int64) (*RotateWriter, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return &RotateWriter{path: path, maxSize: maxSize, size: info.Size(), f: f}, nil
}

// renameFile 便于测试注入 rename 故障。
var renameFile = os.Rename

func (w *RotateWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil { // 此前轮转失败已降级:丢弃本条,绝不 panic
		w.dropped++
		return 0, os.ErrClosed
	}
	if w.maxSize > 0 && w.size+int64(len(p)) > w.maxSize {
		if err := w.rotate(); err != nil {
			w.dropped++
			if w.f == nil { // 轮转失败且句柄无法恢复 → 本条丢弃
				return 0, err
			}
			// 轮转失败但句柄已重开:继续写入当前文件,不丢日志
		}
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	if err != nil {
		w.dropped++
	}
	return n, err
}

// rotate 关闭当前文件 → 重命名为带时间戳的历史文件 → 重开新文件。
// 失败路径必须恢复可用句柄(重开原文件)或置空降级,绝不留下已关闭的 f,
// 否则后续 Write 会反复在已关闭句柄上写失败,文件日志永久 wedge。
func (w *RotateWriter) rotate() error {
	if err := w.f.Close(); err != nil {
		w.recoverHandle()
		return err
	}
	base := strings.TrimSuffix(w.path, filepath.Ext(w.path))
	rotated := fmt.Sprintf("%s-%s.log", base, time.Now().Format("20060102-150405"))
	if err := renameFile(w.path, rotated); err != nil {
		w.recoverHandle()
		return err
	}
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		w.recoverHandle()
		return err
	}
	w.f, w.size = f, 0
	return nil
}

// recoverHandle 轮转失败后的恢复:重开原路径文件继续追加(同一文件,size 保持);
// 重开也失败时置 f=nil 降级为丢弃写入(控制台 handler 不受影响,仍可观测)。
func (w *RotateWriter) recoverHandle() {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		w.f = f
		return
	}
	slog.Warn("日志文件轮转失败且重开失败,文件写入降级丢弃", "path", w.path, "err", err)
	w.f = nil
}

func (w *RotateWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return nil
	}
	err := w.f.Close()
	w.f = nil
	return err
}

// maskRe 匹配键名包含敏感子串(password/api_key/token/secret,大小写不敏感)的值。
var maskRe = regexp.MustCompile(`(?i)("[a-z0-9_]*(?:password|api_key|token|secret)[a-z0-9_]*"\s*[:=]\s*")([^"]*)(")`)

// Mask 把文本中敏感键的值替换为 ***。
func Mask(s string) string {
	return maskRe.ReplaceAllString(s, `${1}***${3}`)
}

// Truncate 为日志预览截断文本:≤n 个 rune 原样返回,超出截断并追加标记(rune 安全)。
// 注意:config 与 llm 包不能 import 本包(logger→config→llm 成环),那两处用本地等价实现。
func Truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…(截断)"
}

// GinMiddleware 访问日志:method/path/status/latency_ms/uid/role;
// 绝不记录 Authorization 头与请求体;4xx→Warn、5xx→Error、其余→Info;
// debug 级附加原始 query 参数。
func GinMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		status := c.Writer.Status()
		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", status,
			"latency_ms", time.Since(start).Milliseconds(),
			"uid", c.GetUint(auth.CtxUserID),
			"role", c.GetString(auth.CtxRoleCode),
		}
		if level.Level() == slog.LevelDebug && c.Request.URL.RawQuery != "" {
			attrs = append(attrs, "query", c.Request.URL.RawQuery)
		}
		switch {
		case status >= 500:
			slog.Error("http请求", attrs...)
		case status >= 400:
			slog.Warn("http请求", attrs...)
		default:
			slog.Info("http请求", attrs...)
		}
	}
}
