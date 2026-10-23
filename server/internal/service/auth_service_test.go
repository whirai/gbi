package service_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"gbi/server/internal/db"
	"gbi/server/internal/service"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	require.NoError(t, db.Migrate(d))
	require.NoError(t, db.Seed(d))
	return d
}

// newTestDBDataDir 返回一个独立临时目录(主题库文件将落在此目录)。
func newTestDBDataDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func newAuthService(d *gorm.DB) *service.AuthService {
	return service.NewAuthService(d, []byte("test-secret"), time.Hour)
}

func TestSeedCreatesAdmin(t *testing.T) {
	d := newTestDB(t)
	svc := newAuthService(d)
	res, err := svc.Login("admin", "admin123")
	require.NoError(t, err)
	require.Equal(t, "admin", res.RoleCode)
	require.NotEmpty(t, res.Token)
}

func TestLoginWrongPassword(t *testing.T) {
	d := newTestDB(t)
	svc := newAuthService(d)
	_, err := svc.Login("admin", "badpass")
	require.ErrorIs(t, err, service.ErrInvalidCredential)
}

func TestLoginDisabledUser(t *testing.T) {
	d := newTestDB(t)
	svc := newAuthService(d)
	res, err := svc.Login("admin", "admin123")
	require.NoError(t, err)
	require.NoError(t, d.Model(res.User).Update("status", "disabled").Error)
	_, err = svc.Login("admin", "admin123")
	require.ErrorIs(t, err, service.ErrUserDisabled)
}

// TestLoginDBFailureNotCredential 数据库故障不得被映射为 ErrInvalidCredential(仅 record not found 才映射)。
func TestLoginDBFailureNotCredential(t *testing.T) {
	d := newTestDB(t)
	svc := newAuthService(d)
	sqlDB, err := d.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close()) // 模拟 DB 不可用

	_, err = svc.Login("admin", "admin123")
	require.Error(t, err)
	require.NotErrorIs(t, err, service.ErrInvalidCredential)
}

func TestChangePassword(t *testing.T) {
	d := newTestDB(t)
	svc := newAuthService(d)
	res, err := svc.Login("admin", "admin123")
	require.NoError(t, err)

	require.Error(t, svc.ChangePassword(res.User.ID, "wrong-old", "newpass1"))
	require.NoError(t, svc.ChangePassword(res.User.ID, "admin123", "newpass1"))

	_, err = svc.Login("admin", "admin123")
	require.Error(t, err)
	_, err = svc.Login("admin", "newpass1")
	require.NoError(t, err)
}
