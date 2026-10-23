package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"gbi/server/internal/api"
	"gbi/server/internal/config"
	"gbi/server/internal/db"
	"gbi/server/internal/rbac"
	"gbi/server/internal/service"
)

type env struct{ h http.Handler }

func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	require.NoError(t, db.Migrate(d))
	require.NoError(t, db.Seed(d))
	e, err := rbac.NewEnforcer(d)
	require.NoError(t, err)
	require.NoError(t, db.SeedRBAC(d, e))
	cfg := &config.Config{Addr: ":0", DBPath: filepath.Join(dir, "test.db"), JWTSecret: "test-secret", JWTTTL: time.Hour, CORSOrigin: "*"}
	return &env{h: api.NewRouter(d, e, cfg)}
}

func (e *env) do(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	return w
}

func extractToken(t *testing.T, body string) string {
	t.Helper()
	re := regexp.MustCompile(`"token":"([^"]+)"`)
	m := re.FindStringSubmatch(body)
	require.NotEmpty(t, m, "response has no token: %s", body)
	return m[1]
}

func extractID(t *testing.T, body string) int {
	t.Helper()
	re := regexp.MustCompile(`"id":(\d+)`)
	m := re.FindStringSubmatch(body)
	require.NotEmpty(t, m, "response has no id: %s", body)
	id, err := strconv.Atoi(m[1])
	require.NoError(t, err)
	return id
}

func loginAdmin(t *testing.T, e *env) string {
	t.Helper()
	w := e.do(t, "POST", "/api/v1/auth/login", "", map[string]string{
		"username": "admin", "password": "admin123",
	})
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Data.Token)
	return resp.Data.Token
}

func TestHealth(t *testing.T) {
	e := newEnv(t)
	w := e.do(t, "GET", "/api/v1/health", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestLoginFlow(t *testing.T) {
	e := newEnv(t)
	token := loginAdmin(t, e)

	w := e.do(t, "GET", "/api/v1/auth/me", token, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"username":"admin"`)
}

func TestLoginWrongPassword(t *testing.T) {
	e := newEnv(t)
	w := e.do(t, "POST", "/api/v1/auth/login", "", map[string]string{
		"username": "admin", "password": "nope",
	})
	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Contains(t, w.Body.String(), "用户名或密码错误")
}

func TestMeRequiresToken(t *testing.T) {
	e := newEnv(t)
	w := e.do(t, "GET", "/api/v1/auth/me", "", nil)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestChangePasswordRejectsShort(t *testing.T) {
	e := newEnv(t)
	token := loginAdmin(t, e)
	w := e.do(t, "POST", "/api/v1/auth/password", token, map[string]string{
		"old_password": db.AdminPassword, "new_password": "123",
	})
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "新密码至少6位")
}

// TestChangePasswordWrongOld 原密码错误返回 400(而非 401):本请求已鉴权,
// 401 会被 web client 误判为会话过期并强制登出。
func TestChangePasswordWrongOld(t *testing.T) {
	e := newEnv(t)
	token := loginAdmin(t, e)
	w := e.do(t, "POST", "/api/v1/auth/password", token, map[string]string{
		"old_password": "wrong-old", "new_password": "newpass9",
	})
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "原密码错误")
	require.Contains(t, w.Body.String(), "1001") // 业务码保留
}

func TestRBACDeniesUnprivilegedPath(t *testing.T) {
	e := newEnv(t)
	// admin 可访问 /api/v1/users(Task 5 才有真实路由,此处只验证鉴权放行到 404/200)
	token := loginAdmin(t, e)
	w := e.do(t, "GET", "/api/v1/health", token, nil)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestDeptAPIFlow(t *testing.T) {
	e := newEnv(t)
	token := loginAdmin(t, e)

	w := e.do(t, "POST", "/api/v1/departments", token, map[string]any{
		"name": "总公司", "parent_id": nil, "sort": 0,
	})
	require.Equal(t, http.StatusOK, w.Code)
	var created struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.Positive(t, created.Data.ID)

	w = e.do(t, "GET", "/api/v1/departments", token, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "总公司")

	w = e.do(t, "DELETE", "/api/v1/departments/999999", token, nil)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestRegularUserDeniedUserAPI(t *testing.T) {
	e := newEnv(t)
	adminToken := loginAdmin(t, e)

	// 创建普通用户(role_id=3)
	w := e.do(t, "POST", "/api/v1/users", adminToken, map[string]any{
		"username": "plain1", "password": "pass1234",
		"name": "普通甲", "role_id": 3, "dept_id": nil,
	})
	require.Equal(t, http.StatusOK, w.Code)

	// 普通用户登录
	w = e.do(t, "POST", "/api/v1/auth/login", "", map[string]string{
		"username": "plain1", "password": "pass1234",
	})
	require.Equal(t, http.StatusOK, w.Code)
	var lr struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &lr))

	// 能访问 /auth/me,不能访问 /users
	w = e.do(t, "GET", "/api/v1/auth/me", lr.Data.Token, nil)
	require.Equal(t, http.StatusOK, w.Code)
	w = e.do(t, "GET", "/api/v1/users", lr.Data.Token, nil)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestFullOrgFlow(t *testing.T) {
	e := newEnv(t)
	token := loginAdmin(t, e)

	// 角色列表
	w := e.do(t, "GET", "/api/v1/roles", token, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "developer")

	// 部门
	w = e.do(t, "POST", "/api/v1/departments", token, map[string]any{"name": "销售部"})
	require.Equal(t, http.StatusOK, w.Code)

	// 用户
	w = e.do(t, "POST", "/api/v1/users", token, map[string]any{
		"username": "flow1", "password": "pass1234", "name": "流程甲", "role_id": 2,
	})
	require.Equal(t, http.StatusOK, w.Code)
	var ur struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ur))

	// 组 + 成员
	w = e.do(t, "POST", "/api/v1/groups", token, map[string]any{"name": "流程组", "desc": ""})
	require.Equal(t, http.StatusOK, w.Code)
	var gr struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &gr))

	w = e.do(t, "POST", "/api/v1/groups/"+strconv.Itoa(int(gr.Data.ID))+"/members", token,
		map[string]any{"user_ids": []uint{ur.Data.ID}})
	require.Equal(t, http.StatusOK, w.Code)

	w = e.do(t, "GET", "/api/v1/groups/"+strconv.Itoa(int(gr.Data.ID))+"/members", token, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "flow1")

	// 重置密码后用新密码登录
	w = e.do(t, "POST", "/api/v1/users/"+strconv.Itoa(int(ur.Data.ID))+"/password", token,
		map[string]any{"password": "newpass9"})
	require.Equal(t, http.StatusOK, w.Code)
	w = e.do(t, "POST", "/api/v1/auth/login", "", map[string]string{
		"username": "flow1", "password": "newpass9",
	})
	require.Equal(t, http.StatusOK, w.Code)
}

func TestSubjectAPIFlowAndRBAC(t *testing.T) {
	e := newEnv(t)
	adminToken := loginAdmin(t, e)

	// admin 创建主题
	w := e.do(t, "POST", "/api/v1/subjects", adminToken, map[string]any{
		"name": "销售主题", "code": "sales", "desc": "",
	})
	require.Equal(t, http.StatusOK, w.Code)

	// 非法 code
	w = e.do(t, "POST", "/api/v1/subjects", adminToken, map[string]any{
		"name": "x", "code": "1bad",
	})
	require.Equal(t, http.StatusBadRequest, w.Code)

	// 创建 developer 用户(role_id=2)并登录
	w = e.do(t, "POST", "/api/v1/users", adminToken, map[string]any{
		"username": "dev2", "password": "pass1234", "name": "开发乙", "role_id": 2,
	})
	require.Equal(t, http.StatusOK, w.Code)
	w = e.do(t, "POST", "/api/v1/auth/login", "", map[string]string{
		"username": "dev2", "password": "pass1234",
	})
	require.Equal(t, http.StatusOK, w.Code)
	devToken := extractToken(t, w.Body.String())

	// developer 可以访问主题 API
	w = e.do(t, "GET", "/api/v1/subjects", devToken, nil)
	require.Equal(t, http.StatusOK, w.Code)

	// 创建普通用户(role_id=3),主题 API 必须 403
	w = e.do(t, "POST", "/api/v1/users", adminToken, map[string]any{
		"username": "plain2", "password": "pass1234", "name": "普通乙", "role_id": 3,
	})
	require.Equal(t, http.StatusOK, w.Code)
	w = e.do(t, "POST", "/api/v1/auth/login", "", map[string]string{
		"username": "plain2", "password": "pass1234",
	})
	require.Equal(t, http.StatusOK, w.Code)
	plainToken := extractToken(t, w.Body.String())
	w = e.do(t, "GET", "/api/v1/subjects", plainToken, nil)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestQueryAPI(t *testing.T) {
	e := newEnv(t)
	token := loginAdmin(t, e)

	// 创建主题 + 表
	w := e.do(t, "POST", "/api/v1/subjects", token, map[string]any{
		"name": "查询主题", "code": "qt",
	})
	require.Equal(t, http.StatusOK, w.Code)
	subID := extractID(t, w.Body.String())

	w = e.do(t, "POST", "/api/v1/subjects/"+strconv.Itoa(subID)+"/tables", token, map[string]any{
		"name": "orders", "kind": "DWD",
		"columns": []map[string]any{{"name": "region", "type": "TEXT"}},
	})
	require.Equal(t, http.StatusOK, w.Code)

	// 空表查询:返回列名与空行
	w = e.do(t, "POST", "/api/v1/query", token, map[string]any{
		"sql": "SELECT region FROM qt.dwd_orders", "subject_ids": []uint{uint(subID)},
	})
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"columns":["region"]`)

	// 非法 SQL → 400
	w = e.do(t, "POST", "/api/v1/query", token, map[string]any{
		"sql": "DELETE FROM qt.dwd_orders", "subject_ids": []uint{uint(subID)},
	})
	require.Equal(t, http.StatusBadRequest, w.Code)

	// 未选主题 → 400
	w = e.do(t, "POST", "/api/v1/query", token, map[string]any{
		"sql": "SELECT 1", "subject_ids": []uint{},
	})
	require.Equal(t, http.StatusBadRequest, w.Code)

	// 主题不存在 → 404
	w = e.do(t, "POST", "/api/v1/query", token, map[string]any{
		"sql": "SELECT 1", "subject_ids": []uint{999999},
	})
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestPageAPIFlowAndRBAC(t *testing.T) {
	e := newEnv(t)
	adminToken := loginAdmin(t, e)

	// admin 创建页面(draft)
	w := e.do(t, "POST", "/api/v1/pages", adminToken, map[string]any{"name": "销售总览", "desc": "d"})
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"status":"draft"`)
	pageID := extractID(t, w.Body.String())

	// 更新 config → 生成版本
	w = e.do(t, "PUT", "/api/v1/pages/"+strconv.Itoa(pageID), adminToken, map[string]any{
		"config": map[string]any{
			"version": 1, "layout": "fluid-grid",
			"charts": []map[string]any{{"id": "c1", "title": "t", "width": "full", "sql": "SELECT 1"}},
		},
	})
	require.Equal(t, http.StatusOK, w.Code)
	w = e.do(t, "GET", "/api/v1/pages/"+strconv.Itoa(pageID)+"/versions", adminToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"version":1`)

	// 非法 config → 400
	w = e.do(t, "PUT", "/api/v1/pages/"+strconv.Itoa(pageID), adminToken, map[string]any{
		"config": map[string]any{
			"charts": []map[string]any{{"width": "bad", "sql": "SELECT 1"}},
		},
	})
	require.Equal(t, http.StatusBadRequest, w.Code)

	// 发布
	w = e.do(t, "POST", "/api/v1/pages/"+strconv.Itoa(pageID)+"/publish", adminToken, nil)
	require.Equal(t, http.StatusOK, w.Code)

	// 创建普通用户(role 3)
	w = e.do(t, "POST", "/api/v1/users", adminToken, map[string]any{
		"username": "pv1", "password": "pass1234", "name": "页面访客", "role_id": 3,
	})
	require.Equal(t, http.StatusOK, w.Code)
	w = e.do(t, "POST", "/api/v1/auth/login", "", map[string]string{"username": "pv1", "password": "pass1234"})
	require.Equal(t, http.StatusOK, w.Code)
	userToken := extractToken(t, w.Body.String())

	// 普通用户:管理列表 403;未授权详情 404(不泄露存在性)
	w = e.do(t, "GET", "/api/v1/pages", userToken, nil)
	require.Equal(t, http.StatusForbidden, w.Code)
	w = e.do(t, "GET", "/api/v1/pages/"+strconv.Itoa(pageID), userToken, nil)
	require.Equal(t, http.StatusNotFound, w.Code)

	// 门户端点在 Task 6 注册;此处验证未授权详情页恢复版本需 admin
	w = e.do(t, "POST", "/api/v1/pages/"+strconv.Itoa(pageID)+"/versions/999999/restore", adminToken, nil)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestPageACLFlow(t *testing.T) {
	e := newEnv(t)
	adminToken := loginAdmin(t, e)

	w := e.do(t, "POST", "/api/v1/pages", adminToken, map[string]any{"name": "ACL页"})
	require.Equal(t, http.StatusOK, w.Code)
	pageID := extractID(t, w.Body.String())
	w = e.do(t, "POST", "/api/v1/pages/"+strconv.Itoa(pageID)+"/publish", adminToken, nil)
	require.Equal(t, http.StatusOK, w.Code)

	// 创建普通用户
	w = e.do(t, "POST", "/api/v1/users", adminToken, map[string]any{
		"username": "aclx", "password": "pass1234", "name": "ACL访客", "role_id": 3,
	})
	require.Equal(t, http.StatusOK, w.Code)
	userID := extractID(t, w.Body.String())
	w = e.do(t, "POST", "/api/v1/auth/login", "", map[string]string{"username": "aclx", "password": "pass1234"})
	userToken := extractToken(t, w.Body.String())

	// 未授权 → 详情 404
	w = e.do(t, "GET", "/api/v1/pages/"+strconv.Itoa(pageID), userToken, nil)
	require.Equal(t, http.StatusNotFound, w.Code)

	// 授权 user → 详情可见
	w = e.do(t, "PUT", "/api/v1/pages/"+strconv.Itoa(pageID)+"/acl", adminToken, map[string]any{
		"grants": []map[string]any{{"subject_type": "user", "subject_id": userID}},
	})
	require.Equal(t, http.StatusOK, w.Code)
	w = e.do(t, "GET", "/api/v1/pages/"+strconv.Itoa(pageID), userToken, nil)
	require.Equal(t, http.StatusOK, w.Code)

	// GET acl 返回 1 条
	w = e.do(t, "GET", "/api/v1/pages/"+strconv.Itoa(pageID)+"/acl", adminToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"subject_type":"user"`)

	// 清空授权 → 又不可见
	w = e.do(t, "PUT", "/api/v1/pages/"+strconv.Itoa(pageID)+"/acl", adminToken, map[string]any{
		"grants": []map[string]any{},
	})
	require.Equal(t, http.StatusOK, w.Code)
	w = e.do(t, "GET", "/api/v1/pages/"+strconv.Itoa(pageID), userToken, nil)
	require.Equal(t, http.StatusNotFound, w.Code)

	// developer(role 2)管理 ACL → 403(权限管理仅 admin)
	w = e.do(t, "POST", "/api/v1/users", adminToken, map[string]any{
		"username": "acldev", "password": "pass1234", "name": "ACL开发者", "role_id": 2,
	})
	require.Equal(t, http.StatusOK, w.Code)
	w = e.do(t, "POST", "/api/v1/auth/login", "", map[string]string{"username": "acldev", "password": "pass1234"})
	require.Equal(t, http.StatusOK, w.Code)
	devToken := extractToken(t, w.Body.String())
	w = e.do(t, "GET", "/api/v1/pages/"+strconv.Itoa(pageID)+"/acl", devToken, nil)
	require.Equal(t, http.StatusForbidden, w.Code)
	w = e.do(t, "PUT", "/api/v1/pages/"+strconv.Itoa(pageID)+"/acl", devToken, map[string]any{
		"grants": []map[string]any{},
	})
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestMenuAndPortalFlow(t *testing.T) {
	e := newEnv(t)
	adminToken := loginAdmin(t, e)

	// 页面:创建+发布
	w := e.do(t, "POST", "/api/v1/pages", adminToken, map[string]any{"name": "门户页"})
	require.Equal(t, http.StatusOK, w.Code)
	pageID := extractID(t, w.Body.String())
	w = e.do(t, "POST", "/api/v1/pages/"+strconv.Itoa(pageID)+"/publish", adminToken, nil)
	require.Equal(t, http.StatusOK, w.Code)

	// 菜单+菜单项
	w = e.do(t, "POST", "/api/v1/menus", adminToken, map[string]any{"name": "销售菜单", "desc": "", "sort": 1})
	require.Equal(t, http.StatusOK, w.Code)
	menuID := extractID(t, w.Body.String())
	w = e.do(t, "POST", "/api/v1/menus/"+strconv.Itoa(menuID)+"/items", adminToken, map[string]any{
		"page_id": pageID, "name": "总览", "sort": 1,
	})
	require.Equal(t, http.StatusOK, w.Code)

	// 普通用户(未授权):门户为空;授权后可见 1 菜单 1 项
	w = e.do(t, "POST", "/api/v1/users", adminToken, map[string]any{
		"username": "portal1", "password": "pass1234", "name": "门户访客", "role_id": 3,
	})
	require.Equal(t, http.StatusOK, w.Code)
	userID := extractID(t, w.Body.String())
	w = e.do(t, "POST", "/api/v1/auth/login", "", map[string]string{"username": "portal1", "password": "pass1234"})
	userToken := extractToken(t, w.Body.String())

	w = e.do(t, "GET", "/api/v1/portal/menus", userToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"data":[]`)

	w = e.do(t, "PUT", "/api/v1/pages/"+strconv.Itoa(pageID)+"/acl", adminToken, map[string]any{
		"grants": []map[string]any{{"subject_type": "user", "subject_id": userID}},
	})
	require.Equal(t, http.StatusOK, w.Code)

	w = e.do(t, "GET", "/api/v1/portal/menus", userToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "销售菜单")
	require.Contains(t, w.Body.String(), "总览")

	w = e.do(t, "GET", "/api/v1/portal/pages", userToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "门户页")

	// 普通用户管理菜单 → 403(casbin)
	w = e.do(t, "POST", "/api/v1/menus", userToken, map[string]any{"name": "x"})
	require.Equal(t, http.StatusForbidden, w.Code)

	// 开发者可管理菜单(role 2)
	w = e.do(t, "POST", "/api/v1/users", adminToken, map[string]any{
		"username": "mdev1", "password": "pass1234", "name": "菜单开发者", "role_id": 2,
	})
	require.Equal(t, http.StatusOK, w.Code)
	w = e.do(t, "POST", "/api/v1/auth/login", "", map[string]string{"username": "mdev1", "password": "pass1234"})
	devToken := extractToken(t, w.Body.String())
	w = e.do(t, "GET", "/api/v1/menus", devToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
}

// newEnvWithLLM 与 newEnv 相同,但播种当前模型(指向 fake Anthropic 端点)并注入 SkillsDir(启用 studio 接线)。
func newEnvWithLLM(t *testing.T, llmURL, skillsDir string) *env {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	require.NoError(t, db.Migrate(d))
	require.NoError(t, db.Seed(d))
	e, err := rbac.NewEnforcer(d)
	require.NoError(t, err)
	require.NoError(t, db.SeedRBAC(d, e))
	require.NoError(t, service.NewLLMConfigService(d).Seed(service.SeedInput{
		ProviderName: "fake", BaseURL: llmURL, APIKey: "fake",
		ModelName: "fake-model", ContextWindow: 384000, MaxTokens: 16384,
	}))
	cfg := &config.Config{
		Addr: ":0", DBPath: filepath.Join(dir, "test.db"),
		JWTSecret: "test-secret", JWTTTL: time.Hour,
		CORSOrigin: "*",
		SkillsDir:  skillsDir,
	}
	return &env{h: api.NewRouter(d, e, cfg)}
}

// writeSkills 在临时目录写两个最小 SKILL.md。
func writeSkills(t *testing.T) string {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "text2sql"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "antv-g2-chart"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "text2sql", "SKILL.md"), []byte("t2s"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "antv-g2-chart", "SKILL.md"), []byte("g2"), 0o644))
	return dir
}

// writeTextSSE 固定返回文本 end_turn(不走工具)。
func writeTextSSE(w http.ResponseWriter, text string) {
	delta, _ := json.Marshal(text)
	fmt.Fprintf(w, "event: message_start\ndata: %s\n\n", `{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","model":"fake","content":[],"stop_reason":null,"usage":{"input_tokens":1,"output_tokens":1}}}`)
	fmt.Fprintf(w, "event: content_block_start\ndata: %s\n\n", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
	fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":`+string(delta)+`}}`)
	fmt.Fprintf(w, "event: content_block_stop\ndata: %s\n\n", `{"type":"content_block_stop","index":0}`)
	fmt.Fprintf(w, "event: message_delta\ndata: %s\n\n", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`)
	fmt.Fprintf(w, "event: message_stop\ndata: %s\n\n", `{"type":"message_stop"}`)
}

func TestStudioChatSSE(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeTextSSE(w, "好的")
	}))
	defer fake.Close()

	e := newEnvWithLLM(t, fake.URL, writeSkills(t))
	token := loginAdmin(t, e)

	// 主题 + 页面
	w := e.do(t, "POST", "/api/v1/subjects", token, map[string]any{"name": "s", "code": "s"})
	require.Equal(t, http.StatusOK, w.Code)
	subID := extractID(t, w.Body.String())
	w = e.do(t, "POST", "/api/v1/pages", token, map[string]any{"name": "p", "desc": ""})
	require.Equal(t, http.StatusOK, w.Code)
	pageID := extractID(t, w.Body.String())

	// SSE 聊天
	w = e.do(t, "POST", "/api/v1/studio/chat", token, map[string]any{
		"page_id": pageID, "subject_ids": []uint{uint(subID)}, "message": "画个图",
	})
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "event: message_delta")
	require.Contains(t, w.Body.String(), "event: done")
	// 流必须以 done 事件收尾,且连接显式 close(经代理转发时避免
	// chunked 尾块竞态导致客户端 ERR_INCOMPLETE_CHUNKED_ENCODING,线上事故回归)
	require.True(t, strings.HasSuffix(w.Body.String(), "event: done\ndata: {}\n\n"),
		"SSE 流应完整以 done 事件收尾")
	require.Equal(t, "close", w.Header().Get("Connection"),
		"SSE 响应应声明 Connection: close,由服务端确定性关闭流")

	// 会话持久化
	w = e.do(t, "GET", "/api/v1/studio/conversations/"+strconv.Itoa(pageID), token, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"role":"user"`)
	require.Contains(t, w.Body.String(), `"role":"assistant"`)
	require.Contains(t, w.Body.String(), "画个图")
	require.Contains(t, w.Body.String(), "好的")
}

// TestStudioDeleteConversation 新对话:第二个 manager 删除会话后消息清空,页面配置不受影响。
// 回归:DeleteConversation 必须按 page_id 定位(与 GetOrCreate 对称),若仍带 created_by 条件,
// 页面被第二个 manager 打开时「新对话」会静默 no-op 却返回 deleted:true,消息残留于此可被发现。
func TestStudioDeleteConversation(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeTextSSE(w, "好的")
	}))
	defer fake.Close()

	e := newEnvWithLLM(t, fake.URL, writeSkills(t))
	token := loginAdmin(t, e)

	// 主题 + 页面
	w := e.do(t, "POST", "/api/v1/subjects", token, map[string]any{"name": "s", "code": "s"})
	require.Equal(t, http.StatusOK, w.Code)
	subID := extractID(t, w.Body.String())
	w = e.do(t, "POST", "/api/v1/pages", token, map[string]any{"name": "p", "desc": ""})
	require.Equal(t, http.StatusOK, w.Code)
	pageID := extractID(t, w.Body.String())

	// admin 经 chat 造消息(会话 created_by=admin,含 user+assistant 两条)
	w = e.do(t, "POST", "/api/v1/studio/chat", token, map[string]any{
		"page_id": pageID, "subject_ids": []uint{uint(subID)}, "message": "画个图",
	})
	require.Equal(t, http.StatusOK, w.Code)
	w = e.do(t, "GET", "/api/v1/studio/conversations/"+strconv.Itoa(pageID), token, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"role":"user"`)

	// 第二个 manager(developer,非会话创建者)点「新对话」删除会话
	w = e.do(t, "POST", "/api/v1/users", token, map[string]any{
		"username": "deldev", "password": "pass1234", "name": "删除开发者", "role_id": 2,
	})
	require.Equal(t, http.StatusOK, w.Code)
	w = e.do(t, "POST", "/api/v1/auth/login", "", map[string]string{"username": "deldev", "password": "pass1234"})
	require.Equal(t, http.StatusOK, w.Code)
	devToken := extractToken(t, w.Body.String())

	// 删除会话(页面配置不受影响)
	w = e.do(t, "DELETE", "/api/v1/studio/conversations/"+strconv.Itoa(pageID), devToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"deleted":true`)
	w = e.do(t, "GET", "/api/v1/pages/"+strconv.Itoa(pageID), token, nil)
	require.Equal(t, http.StatusOK, w.Code)

	// 再 GET:会话重建且消息为空(若静默 no-op,此处会残留"画个图"/"好的")
	w = e.do(t, "GET", "/api/v1/studio/conversations/"+strconv.Itoa(pageID), token, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"messages":[]`)
}

// TestPageRenderErrorReport 前端渲染错误上报:作者可报 200;缺 message 400;不存在页面 404。
func TestPageRenderErrorReport(t *testing.T) {
	e := newEnv(t)
	token := loginAdmin(t, e)

	w := e.do(t, "POST", "/api/v1/pages", token, map[string]any{"name": "p", "desc": ""})
	require.Equal(t, http.StatusOK, w.Code)
	pageID := extractID(t, w.Body.String())

	// 正常上报
	w = e.do(t, "POST", "/api/v1/pages/"+strconv.Itoa(pageID)+"/render-errors", token, map[string]any{
		"chart_id": "c1", "message": "Unknown Component: labelTransform.overall",
	})
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"reported":true`)

	// 缺 message → 400
	w = e.do(t, "POST", "/api/v1/pages/"+strconv.Itoa(pageID)+"/render-errors", token, map[string]any{"chart_id": "c1"})
	require.Equal(t, http.StatusBadRequest, w.Code)

	// 不存在页面 → 404
	w = e.do(t, "POST", "/api/v1/pages/9999/render-errors", token, map[string]any{"chart_id": "c1", "message": "x"})
	require.Equal(t, http.StatusNotFound, w.Code)
}

// TestPortalUserRenderErrorReport 门户用户(role:user)对已授权页面必须能上报渲染错误:
// RBAC 策略 /api/v1/pages/* 已扩为 GET|POST;此前仅 GET 权限,门户上报被 403 静默吞掉,
// studio 渲染校验闭环看不到真实用户侧的失败。未授权页面仍由 CanView 对象级校验 403。
func TestPortalUserRenderErrorReport(t *testing.T) {
	e := newEnv(t)
	admin := loginAdmin(t, e)

	w := e.do(t, "POST", "/api/v1/pages", admin, map[string]any{"name": "门户页", "desc": ""})
	require.Equal(t, http.StatusOK, w.Code)
	pageID := extractID(t, w.Body.String())
	w = e.do(t, "POST", "/api/v1/pages/"+strconv.Itoa(pageID)+"/publish", admin, nil)
	require.Equal(t, http.StatusOK, w.Code)

	// 第二个页面:已发布但不授权
	w = e.do(t, "POST", "/api/v1/pages", admin, map[string]any{"name": "未授权页", "desc": ""})
	require.Equal(t, http.StatusOK, w.Code)
	otherID := extractID(t, w.Body.String())
	w = e.do(t, "POST", "/api/v1/pages/"+strconv.Itoa(otherID)+"/publish", admin, nil)
	require.Equal(t, http.StatusOK, w.Code)

	// 普通用户(role 3)
	w = e.do(t, "POST", "/api/v1/users", admin, map[string]any{
		"username": "rep1", "password": "pass1234", "name": "上报用户", "role_id": 3,
	})
	require.Equal(t, http.StatusOK, w.Code)
	userID := extractID(t, w.Body.String())
	w = e.do(t, "POST", "/api/v1/auth/login", "", map[string]string{"username": "rep1", "password": "pass1234"})
	require.Equal(t, http.StatusOK, w.Code)
	userToken := extractToken(t, w.Body.String())

	// 未授权页面:RBAC 放行,CanView 对象级拒绝
	w = e.do(t, "POST", "/api/v1/pages/"+strconv.Itoa(otherID)+"/render-errors", userToken,
		map[string]any{"chart_id": "c1", "message": "x"})
	require.Equal(t, http.StatusForbidden, w.Code)

	// 授权后:上报成功(而非被 RBAC 403 拦截)
	w = e.do(t, "PUT", "/api/v1/pages/"+strconv.Itoa(pageID)+"/acl", admin, map[string]any{
		"grants": []map[string]any{{"subject_type": "user", "subject_id": userID}},
	})
	require.Equal(t, http.StatusOK, w.Code)
	w = e.do(t, "POST", "/api/v1/pages/"+strconv.Itoa(pageID)+"/render-errors", userToken, map[string]any{
		"chart_id": "c1", "message": "Unknown Component: labelTransform.overall",
	})
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"reported":true`)
}

// TestDeveloperStudioAccess developer(role 2)必须通过 RBAC 访问 studio(生成式工作室是 BI 页面的编辑机制)。
func TestDeveloperStudioAccess(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeTextSSE(w, "好的")
	}))
	defer fake.Close()

	e := newEnvWithLLM(t, fake.URL, writeSkills(t))
	adminToken := loginAdmin(t, e)

	// admin 创建页面
	w := e.do(t, "POST", "/api/v1/pages", adminToken, map[string]any{"name": "studio页", "desc": ""})
	require.Equal(t, http.StatusOK, w.Code)
	pageID := extractID(t, w.Body.String())

	// 创建 developer 用户(role_id=2)并登录
	w = e.do(t, "POST", "/api/v1/users", adminToken, map[string]any{
		"username": "sddev", "password": "pass1234", "name": "工作室开发者", "role_id": 2,
	})
	require.Equal(t, http.StatusOK, w.Code)
	w = e.do(t, "POST", "/api/v1/auth/login", "", map[string]string{
		"username": "sddev", "password": "pass1234",
	})
	require.Equal(t, http.StatusOK, w.Code)
	devToken := extractToken(t, w.Body.String())

	// developer 通过 RBAC 访问 studio 会话接口(只验证放行,不断言聊天流程)
	w = e.do(t, "GET", "/api/v1/studio/conversations/"+strconv.Itoa(pageID), devToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
}

func loginAs(t *testing.T, e *env, username, password string) string {
	t.Helper()
	w := e.do(t, "POST", "/api/v1/auth/login", "", map[string]string{
		"username": username, "password": password,
	})
	require.Equal(t, http.StatusOK, w.Code)
	return extractToken(t, w.Body.String())
}

// TestPortalUserChartQueryAuthz 门户图表取数授权(线上事故回归):
// 普通用户被授权页面后,图表所用主题可查;未授权主题 403;developer 不受对象级限制。
func TestPortalUserChartQueryAuthz(t *testing.T) {
	e := newEnv(t)
	admin := loginAdmin(t, e)

	createSub := func(code string) int {
		w := e.do(t, "POST", "/api/v1/subjects", admin, map[string]any{"name": code, "code": code})
		require.Equal(t, http.StatusOK, w.Code)
		id := extractID(t, w.Body.String())
		w = e.do(t, "POST", "/api/v1/subjects/"+strconv.Itoa(id)+"/tables", admin, map[string]any{
			"name": "t1", "kind": "DWD",
			"columns": []map[string]any{{"name": "v", "type": "TEXT"}},
		})
		require.Equal(t, http.StatusOK, w.Code)
		return id
	}
	sub1 := createSub("qzs1")
	sub2 := createSub("qzs2")

	// 页面引用 sub1 → 发布 → 授权给普通用户
	w := e.do(t, "POST", "/api/v1/pages", admin, map[string]any{"name": "门户授权页", "desc": ""})
	require.Equal(t, http.StatusOK, w.Code)
	pageID := extractID(t, w.Body.String())
	w = e.do(t, "PUT", "/api/v1/pages/"+strconv.Itoa(pageID), admin, map[string]any{
		"config": map[string]any{
			"version": 1, "layout": "fluid-grid",
			"charts": []map[string]any{{
				"id": "c1", "title": "t", "width": "full", "type": "interval",
				"sql": "SELECT v FROM t1", "subjects": []uint{uint(sub1)},
				"spec": map[string]any{"type": "interval"},
			}},
		},
	})
	require.Equal(t, http.StatusOK, w.Code)
	w = e.do(t, "POST", "/api/v1/pages/"+strconv.Itoa(pageID)+"/publish", admin, nil)
	require.Equal(t, http.StatusOK, w.Code)

	w = e.do(t, "POST", "/api/v1/users", admin, map[string]any{
		"username": "qzviewer", "password": "pass1234", "name": "qzviewer", "role_id": 3,
	})
	require.Equal(t, http.StatusOK, w.Code)
	uid := extractID(t, w.Body.String())
	w = e.do(t, "PUT", "/api/v1/pages/"+strconv.Itoa(pageID)+"/acl", admin, map[string]any{
		"grants": []map[string]any{{"subject_type": "user", "subject_id": uid}},
	})
	require.Equal(t, http.StatusOK, w.Code)

	vt := loginAs(t, e, "qzviewer", "pass1234")

	// 页面可见(既有行为)
	w = e.do(t, "GET", "/api/v1/pages/"+strconv.Itoa(pageID), vt, nil)
	require.Equal(t, http.StatusOK, w.Code)

	// 页面引用的主题 → 200
	w = e.do(t, "POST", "/api/v1/query", vt, map[string]any{
		"sql": "SELECT v FROM qzs1.dwd_t1", "subject_ids": []uint{uint(sub1)},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// 未授权主题 → 403
	w = e.do(t, "POST", "/api/v1/query", vt, map[string]any{
		"sql": "SELECT v FROM qzs2.dwd_t1", "subject_ids": []uint{uint(sub2)},
	})
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

	// developer 数据探查不受对象级限制
	w = e.do(t, "POST", "/api/v1/users", admin, map[string]any{
		"username": "qzdev", "password": "pass1234", "name": "qzdev", "role_id": 2,
	})
	require.Equal(t, http.StatusOK, w.Code)
	dt := loginAs(t, e, "qzdev", "pass1234")
	w = e.do(t, "POST", "/api/v1/query", dt, map[string]any{
		"sql": "SELECT v FROM qzs2.dwd_t1", "subject_ids": []uint{uint(sub2)},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestCORSHeadersAndPreflight(t *testing.T) {
	e := newEnv(t)

	w := e.do(t, "GET", "/api/v1/auth/me", "", nil)
	require.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	require.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))

	// OPTIONS 预检:直接 200,不落入业务 handler
	w = e.do(t, "OPTIONS", "/api/v1/auth/me", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "OPTIONS")
}

func TestDebugStatusAdminOnly(t *testing.T) {
	e := newEnv(t)
	// 注册调试端点(模拟 main 装配,注入假状态)
	r, ok := e.h.(*gin.Engine)
	require.True(t, ok)
	api.RegisterDebugRoutes(r, []byte("test-secret"), func() map[string]any {
		return map[string]any{"server": map[string]any{"goroutines": 1}}
	})

	admin := loginAdmin(t, e)
	w := e.do(t, "GET", "/debug/status", admin, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"goroutines"`)

	// 普通用户 403
	w = e.do(t, "POST", "/api/v1/users", admin, map[string]any{
		"username": "dstatus", "password": "pass1234", "name": "dstatus", "role_id": 3,
	})
	require.Equal(t, http.StatusOK, w.Code)
	ut := loginAs(t, e, "dstatus", "pass1234")
	w = e.do(t, "GET", "/debug/status", ut, nil)
	require.Equal(t, http.StatusForbidden, w.Code)

	// 未登录 401
	w = e.do(t, "GET", "/debug/status", "", nil)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestStudioChatWithoutCurrentModel 未配置当前模型时 Chat 返回 400(不进 SSE),提示指向控制台。
func TestStudioChatWithoutCurrentModel(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	require.NoError(t, db.Migrate(d))
	require.NoError(t, db.Seed(d))
	e, err := rbac.NewEnforcer(d)
	require.NoError(t, err)
	require.NoError(t, db.SeedRBAC(d, e))
	cfg := &config.Config{
		Addr: ":0", DBPath: filepath.Join(dir, "test.db"),
		JWTSecret: "test-secret", JWTTTL: time.Hour,
		CORSOrigin: "*", SkillsDir: writeSkills(t),
	}
	env := &env{h: api.NewRouter(d, e, cfg)}
	token := loginAdmin(t, env)

	w := env.do(t, "POST", "/api/v1/subjects", token, map[string]any{"name": "s", "code": "s400"})
	require.Equal(t, http.StatusOK, w.Code)
	sid := extractID(t, w.Body.String())
	w = env.do(t, "POST", "/api/v1/pages", token, map[string]any{"name": "p", "desc": ""})
	require.Equal(t, http.StatusOK, w.Code)
	pid := extractID(t, w.Body.String())

	w = env.do(t, "POST", "/api/v1/studio/chat", token, map[string]any{
		"page_id": pid, "subject_ids": []int{sid}, "message": "hi",
	})
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "大模型设置")
}

// TestLLMRoutesAdminOnly 普通用户访问大模型设置接口必须 403(admin 通配策略之外的路径)。
func TestLLMRoutesAdminOnly(t *testing.T) {
	e := newEnv(t)
	adminToken := loginAdmin(t, e)
	w := e.do(t, "POST", "/api/v1/users", adminToken, map[string]any{
		"username": "plain_llm", "password": "pass1234", "name": "普通丙", "role_id": 3,
	})
	require.Equal(t, http.StatusOK, w.Code)
	w = e.do(t, "POST", "/api/v1/auth/login", "", map[string]string{
		"username": "plain_llm", "password": "pass1234",
	})
	require.Equal(t, http.StatusOK, w.Code)
	plainToken := extractToken(t, w.Body.String())
	w = e.do(t, "GET", "/api/v1/llm/providers", plainToken, nil)
	require.Equal(t, http.StatusForbidden, w.Code)
}

// TestBrandingPublicAndAdminOnly 公共品牌只读免鉴权;写接口仅 admin。
func TestBrandingPublicAndAdminOnly(t *testing.T) {
	e := newEnv(t)

	// 公共 GET 免鉴权
	w := e.do(t, "GET", "/api/v1/branding", "", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "生成式 BI")

	adminToken := loginAdmin(t, e)
	w = e.do(t, "PUT", "/api/v1/branding", adminToken, map[string]any{"system_name": "我的BI", "browser_title": "我的标题"})
	require.Equal(t, http.StatusOK, w.Code)
	w = e.do(t, "GET", "/api/v1/branding", "", nil)
	require.Contains(t, w.Body.String(), "我的BI")

	// 普通用户写 → 403;读公共仍 200
	w = e.do(t, "POST", "/api/v1/users", adminToken, map[string]any{
		"username": "plain_brand", "password": "pass1234", "name": "普通丁", "role_id": 3,
	})
	require.Equal(t, http.StatusOK, w.Code)
	w = e.do(t, "POST", "/api/v1/auth/login", "", map[string]string{"username": "plain_brand", "password": "pass1234"})
	require.Equal(t, http.StatusOK, w.Code)
	plainToken := extractToken(t, w.Body.String())
	w = e.do(t, "PUT", "/api/v1/branding", plainToken, map[string]any{"system_name": "x", "browser_title": "y"})
	require.Equal(t, http.StatusForbidden, w.Code)

	// 删除不存在的图片幂等 200
	w = e.do(t, "DELETE", "/api/v1/branding/logo", adminToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
	// 未知 kind → 400
	w = e.do(t, "DELETE", "/api/v1/branding/banner", adminToken, nil)
	require.Equal(t, http.StatusBadRequest, w.Code)
	// 恢复默认
	w = e.do(t, "DELETE", "/api/v1/branding", adminToken, nil)
	require.Equal(t, http.StatusOK, w.Code)
}
