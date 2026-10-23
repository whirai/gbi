package service

import (
	"errors"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"gbi/server/internal/auth"
	"gbi/server/internal/logger"
	"gbi/server/internal/model"
)

var (
	ErrInvalidCredential = errors.New("用户名或密码错误")
	ErrUserDisabled      = errors.New("用户已被停用")
)

type AuthService struct {
	db     *gorm.DB
	secret []byte
	ttl    time.Duration
}

func NewAuthService(d *gorm.DB, secret []byte, ttl time.Duration) *AuthService {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.NewAuthService", "ttl", ttl)
	defer func() {
		slog.Debug("方法离开", "fn", "service.NewAuthService", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &AuthService{db: d, secret: secret, ttl: ttl}
}

type LoginResult struct {
	Token    string      `json:"token"`
	User     *model.User `json:"user"`
	RoleCode string      `json:"role_code"`
}

type MeResult struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	RoleCode string `json:"role_code"`
	Status   string `json:"status"`
}

func (s *AuthService) Login(username, password string) (res *LoginResult, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "AuthService.Login", "username", logger.Truncate(username, 64))
	defer func() {
		slog.Debug("方法离开", "fn", "AuthService.Login", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	slog.Debug("AuthService.Login", "username", username)
	var u model.User
	if err := s.db.Where("username = ?", username).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Debug("AuthService.Login错误", "err", ErrInvalidCredential)
			return nil, ErrInvalidCredential
		}
		// 其余错误(DB 不可用/超时/schema 异常)原样上抛,避免误报为凭证错误
		slog.Debug("AuthService.Login错误", "err", err)
		return nil, err
	}
	if !auth.CheckPassword(u.PasswordHash, password) {
		slog.Debug("AuthService.Login错误", "err", ErrInvalidCredential)
		return nil, ErrInvalidCredential
	}
	if u.Status == "disabled" {
		slog.Debug("AuthService.Login错误", "err", ErrUserDisabled)
		return nil, ErrUserDisabled
	}
	var role model.Role
	if err := s.db.First(&role, u.RoleID).Error; err != nil {
		slog.Debug("AuthService.Login错误", "err", err)
		return nil, err
	}
	token, err := auth.Sign(s.secret, &u, role.Code, s.ttl)
	if err != nil {
		slog.Debug("AuthService.Login错误", "err", err)
		return nil, err
	}
	slog.Debug("AuthService.Login成功", "user_id", u.ID, "username", username, "role", role.Code)
	return &LoginResult{Token: token, User: &u, RoleCode: role.Code}, nil
}

func (s *AuthService) Me(uid uint) (res *MeResult, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "AuthService.Me", "uid", uid)
	defer func() {
		slog.Debug("方法离开", "fn", "AuthService.Me", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var u model.User
	if err := s.db.First(&u, uid).Error; err != nil {
		return nil, err
	}
	var role model.Role
	if err := s.db.First(&role, u.RoleID).Error; err != nil {
		return nil, err
	}
	return &MeResult{ID: u.ID, Username: u.Username, Name: u.Name, RoleCode: role.Code, Status: u.Status}, nil
}

func (s *AuthService) ChangePassword(uid uint, oldPwd, newPwd string) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "AuthService.ChangePassword", "user_id", uid)
	defer func() {
		slog.Debug("方法离开", "fn", "AuthService.ChangePassword", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	slog.Debug("AuthService.ChangePassword", "user_id", uid)
	var u model.User
	if err := s.db.First(&u, uid).Error; err != nil {
		slog.Debug("AuthService.ChangePassword错误", "err", err)
		return err
	}
	if !auth.CheckPassword(u.PasswordHash, oldPwd) {
		slog.Debug("AuthService.ChangePassword错误", "err", ErrInvalidCredential)
		return ErrInvalidCredential
	}
	hash, err := auth.HashPassword(newPwd)
	if err != nil {
		slog.Debug("AuthService.ChangePassword错误", "err", err)
		return err
	}
	err = s.db.Model(&u).Update("password_hash", hash).Error
	if err != nil {
		slog.Debug("AuthService.ChangePassword错误", "err", err)
		return err
	}
	slog.Debug("AuthService.ChangePassword成功")
	return nil
}
