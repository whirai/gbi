package service_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gbi/server/internal/db"
	"gbi/server/internal/rbac"
	"gbi/server/internal/service"
)

func newUserEnv(t *testing.T) (*service.UserService, *service.DeptService, func(sub, obj, act string) bool) {
	d := newTestDB(t)
	e, err := rbac.NewEnforcer(d)
	require.NoError(t, err)
	require.NoError(t, db.SeedRBAC(d, e))
	return service.NewUserService(d, e), service.NewDeptService(d), func(sub, obj, act string) bool {
		ok, err := e.Enforce(sub, obj, act)
		require.NoError(t, err)
		return ok
	}
}

func TestUserCreateAndList(t *testing.T) {
	svc, depts, enforce := newUserEnv(t)
	dept, err := depts.Create("研发部", nil, 0)
	require.NoError(t, err)

	// role_id=2 为 developer(种子顺序:admin=1, developer=2, user=3)
	dto, err := svc.Create(service.UserCreate{
		Username: "dev1", Password: "pass1234", Name: "开发甲",
		DeptID: &dept.ID, RoleID: 2,
	})
	require.NoError(t, err)
	require.Equal(t, "developer", dto.RoleCode)
	require.Equal(t, "研发部", dto.DeptName)

	// casbin 分组已建立:user:N 拥有 developer 的权限路径之外无权限
	require.True(t, enforce(rbac.UserSubject(dto.ID), "/api/v1/auth/me", "GET"))

	items, total, err := svc.List(1, 20, "开发")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, "dev1", items[0].Username)

	_, err = svc.Create(service.UserCreate{
		Username: "dev1", Password: "pass1234", Name: "重复", RoleID: 2,
	})
	require.ErrorIs(t, err, service.ErrDuplicateUsername)
}

func TestUserUpdateRoleSwitchesCasbin(t *testing.T) {
	svc, _, enforce := newUserEnv(t)
	dto, err := svc.Create(service.UserCreate{
		Username: "u1", Password: "pass1234", Name: "某人", RoleID: 3,
	})
	require.NoError(t, err)
	sub := rbac.UserSubject(dto.ID)
	require.False(t, enforce(sub, "/api/v1/departments", "GET"))

	updated, err := svc.Update(dto.ID, service.UserUpdate{Name: "某人", RoleID: 1, Status: "active"})
	require.NoError(t, err)
	require.Equal(t, "admin", updated.RoleCode)
	require.True(t, enforce(sub, "/api/v1/departments", "GET"))
}

func TestUserDisableRevokesAccess(t *testing.T) {
	svc, _, enforce := newUserEnv(t)
	dto, err := svc.Create(service.UserCreate{
		Username: "u4", Password: "pass1234", Name: "停用甲", RoleID: 3,
	})
	require.NoError(t, err)
	sub := rbac.UserSubject(dto.ID)
	require.True(t, enforce(sub, "/api/v1/auth/me", "GET"))

	// 停用后角色分组被移除,即使 JWT 未过期也无权限
	_, err = svc.Update(dto.ID, service.UserUpdate{Name: "停用甲", RoleID: 3, Status: "disabled"})
	require.NoError(t, err)
	require.False(t, enforce(sub, "/api/v1/auth/me", "GET"))

	// 重新启用后恢复权限
	_, err = svc.Update(dto.ID, service.UserUpdate{Name: "停用甲", RoleID: 3, Status: "active"})
	require.NoError(t, err)
	require.True(t, enforce(sub, "/api/v1/auth/me", "GET"))
}

func TestUserDeleteAndBuiltInGuard(t *testing.T) {
	svc, _, enforce := newUserEnv(t)
	dto, err := svc.Create(service.UserCreate{
		Username: "u2", Password: "pass1234", Name: "待删", RoleID: 3,
	})
	require.NoError(t, err)

	// 内置 admin(ID 由 seed 创建)不可删
	require.ErrorIs(t, svc.Delete(1), service.ErrBuiltInUser)

	require.NoError(t, svc.Delete(dto.ID))
	require.False(t, enforce(rbac.UserSubject(dto.ID), "/api/v1/auth/me", "GET"))
}

func TestResetPassword(t *testing.T) {
	svc, _, _ := newUserEnv(t)
	dto, err := svc.Create(service.UserCreate{
		Username: "u3", Password: "pass1234", Name: "改密", RoleID: 3,
	})
	require.NoError(t, err)
	require.NoError(t, svc.ResetPassword(dto.ID, "newpass9"))
}
