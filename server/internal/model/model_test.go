package model_test

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"gbi/server/internal/db"
	"gbi/server/internal/model"
)

func TestMarshalPageConfigDefaults(t *testing.T) {
	s, err := model.MarshalPageConfig(model.PageConfig{})
	require.NoError(t, err)
	require.Contains(t, s, `"version":1`)
	require.Contains(t, s, `"layout":"fluid-grid"`)
	require.Contains(t, s, `"charts":[]`)
}

func TestMarshalPageConfigValidation(t *testing.T) {
	ok := model.PageConfig{Charts: []model.ChartConfig{{
		ID: "c1", Title: "t", Width: "1/2", Type: "line",
		SQL: "SELECT 1", Subjects: []uint{1}, Spec: map[string]any{"type": "line"},
	}}}
	_, err := model.MarshalPageConfig(ok)
	require.NoError(t, err)

	noID := ok
	noID.Charts = []model.ChartConfig{{Title: "t", Width: "full", SQL: "SELECT 1"}}
	_, err = model.MarshalPageConfig(noID)
	require.Error(t, err)

	noSQL := ok
	noSQL.Charts = []model.ChartConfig{{ID: "c1", Width: "full"}}
	_, err = model.MarshalPageConfig(noSQL)
	require.Error(t, err)

	badWidth := ok
	badWidth.Charts = []model.ChartConfig{{ID: "c1", Width: "1/4", SQL: "SELECT 1"}}
	_, err = model.MarshalPageConfig(badWidth)
	require.Error(t, err)

	badLayout := ok
	badLayout.Layout = "free"
	_, err = model.MarshalPageConfig(badLayout)
	require.Error(t, err)
}

func TestMigratePhase3Models(t *testing.T) {
	d, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "t.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	// 释放 SQLite 文件句柄,避免 Windows 上 t.TempDir 清理因文件被占用失败
	// (与 db.Open 的 SetMaxIdleConns(0) 同理)。
	sqlDB, cerr := d.DB()
	require.NoError(t, cerr)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.Migrate(d))
	for _, m := range []any{&model.Page{}, &model.PageVersion{}, &model.Menu{}, &model.MenuItem{}} {
		require.True(t, d.Migrator().HasTable(m))
	}
}

func TestMigratePhase4Models(t *testing.T) {
	d, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "t.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	sqlDB, cerr := d.DB()
	require.NoError(t, cerr)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.Migrate(d))
	require.True(t, d.Migrator().HasTable(&model.Conversation{}))
	require.True(t, d.Migrator().HasTable(&model.Message{}))
}
