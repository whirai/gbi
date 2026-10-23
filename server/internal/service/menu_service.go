package service

import (
	"errors"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"gbi/server/internal/logger"
	"gbi/server/internal/model"
)

var (
	ErrMenuNotFound     = errors.New("菜单不存在")
	ErrMenuItemNotFound = errors.New("菜单项不存在")
)

type MenuService struct {
	db    *gorm.DB
	pages *PageService
}

func NewMenuService(db *gorm.DB, pages *PageService) *MenuService {
	start := time.Now()
	slog.Debug("方法进入", "fn", "service.NewMenuService")
	defer func() {
		slog.Debug("方法离开", "fn", "service.NewMenuService", "elapsed_ms", time.Since(start).Milliseconds())
	}()
	return &MenuService{db: db, pages: pages}
}

type MenuCreate struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
	Sort int    `json:"sort"`
}

type MenuUpdate struct {
	Name *string `json:"name"`
	Desc *string `json:"desc"`
	Sort *int    `json:"sort"`
}

type MenuItemCreate struct {
	PageID uint   `json:"page_id"`
	Name   string `json:"name"`
	Sort   int    `json:"sort"`
}

type MenuItemUpdate struct {
	PageID *uint   `json:"page_id"`
	Name   *string `json:"name"`
	Sort   *int    `json:"sort"`
}

type VisibleMenuItem struct {
	ID     uint   `json:"id"`
	Name   string `json:"name"`
	PageID uint   `json:"page_id"`
}

type VisibleMenu struct {
	ID    uint              `json:"id"`
	Name  string            `json:"name"`
	Items []VisibleMenuItem `json:"items"`
}

func (s *MenuService) ListMenus() (menus []model.Menu, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "MenuService.ListMenus")
	defer func() {
		slog.Debug("方法离开", "fn", "MenuService.ListMenus", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	err = s.db.Order("sort, id").Find(&menus).Error
	return menus, err
}

func (s *MenuService) getMenu(id uint) (res *model.Menu, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "MenuService.getMenu", "id", id)
	defer func() {
		slog.Debug("方法离开", "fn", "MenuService.getMenu", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var m model.Menu
	if err := s.db.First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMenuNotFound
		}
		return nil, err
	}
	return &m, nil
}

func (s *MenuService) CreateMenu(in MenuCreate) (res *model.Menu, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "MenuService.CreateMenu", "name", logger.Truncate(in.Name, 64))
	defer func() {
		slog.Debug("方法离开", "fn", "MenuService.CreateMenu", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	slog.Debug("MenuService.CreateMenu", "name", in.Name)
	m := &model.Menu{Name: in.Name, Desc: in.Desc, Sort: in.Sort}
	if err := s.db.Create(m).Error; err != nil {
		slog.Debug("MenuService.CreateMenu错误", "err", err)
		return nil, err
	}
	slog.Debug("MenuService.CreateMenu成功", "id", m.ID)
	return m, nil
}

func (s *MenuService) UpdateMenu(id uint, in MenuUpdate) (res *model.Menu, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "MenuService.UpdateMenu", "id", id)
	defer func() {
		slog.Debug("方法离开", "fn", "MenuService.UpdateMenu", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	m, err := s.getMenu(id)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		m.Name = *in.Name
	}
	if in.Desc != nil {
		m.Desc = *in.Desc
	}
	if in.Sort != nil {
		m.Sort = *in.Sort
	}
	if err := s.db.Save(m).Error; err != nil {
		return nil, err
	}
	return m, nil
}

// DeleteMenu 级联删除菜单项与菜单行:两条删除同事务(参照 GroupService.Delete),
// 避免「项已删而菜单行残留」的不可修复半删状态。
func (s *MenuService) DeleteMenu(id uint) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "MenuService.DeleteMenu", "id", id)
	defer func() {
		slog.Debug("方法离开", "fn", "MenuService.DeleteMenu", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	slog.Debug("MenuService.DeleteMenu", "id", id)
	if _, err := s.getMenu(id); err != nil {
		slog.Debug("MenuService.DeleteMenu错误", "err", err)
		return err
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&model.MenuItem{}, "menu_id = ?", id).Error; err != nil {
			return err
		}
		return tx.Delete(&model.Menu{}, id).Error
	})
	if err != nil {
		slog.Debug("MenuService.DeleteMenu错误", "err", err)
		return err
	}
	slog.Debug("MenuService.DeleteMenu成功")
	return nil
}

func (s *MenuService) ListItems(menuID uint) (items []model.MenuItem, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "MenuService.ListItems", "menu_id", menuID)
	defer func() {
		slog.Debug("方法离开", "fn", "MenuService.ListItems", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	if _, err := s.getMenu(menuID); err != nil {
		return nil, err
	}
	err = s.db.Where("menu_id = ?", menuID).Order("sort, id").Find(&items).Error
	return items, err
}

func (s *MenuService) getItem(menuID, itemID uint) (res *model.MenuItem, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "MenuService.getItem", "menu_id", menuID, "item_id", itemID)
	defer func() {
		slog.Debug("方法离开", "fn", "MenuService.getItem", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	var item model.MenuItem
	if err := s.db.First(&item, itemID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMenuItemNotFound
		}
		return nil, err
	}
	if item.MenuID != menuID {
		return nil, ErrMenuItemNotFound
	}
	return &item, nil
}

func (s *MenuService) CreateItem(menuID uint, in MenuItemCreate) (res *model.MenuItem, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "MenuService.CreateItem", "menu_id", menuID, "page_id", in.PageID, "name", logger.Truncate(in.Name, 64))
	defer func() {
		slog.Debug("方法离开", "fn", "MenuService.CreateItem", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	slog.Debug("MenuService.CreateItem", "menu_id", menuID, "name", in.Name)
	if _, err := s.getMenu(menuID); err != nil {
		slog.Debug("MenuService.CreateItem错误", "err", err)
		return nil, err
	}
	if _, err := s.pages.Get(in.PageID); err != nil {
		slog.Debug("MenuService.CreateItem错误", "err", err)
		return nil, err
	}
	item := &model.MenuItem{MenuID: menuID, PageID: in.PageID, Name: in.Name, Sort: in.Sort}
	if err := s.db.Create(item).Error; err != nil {
		slog.Debug("MenuService.CreateItem错误", "err", err)
		return nil, err
	}
	slog.Debug("MenuService.CreateItem成功", "id", item.ID)
	return item, nil
}

func (s *MenuService) UpdateItem(menuID, itemID uint, in MenuItemUpdate) (res *model.MenuItem, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "MenuService.UpdateItem", "menu_id", menuID, "item_id", itemID)
	defer func() {
		slog.Debug("方法离开", "fn", "MenuService.UpdateItem", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	item, err := s.getItem(menuID, itemID)
	if err != nil {
		return nil, err
	}
	if in.PageID != nil {
		if _, err := s.pages.Get(*in.PageID); err != nil {
			return nil, err
		}
		item.PageID = *in.PageID
	}
	if in.Name != nil {
		item.Name = *in.Name
	}
	if in.Sort != nil {
		item.Sort = *in.Sort
	}
	if err := s.db.Save(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func (s *MenuService) DeleteItem(menuID, itemID uint) (err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "MenuService.DeleteItem", "menu_id", menuID, "item_id", itemID)
	defer func() {
		slog.Debug("方法离开", "fn", "MenuService.DeleteItem", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	item, err := s.getItem(menuID, itemID)
	if err != nil {
		return err
	}
	return s.db.Delete(&model.MenuItem{}, item.ID).Error
}

// VisibleMenus 仅含 published 且 CanView 的项;无可见项的菜单不下发。
func (s *MenuService) VisibleMenus(userID uint) (result []VisibleMenu, err error) {
	start := time.Now()
	slog.Debug("方法进入", "fn", "MenuService.VisibleMenus", "user_id", userID)
	defer func() {
		slog.Debug("方法离开", "fn", "MenuService.VisibleMenus", "elapsed_ms", time.Since(start).Milliseconds(), "err", err)
	}()
	menus, err := s.ListMenus()
	if err != nil {
		return nil, err
	}
	result = make([]VisibleMenu, 0, len(menus))
	for i := range menus {
		var items []model.MenuItem
		if err := s.db.Where("menu_id = ?", menus[i].ID).Order("sort, id").Find(&items).Error; err != nil {
			return nil, err
		}
		vis := make([]VisibleMenuItem, 0, len(items))
		for j := range items {
			p, err := s.pages.Get(items[j].PageID)
			if err != nil {
				if errors.Is(err, ErrPageNotFound) {
					continue // 页面已删,跳过孤儿项
				}
				return nil, err // 其余错误(DB 故障)不得静默吞掉
			}
			if p.Status != "published" {
				continue
			}
			ok, err := s.pages.CanView(userID, p)
			if err != nil {
				return nil, err
			}
			if ok {
				vis = append(vis, VisibleMenuItem{ID: items[j].ID, Name: items[j].Name, PageID: items[j].PageID})
			}
		}
		if len(vis) > 0 {
			result = append(result, VisibleMenu{ID: menus[i].ID, Name: menus[i].Name, Items: vis})
		}
	}
	return result, nil
}
