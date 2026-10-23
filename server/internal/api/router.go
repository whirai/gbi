package api

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"gbi/server/internal/agent"
	"gbi/server/internal/assets"
	"gbi/server/internal/auth"
	"gbi/server/internal/config"
	"gbi/server/internal/logger"
	"gbi/server/internal/rbac"
	"gbi/server/internal/service"
)

func NewRouter(d *gorm.DB, e *casbin.Enforcer, cfg *config.Config) *gin.Engine {
	start := time.Now()
	slog.Debug("方法进入", "fn", "api.NewRouter")
	defer func() {
		slog.Debug("方法离开", "fn", "api.NewRouter", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	llmCfg := service.NewLLMConfigService(d) // 当前模型服务:studio 每请求解析(管理 API 复用同一实例)
	r := gin.New()
	r.Use(logger.GinMiddleware(), gin.Recovery())
	r.Use(corsMiddleware(cfg))
	r.GET("/api/v1/health", func(c *gin.Context) {
		OK(c, gin.H{"status": "ok"})
	})

	// 品牌公共只读(免鉴权):登录页/门户运行时注入系统名与图标
	bh := NewBrandingHandler(service.NewBrandingService(d, cfg.DataDir()))
	r.GET("/api/v1/branding", bh.Get)
	r.GET("/api/v1/branding/image/:kind", bh.Image)

	authSvc := service.NewAuthService(d, []byte(cfg.JWTSecret), cfg.JWTTTL)
	ah := NewAuthHandler(authSvc)
	r.POST("/api/v1/auth/login", ah.Login)

	authed := r.Group("/api/v1", auth.Middleware([]byte(cfg.JWTSecret)), rbac.Middleware(e))
	authed.GET("/auth/me", ah.Me)
	authed.POST("/auth/password", ah.ChangePassword)

	dh := NewDeptHandler(service.NewDeptService(d))
	authed.GET("/departments", dh.Tree)
	authed.POST("/departments", dh.Create)
	authed.PUT("/departments/:id", dh.Update)
	authed.DELETE("/departments/:id", dh.Delete)

	uh := NewUserHandler(service.NewUserService(d, e))
	authed.GET("/users", uh.List)
	authed.POST("/users", uh.Create)
	authed.PUT("/users/:id", uh.Update)
	authed.DELETE("/users/:id", uh.Delete)
	authed.POST("/users/:id/password", uh.ResetPassword)

	gh := NewGroupHandler(service.NewGroupService(d, e))
	authed.GET("/groups", gh.List)
	authed.POST("/groups", gh.Create)
	authed.PUT("/groups/:id", gh.Update)
	authed.DELETE("/groups/:id", gh.Delete)
	authed.GET("/groups/:id/members", gh.Members)
	authed.POST("/groups/:id/members", gh.AddMembers)
	authed.DELETE("/groups/:id/members/:uid", gh.RemoveMember)

	rh := NewRoleHandler(service.NewRoleService(d))
	authed.GET("/roles", rh.List)
	authed.PUT("/roles/:id", rh.Rename)

	pageSvc := service.NewPageService(d, e)
	ph := NewPageHandler(pageSvc)
	authed.GET("/pages", ph.List)
	authed.POST("/pages", ph.Create)
	authed.GET("/pages/:id", ph.Get)
	authed.PUT("/pages/:id", ph.Update)
	authed.DELETE("/pages/:id", ph.Delete)
	authed.POST("/pages/:id/publish", ph.Publish)
	authed.POST("/pages/:id/unpublish", ph.Unpublish)
	authed.GET("/pages/:id/versions", ph.Versions)
	authed.POST("/pages/:id/versions/:vid/restore", ph.Restore)
	authed.GET("/pages/:id/acl", ph.GetACL)
	authed.PUT("/pages/:id/acl", ph.SetACL)

	reh := NewRenderErrorHandler(service.NewRenderErrorService(d), pageSvc)
	authed.POST("/pages/:id/render-errors", reh.Report)

	menuSvc := service.NewMenuService(d, pageSvc)
	mh := NewMenuHandler(menuSvc)
	authed.GET("/menus", mh.List)
	authed.POST("/menus", mh.Create)
	authed.PUT("/menus/:id", mh.Update)
	authed.DELETE("/menus/:id", mh.Delete)
	authed.GET("/menus/:id/items", mh.ListItems)
	authed.POST("/menus/:id/items", mh.CreateItem)
	authed.PUT("/menus/:id/items/:iid", mh.UpdateItem)
	authed.DELETE("/menus/:id/items/:iid", mh.DeleteItem)

	poh := NewPortalHandler(pageSvc, menuSvc)
	authed.GET("/portal/pages", poh.Pages)
	authed.GET("/portal/menus", poh.Menus)

	subjectSvc := service.NewSubjectService(d, cfg)
	tableSvc := service.NewTableService(d, subjectSvc)
	sh := NewSubjectHandler(subjectSvc)
	th := NewTableHandler(tableSvc, service.NewRowsService(d, tableSvc))
	authed.GET("/subjects", sh.List)
	authed.POST("/subjects", sh.Create)
	authed.PUT("/subjects/:id", sh.Update)
	authed.DELETE("/subjects/:id", sh.Delete)
	authed.GET("/subjects/:id/tables", th.List)
	authed.POST("/subjects/:id/tables", th.Create)
	authed.DELETE("/subjects/:id/tables/:tid", th.Delete)
	authed.GET("/subjects/:id/tables/:tid/rows", th.ListRows)
	authed.PUT("/subjects/:id/tables/:tid/rows/:rowid", th.UpdateCell)
	authed.DELETE("/subjects/:id/tables/:tid/rows/:rowid", th.DeleteRow)
	// 列筛选下拉的候选值(只读)
	authed.GET("/subjects/:id/tables/:tid/columns/:column/values", th.DistinctValues)
	ih := NewImportHandler(service.NewImportService(d, subjectSvc, tableSvc))
	authed.POST("/subjects/:id/import", ih.Run)

	qh := NewQueryHandler(service.NewQueryService(d), pageSvc)
	authed.POST("/query", qh.Run)

	// 品牌写接口(仅 admin;admin 策略 /api/v1/* 通配无需新增 Casbin 规则)
	authed.PUT("/branding", bh.Update)
	authed.POST("/branding/:kind", bh.Upload)
	authed.DELETE("/branding/:kind", bh.DeleteImage)
	authed.DELETE("/branding", bh.Reset)

	lh := NewLLMHandler(llmCfg)
	authed.GET("/llm/providers", lh.ListProviders)
	authed.POST("/llm/providers", lh.CreateProvider)
	authed.PUT("/llm/providers/:id", lh.UpdateProvider)
	authed.DELETE("/llm/providers/:id", lh.DeleteProvider)
	authed.GET("/llm/providers/:id/models", lh.ListModels)
	authed.POST("/llm/providers/:id/models", lh.CreateModel)
	authed.PUT("/llm/models/:id", lh.UpdateModel)
	authed.DELETE("/llm/models/:id", lh.DeleteModel)
	authed.POST("/llm/models/:id/current", lh.SetCurrentModel)
	authed.GET("/llm/current", lh.Current)

	// studio(阶段 4):skills 目录配置后才启用;磁盘目录存在优先,否则内置(分发版)
	if cfg.SkillsDir != "" {
		skillsFS, skillsSource := assets.Resolve(cfg.SkillsDir, assets.SkillsFS)
		t2s, g2, serr := agent.LoadSkillsFS(skillsFS)
		if serr != nil {
			// 磁盘与内置两来源均不可读 → 启动期暴露
			panic(fmt.Sprintf("skills 加载失败(已尝试磁盘 %q 与内置): %v", cfg.SkillsDir, serr))
		}
		slog.Info("skills 已加载", "source", skillsSource, "dir", cfg.SkillsDir)
		exec := agent.NewToolExecutor(d, e, subjectSvc, tableSvc, pageSvc,
			service.NewRenderErrorService(d), time.Duration(cfg.RenderCheckWaitMs)*time.Millisecond)
		studioHandler := NewStudioHandler(service.NewStudioService(d), pageSvc, exec, t2s, g2, llmCfg, cfg.HistoryKeep)
		authed.POST("/studio/chat", studioHandler.Chat)
		authed.GET("/studio/conversations/:page_id", studioHandler.Conversation)
		authed.DELETE("/studio/conversations/:page_id", studioHandler.Delete)
	}
	return r
}
