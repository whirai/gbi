package agent_test

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"gbi/server/internal/agent"
)

// TestG2SkillAntiPatterns 线上事故回归:G2 skill 必须显式声明平台约束
// (encode 字段须与 SQL 返回列一致、theta 环图 innerRadius 在 coordinate 上、
// stackY 需 color 通道),否则模型会在没有渲染反馈的情况下写出渲染失败的 spec。
func TestG2SkillAntiPatterns(t *testing.T) {
	dir := filepath.Join("..", "..", "skills")
	_, g2, err := agent.LoadSkills(dir)
	require.NoError(t, err)
	for _, kw := range []string{"encode", "列名", "coordinate", "innerRadius", "stackY", "color"} {
		require.Contains(t, g2, kw, "G2 skill 应包含反模式规则关键词 %s", kw)
	}
}

func TestLoadSkills(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "text2sql"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "antv-g2-chart"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "text2sql", "SKILL.md"), []byte("T2S"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "antv-g2-chart", "SKILL.md"), []byte("G2"), 0o644))

	t2s, g2, err := agent.LoadSkills(dir)
	require.NoError(t, err)
	require.Equal(t, "T2S", t2s)
	require.Equal(t, "G2", g2)

	_, _, err = agent.LoadSkills(filepath.Join(dir, "missing"))
	require.Error(t, err)
}

// TestLoadSkillsFS 任意 fs.FS(内置资源即此形态)可加载;缺文件报错。
func TestLoadSkillsFS(t *testing.T) {
	fsys := fstest.MapFS{
		"text2sql/SKILL.md":      {Data: []byte("T2S-FS")},
		"antv-g2-chart/SKILL.md": {Data: []byte("G2-FS")},
	}
	t2s, g2, err := agent.LoadSkillsFS(fsys)
	require.NoError(t, err)
	require.Equal(t, "T2S-FS", t2s)
	require.Equal(t, "G2-FS", g2)

	_, _, err = agent.LoadSkillsFS(fstest.MapFS{})
	require.Error(t, err)
}
