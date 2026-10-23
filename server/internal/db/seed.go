package db

import (
	"log/slog"
	"time"

	"github.com/casbin/casbin/v2"
	"gorm.io/gorm"

	"gbi/server/internal/auth"
	"gbi/server/internal/model"
	"gbi/server/internal/rbac"
)

const AdminPassword = "admin123"

// Seed 初始化内置角色与内置管理员,幂等。
func Seed(d *gorm.DB) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "db.Seed")
	defer func() {
		slog.Debug("方法离开", "fn", "db.Seed", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	roles := []model.Role{
		{Code: "admin", Name: "管理员"},
		{Code: "developer", Name: "开发者"},
		{Code: "user", Name: "普通用户"},
	}
	for _, r := range roles {
		if err := d.Where("code = ?", r.Code).FirstOrCreate(&r).Error; err != nil {
			return err
		}
	}

	var adminRole model.Role
	if err := d.Where("code = ?", "admin").First(&adminRole).Error; err != nil {
		return err
	}
	var n int64
	if err := d.Model(&model.User{}).Where("username = ?", "admin").Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		hash, err := auth.HashPassword(AdminPassword)
		if err != nil {
			return err
		}
		admin := model.User{
			Username:     "admin",
			Name:         "系统管理员",
			PasswordHash: hash,
			RoleID:       adminRole.ID,
			Status:       "active",
			IsBuiltIn:    true,
		}
		if err := d.Create(&admin).Error; err != nil {
			return err
		}
	}
	return nil
}

// SeedRBAC 写入初始策略,幂等。
func SeedRBAC(d *gorm.DB, e *casbin.Enforcer) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "db.SeedRBAC")
	defer func() {
		slog.Debug("方法离开", "fn", "db.SeedRBAC", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	// 动作使用 ".*"(regexMatch 要求合法正则,"*" 不是合法正则)。
	policies := [][]string{
		{"role:admin", "/api/v1/*", ".*"},
		{"role:developer", "/api/v1/auth/*", ".*"},
		{"role:user", "/api/v1/auth/*", ".*"},
		{"role:developer", "/api/v1/subjects*", ".*"},
		{"role:developer", "/api/v1/query", "GET|POST"},
		{"role:developer", "/api/v1/pages*", ".*"},
		{"role:developer", "/api/v1/menus*", ".*"},
		{"role:developer", "/api/v1/studio*", ".*"},
		{"role:developer", "/api/v1/portal/*", "GET"},
		{"role:user", "/api/v1/portal/*", "GET"},
		// 门户用户需上报渲染错误(POST /api/v1/pages/:id/render-errors,G2Chart 自动上报;
		// 此前仅 GET 权限导致上报被 403 静默吞掉)。matcher 为 keyMatch(前缀匹配,`*` 之后
		// 不再比较),无法用精确策略收窄,故扩到 GET|POST;已核对 /api/v1/pages/* 下其余
		// POST 路由(publish/unpublish/versions restore)在 handler 内均有 requireManager 兜底,
		// render-errors 自身按 CanView 对象级放行。
		{"role:user", "/api/v1/pages/*", "GET|POST"},
		// 门户图表取数:对象级授权(可见页面引用的主题)在 query handler 校验
		{"role:user", "/api/v1/query", "POST"},
	}
	for _, p := range policies {
		has, err := e.HasPolicy(p[0], p[1], p[2])
		if err != nil {
			return err
		}
		if !has {
			if _, err := e.AddPolicy(p[0], p[1], p[2]); err != nil {
				return err
			}
		}
	}

	var admin model.User
	if err := d.Where("username = ?", "admin").First(&admin).Error; err != nil {
		return err
	}
	sub, role := rbac.UserSubject(admin.ID), rbac.RoleSubject("admin")
	has, err := e.HasGroupingPolicy(sub, role)
	if err != nil {
		return err
	}
	if !has {
		if _, err := e.AddGroupingPolicy(sub, role); err != nil {
			return err
		}
	}
	return nil
}
