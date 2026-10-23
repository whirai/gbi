package datacore

import (
	"log/slog"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// OpenSubject 打开主题库。readOnly 使用 mode=ro + PRAGMA query_only 双保险。
// 读路径单连接池:PRAGMA 是连接级设置,只有固化在池中唯一连接上才真实生效
// (多连接池下其余新连接不会带上 pragma,双保险名存实亡)。
// 返回的 closeFn 必须调用,以释放 Windows 上的文件句柄。
func OpenSubject(path string, readOnly bool) (d *gorm.DB, closeFn func(), err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "datacore.OpenSubject", "path", path, "readonly", readOnly)
	defer func() {
		slog.Debug("方法离开", "fn", "datacore.OpenSubject", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	dsn := path
	if readOnly {
		dsn = "file:" + filepath.ToSlash(path) + "?mode=ro"
	}
	d, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, nil, err
	}
	sqlDB, err := d.DB()
	if err != nil {
		return nil, nil, err
	}
	if readOnly {
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
	} else {
		// 写路径不保留空闲连接,避免 Windows 上文件句柄阻碍清理。
		sqlDB.SetMaxIdleConns(0)
	}
	closeFn = func() { _ = sqlDB.Close() }
	if readOnly {
		if err := d.Exec("PRAGMA query_only = ON").Error; err != nil {
			closeFn()
			return nil, nil, err
		}
	}
	return d, closeFn, nil
}
