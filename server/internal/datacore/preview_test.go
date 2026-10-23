package datacore

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPreviewRows(t *testing.T) {
	cols := []string{"a", "b"}
	require.Equal(t, "(无行)", previewRows(cols, nil, 2, 60))

	rows := [][]any{{"x", 1}, {"y", 2}, {"z", 3}}
	out := previewRows(cols, rows, 2, 60)
	require.Contains(t, out, "a=x,b=1")
	require.Contains(t, out, "a=y,b=2")
	require.NotContains(t, out, "a=z") // 只取前 2 行
	require.Contains(t, out, "(共3行)")

	// 长字段按 rune 截断且整体封顶
	long := strings.Repeat("中", 100)
	out = previewRows([]string{"c"}, [][]any{{long}}, 2, 10)
	require.Contains(t, out, "…")
	require.LessOrEqual(t, len([]rune(out)), 620)
}
