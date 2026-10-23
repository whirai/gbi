package service

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"

	"gbi/server/internal/model"
)

const (
	DefaultBrandName   = "生成式 BI"
	MaxBrandNameRunes  = 32
	MaxBrandTitleRunes = 64
	MaxBrandImageBytes = 512 << 10
	brandDirName       = "branding"
)

var (
	ErrBrandNameInvalid   = fmt.Errorf("系统名称不能为空且不超过 %d 个字符", MaxBrandNameRunes)
	ErrBrandTitleInvalid  = fmt.Errorf("浏览器标题不能为空且不超过 %d 个字符", MaxBrandTitleRunes)
	ErrBrandKindInvalid   = errors.New("未知的品牌图片类型(仅 logo/favicon)")
	ErrBrandImageType     = errors.New("仅支持 png 或 ico 图片")
	ErrBrandImageTooLarge = fmt.Errorf("图片不能超过 %dKB", MaxBrandImageBytes/1024)
	ErrBrandImageNotFound = errors.New("品牌图片不存在")
)

type BrandingDTO struct {
	SystemName   string  `json:"system_name"`
	BrowserTitle string  `json:"browser_title"`
	LogoURL      *string `json:"logo_url"`
	FaviconURL   *string `json:"favicon_url"`
}

type BrandingService struct {
	db      *gorm.DB
	dataDir string
}

func NewBrandingService(db *gorm.DB, dataDir string) *BrandingService {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.NewBrandingService", "data_dir", dataDir)
	defer func() {
		slog.Debug("方法离开", "fn", "service.NewBrandingService", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &BrandingService{db: db, dataDir: dataDir}
}

func (s *BrandingService) getConfig() (cfg model.BrandConfig, found bool, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "BrandingService.getConfig")
	defer func() {
		slog.Debug("方法离开", "fn", "BrandingService.getConfig", "elapsed_ms", time.Since(start).Milliseconds(), "found", found, "err", err)
	}()
	err = s.db.First(&cfg, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.BrandConfig{}, false, nil
	}
	if err != nil {
		return model.BrandConfig{}, false, err
	}
	return cfg, true, nil
}

func (s *BrandingService) saveConfig(mutate func(*model.BrandConfig)) (cfg model.BrandConfig, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "BrandingService.saveConfig")
	defer func() {
		slog.Debug("方法离开", "fn", "BrandingService.saveConfig", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	prev, found, err := s.getConfig()
	if err != nil {
		return model.BrandConfig{}, err
	}
	cfg = prev
	if !found {
		cfg = model.BrandConfig{ID: 1}
	}
	mutate(&cfg)
	cfg.Version = time.Now().UnixNano()
	if !found {
		err = s.db.Create(&cfg).Error
	} else {
		err = s.db.Save(&cfg).Error
	}
	return cfg, err
}

func (s *BrandingService) Get() (dto BrandingDTO, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "BrandingService.Get")
	defer func() {
		slog.Debug("方法离开", "fn", "BrandingService.Get", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	cfg, found, err := s.getConfig()
	if err != nil {
		return dto, err
	}
	name, title := DefaultBrandName, DefaultBrandName
	if found {
		if strings.TrimSpace(cfg.SystemName) != "" {
			name = cfg.SystemName
		}
		if strings.TrimSpace(cfg.BrowserTitle) != "" {
			title = cfg.BrowserTitle
		}
	}
	dto = BrandingDTO{SystemName: name, BrowserTitle: title}
	if found && cfg.LogoExt != "" {
		u := fmt.Sprintf("/api/v1/branding/image/logo?v=%d", cfg.Version)
		dto.LogoURL = &u
	}
	if found && cfg.FaviconExt != "" {
		u := fmt.Sprintf("/api/v1/branding/image/favicon?v=%d", cfg.Version)
		dto.FaviconURL = &u
	}
	return dto, nil
}

func (s *BrandingService) UpdateNames(systemName, browserTitle string) (dto BrandingDTO, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "BrandingService.UpdateNames")
	defer func() {
		slog.Debug("方法离开", "fn", "BrandingService.UpdateNames", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	name, title := strings.TrimSpace(systemName), strings.TrimSpace(browserTitle)
	if name == "" || len([]rune(name)) > MaxBrandNameRunes {
		return dto, ErrBrandNameInvalid
	}
	if title == "" || len([]rune(title)) > MaxBrandTitleRunes {
		return dto, ErrBrandTitleInvalid
	}
	if _, err := s.saveConfig(func(c *model.BrandConfig) {
		c.SystemName = name
		c.BrowserTitle = title
	}); err != nil {
		return dto, err
	}
	return s.Get()
}

func (s *BrandingService) kindExt(cfg model.BrandConfig, kind string) (ext string) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "BrandingService.kindExt", "kind", kind)
	defer func() {
		slog.Debug("方法离开", "fn", "BrandingService.kindExt", "elapsed_ms", time.Since(start).Milliseconds(), "ext", ext)
	}()
	if kind == "logo" {
		return cfg.LogoExt
	}
	return cfg.FaviconExt
}

func (s *BrandingService) SaveImage(kind string, data []byte) (dto BrandingDTO, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "BrandingService.SaveImage", "kind", kind, "bytes", len(data))
	defer func() {
		slog.Debug("方法离开", "fn", "BrandingService.SaveImage", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	if kind != "logo" && kind != "favicon" {
		return dto, ErrBrandKindInvalid
	}
	if len(data) > MaxBrandImageBytes {
		return dto, ErrBrandImageTooLarge
	}
	ext, _, ok := sniffImage(data)
	if !ok {
		return dto, ErrBrandImageType
	}
	path := filepath.Join(s.dataDir, brandDirName, kind+"."+ext)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return dto, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return dto, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return dto, err
	}
	prev, found, err := s.getConfig()
	if err != nil {
		return dto, err
	}
	oldExt := ""
	if found {
		oldExt = s.kindExt(prev, kind)
	}
	if _, err := s.saveConfig(func(c *model.BrandConfig) {
		if kind == "logo" {
			c.LogoExt = ext
		} else {
			c.FaviconExt = ext
		}
	}); err != nil {
		return dto, err
	}
	if oldExt != "" && oldExt != ext {
		_ = os.Remove(filepath.Join(s.dataDir, brandDirName, kind+"."+oldExt))
	}
	return s.Get()
}

func (s *BrandingService) DeleteImage(kind string) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "BrandingService.DeleteImage", "kind", kind)
	defer func() {
		slog.Debug("方法离开", "fn", "BrandingService.DeleteImage", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	if kind != "logo" && kind != "favicon" {
		return ErrBrandKindInvalid
	}
	prev, found, err := s.getConfig()
	if err != nil || !found {
		return err
	}
	oldExt := s.kindExt(prev, kind)
	if oldExt == "" {
		return nil
	}
	if _, err := s.saveConfig(func(c *model.BrandConfig) {
		if kind == "logo" {
			c.LogoExt = ""
		} else {
			c.FaviconExt = ""
		}
	}); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(s.dataDir, brandDirName, kind+"."+oldExt))
	return nil
}

func (s *BrandingService) Reset() (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "BrandingService.Reset")
	defer func() {
		slog.Debug("方法离开", "fn", "BrandingService.Reset", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	prev, found, err := s.getConfig()
	if err != nil || !found {
		return err
	}
	if _, err := s.saveConfig(func(c *model.BrandConfig) {
		c.SystemName = ""
		c.BrowserTitle = ""
		c.LogoExt = ""
		c.FaviconExt = ""
	}); err != nil {
		return err
	}
	for _, kind := range []string{"logo", "favicon"} {
		if ext := s.kindExt(prev, kind); ext != "" {
			_ = os.Remove(filepath.Join(s.dataDir, brandDirName, kind+"."+ext))
		}
	}
	return nil
}

// ImageBytes 返回品牌图片字节与 Content-Type;未上传/文件缺失 → ErrBrandImageNotFound。
func (s *BrandingService) ImageBytes(kind string) (data []byte, contentType string, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "BrandingService.ImageBytes", "kind", kind)
	defer func() {
		slog.Debug("方法离开", "fn", "BrandingService.ImageBytes", "elapsed_ms", time.Since(start).Milliseconds(), "bytes", len(data), "err", err)
	}()
	if kind != "logo" && kind != "favicon" {
		return nil, "", ErrBrandKindInvalid
	}
	cfg, found, err := s.getConfig()
	if err != nil {
		return nil, "", err
	}
	if !found {
		return nil, "", ErrBrandImageNotFound
	}
	ext := s.kindExt(cfg, kind)
	if ext == "" {
		return nil, "", ErrBrandImageNotFound
	}
	contentType = "image/png"
	if ext == "ico" {
		contentType = "image/vnd.microsoft.icon"
	}
	data, err = os.ReadFile(filepath.Join(s.dataDir, brandDirName, kind+"."+ext))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", ErrBrandImageNotFound
		}
		return nil, "", err
	}
	return data, contentType, nil
}

// sniffImage 按 magic bytes 识别 PNG/ICO(SVG 等一律拒绝)。
func sniffImage(data []byte) (ext, contentType string, ok bool) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.sniffImage", "bytes", len(data))
	defer func() {
		slog.Debug("方法离开", "fn", "service.sniffImage", "elapsed_ms", time.Since(start).Milliseconds(), "ext", ext)
	}()
	pngMagic := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	if len(data) >= len(pngMagic) && bytes.Equal(data[:len(pngMagic)], pngMagic) {
		return "png", "image/png", true
	}
	if len(data) >= 4 && data[0] == 0x00 && data[1] == 0x00 && data[2] == 0x01 && data[3] == 0x00 {
		return "ico", "image/vnd.microsoft.icon", true
	}
	return "", "", false
}
