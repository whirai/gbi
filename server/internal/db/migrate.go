package db

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"

	"gbi/server/internal/model"
)

// SchemaMigration 版本章:单行(ID=1)记录当前 schema 版本。
type SchemaMigration struct {
	ID        uint `gorm:"primaryKey"`
	Version   int
	UpdatedAt time.Time
}

// CurrentSchemaVersion 程序期望的 schema 版本。破坏性/数据级变更时:
// 在 SchemaSteps 追加条目并递增此常量;历史步骤永不删除。
const CurrentSchemaVersion = 1

// SchemaStep 单个迁移步骤:版本从 To-1 迁到 To,Apply 在事务中执行,须尽量幂等(失败重试安全)。
type SchemaStep struct {
	To    int
	Desc  string
	Apply func(tx *gorm.DB) error
}

// SchemaSteps 有序迁移注册表(按 To 升序;只增不改)。引入时为基线,无待办步骤。
var SchemaSteps = []SchemaStep{}

// Upgrade 启动期数据库升级:版本章检测 →(既有库)备份 → AutoMigrate →
// 有序步骤(单事务,含盖章)→ 完整性校验。返回 nil 表示库已就绪。
// 失败语义:步骤事务回滚,版本章停在旧值;备份失败中止升级。
func Upgrade(d *gorm.DB, dbPath string, steps []SchemaStep) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "db.Upgrade", "db_path", dbPath, "current_version", CurrentSchemaVersion)
	defer func() {
		slog.Debug("方法离开", "fn", "db.Upgrade", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()

	recorded, err := readSchemaVersion(d)
	if err != nil {
		return fmt.Errorf("读取数据库版本失败: %w", err)
	}
	slog.Info("数据库版本检查", "recorded", recorded, "current", CurrentSchemaVersion)

	if recorded > CurrentSchemaVersion {
		return fmt.Errorf("数据库 schema 版本(v%d)高于当前程序(v%d):请使用新版程序,或从 data/backup/ 恢复备份", recorded, CurrentSchemaVersion)
	}
	if recorded == CurrentSchemaVersion {
		// 快速路径:保持既有行为(AutoMigrate 幂等加列),不备份、不执行步骤
		return Migrate(d)
	}

	// 升级路径。既有库(已有业务表)先备份,保证可回滚;本次新建的空文件无数据可备
	if d.Migrator().HasTable(&model.User{}) {
		backupPath, berr := Backup(d, dbPath, recorded)
		if berr != nil {
			return fmt.Errorf("迁移前备份失败,已中止升级: %w", berr)
		}
		slog.Info("迁移前备份完成", "backup", backupPath, "from_version", recorded)
	}
	if err := Migrate(d); err != nil {
		return err
	}
	if err := d.Transaction(func(tx *gorm.DB) error {
		for _, s := range steps {
			if s.To <= recorded || s.To > CurrentSchemaVersion {
				continue
			}
			slog.Info("执行迁移步骤", "to", s.To, "desc", s.Desc)
			if aerr := s.Apply(tx); aerr != nil {
				return fmt.Errorf("迁移步骤 v%d(%s) 失败: %w", s.To, s.Desc, aerr)
			}
		}
		return writeSchemaVersion(tx, CurrentSchemaVersion)
	}); err != nil {
		return err
	}
	if err := integrityCheck(d); err != nil {
		return err
	}
	slog.Info("数据库升级完成", "from", recorded, "to", CurrentSchemaVersion)
	return nil
}

// readSchemaVersion 读版本章;表不存在/无行 → 0(全新库或版本化之前的库)。
func readSchemaVersion(d *gorm.DB) (v int, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "db.readSchemaVersion")
	defer func() {
		slog.Debug("方法离开", "fn", "db.readSchemaVersion", "elapsed_ms", time.Since(start).Milliseconds(), "version", v, "err", err)
	}()
	if !d.Migrator().HasTable(&SchemaMigration{}) {
		return 0, nil
	}
	var m SchemaMigration
	if ferr := d.Order("id").First(&m).Error; ferr != nil {
		if errors.Is(ferr, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, ferr
	}
	return m.Version, nil
}

// writeSchemaVersion 在事务内盖版本章(单行 upsert)。
func writeSchemaVersion(tx *gorm.DB, v int) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "db.writeSchemaVersion", "version", v)
	defer func() {
		slog.Debug("方法离开", "fn", "db.writeSchemaVersion", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	return tx.Save(&SchemaMigration{ID: 1, Version: v, UpdatedAt: time.Now()}).Error
}

// integrityCheck 迁移后结构完整性校验(PRAGMA integrity_check 期望 ok);
// 失败不自动恢复覆盖,由运维从 data/backup/ 处理。
func integrityCheck(d *gorm.DB) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "db.integrityCheck")
	defer func() {
		slog.Debug("方法离开", "fn", "db.integrityCheck", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var res string
	if serr := d.Raw("PRAGMA integrity_check").Row().Scan(&res); serr != nil {
		return fmt.Errorf("完整性校验执行失败: %w", serr)
	}
	if res != "ok" {
		first := strings.SplitN(res, "\n", 2)[0]
		return fmt.Errorf("数据库完整性校验未通过: %s(可从 data/backup/ 恢复备份)", first)
	}
	return nil
}
