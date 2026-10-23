package webapp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"gbi/server/internal/config"
	"gbi/server/internal/webapp"
)

// newTestWebapp 在临时目录建 dist(index.html + assets/app.js),以磁盘源构建 handler。
func newTestWebapp(t *testing.T, spaMode bool) (http.Handler, *webapp.Status) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>app</html>"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "assets"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("console.log(1)"), 0o644))

	cfg := &config.Config{StaticRoot: dir, SPAMode: spaMode, CORSOrigin: "*"}
	h, st := webapp.New(cfg, os.DirFS(dir), "disk")
	return h, st
}

// newEmbeddedWebapp 以 fstest.MapFS(内置源)构建 handler;磁盘路径不存在。
func newEmbeddedWebapp(t *testing.T, spaMode bool) (http.Handler, *webapp.Status) {
	t.Helper()
	cfg := &config.Config{StaticRoot: filepath.Join(t.TempDir(), "nope"), SPAMode: spaMode, CORSOrigin: "*"}
	fsys := fstest.MapFS{
		"index.html":    {Data: []byte("<html>embedded</html>")},
		"assets/app.js": {Data: []byte("console.log(2)")},
	}
	h, st := webapp.New(cfg, fsys, "embedded")
	return h, st
}

func doReq(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", path, nil)
	h.ServeHTTP(w, req)
	return w
}

func TestStaticFileServed(t *testing.T) {
	h, _ := newTestWebapp(t, true)
	w := doReq(t, h, "/")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "app")
	require.Equal(t, "no-cache, must-revalidate", w.Header().Get("Cache-Control"))
}

func TestAssetImmutableCache(t *testing.T) {
	h, _ := newTestWebapp(t, true)
	w := doReq(t, h, "/assets/app.js")
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "public, max-age=31536000, immutable", w.Header().Get("Cache-Control"))
}

func TestSPAFallbackDeepLink(t *testing.T) {
	h, _ := newTestWebapp(t, true)
	w := doReq(t, h, "/studio?page=1")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "app") // 深链回落 index.html
}

func TestNoSPAFallback404(t *testing.T) {
	h, _ := newTestWebapp(t, false)
	w := doReq(t, h, "/studio")
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestStaticCORSHeaders(t *testing.T) {
	h, _ := newTestWebapp(t, true)
	w := doReq(t, h, "/")
	require.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	require.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
}

// TestTraversalRejected 目录穿越路径不得命中根目录之外的文件。
func TestTraversalRejected(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>app</html>"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(dir), "secret.txt"), []byte("SECRET-DATA"), 0o644))

	cfg := &config.Config{StaticRoot: dir, SPAMode: false, CORSOrigin: "*"}
	h, _ := webapp.New(cfg, os.DirFS(dir), "disk")
	for _, p := range []string{"/../secret.txt", "/..%2Fsecret.txt", "/a/../../secret.txt"} {
		w := doReq(t, h, p)
		require.NotContains(t, w.Body.String(), "SECRET-DATA", "路径 %s 不得穿越", p)
	}
}

func TestHealth(t *testing.T) {
	h, _ := newTestWebapp(t, true)
	w := doReq(t, h, "/health")
	require.Equal(t, http.StatusOK, w.Code)
	body := map[string]any{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "healthy", body["status"])
	require.Equal(t, true, body["static_ok"])
	require.Equal(t, "disk", body["static_source"])

	// 磁盘源且目录丢失 → degraded 503
	cfg2 := &config.Config{StaticRoot: filepath.Join(t.TempDir(), "nope"), SPAMode: false, CORSOrigin: "*"}
	h3, _ := webapp.New(cfg2, os.DirFS(cfg2.StaticRoot), "disk")
	w = doReq(t, h3, "/health")
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	body = map[string]any{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "degraded", body["status"])
}

// TestEmbeddedServing 内置源:文件命中/SPA 回落/健康始终 ok(内置不会丢)。
func TestEmbeddedServing(t *testing.T) {
	h, st := newEmbeddedWebapp(t, true)

	w := doReq(t, h, "/assets/app.js")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "console.log(2)")
	require.Equal(t, "public, max-age=31536000, immutable", w.Header().Get("Cache-Control"))

	w = doReq(t, h, "/studio/deep")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "embedded")

	w = doReq(t, h, "/health")
	require.Equal(t, http.StatusOK, w.Code)
	body := map[string]any{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, true, body["static_ok"])
	require.Equal(t, "embedded", body["static_source"])

	require.Equal(t, "embedded", st.Source)
}

func TestStatusJSONContainsCounters(t *testing.T) {
	h, st := newTestWebapp(t, true)
	doReq(t, h, "/")
	doReq(t, h, "/assets/app.js")

	j := st.JSON(&config.Config{StaticRoot: "x", SPAMode: true, CORSOrigin: "*", Addr: ":8080"})
	server := j["server"].(map[string]any)
	require.Equal(t, int64(2), server["total_requests"])
	// 仅 /assets/app.js 为真实文件命中;根路径 / 走 SPA fallback(index.html),不计入静态请求。
	require.Equal(t, int64(1), server["static_requests"])
	stat := j["static"].(map[string]any)
	require.Equal(t, "disk", stat["source"])
	cfg := j["config"].(map[string]any)
	require.Equal(t, ":8080", cfg["addr"])
	// 不泄漏敏感键
	_, hasKey := cfg["llm_api_key"]
	require.False(t, hasKey)
}

// TestStatusJSONEmbeddedSource 内置源时 static.path 显示 "embedded" 且 exists=true。
func TestStatusJSONEmbeddedSource(t *testing.T) {
	_, st := newEmbeddedWebapp(t, true)
	j := st.JSON(&config.Config{StaticRoot: "x", SPAMode: true, CORSOrigin: "*", Addr: ":8080"})
	stat := j["static"].(map[string]any)
	require.Equal(t, "embedded", stat["source"])
	require.Equal(t, "embedded", stat["path"])
	require.Equal(t, true, stat["exists"])
}
