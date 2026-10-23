package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

type Role struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Code string `gorm:"uniqueIndex;size:32" json:"code"`
	Name string `gorm:"size:64" json:"name"`
}

type Department struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	ParentID *uint  `json:"parent_id"`
	Name     string `gorm:"size:64;not null" json:"name"`
	Sort     int    `json:"sort"`
}

type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"uniqueIndex;size:64;not null" json:"username"`
	PasswordHash string    `gorm:"size:128" json:"-"`
	Name         string    `gorm:"size:64" json:"name"`
	DeptID       *uint     `json:"dept_id"`
	RoleID       uint      `gorm:"not null" json:"role_id"`
	Status       string    `gorm:"size:16;default:active" json:"status"` // active | disabled
	IsBuiltIn    bool      `json:"is_built_in"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Group struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Name string `gorm:"uniqueIndex;size:64" json:"name"`
	Desc string `gorm:"size:255" json:"desc"`
}

type GroupMember struct {
	GroupID uint `gorm:"primaryKey"`
	UserID  uint `gorm:"primaryKey"`
}

// Subject 数据主题;每个主题一个独立 SQLite 库文件。
type Subject struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:64;not null" json:"name"`
	Code      string    `gorm:"uniqueIndex;size:32;not null" json:"code"`
	Desc      string    `gorm:"size:255" json:"desc"`
	DbFile    string    `gorm:"size:255" json:"db_file"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SubjectTable 主题下的表元数据;物理表在主题的独立库中。
type SubjectTable struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	SubjectID uint   `gorm:"not null;uniqueIndex:idx_subject_table,priority:1" json:"subject_id"`
	Name      string `gorm:"size:64;not null;uniqueIndex:idx_subject_table,priority:2" json:"name"`
	Kind      string `gorm:"size:8;not null" json:"kind"` // DWD | DIM
	Columns   string `gorm:"not null" json:"-"`           // JSON 序列化的 []ColumnDef
}

// ColumnDef 列定义。
type ColumnDef struct {
	Name    string `json:"name"`
	Type    string `json:"type"` // TEXT | INTEGER | REAL | DATE
	Comment string `json:"comment"`
}

// Page BI 页面;Config 为 JSON(PageConfig 序列化);draft/published。
type Page struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:64;not null" json:"name"`
	Desc      string    `gorm:"size:255" json:"desc"`
	Config    string    `gorm:"not null" json:"-"`
	Status    string    `gorm:"size:16;not null;default:'draft'" json:"status"`
	CreatedBy uint      `gorm:"not null" json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PageVersion 页面配置快照(每次 config 变化自动记录)。
type PageVersion struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	PageID    uint      `gorm:"index;not null" json:"page_id"`
	Version   int       `gorm:"not null" json:"version"`
	Config    string    `gorm:"not null" json:"-"`
	SavedBy   uint      `gorm:"not null" json:"saved_by"`
	CreatedAt time.Time `json:"created_at"`
}

// PageRenderError 前端渲染错误上报(供 check_page_render 工具核验)。
type PageRenderError struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	PageID    uint      `gorm:"index;not null" json:"page_id"`
	ChartID   string    `gorm:"size:64;not null" json:"chart_id"`
	Message   string    `gorm:"not null" json:"message"`
	Seen      bool      `gorm:"not null;default:false" json:"seen"`
	CreatedAt time.Time `json:"created_at"`
}

// Menu 导航菜单(可多个,按权限过滤后下发)。
type Menu struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:64;not null" json:"name"`
	Desc      string    `gorm:"size:255" json:"desc"`
	Sort      int       `json:"sort"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MenuItem 菜单项,指向 BI 页面。
type MenuItem struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	MenuID uint   `gorm:"not null;index" json:"menu_id"`
	PageID uint   `gorm:"not null" json:"page_id"`
	Name   string `gorm:"size:64;not null" json:"name"`
	Sort   int    `json:"sort"`
}

// Conversation 每页一个会话(生成式工作台)。
type Conversation struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	PageID        uint      `gorm:"uniqueIndex;not null" json:"page_id"`
	Title         string    `gorm:"size:128" json:"title"`
	CreatedBy     uint      `gorm:"not null" json:"created_by"`
	Summary       string    `gorm:"size:4096" json:"summary"`
	SummaryUptoID uint      `gorm:"not null;default:0" json:"summary_upto_id"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Message 会话消息;role 仅 user|assistant(工具往返为瞬态,不入库)。
type Message struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	ConversationID uint      `gorm:"index;not null" json:"conversation_id"`
	Role           string    `gorm:"size:16;not null" json:"role"`
	Content        string    `gorm:"not null" json:"content"`
	CreatedAt      time.Time `json:"created_at"`
}

// ChartConfig 单个图表契约。Spec 为 @antv/g2 spec(原样透传)。
type ChartConfig struct {
	ID       string         `json:"id"`
	Title    string         `json:"title"`
	Width    string         `json:"width"` // full | 1/2 | 1/3 | 2/3
	Type     string         `json:"type"`
	SQL      string         `json:"sql"`
	Subjects []uint         `json:"subjects"` // 数据主题 ID
	Spec     map[string]any `json:"spec"`
}

// PageConfig 页面配置契约。
type PageConfig struct {
	Version int           `json:"version"`
	Layout  string        `json:"layout"` // fluid-grid
	Charts  []ChartConfig `json:"charts"`
}

func DefaultPageConfig() PageConfig {
	start := time.Now()
	slog.Debug("方法进入", "fn", "model.DefaultPageConfig")
	defer func() {
		slog.Debug("方法离开", "fn", "model.DefaultPageConfig", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return PageConfig{Version: 1, Layout: "fluid-grid", Charts: []ChartConfig{}}
}

var validWidths = map[string]bool{"full": true, "1/2": true, "1/3": true, "2/3": true}

// MarshalPageConfig 校验结构并归一化序列化。
func MarshalPageConfig(cfg PageConfig) (out string, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "model.MarshalPageConfig", "charts", len(cfg.Charts))
	defer func() {
		slog.Debug("方法离开", "fn", "model.MarshalPageConfig", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	if cfg.Layout == "" {
		cfg.Layout = "fluid-grid"
	}
	if cfg.Layout != "fluid-grid" {
		return "", errors.New("layout 必须为 fluid-grid")
	}
	if cfg.Charts == nil {
		cfg.Charts = []ChartConfig{}
	}
	for _, c := range cfg.Charts {
		if c.ID == "" {
			return "", errors.New("图表缺少 id")
		}
		if c.SQL == "" {
			return "", errors.New("图表缺少 sql")
		}
		if !validWidths[c.Width] {
			return "", fmt.Errorf("图表宽度非法: %s", c.Width)
		}
	}
	b, err := json.Marshal(cfg)
	return string(b), err
}

// LLMProvider 大模型供应商(Anthropic 兼容端点);APIKey 永不出 JSON。
type LLMProvider struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:64;not null;uniqueIndex" json:"name"`
	BaseURL   string    `gorm:"size:255;not null" json:"base_url"`
	APIKey    string    `gorm:"size:255;not null;default:''" json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LLMModel 供应商下的模型;IsCurrent 全局唯一(由服务层事务保证)。
type LLMModel struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ProviderID    uint      `gorm:"not null;uniqueIndex:idx_provider_model,priority:1;index" json:"provider_id"`
	Name          string    `gorm:"size:128;not null;uniqueIndex:idx_provider_model,priority:2" json:"name"`
	ContextWindow int       `gorm:"not null" json:"context_window"`
	MaxTokens     int       `gorm:"not null" json:"max_tokens"`
	IsCurrent     bool      `gorm:"not null;default:false" json:"is_current"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// BrandConfig 品牌配置(单行 id=1);空名称/标题回退默认,LogoExt/FaviconExt 为空表示未上传。
type BrandConfig struct {
	ID           uint      `gorm:"primaryKey"`
	SystemName   string    `gorm:"size:32;not null"`
	BrowserTitle string    `gorm:"size:64;not null"`
	LogoExt      string    `gorm:"size:8;not null;default:''"`
	FaviconExt   string    `gorm:"size:8;not null;default:''"`
	Version      int64     `gorm:"not null;default:0"`
	UpdatedAt    time.Time
}
