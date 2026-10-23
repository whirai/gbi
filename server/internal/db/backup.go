package db

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gorm.io/gorm"
)

// maxBackups 备份保留份数(超出按修改时间清理最旧)。
const maxBackups = 5

// Backup 用 VACUUM INTO 生成一致性快照到 <dir(dbPath)>/backup/
// (连接打开状态下安全,Windows 免文件锁);随后清理超出保留份数的旧备份。
func Backup(d *gorm.DB, dbPath string, fromVersion int) (outPath string, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "db.Backup", "db_path", dbPath, "from_version", fromVersion)
	defer func() {
		slog.Debug("方法离开", "fn", "db.Backup", "elapsed_ms", time.Since(start).Milliseconds(), "path", outPath, "err", err)
	}()
	dir := filepath.Join(filepath.Dir(dbPath), "backup")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ts := time.Now().Format("20060102-150405")
	outPath = filepath.Join(dir, fmt.Sprintf("gbi-v%d-%s.db", fromVersion, ts))
	// VACUUM INTO 要求目标不存在;同秒重复备份追加序号
	for i := 1; ; i++ {
		if _, serr := os.Stat(outPath); os.IsNotExist(serr) {
			break
		}
		outPath = filepath.Join(dir, fmt.Sprintf("gbi-v%d-%s-%d.db", fromVersion, ts, i))
	}
	if verr := d.Exec("VACUUM INTO ?", filepath.ToSlash(outPath)).Error; verr != nil {
		return "", fmt.Errorf("VACUUM INTO 失败: %w", verr)
	}
	pruneBackups(dir)
	return outPath, nil
}

// pruneBackups 保留 maxBackups 份最新的 gbi-v*.db,其余删除(失败仅告警)。
func pruneBackups(dir string) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "db.pruneBackups", "dir", dir)
	defer func() {
		slog.Debug("方法离开", "fn", "db.pruneBackups", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	matches, err := filepath.Glob(filepath.Join(dir, "gbi-v*.db"))
	if err != nil || len(matches) <= maxBackups {
		return
	}
	type backupFile struct {
		path string
		mod  time.Time
	}
	files := make([]backupFile, 0, len(matches))
	for _, m := range matches {
		info, serr := os.Stat(m)
		if serr != nil {
			continue
		}
		files = append(files, backupFile{path: m, mod: info.ModTime()})
	}
	if len(files) <= maxBackups {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })
	for _, f := range files[:len(files)-maxBackups] {
		if rerr := os.Remove(f.path); rerr != nil {
			slog.Warn("清理旧备份失败", "path", f.path, "err", rerr)
			continue
		}
		slog.Info("已清理旧备份", "path", f.path)
	}
}
