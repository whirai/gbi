package service

import (
	"errors"
	"log/slog"
	"time"

	"github.com/casbin/casbin/v2"
	"gorm.io/gorm"

	"gbi/server/internal/logger"
	"gbi/server/internal/model"
	"gbi/server/internal/rbac"
)

var (
	ErrDuplicateGroupName = errors.New("用户组名称已存在")
	ErrGroupNotFound      = errors.New("用户组不存在")
	ErrInvalidUserID      = errors.New("无效的用户ID")
	ErrMemberUserNotFound = errors.New("成员用户不存在")
)

type GroupService struct {
	db *gorm.DB
	e  *casbin.Enforcer
}

func NewGroupService(db *gorm.DB, e *casbin.Enforcer) *GroupService {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.NewGroupService")
	defer func() {
		slog.Debug("方法离开", "fn", "service.NewGroupService", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &GroupService{db: db, e: e}
}

func (s *GroupService) List() (groups []model.Group, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "GroupService.List")
	defer func() {
		slog.Debug("方法离开", "fn", "GroupService.List", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	err = s.db.Order("id").Find(&groups).Error
	return groups, err
}

func (s *GroupService) Create(name, desc string) (res *model.Group, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "GroupService.Create", "name", logger.Truncate(name, 64))
	defer func() {
		slog.Debug("方法离开", "fn", "GroupService.Create", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var n int64
	if err := s.db.Model(&model.Group{}).Where("name = ?", name).Count(&n).Error; err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, ErrDuplicateGroupName
	}
	g := &model.Group{Name: name, Desc: desc}
	if err := s.db.Create(g).Error; err != nil {
		return nil, err
	}
	return g, nil
}

func (s *GroupService) Update(id uint, name, desc string) (res *model.Group, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "GroupService.Update", "id", id, "name", logger.Truncate(name, 64))
	defer func() {
		slog.Debug("方法离开", "fn", "GroupService.Update", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var g model.Group
	if err := s.db.First(&g, id).Error; err != nil {
		return nil, ErrGroupNotFound
	}
	var n int64
	if err := s.db.Model(&model.Group{}).Where("name = ? AND id <> ?", name, id).Count(&n).Error; err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, ErrDuplicateGroupName
	}
	g.Name = name
	g.Desc = desc
	if err := s.db.Save(&g).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *GroupService) Delete(id uint) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "GroupService.Delete", "id", id)
	defer func() {
		slog.Debug("方法离开", "fn", "GroupService.Delete", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var g model.Group
	if err := s.db.First(&g, id).Error; err != nil {
		return ErrGroupNotFound
	}
	// casbin 先行(fail-closed):先快照,删除失败则按快照恢复;casbin 清理成功后再走 DB 事务。
	sub := rbac.GroupSubject(id)
	gSnap, err := s.e.GetFilteredGroupingPolicy(1, sub)
	if err != nil {
		return err
	}
	pSnap, err := s.e.GetFilteredPolicy(0, sub)
	if err != nil {
		return err
	}
	restore := func() {
		for _, gg := range gSnap {
			s.e.AddGroupingPolicy(strToIface(gg)...)
		}
		for _, pp := range pSnap {
			s.e.AddPolicy(strToIface(pp)...)
		}
	}
	if _, err := s.e.RemoveFilteredGroupingPolicy(1, sub); err != nil {
		return err
	}
	if _, err := s.e.RemoveFilteredPolicy(0, sub); err != nil {
		restore()
		return err
	}
	tx := s.db.Begin()
	if err := tx.Delete(&model.Group{}, id).Error; err != nil {
		tx.Rollback()
		restore()
		return err
	}
	if err := tx.Delete(&model.GroupMember{}, "group_id = ?", id).Error; err != nil {
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

func (s *GroupService) Members(gid uint) (dtos []UserDTO, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "GroupService.Members", "gid", gid)
	defer func() {
		slog.Debug("方法离开", "fn", "GroupService.Members", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var g model.Group
	if err := s.db.First(&g, gid).Error; err != nil {
		return nil, ErrGroupNotFound
	}
	var users []model.User
	err = s.db.
		Joins("JOIN group_members ON group_members.user_id = users.id").
		Where("group_members.group_id = ?", gid).
		Order("users.id").
		Find(&users).Error
	if err != nil {
		return nil, err
	}
	dtos = make([]UserDTO, 0, len(users))
	for i := range users {
		dtos = append(dtos, userDTO(s.db, &users[i]))
	}
	return dtos, nil
}

func (s *GroupService) AddMembers(gid uint, userIDs []uint) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "GroupService.AddMembers", "gid", gid, "users", len(userIDs))
	defer func() {
		slog.Debug("方法离开", "fn", "GroupService.AddMembers", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var g model.Group
	if err := s.db.First(&g, gid).Error; err != nil {
		return ErrGroupNotFound
	}
	// 先整体校验再写入,拒绝 0 与不存在的用户ID,避免脏成员行/casbin 规则被后建用户继承。
	// 先对入参去重:Count 统计的是 distinct 行数,重复 id([5,5])会让计数与长度不等而误判不存在。
	seen := make(map[uint]struct{}, len(userIDs))
	uniq := make([]uint, 0, len(userIDs))
	for _, uid := range userIDs {
		if uid == 0 {
			return ErrInvalidUserID
		}
		if _, ok := seen[uid]; ok {
			continue
		}
		seen[uid] = struct{}{}
		uniq = append(uniq, uid)
	}
	var n int64
	if err := s.db.Model(&model.User{}).Where("id IN ?", uniq).Count(&n).Error; err != nil {
		return err
	}
	if int(n) != len(uniq) {
		return ErrMemberUserNotFound
	}
	for _, uid := range uniq {
		gm := model.GroupMember{GroupID: gid, UserID: uid}
		if err := s.db.Where(gm).FirstOrCreate(&gm).Error; err != nil {
			return err
		}
		has, err := s.e.HasGroupingPolicy(rbac.UserSubject(uid), rbac.GroupSubject(gid))
		if err != nil {
			return err
		}
		if !has {
			if _, err := s.e.AddGroupingPolicy(rbac.UserSubject(uid), rbac.GroupSubject(gid)); err != nil {
				// 补偿:casbin 写入失败时删除该成员行,避免两库不一致。
				s.db.Delete(&model.GroupMember{}, "group_id = ? AND user_id = ?", gid, uid)
				return err
			}
		}
	}
	return nil
}

func (s *GroupService) RemoveMember(gid, uid uint) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "GroupService.RemoveMember", "gid", gid, "uid", uid)
	defer func() {
		slog.Debug("方法离开", "fn", "GroupService.RemoveMember", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	// casbin 先行(fail-closed):分组删除失败则 DB 不动。
	if _, err := s.e.RemoveGroupingPolicy(rbac.UserSubject(uid), rbac.GroupSubject(gid)); err != nil {
		return err
	}
	if err := s.db.Delete(&model.GroupMember{}, "group_id = ? AND user_id = ?", gid, uid).Error; err != nil {
		s.e.AddGroupingPolicy(rbac.UserSubject(uid), rbac.GroupSubject(gid)) // 尽力恢复分组
		return err
	}
	return nil
}
