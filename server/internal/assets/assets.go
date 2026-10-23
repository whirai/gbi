// Package assets 内嵌分发资源:前端构建产物(dist/)与技能文档(skills/)。
// 目录内容由 scripts/build-release.sh 在构建期间拷入,构建结束自动清空(仅留 .gitkeep);
// git 中仅保留占位(保证裸 go build / go run 永远可编译)。因此裸构建的二进制不含前端,
// SPA 模式启动会显式失败——分发必须使用 scripts/build-release.sh 的产物,避免静默携带陈旧前端。
// 磁盘目录存在时优先磁盘,否则使用内置(见 Resolve)。
package assets

import (
	"embed"
	"io/fs"
	"log/slog"
	"os"
	"time"
)

//go:embed all:dist
var distRoot embed.FS

//go:embed all:skills
var skillsRoot embed.FS

// DistFS 根化到 dist/ 内容(如 index.html、assets/)。
var DistFS fs.FS = mustSub(distRoot, "dist")

// SkillsFS 根化到 skills/ 内容(如 text2sql/SKILL.md)。
var SkillsFS fs.FS = mustSub(skillsRoot, "skills")

// mustSub 把 embed.FS 根化到子目录;目录由 //go:embed 指令保证存在,
// 报错只可能是包内结构损坏(编码错误),直接 panic 暴露。
func mustSub(f embed.FS, dir string) fs.FS {
	start := time.Now()
	slog.Debug("方法进入", "fn", "assets.mustSub", "dir", dir)
	defer func() {
		slog.Debug("方法离开", "fn", "assets.mustSub", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	sub, err := fs.Sub(f, dir)
	if err != nil {
		panic("assets: fs.Sub " + dir + ": " + err.Error())
	}
	return sub
}

// Resolve 返回磁盘或内置资源源:diskRoot 非空且目录存在 → os.DirFS(diskRoot)+"disk";
// 否则 embedded+"embedded"。磁盘优先保证开发热迭代与运维热替换。
func Resolve(diskRoot string, embedded fs.FS) (fsys fs.FS, source string) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "assets.Resolve", "disk_root", diskRoot)
	defer func() {
		slog.Debug("方法离开", "fn", "assets.Resolve", "elapsed_ms", time.Since(start).Milliseconds(), "source", source)
	}()
	if diskRoot != "" && dirExists(diskRoot) {
		return os.DirFS(diskRoot), "disk"
	}
	return embedded, "embedded"
}

// dirExists 判断路径是否为已存在的目录(与 webapp.dirExists 语义一致,刻意各留一份保持包解耦)。
func dirExists(path string) bool {
	start := time.Now()
	slog.Debug("方法进入", "fn", "assets.dirExists", "path", path)
	defer func() {
		slog.Debug("方法离开", "fn", "assets.dirExists", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
