package db_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"gbi/server/internal/db"
	"gbi/server/internal/model"
)

func openAt(t *testing.T, path string) *gorm.DB {
	t.Helper()
	d, err := db.Open(path)
	require.NoError(t, err)
	return d
}

func versionOf(t *testing.T, d *gorm.DB) int {
	t.Helper()
	var m db.SchemaMigration
	err := d.Order("id").First(&m).Error
	if err != nil {
		return 0
	}
	return m.Version
}

// markerStep 测试步骤:建 marker 表(幂等),用于断言步骤是否执行。
func markerStep() db.SchemaStep {
	return db.SchemaStep{To: 1, Desc: "基线步骤:建 marker 表", Apply: func(tx *gorm.DB) error {
		return tx.Exec("CREATE TABLE IF NOT EXISTS marker (x TEXT)").Error
	}}
}

// TestUpgradeFreshDB 全新库:盖章=current、无备份目录、表已建、步骤执行、完整性 ok。
func TestUpgradeFreshDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fresh.db")
	d := openAt(t, path)

	require.NoError(t, db.Upgrade(d, path, []db.SchemaStep{markerStep()}))

	require.Equal(t, db.CurrentSchemaVersion, versionOf(t, d))
	require.True(t, d.Migrator().HasTable(&model.User{}))
	require.True(t, d.Migrator().HasTable("marker"), "步骤应执行")
	_, err := os.Stat(filepath.Join(dir, "backup"))
	require.True(t, os.IsNotExist(err), "全新库不应产生备份")
}

// TestUpgradePreVersioningExistingDB 无版本章的既有库(带业务数据):
// 先备份(快照含旧数据)→ 步骤执行 → 盖章 → 数据完好。
func TestUpgradePreVersioningExistingDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.db")
	d := openAt(t, path)
	require.NoError(t, db.Migrate(d)) // 模拟版本化之前的库
	require.NoError(t, d.Create(&model.Role{Code: "r1", Name: "R1"}).Error)
	require.Zero(t, versionOf(t, d))

	require.NoError(t, db.Upgrade(d, path, []db.SchemaStep{markerStep()}))

	require.Equal(t, db.CurrentSchemaVersion, versionOf(t, d))
	require.True(t, d.Migrator().HasTable("marker"))
	var n int64
	d.Model(&model.Role{}).Count(&n)
	require.Equal(t, int64(1), n, "业务数据必须完好")

	// 备份存在且为迁移前快照(可打开、含旧数据、无 marker 表)
	matches, err := filepath.Glob(filepath.Join(dir, "backup", "gbi-v0-*.db"))
	require.NoError(t, err)
	require.Len(t, matches, 1, "既有库必须产生备份")
	bd := openAt(t, matches[0])
	var bn int64
	bd.Model(&model.Role{}).Count(&bn)
	require.Equal(t, int64(1), bn, "备份应含迁移前数据")
	require.False(t, bd.Migrator().HasTable("marker"), "备份应为迁移前快照")
}

// TestUpgradeFastPath 版本相等:仅 AutoMigrate,不备份、不执行步骤。
func TestUpgradeFastPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "current.db")
	d := openAt(t, path)
	require.NoError(t, db.Migrate(d)) // 先建表才能写版本章
	require.NoError(t, d.Save(&db.SchemaMigration{ID: 1, Version: db.CurrentSchemaVersion}).Error)

	require.NoError(t, db.Upgrade(d, path, []db.SchemaStep{markerStep()}))
	require.False(t, d.Migrator().HasTable("marker"), "快速路径不执行步骤")
	_, err := os.Stat(filepath.Join(dir, "backup"))
	require.True(t, os.IsNotExist(err))
}

// TestUpgradeRefusesDowngrade 库版本高于程序 → 拒绝启动。
func TestUpgradeRefusesDowngrade(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "newer.db")
	d := openAt(t, path)
	require.NoError(t, db.Migrate(d))
	require.NoError(t, d.Save(&db.SchemaMigration{ID: 1, Version: db.CurrentSchemaVersion + 1}).Error)

	err := db.Upgrade(d, path, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "高于当前程序")
}

// TestUpgradeStepFailureRollback 步骤失败:事务回滚(半程表不残留)、版本章不写、备份已留。
func TestUpgradeStepFailureRollback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fail.db")
	d := openAt(t, path)
	require.NoError(t, db.Migrate(d))
	require.NoError(t, d.Create(&model.Role{Code: "keep", Name: "K"}).Error)

	bad := db.SchemaStep{To: 1, Desc: "坏步骤", Apply: func(tx *gorm.DB) error {
		if err := tx.Exec("CREATE TABLE tmp_half (x TEXT)").Error; err != nil {
			return err
		}
		return tx.Exec("THIS IS NOT SQL").Error
	}}
	err := db.Upgrade(d, path, []db.SchemaStep{bad})
	require.Error(t, err)
	require.Contains(t, err.Error(), "坏步骤")
	require.False(t, d.Migrator().HasTable("tmp_half"), "失败步骤的 schema 变更必须回滚")
	require.Zero(t, versionOf(t, d), "版本章不得写入")
	var n int64
	d.Model(&model.Role{}).Count(&n)
	require.Equal(t, int64(1), n)

	matches, gerr := filepath.Glob(filepath.Join(dir, "backup", "gbi-v0-*.db"))
	require.NoError(t, gerr)
	require.Len(t, matches, 1, "升级中止但备份应已保留")
}

// TestBackupPrune 备份超过 5 份时清理最旧。
func TestBackupPrune(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prune.db")
	d := openAt(t, path)
	require.NoError(t, db.Migrate(d))

	for i := 0; i < 7; i++ {
		_, err := db.Backup(d, path, 0)
		require.NoError(t, err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "backup", "gbi-v*.db"))
	require.NoError(t, err)
	require.Len(t, matches, 5, "只保留最近 5 份")
}
