package logger_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gbi/server/internal/logger"
)

func TestTruncate(t *testing.T) {
	require.Equal(t, "", logger.Truncate("abc", 0))
	require.Equal(t, "abc", logger.Truncate("abc", 5))
	require.Equal(t, "abc", logger.Truncate("abc", 3))

	// rune 安全:按字符截断,不产生半个汉字
	require.Equal(t, "这是一…(截断)", logger.Truncate("这是一段很长的中文文本", 3))
	require.Equal(t, "ok", logger.Truncate("ok", 2))
}
