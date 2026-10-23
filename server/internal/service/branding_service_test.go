package service_test

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"gbi/server/internal/service"
)

// 1x1 合法 PNG(67 bytes)
var pngBytes = mustDecodePNG()
var icoBytes = []byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00} // ICO 头 + 占位

func mustDecodePNG() []byte {
	const b64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	// base64 解码(避免手写字节)
	out, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		panic(err)
	}
	return out
}

func newBrandEnv(t *testing.T) (*service.BrandingService, string) {
	t.Helper()
	dir := t.TempDir()
	return service.NewBrandingService(newTestDB(t), dir), dir
}

func TestBrandingDefaultsAndNames(t *testing.T) {
	s, _ := newBrandEnv(t)
	dto, err := s.Get()
	require.NoError(t, err)
	require.Equal(t, "生成式 BI", dto.SystemName)
	require.Equal(t, "生成式 BI", dto.BrowserTitle)
	require.Nil(t, dto.LogoURL)
	require.Nil(t, dto.FaviconURL)

	dto, err = s.UpdateNames("  我的BI  ", "  我的标题 ")
	require.NoError(t, err)
	require.Equal(t, "我的BI", dto.SystemName)
	require.Equal(t, "我的标题", dto.BrowserTitle)

	_, err = s.UpdateNames("", "x")
	require.ErrorIs(t, err, service.ErrBrandNameInvalid)
	_, err = s.UpdateNames("x", "")
	require.ErrorIs(t, err, service.ErrBrandTitleInvalid)
	_, err = s.UpdateNames(strings.Repeat("名", 33), "x")
	require.ErrorIs(t, err, service.ErrBrandNameInvalid)
	_, err = s.UpdateNames("x", strings.Repeat("题", 65))
	require.ErrorIs(t, err, service.ErrBrandTitleInvalid)
}

func TestBrandingImagesLifecycle(t *testing.T) {
	s, dir := newBrandEnv(t)

	// PNG 上传 → URL 与文件
	dto, err := s.SaveImage("logo", pngBytes)
	require.NoError(t, err)
	require.NotNil(t, dto.LogoURL)
	require.Contains(t, *dto.LogoURL, "/api/v1/branding/image/logo?v=")
	_, err = os.Stat(filepath.Join(dir, "branding", "logo.png"))
	require.NoError(t, err)
	data, ct, err := s.ImageBytes("logo")
	require.NoError(t, err)
	require.Equal(t, pngBytes, data)
	require.Equal(t, "image/png", ct)

	// 版本号随写操作变化;替换为 ICO 后旧 PNG 被删除
	v1 := *dto.LogoURL
	dto, err = s.SaveImage("logo", icoBytes)
	require.NoError(t, err)
	require.NotEqual(t, v1, *dto.LogoURL)
	_, err = os.Stat(filepath.Join(dir, "branding", "logo.png"))
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(dir, "branding", "logo.ico"))
	require.NoError(t, err)

	// 非法类型/超限/未知 kind
	_, err = s.SaveImage("logo", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`))
	require.ErrorIs(t, err, service.ErrBrandImageType)
	_, err = s.SaveImage("logo", []byte("GIF89a"))
	require.ErrorIs(t, err, service.ErrBrandImageType)
	_, err = s.SaveImage("logo", append(append([]byte{}, pngBytes...), make([]byte, service.MaxBrandImageBytes)...))
	require.ErrorIs(t, err, service.ErrBrandImageTooLarge)
	_, err = s.SaveImage("banner", pngBytes)
	require.ErrorIs(t, err, service.ErrBrandKindInvalid)

	// 删除幂等;未上传时取图 404 语义
	require.NoError(t, s.DeleteImage("logo"))
	require.NoError(t, s.DeleteImage("logo"))
	_, _, err = s.ImageBytes("logo")
	require.ErrorIs(t, err, service.ErrBrandImageNotFound)

	// favicon 独立
	dto, err = s.SaveImage("favicon", pngBytes)
	require.NoError(t, err)
	require.NotNil(t, dto.FaviconURL)
	require.Nil(t, dto.LogoURL)
}

func TestBrandingReset(t *testing.T) {
	s, dir := newBrandEnv(t)
	_, err := s.UpdateNames("品牌", "标题")
	require.NoError(t, err)
	_, err = s.SaveImage("logo", pngBytes)
	require.NoError(t, err)
	_, err = s.SaveImage("favicon", pngBytes)
	require.NoError(t, err)

	require.NoError(t, s.Reset())
	dto, err := s.Get()
	require.NoError(t, err)
	require.Equal(t, "生成式 BI", dto.SystemName)
	require.Equal(t, "生成式 BI", dto.BrowserTitle)
	require.Nil(t, dto.LogoURL)
	require.Nil(t, dto.FaviconURL)
	_, err = os.Stat(filepath.Join(dir, "branding", "logo.png"))
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(dir, "branding", "favicon.png"))
	require.True(t, os.IsNotExist(err))
}
