package db

import (
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"gbi/server/internal/model"
)

func Open(path string) (d *gorm.DB, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "db.Open", "path", path)
	defer func() {
		slog.Debug("方法离开", "fn", "db.Open", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	d, err = gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}
	// 不保留空闲连接:SQLite 文件句柄随用随关,
	// 避免 Windows 上测试临时目录因文件被占用无法清理。
	sqlDB, err := d.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxIdleConns(0)
	return d, nil
}

func Migrate(d *gorm.DB) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "db.Migrate")
	defer func() {
		slog.Debug("方法离开", "fn", "db.Migrate", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	return d.AutoMigrate(
		&SchemaMigration{},
		&model.Role{}, &model.Department{}, &model.User{},
		&model.Group{}, &model.GroupMember{},
		&model.Subject{}, &model.SubjectTable{},
		&model.Page{}, &model.PageVersion{}, &model.Menu{}, &model.MenuItem{},
		&model.Conversation{}, &model.Message{},
		&model.PageRenderError{},
		&model.LLMProvider{}, &model.LLMModel{},
		&model.BrandConfig{},
	)
}
