package service

import (
	"errors"
	"log/slog"
	"time"

	"github.com/casbin/casbin/v2"
	"gorm.io/gorm"

	"gbi/server/internal/auth"
	"gbi/server/internal/logger"
	"gbi/server/internal/model"
	"gbi/server/internal/rbac"
)

var (
	ErrUserNotFound      = errors.New("用户不存在")
	ErrDuplicateUsername = errors.New("用户名已存在")
	ErrBuiltInUser       = errors.New("内置用户不可删除")
)

type UserService struct {
	db *gorm.DB
	e  *casbin.Enforcer
}

func NewUserService(db *gorm.DB, e *casbin.Enforcer) *UserService {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.NewUserService")
	defer func() {
		slog.Debug("方法离开", "fn", "service.NewUserService", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &UserService{db: db, e: e}
}

type UserDTO struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	DeptID   *uint  `json:"dept_id"`
	DeptName string `json:"dept_name"`
	RoleID   uint   `json:"role_id"`
	RoleCode string `json:"role_code"`
	Status   string `json:"status"`
}

type UserCreate struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Name     string `json:"name"`
	DeptID   *uint  `json:"dept_id"`
	RoleID   uint   `json:"role_id"`
}

type UserUpdate struct {
	Name   string `json:"name"`
	DeptID *uint  `json:"dept_id"`
	RoleID uint   `json:"role_id"`
	Status string `json:"status"`
}

// userDTO 供本包各 service 复用。
func userDTO(db *gorm.DB, u *model.User) UserDTO {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.userDTO", "user_id", u.ID)
	defer func() {
		slog.Debug("方法离开", "fn", "service.userDTO", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	dto := UserDTO{
		ID: u.ID, Username: u.Username, Name: u.Name,
		DeptID: u.DeptID, RoleID: u.RoleID, Status: u.Status,
	}
	if u.DeptID != nil {
		var d model.Department
		if db.First(&d, *u.DeptID).Error == nil {
			dto.DeptName = d.Name
		}
	}
	var r model.Role
	if db.First(&r, u.RoleID).Error == nil {
		dto.RoleCode = r.Code
	}
	return dto
}

func (s *UserService) List(page, size int, keyword string) (dtos []UserDTO, total int64, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "UserService.List", "page", page, "size", size, "keyword", logger.Truncate(keyword, 64))
	defer func() {
		slog.Debug("方法离开", "fn", "UserService.List", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	q := s.db.Model(&model.User{})
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("username LIKE ? OR name LIKE ?", like, like)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var users []model.User
	if err := q.Order("id").Offset((page - 1) * size).Limit(size).Find(&users).Error; err != nil {
		return nil, 0, err
	}
	dtos = make([]UserDTO, 0, len(users))
	for i := range users {
		dtos = append(dtos, userDTO(s.db, &users[i]))
	}
	return dtos, total, nil
}

func (s *UserService) Create(in UserCreate) (res *UserDTO, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "UserService.Create", "username", logger.Truncate(in.Username, 64), "role_id", in.RoleID)
	defer func() {
		slog.Debug("方法离开", "fn", "UserService.Create", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var n int64
	if err := s.db.Model(&model.User{}).Where("username = ?", in.Username).Count(&n).Error; err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, ErrDuplicateUsername
	}
	var role model.Role
	if err := s.db.First(&role, in.RoleID).Error; err != nil {
		return nil, errors.New("角色不存在")
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	u := &model.User{
		Username: in.Username, PasswordHash: hash, Name: in.Name,
		DeptID: in.DeptID, RoleID: in.RoleID, Status: "active",
	}
	if err := s.db.Create(u).Error; err != nil {
		return nil, err
	}
	if _, err := s.e.AddGroupingPolicy(rbac.UserSubject(u.ID), rbac.RoleSubject(role.Code)); err != nil {
		// 补偿:casbin 写入失败时删除刚创建的用户行,避免两库不一致。
		s.db.Delete(&model.User{}, u.ID)
		return nil, err
	}
	dto := userDTO(s.db, u)
	return &dto, nil
}

func (s *UserService) Update(id uint, in UserUpdate) (res *UserDTO, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "UserService.Update", "id", id, "role_id", in.RoleID, "status", logger.Truncate(in.Status, 64))
	defer func() {
		slog.Debug("方法离开", "fn", "UserService.Update", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var u model.User
	if err := s.db.First(&u, id).Error; err != nil {
		return nil, ErrUserNotFound
	}
	var newRole model.Role
	if err := s.db.First(&newRole, in.RoleID).Error; err != nil {
		return nil, errors.New("角色不存在")
	}
	var oldRole model.Role
	if err := s.db.First(&oldRole, u.RoleID).Error; err != nil {
		return nil, err
	}
	u.Name = in.Name
	u.DeptID = in.DeptID
	u.RoleID = in.RoleID
	if in.Status == "active" || in.Status == "disabled" {
		u.Status = in.Status
	}
	// DB 为唯一事实源:先落库,再把 casbin 收敛到「最终角色 + 最终状态」的终态。
	if err := s.db.Save(&u).Error; err != nil {
		return nil, err
	}
	// Phase 1 权限仅依赖角色;用户→组的 casbin 分组尚不存在(组策略未建立),此处不动。
	// 注:casbin 在 DB 落库之后执行,失败时仅返回错误,不回滚 DB。
	sub := rbac.UserSubject(id)
	if oldRole.Code != newRole.Code {
		// 角色切换:移除旧角色分组(不存在时无操作)
		if _, err := s.e.RemoveGroupingPolicy(sub, rbac.RoleSubject(oldRole.Code)); err != nil {
			return nil, err
		}
	}
	finalRole := rbac.RoleSubject(newRole.Code)
	if u.Status == "disabled" {
		// 停用即失效:移除最终角色分组(不存在时无操作)
		if _, err := s.e.RemoveGroupingPolicy(sub, finalRole); err != nil {
			return nil, err
		}
	} else { // status 仅经合法 active/disabled 写入,此分支即 active
		// 启用/保持:确保最终角色分组存在(不重复添加)
		has, err := s.e.HasGroupingPolicy(sub, finalRole)
		if err != nil {
			return nil, err
		}
		if !has {
			if _, err := s.e.AddGroupingPolicy(sub, finalRole); err != nil {
				return nil, err
			}
		}
	}
	dto := userDTO(s.db, &u)
	return &dto, nil
}

func (s *UserService) Delete(id uint) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "UserService.Delete", "id", id)
	defer func() {
		slog.Debug("方法离开", "fn", "UserService.Delete", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var u model.User
	if err := s.db.First(&u, id).Error; err != nil {
		return ErrUserNotFound
	}
	if u.IsBuiltIn {
		return ErrBuiltInUser
	}
	// casbin 先行(fail-closed):先快照,删除失败则按快照恢复;casbin 清理成功后再走 DB 事务。
	sub := rbac.UserSubject(id)
	gSnap, err := s.e.GetFilteredGroupingPolicy(0, sub)
	if err != nil {
		return err
	}
	pSnap, err := s.e.GetFilteredPolicy(0, sub)
	if err != nil {
		return err
	}
	restore := func() {
		for _, g := range gSnap {
			s.e.AddGroupingPolicy(strToIface(g)...)
		}
		for _, p := range pSnap {
			s.e.AddPolicy(strToIface(p)...)
		}
	}
	if _, err := s.e.RemoveFilteredGroupingPolicy(0, sub); err != nil {
		return err
	}
	if _, err := s.e.RemoveFilteredPolicy(0, sub); err != nil {
		restore()
		return err
	}
	tx := s.db.Begin()
	if err := tx.Delete(&model.GroupMember{}, "user_id = ?", id).Error; err != nil {
		tx.Rollback()
		restore()
		return err
	}
	if err := tx.Delete(&model.User{}, id).Error; err != nil {
		tx.Rollback()
		restore()
		return err
	}
	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		restore()
		return err
	}
	return nil
}

// strToIface 将 casbin 规则([]string)转换为 AddPolicy/AddGroupingPolicy 的可变参数。
func strToIface(ss []string) []interface{} {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.strToIface", "count", len(ss))
	defer func() {
		slog.Debug("方法离开", "fn", "service.strToIface", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	vs := make([]interface{}, len(ss))
	for i, v := range ss {
		vs[i] = v
	}
	return vs
}

func (s *UserService) ResetPassword(id uint, pwd string) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "UserService.ResetPassword", "id", id)
	defer func() {
		slog.Debug("方法离开", "fn", "UserService.ResetPassword", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var u model.User
	if err := s.db.First(&u, id).Error; err != nil {
		return ErrUserNotFound
	}
	hash, err := auth.HashPassword(pwd)
	if err != nil {
		return err
	}
	return s.db.Model(&u).Update("password_hash", hash).Error
}
