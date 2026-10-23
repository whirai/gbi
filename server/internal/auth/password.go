// password.go
package auth

import (
	"log/slog"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func HashPassword(p string) (hash string, err error) {
	start := time.Now()
	// 安全红线:密码原文绝不入日志(仅长度)。
	slog.Debug("方法进入", "fn", "auth.HashPassword", "pwd_len", len(p))
	defer func() {
		slog.Debug("方法离开", "fn", "auth.HashPassword", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	b, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, p string) bool {
	start := time.Now()
	// 安全红线:密码与 hash 原文绝不入日志(仅长度)。
	slog.Debug("方法进入", "fn", "auth.CheckPassword", "pwd_len", len(p))
	defer func() {
		slog.Debug("方法离开", "fn", "auth.CheckPassword", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(p)) == nil
}
