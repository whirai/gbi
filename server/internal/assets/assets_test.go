package assets_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"gbi/server/internal/assets"
)

func TestResolveDiskPreferred(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("disk"), 0o644))
	embedded := fstest.MapFS{"index.html": {Data: []byte("embedded")}}

	fsys, source := assets.Resolve(dir, embedded)
	require.Equal(t, "disk", source)
	b, err := fs.ReadFile(fsys, "index.html")
	require.NoError(t, err)
	require.Equal(t, "disk", string(b))
}

func TestResolveEmbeddedFallback(t *testing.T) {
	embedded := fstest.MapFS{"index.html": {Data: []byte("embedded")}}

	// 目录不存在 → 内置
	fsys, source := assets.Resolve(filepath.Join(t.TempDir(), "nope"), embedded)
	require.Equal(t, "embedded", source)

	// 空配置 → 内置
	fsys, source = assets.Resolve("", embedded)
	require.Equal(t, "embedded", source)

	b, err := fs.ReadFile(fsys, "index.html")
	require.NoError(t, err)
	require.Equal(t, "embedded", string(b))
}

// TestEmbeddedFSRooted 内嵌 FS 根化且非空(占位 .gitkeep 保证裸编译可过;
// 分发构建后为真实产物,断言对两种情况都成立)。
func TestEmbeddedFSRooted(t *testing.T) {
	for name, fsys := range map[string]fs.FS{"dist": assets.DistFS, "skills": assets.SkillsFS} {
		entries, err := fs.ReadDir(fsys, ".")
		require.NoError(t, err, "%s 根目录应可读", name)
		require.NotEmpty(t, entries, "%s 根目录不应为空", name)
	}
}
