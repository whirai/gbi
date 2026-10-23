package service_test

import (
	"testing"

	"github.com/casbin/casbin/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"gbi/server/internal/auth"
	"gbi/server/internal/db"
	"gbi/server/internal/model"
	"gbi/server/internal/rbac"
	"gbi/server/internal/service"
)

func TestSetAndGetACL(t *testing.T) {
	svc, e, d, mkUser := newPageEnvWithDB(t)
	usr := mkUser("acl1", 3)
	usr2 := mkUser("acl2", 3)

	p, err := svc.Create(1, service.PageCreate{Name: "授权页"})
	require.NoError(t, err)
	require.NoError(t, svc.SetStatus(p.ID, "published"))

	// 建组并加成员。成员行直接建;组成员→组的 casbin 分组正常由
	// GroupService.AddMembers 维护,此处手工补 g 规则,
	// 组继承经 g(user:N, group:M) 生效;页面授权走 SetACL。
	grp := model.Group{Name: "授权组"}
	require.NoError(t, d.Create(&grp).Error)
	require.NoError(t, d.Create(&model.GroupMember{GroupID: grp.ID, UserID: usr2}).Error)
	_, err = e.AddGroupingPolicy(rbac.UserSubject(usr2), rbac.GroupSubject(grp.ID))
	require.NoError(t, err)

	// SetACL:用户 usr + 组 grp
	err = svc.SetACL(p.ID, []service.ACLGrantInput{
		{SubjectType: "user", SubjectID: usr},
		{SubjectType: "group", SubjectID: grp.ID},
	})
	require.NoError(t, err)

	grants, err := svc.GetACL(p.ID)
	require.NoError(t, err)
	require.Len(t, grants, 2)

	// usr 直接授权可见;usr2 经组可见;usr 无组关系但直接授权
	ok, _ := svc.CanView(usr, p)
	require.True(t, ok)
	ok, _ = svc.CanView(usr2, p)
	require.True(t, ok)

	// 全量替换:只留 usr2 → usr 失去权限
	err = svc.SetACL(p.ID, []service.ACLGrantInput{{SubjectType: "user", SubjectID: usr2}})
	require.NoError(t, err)
	ok, _ = svc.CanView(usr, p)
	require.False(t, ok)
	ok, _ = svc.CanView(usr2, p)
	require.True(t, ok)

	// 非法对象
	err = svc.SetACL(p.ID, []service.ACLGrantInput{{SubjectType: "user", SubjectID: 999999}})
	require.Error(t, err)
	err = svc.SetACL(p.ID, []service.ACLGrantInput{{SubjectType: "robot", SubjectID: usr}})
	require.Error(t, err)
}

// newPageEnvWithDB 与 newPageEnv 相同,但额外返回 enforcer 与 db 句柄。
func newPageEnvWithDB(t *testing.T) (*service.PageService, *casbin.Enforcer, *gorm.DB, func(username string, roleID uint) uint) {
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
	return svc, e, d, mkUser
}
