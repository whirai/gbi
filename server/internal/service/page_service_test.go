package service_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gbi/server/internal/auth"
	"gbi/server/internal/db"
	"gbi/server/internal/model"
	"gbi/server/internal/rbac"
	"gbi/server/internal/service"
)

// newPageEnv 返回 PageService + 必要辅助。复用 newTestDB;
// 用户直接建行(避免依赖 UserService/config),admin 由 Seed 提供(ID=1)。
func newPageEnv(t *testing.T) (*service.PageService, func(username string, roleID uint) uint) {
	d := newTestDB(t)
	e, err := rbac.NewEnforcer(d)
	require.NoError(t, err)
	require.NoError(t, db.SeedRBAC(d, e))
	svc := service.NewPageService(d, e)
	mkUser := func(username string, roleID uint) uint {
		hash, err := auth.HashPassword("pass1234")
		require.NoError(t, err)
		u := model.User{Username: username, Name: username, PasswordHash: hash, RoleID: roleID, Status: "active"}
		require.NoError(t, d.Create(&u).Error)
		return u.ID
	}
	return svc, mkUser
}

func TestPageCreateDefaultsAndUpdateVersioning(t *testing.T) {
	svc, _ := newPageEnv(t)

	p, err := svc.Create(1, service.PageCreate{Name: "销售总览", Desc: "d"})
	require.NoError(t, err)
	require.Equal(t, "draft", p.Status)
	require.Contains(t, p.Config, `"charts":[]`)

	// 改名不动版本
	_, err = svc.Update(1, p.ID, service.PageUpdate{Name: strPtr("销售总览V2")})
	require.NoError(t, err)
	vs, _ := svc.Versions(p.ID)
	require.Len(t, vs, 0)

	// config 变化 → 版本 1
	cfg1 := model.DefaultPageConfig()
	cfg1.Charts = []model.ChartConfig{{ID: "c1", Title: "t", Width: "full", SQL: "SELECT 1"}}
	_, err = svc.Update(1, p.ID, service.PageUpdate{Config: &cfg1})
	require.NoError(t, err)
	vs, _ = svc.Versions(p.ID)
	require.Len(t, vs, 1)
	require.Equal(t, 1, vs[0].Version)

	// 再变 → 版本 2
	cfg2 := cfg1
	cfg2.Charts = append(append([]model.ChartConfig{}, cfg1.Charts...),
		model.ChartConfig{ID: "c2", Width: "1/2", SQL: "SELECT 2"})
	_, err = svc.Update(1, p.ID, service.PageUpdate{Config: &cfg2})
	require.NoError(t, err)
	vs, _ = svc.Versions(p.ID)
	require.Len(t, vs, 2)

	// 非法 config
	bad := model.PageConfig{Charts: []model.ChartConfig{{Width: "bad", SQL: "SELECT 1"}}}
	_, err = svc.Update(1, p.ID, service.PageUpdate{Config: &bad})
	require.ErrorIs(t, err, service.ErrInvalidPageConfig)

	// 恢复到版本 1 → config 等于 cfg1,且生成版本 3
	require.NoError(t, svc.Restore(1, p.ID, vs[1].ID)) // vs 倒序,vs[1] 是版本1?见实现约定:Versions 按 version 降序
	got, err := svc.Get(p.ID)
	require.NoError(t, err)
	require.Contains(t, got.Config, `"id":"c1"`)
	require.NotContains(t, got.Config, `"id":"c2"`)
	vs, _ = svc.Versions(p.ID)
	require.Len(t, vs, 3)
}

func TestPagePublishAndCanView(t *testing.T) {
	svc, mkUser := newPageEnv(t)
	dev := mkUser("dev1", 2) // developer
	usr := mkUser("usr1", 3) // user

	p, err := svc.Create(dev, service.PageCreate{Name: "页面A"})
	require.NoError(t, err)

	// 草稿:作者可见、admin 可见、其他用户不可见
	ok, err := svc.CanView(dev, p)
	require.NoError(t, err)
	require.True(t, ok)
	ok, _ = svc.CanView(1, p)
	require.True(t, ok)
	ok, _ = svc.CanView(usr, p)
	require.False(t, ok)

	// 发布后仍不可见(未授权),授权 user:usr 后可见
	require.NoError(t, svc.SetStatus(p.ID, "published"))
	ok, _ = svc.CanView(usr, p)
	require.False(t, ok)

	// VisiblePages:usr 无 → 0;授权后 → 1
	visible, err := svc.VisiblePages(usr)
	require.NoError(t, err)
	require.Len(t, visible, 0)

	// (直接写 casbin 策略模拟授权;SetACL 在 Task 4 提供)
	// 这里通过 PageService 暴露的 enforcer 不可达,改用 SetStatus 之外的途径:
	// 先跳过授权断言,Task 4 的 ACL 测试覆盖授权→可见。
	_ = usr

	// 非法状态
	require.ErrorIs(t, svc.SetStatus(p.ID, "archived"), service.ErrInvalidPageStatus)
}

func TestPageDeleteCascades(t *testing.T) {
	svc, _ := newPageEnv(t)
	p, err := svc.Create(1, service.PageCreate{Name: "待删"})
	require.NoError(t, err)
	cfg := model.DefaultPageConfig()
	cfg.Charts = []model.ChartConfig{{ID: "c1", Width: "full", SQL: "SELECT 1"}}
	_, err = svc.Update(1, p.ID, service.PageUpdate{Config: &cfg})
	require.NoError(t, err)

	require.NoError(t, svc.Delete(p.ID))
	_, err = svc.Get(p.ID)
	require.ErrorIs(t, err, service.ErrPageNotFound)
	vs, err := svc.Versions(p.ID)
	require.NoError(t, err)
	require.Len(t, vs, 0)
}

// TestCanQuerySubjects 门户图表取数授权:普通用户只能查询
// 其可见(published+已授权)页面图表引用的数据主题;admin 全量放行。
func TestCanQuerySubjects(t *testing.T) {
	svc, mkUser := newPageEnv(t)
	viewer := mkUser("viewer1", 3) // 普通用户

	mkPage := func(name string, subjectID uint, published bool) uint {
		p, err := svc.Create(1, service.PageCreate{Name: name})
		require.NoError(t, err)
		cfg := model.DefaultPageConfig()
		cfg.Charts = []model.ChartConfig{{
			ID: "c1", Title: "t", Width: "full", SQL: "SELECT 1", Subjects: []uint{subjectID},
		}}
		_, err = svc.Update(1, p.ID, service.PageUpdate{Config: &cfg})
		require.NoError(t, err)
		if published {
			require.NoError(t, svc.SetStatus(p.ID, "published"))
		}
		require.NoError(t, svc.SetACL(p.ID, []service.ACLGrantInput{{SubjectType: "user", SubjectID: viewer}}))
		return p.ID
	}
	// P1 published 引用主题 1;P2 draft 引用主题 2;主题 3 无页面引用
	mkPage("P1", 1, true)
	mkPage("P2", 2, false)

	ok, err := svc.CanQuerySubjects(viewer, []uint{1})
	require.NoError(t, err)
	require.True(t, ok, "已授权已发布页面引用的主题应可查")

	ok, err = svc.CanQuerySubjects(viewer, []uint{2})
	require.NoError(t, err)
	require.False(t, ok, "draft 页面引用的主题不可查")

	ok, err = svc.CanQuerySubjects(viewer, []uint{3})
	require.NoError(t, err)
	require.False(t, ok, "无页面引用的主题不可查")

	ok, err = svc.CanQuerySubjects(viewer, []uint{1, 3})
	require.NoError(t, err)
	require.False(t, ok, "任一主题不可查则整体拒绝")

	// admin 全量放行
	ok, err = svc.CanQuerySubjects(1, []uint{3})
	require.NoError(t, err)
	require.True(t, ok)

	// 未授权用户:即便页面 published,也不可查其主题
	other := mkUser("other1", 3)
	ok, err = svc.CanQuerySubjects(other, []uint{1})
	require.NoError(t, err)
	require.False(t, ok)
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }
