package agent

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"time"
)

// LoadSkills 从磁盘目录加载 skills(保留历史签名,e2e/既有测试依赖)。
func LoadSkills(dir string) (text2sql string, g2chart string, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "agent.LoadSkills", "dir", dir)
	defer func() {
		slog.Debug("方法离开", "fn", "agent.LoadSkills", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	return LoadSkillsFS(os.DirFS(dir))
}

// LoadSkillsFS 从任意 fs.FS 读取两个 skill 文件内容;缺失即报错(启动期暴露配置问题)。
func LoadSkillsFS(fsys fs.FS) (text2sql string, g2chart string, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "agent.LoadSkillsFS")
	defer func() {
		slog.Debug("方法离开", "fn", "agent.LoadSkillsFS", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	b, err := fs.ReadFile(fsys, path.Join("text2sql", "SKILL.md"))
	if err != nil {
		return "", "", fmt.Errorf("读取 text2sql skill 失败: %w", err)
	}
	text2sql = string(b)
	b, err = fs.ReadFile(fsys, path.Join("antv-g2-chart", "SKILL.md"))
	if err != nil {
		return "", "", fmt.Errorf("读取 antv-g2-chart skill 失败: %w", err)
	}
	return text2sql, string(b), nil
}
