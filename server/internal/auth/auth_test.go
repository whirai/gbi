package auth_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"gbi/server/internal/auth"
	"gbi/server/internal/model"
)

func TestPasswordHashAndCheck(t *testing.T) {
	hash, err := auth.HashPassword("secret123")
	require.NoError(t, err)
	require.True(t, auth.CheckPassword(hash, "secret123"))
	require.False(t, auth.CheckPassword(hash, "wrong"))
}

func TestJWTSignAndParse(t *testing.T) {
	secret := []byte("test-secret")
	u := &model.User{ID: 7, Username: "alice"}
	token, err := auth.Sign(secret, u, "developer", time.Hour)
	require.NoError(t, err)

	c, err := auth.Parse(secret, token)
	require.NoError(t, err)
	require.Equal(t, uint(7), c.UserID)
	require.Equal(t, "alice", c.Username)
	require.Equal(t, "developer", c.RoleCode)

	_, err = auth.Parse([]byte("other-secret"), token)
	require.Error(t, err)
}

// TestJWTParseRejectsNonHS256 验签算法固定 HS256:HS384 等 HMAC 族算法必须拒绝。
func TestJWTParseRejectsNonHS256(t *testing.T) {
	secret := []byte("test-secret")
	claims := auth.Claims{UserID: 1, Username: "eve", RoleCode: "user"}
	forged, err := jwt.NewWithClaims(jwt.SigningMethodHS384, claims).SignedString(secret)
	require.NoError(t, err)

	_, err = auth.Parse(secret, forged)
	require.Error(t, err)
}
