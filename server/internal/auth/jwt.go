// jwt.go
package auth

import (
	"errors"
	"log/slog"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"gbi/server/internal/model"
)

type Claims struct {
	UserID   uint   `json:"uid"`
	Username string `json:"username"`
	RoleCode string `json:"role"`
	jwt.RegisteredClaims
}

func Sign(secret []byte, u *model.User, roleCode string, ttl time.Duration) (token string, err error) {
	start := time.Now()
	// 安全红线:secret 原文绝不入日志(仅布尔),用户名/角色亦不落。
	slog.Debug("方法进入", "fn", "auth.Sign", "uid", u.ID, "ttl", ttl, "secret_set", len(secret) > 0)
	defer func() {
		slog.Debug("方法离开", "fn", "auth.Sign", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	now := time.Now()
	c := Claims{
		UserID:   u.ID,
		Username: u.Username,
		RoleCode: roleCode,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(secret)
}

func Parse(secret []byte, tokenStr string) (claims *Claims, err error) {
	start := time.Now()
	// 安全红线:token 与 secret 原文绝不入日志(仅长度/布尔)。
	slog.Debug("方法进入", "fn", "auth.Parse", "token_len", len(tokenStr), "secret_set", len(secret) > 0)
	defer func() {
		slog.Debug("方法离开", "fn", "auth.Parse", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	c := &Claims{}
	_, err = jwt.ParseWithClaims(tokenStr, c, func(tk *jwt.Token) (any, error) {
		// 与 Sign 保持一致:仅接受 HS256,避免 HMAC 族内算法混用
		if tk.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		return secret, nil
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}
