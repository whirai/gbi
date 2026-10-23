package rbac_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"gbi/server/internal/db"
	"gbi/server/internal/rbac"
)

func TestEnforcerRBAC(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	require.NoError(t, db.Migrate(d))
	require.NoError(t, db.Seed(d))

	e, err := rbac.NewEnforcer(d)
	require.NoError(t, err)
	require.NoError(t, db.SeedRBAC(d, e))

	// 给 user:1 绑定 admin 角色
	_, err = e.AddGroupingPolicy("user:1", rbac.RoleSubject("admin"))
	require.NoError(t, err)

	ok, err := e.Enforce("user:1", "/api/v1/users", "GET")
	require.NoError(t, err)
	require.True(t, ok)

	// 未授权主体
	ok, err = e.Enforce("user:2", "/api/v1/users", "GET")
	require.NoError(t, err)
	require.False(t, ok)

	// user 角色只能访问 /api/v1/auth/*
	_, err = e.AddGroupingPolicy("user:2", rbac.RoleSubject("user"))
	require.NoError(t, err)
	ok, _ = e.Enforce("user:2", "/api/v1/auth/me", "GET")
	require.True(t, ok)
	ok, _ = e.Enforce("user:2", "/api/v1/users", "GET")
	require.False(t, ok)
}
