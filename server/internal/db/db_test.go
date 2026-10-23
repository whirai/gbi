package db_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"gbi/server/internal/db"
	"gbi/server/internal/model"
	"gbi/server/internal/rbac"
)

func NewTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	require.NoError(t, db.Migrate(d))
	return d
}

func TestMigrateAndSeedRoles(t *testing.T) {
	d := NewTestDB(t)
	require.NoError(t, db.Seed(d))

	var roles []model.Role
	require.NoError(t, d.Order("id").Find(&roles).Error)
	require.Len(t, roles, 3)
	require.Equal(t, "admin", roles[0].Code)

	// 幂等:重复执行不新增
	require.NoError(t, db.Seed(d))
	var n int64
	d.Model(&model.Role{}).Count(&n)
	require.Equal(t, int64(3), n)
}

// TestSeedRBACUserQueryPolicy 普通用户必须能 POST /query(门户图表取数),
// 否则已授权页面能打开但图表全部 403(线上事故回归)。
func TestSeedRBACUserQueryPolicy(t *testing.T) {
	d := NewTestDB(t)
	require.NoError(t, db.Seed(d))
	e, err := rbac.NewEnforcer(d)
	require.NoError(t, err)
	require.NoError(t, db.SeedRBAC(d, e))

	has, err := e.HasPolicy("role:user", "/api/v1/query", "POST")
	require.NoError(t, err)
	require.True(t, has)
	// 绑定普通用户到 role:user 后实际 Enforce 通过
	_, err = e.AddGroupingPolicy(rbac.UserSubject(99), rbac.RoleSubject("user"))
	require.NoError(t, err)
	ok, err := e.Enforce(rbac.UserSubject(99), "/api/v1/query", "POST")
	require.NoError(t, err)
	require.True(t, ok)
}
