package service_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"gbi/server/internal/db"
	"gbi/server/internal/service"
)

func TestRenderErrorServiceFlow(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	require.NoError(t, db.Migrate(d))
	svc := service.NewRenderErrorService(d)

	require.NoError(t, svc.Report(1, "c1", "Unknown Component"))
	require.NoError(t, svc.Report(1, "c2", "invalid format"))

	unseen, err := svc.Unseen(1)
	require.NoError(t, err)
	require.Len(t, unseen, 2)
	require.Equal(t, "c1", unseen[0].ChartID)

	var ids []uint
	for _, e := range unseen {
		ids = append(ids, e.ID)
	}
	require.NoError(t, svc.MarkSeen(ids))

	again, err := svc.Unseen(1)
	require.NoError(t, err)
	require.Empty(t, again)
}

// TestRenderErrorTruncatesFields 长度约束:chart_id ≤64、message ≤2000 rune,
// 防止可见页面的普通用户借上报通道做无界存储放大。
func TestRenderErrorTruncatesFields(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	require.NoError(t, db.Migrate(d))
	svc := service.NewRenderErrorService(d)

	require.NoError(t, svc.Report(1, strings.Repeat("c", 100), strings.Repeat("错", 3000)))

	unseen, err := svc.Unseen(1)
	require.NoError(t, err)
	require.Len(t, unseen, 1)
	require.Len(t, []rune(unseen[0].ChartID), 64)
	require.Len(t, []rune(unseen[0].Message), 2000)

	// 正常长度不截断
	require.NoError(t, svc.Report(1, "c1", "Unknown Component"))
	unseen, _ = svc.Unseen(1)
	require.Equal(t, "c1", unseen[1].ChartID)
	require.Equal(t, "Unknown Component", unseen[1].Message)
}
