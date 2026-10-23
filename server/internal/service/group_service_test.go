package service_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gbi/server/internal/db"
	"gbi/server/internal/rbac"
	"gbi/server/internal/service"
)

func newGroupEnv(t *testing.T) (*service.GroupService, *service.UserService, func(sub, obj, act string) bool) {
	d := newTestDB(t)
	e, err := rbac.NewEnforcer(d)
	require.NoError(t, err)
	require.NoError(t, db.SeedRBAC(d, e))
	return service.NewGroupService(d, e), service.NewUserService(d, e), func(sub, obj, act string) bool {
		ok, err := e.Enforce(sub, obj, act)
		require.NoError(t, err)
		return ok
	}
}

func TestGroupCRUDAndMembers(t *testing.T) {
	gsvc, usvc, enforce := newGroupEnv(t)

	g, err := gsvc.Create("销售分析组", "负责销售域")
	require.NoError(t, err)

	_, err = gsvc.Create("销售分析组", "重复")
	require.ErrorIs(t, err, service.ErrDuplicateGroupName)

	u, err := usvc.Create(service.UserCreate{
		Username: "g1", Password: "pass1234", Name: "组员甲", RoleID: 3,
	})
	require.NoError(t, err)

	require.NoError(t, gsvc.AddMembers(g.ID, []uint{u.ID}))
	members, err := gsvc.Members(g.ID)
	require.NoError(t, err)
	require.Len(t, members, 1)
	require.Equal(t, "g1", members[0].Username)

	// 幂等:重复添加不报错不重复
	require.NoError(t, gsvc.AddMembers(g.ID, []uint{u.ID}))
	members, _ = gsvc.Members(g.ID)
	require.Len(t, members, 1)

	// casbin 分组:user 继承 group
	require.True(t, enforce(rbac.UserSubject(u.ID), "/api/v1/auth/me", "GET"))

	require.NoError(t, gsvc.RemoveMember(g.ID, u.ID))
	members, _ = gsvc.Members(g.ID)
	require.Len(t, members, 0)

	renamed, err := gsvc.Update(g.ID, "销售组", "缩编")
	require.NoError(t, err)
	require.Equal(t, "销售组", renamed.Name)

	require.NoError(t, gsvc.Delete(g.ID))
	list, err := gsvc.List()
	require.NoError(t, err)
	require.Len(t, list, 0)
}

func TestAddMembersRejectsInvalidIDs(t *testing.T) {
	gsvc, _, _ := newGroupEnv(t)

	g, err := gsvc.Create("校验组", "拒绝非法用户ID")
	require.NoError(t, err)

	err = gsvc.AddMembers(g.ID, []uint{0})
	require.ErrorIs(t, err, service.ErrInvalidUserID)

	err = gsvc.AddMembers(g.ID, []uint{999999})
	require.ErrorIs(t, err, service.ErrMemberUserNotFound)

	members, err := gsvc.Members(g.ID)
	require.NoError(t, err)
	require.Len(t, members, 0)
}

// TestAddMembersDeduplicatesIDs 重复 user id([5,5])曾被 Count(distinct) 与长度比对误判为
// ErrMemberUserNotFound;去重后应正常入库且只产生一条成员行。
func TestAddMembersDeduplicatesIDs(t *testing.T) {
	gsvc, usvc, _ := newGroupEnv(t)

	g, err := gsvc.Create("去重组", "重复ID")
	require.NoError(t, err)
	u, err := usvc.Create(service.UserCreate{Username: "dup1", Password: "pass1234", Name: "重复成员", RoleID: 3})
	require.NoError(t, err)

	require.NoError(t, gsvc.AddMembers(g.ID, []uint{u.ID, u.ID}))
	members, err := gsvc.Members(g.ID)
	require.NoError(t, err)
	require.Len(t, members, 1)
	require.Equal(t, u.ID, members[0].ID)

	// 混合场景:去重后仍校验存在性([u.ID, u.ID, 999999] 只有一个不存在 id,应报不存在)
	require.ErrorIs(t, gsvc.AddMembers(g.ID, []uint{u.ID, u.ID, 999999}), service.ErrMemberUserNotFound)
}
