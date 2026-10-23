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

func newMenuEnv(t *testing.T) (*service.MenuService, *service.PageService, func(username string, roleID uint) uint) {
	d := newTestDB(t)
	e, err := rbac.NewEnforcer(d)
	require.NoError(t, err)
	require.NoError(t, db.SeedRBAC(d, e))
	pages := service.NewPageService(d, e)
	menus := service.NewMenuService(d, pages)
	mkUser := func(username string, roleID uint) uint {
		hash, err := auth.HashPassword("pass1234")
		require.NoError(t, err)
		u := model.User{Username: username, Name: username, PasswordHash: hash, RoleID: roleID, Status: "active"}
		require.NoError(t, d.Create(&u).Error)
		return u.ID
	}
	return menus, pages, mkUser
}

func TestMenuCRUD(t *testing.T) {
	svc, pages, _ := newMenuEnv(t)

	m, err := svc.CreateMenu(service.MenuCreate{Name: "销售分析", Desc: "d", Sort: 1})
	require.NoError(t, err)

	_, err = svc.CreateMenu(service.MenuCreate{Name: "x"})
	require.NoError(t, err)
	list, err := svc.ListMenus()
	require.NoError(t, err)
	require.Len(t, list, 2)

	renamed, err := svc.UpdateMenu(m.ID, service.MenuUpdate{Name: strPtr("销售分析V2")})
	require.NoError(t, err)
	require.Equal(t, "销售分析V2", renamed.Name)

	// 页面 + 菜单项
	p, err := pages.Create(1, service.PageCreate{Name: "页1"})
	require.NoError(t, err)
	item, err := svc.CreateItem(m.ID, service.MenuItemCreate{PageID: p.ID, Name: "总览", Sort: 1})
	require.NoError(t, err)

	items, err := svc.ListItems(m.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)

	// 指向不存在的页面
	_, err = svc.CreateItem(m.ID, service.MenuItemCreate{PageID: 999999, Name: "x"})
	require.Error(t, err)

	// 编辑菜单项可改指向页面
	p2, err := pages.Create(1, service.PageCreate{Name: "页2"})
	require.NoError(t, err)
	updated, err := svc.UpdateItem(m.ID, item.ID, service.MenuItemUpdate{PageID: &p2.ID, Sort: intPtr(5)})
	require.NoError(t, err)
	require.Equal(t, p2.ID, updated.PageID)
	require.Equal(t, 5, updated.Sort)

	// 指向不存在的页面 → 拒绝
	badID := uint(999999)
	_, err = svc.UpdateItem(m.ID, item.ID, service.MenuItemUpdate{PageID: &badID})
	require.Error(t, err)
	got, err := svc.ListItems(m.ID)
	require.NoError(t, err)
	require.Equal(t, p2.ID, got[0].PageID)

	// 跨菜单操作拒绝
	other, _ := svc.CreateMenu(service.MenuCreate{Name: "别的"})
	_, err = svc.UpdateItem(other.ID, item.ID, service.MenuItemUpdate{Name: strPtr("y")})
	require.ErrorIs(t, err, service.ErrMenuItemNotFound)
	require.ErrorIs(t, svc.DeleteItem(other.ID, item.ID), service.ErrMenuItemNotFound)

	// 删菜单级联删项
	require.NoError(t, svc.DeleteMenu(m.ID))
	_, err = svc.ListItems(m.ID)
	require.ErrorIs(t, err, service.ErrMenuNotFound)
}

func TestVisibleMenus(t *testing.T) {
	svc, pages, mkUser := newMenuEnv(t)
	usr := mkUser("menu1", 3)

	m, err := svc.CreateMenu(service.MenuCreate{Name: "可见性菜单"})
	require.NoError(t, err)
	pub, _ := pages.Create(1, service.PageCreate{Name: "已发布"})
	require.NoError(t, pages.SetStatus(pub.ID, "published"))
	draft, _ := pages.Create(1, service.PageCreate{Name: "草稿"})
	_ = draft

	_, err = svc.CreateItem(m.ID, service.MenuItemCreate{PageID: pub.ID, Name: "发布页"})
	require.NoError(t, err)
	_, err = svc.CreateItem(m.ID, service.MenuItemCreate{PageID: draft.ID, Name: "草稿页"})
	require.NoError(t, err)

	// 未授权:无可见菜单
	visible, err := svc.VisibleMenus(usr)
	require.NoError(t, err)
	require.Len(t, visible, 0)

	// 授权已发布页 → 菜单出现,仅 1 项(草稿被过滤)
	require.NoError(t, pages.SetACL(pub.ID, []service.ACLGrantInput{{SubjectType: "user", SubjectID: usr}}))
	visible, err = svc.VisibleMenus(usr)
	require.NoError(t, err)
	require.Len(t, visible, 1)
	require.Equal(t, "可见性菜单", visible[0].Name)
	require.Len(t, visible[0].Items, 1)
	require.Equal(t, "发布页", visible[0].Items[0].Name)

	// admin 可见该已发布页(全量),草稿仍被过滤(门户只看 published)
	visible, err = svc.VisibleMenus(1)
	require.NoError(t, err)
	require.Len(t, visible, 1)
	require.Len(t, visible[0].Items, 1)
}
