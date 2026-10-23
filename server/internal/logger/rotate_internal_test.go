// 包内测试:通过 renameFile 注入轮转失败,验证句柄恢复/降级(不 wedge)。
package logger

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRotateFailureReopensFile rename 失败后必须重开原文件:后续 Write 不得永久失败。
func TestRotateFailureReopensFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gbi.log")
	w, err := NewRotateWriter(path, 10) // 10B 阈值
	require.NoError(t, err)

	orig := renameFile
	renameFile = func(_, _ string) error { return errors.New("rename 故障(注入)") }
	t.Cleanup(func() { renameFile = orig })

	// 第一次写入不触发轮转
	_, err = w.Write([]byte("hello\n"))
	require.NoError(t, err)

	// 第二次写入触发轮转 → rename 失败 → 重开原文件,本条继续写入
	_, err = w.Write([]byte("hello\n"))
	require.NoError(t, err)
	require.Equal(t, int64(1), w.dropped) // 轮转失败计一次丢弃,但内容未丢

	// 第三次写入再次触发轮转且再次失败,也不得返回 "file already closed"
	_, err = w.Write([]byte("tail\n"))
	require.NoError(t, err)

	require.NoError(t, w.Close())
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "hello\nhello\ntail\n", string(b))
}

// TestRotateFailureDegradesWithoutPanic rename 与重开均失败 → 降级丢弃,后续 Write 返回错误而非 panic。
func TestRotateFailureDegradesWithoutPanic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gbi.log")
	w, err := NewRotateWriter(path, 10)
	require.NoError(t, err)

	// hook 在 rotate 已关闭句柄之后执行:移除原文件并以同名目录占位,使重开必然失败
	orig := renameFile
	renameFile = func(oldpath, _ string) error {
		_ = os.Remove(oldpath)
		_ = os.Mkdir(oldpath, 0o755)
		return errors.New("rename 故障(注入)")
	}
	t.Cleanup(func() { renameFile = orig })

	_, err = w.Write([]byte("hello-world\n")) // 12B > 阈值 → 触发轮转 → rename 失败 → 重开失败 → 降级
	require.Error(t, err)
	require.Nil(t, w.f)

	// 降级后继续 Write 必须返回错误且不 panic(修复前:nil 句柄会 panic/wedge)
	_, err = w.Write([]byte("again\n"))
	require.Error(t, err)
	require.NoError(t, w.Close())
}
