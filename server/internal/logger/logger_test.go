package logger_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"gbi/server/internal/auth"
	"gbi/server/internal/config"
	"gbi/server/internal/logger"
)

// captureSlog 把全局 slog 输出重定向到 buffer,返回恢复函数。
func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &buf
}

func TestParseLevel(t *testing.T) {
	require.Equal(t, slog.LevelDebug, logger.ParseLevel("debug"))
	require.Equal(t, slog.LevelDebug, logger.ParseLevel("DEBUG"))
	require.Equal(t, slog.LevelWarn, logger.ParseLevel("warn"))
	require.Equal(t, slog.LevelError, logger.ParseLevel("error"))
	require.Equal(t, slog.LevelInfo, logger.ParseLevel("nonsense")) // 非法回退
	require.Equal(t, slog.LevelInfo, logger.ParseLevel(""))         // 空回退
}

func TestMask(t *testing.T) {
	in := `{"api_key":"sk-12345","password":"admin123","name":"demo","token":"abc"}`
	out := logger.Mask(in)
	require.NotContains(t, out, "sk-12345")
	require.NotContains(t, out, "admin123")
	require.NotContains(t, out, `"token":"abc"`)
	require.Contains(t, out, "***")
	require.Contains(t, out, `"name":"demo"`) // 非敏感键不动

	// 键名包含敏感子串(非精确匹配)同样脱敏
	compound := `{"api_token":"tk-abc","access_token":"at-xyz","my_password":"pw-123"}`
	out2 := logger.Mask(compound)
	require.NotContains(t, out2, "tk-abc")
	require.NotContains(t, out2, "at-xyz")
	require.NotContains(t, out2, "pw-123")
	require.Contains(t, out2, "***")
}

func TestRotateWriter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gbi.log")
	w, err := logger.NewRotateWriter(path, 1024) // 1KB 阈值
	require.NoError(t, err)

	chunk := []byte(strings.Repeat("a", 600))
	_, err = w.Write(chunk) // 600B
	require.NoError(t, err)
	_, err = w.Write(chunk) // 600+600=1200 > 1KB → 先轮转,第二个 chunk 落入新文件
	require.NoError(t, err)
	_, err = w.Write([]byte("tail")) // 新文件 600B + tail
	require.NoError(t, err)
	require.NoError(t, w.Close())

	// 当前文件 = 第二个 chunk + tail;轮转文件 = 第一个 chunk
	cur, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(append(chunk, []byte("tail")...)), string(cur))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	var rotatedPath string
	for _, e := range entries {
		if e.Name() != "gbi.log" {
			rotatedPath = filepath.Join(dir, e.Name())
		}
	}
	require.True(t, strings.HasPrefix(filepath.Base(rotatedPath), "gbi-"))
	require.True(t, strings.HasSuffix(rotatedPath, ".log"))
	rot, err := os.ReadFile(rotatedPath)
	require.NoError(t, err)
	require.Equal(t, string(chunk), string(rot))
}

func TestInitFileJSONAndLevelFilter(t *testing.T) {
	// 注意:Windows 上 slog JSON handler 持有的文件句柄在测试结束时仍打开,
	// t.TempDir 的 RemoveAll 会因文件被占用而失败,故手动建目录并容错清理
	// (句柄由 logger.Init 持有,随进程退出释放)。
	dir, err := os.MkdirTemp("", "gbi-logger-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "gbi.log")
	logger.Init(&config.Config{LogLevel: "info", LogMaxSize: 10, LogFile: path})

	slog.Debug("调试信息", "k", "v") // info 级 → 不落文件
	slog.Info("业务里程碑", "subject_id", 1)

	b, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(b), "调试信息")
	require.Contains(t, string(b), "业务里程碑")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(b), &entry))
	require.Equal(t, float64(1), entry["subject_id"]) // JSON 格式
	require.Equal(t, "INFO", entry["level"])
}

func TestGinMiddlewareFieldsAndNoAuth(t *testing.T) {
	buf := captureSlog(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(logger.GinMiddleware(), gin.Recovery())
	r.GET("/x", func(c *gin.Context) {
		c.Set(auth.CtxUserID, uint(7))
		c.Set(auth.CtxRoleCode, "admin")
		c.Status(http.StatusTeapot)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x?page=2", nil)
	req.Header.Set("Authorization", "Bearer SUPER-SECRET-TOKEN")
	r.ServeHTTP(w, req)

	out := buf.String()
	require.Contains(t, out, "status=418")
	require.Contains(t, out, "uid=7")
	require.Contains(t, out, "role=admin")
	require.Contains(t, out, "path=/x")
	require.NotContains(t, out, "SUPER-SECRET-TOKEN") // 绝无 Authorization 值
	require.NotContains(t, out, "page=2")             // info 级不带 query
}
