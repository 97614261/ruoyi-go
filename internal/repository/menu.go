package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"ruoyi-go/internal/model"
)

// menuTreeTypes 参与路由构建的菜单类型，按钮(F)不进路由。
var menuTreeTypes = []string{model.MenuTypeDir, model.MenuTypeMenu}

// SelectMenuTreeAll 查全部可用菜单，供超级管理员使用。
//
// 对应 Java 版 SysMenuMapper.selectMenuTreeAll。
func SelectMenuTreeAll(ctx context.Context) ([]model.SysMenu, error) {
	var menus []model.SysMenu
	err := DB(ctx).
		Where("menu_type IN ?", menuTreeTypes).
		Where("status = ?", model.StatusNormal).
		Order("parent_id, order_num").
		Find(&menus).Error
	if err != nil {
		return nil, fmt.Errorf("查询全部菜单失败: %w", err)
	}
	return menus, nil
}

// SelectMenuTreeByUserID 查指定用户可见的菜单。
//
// 对应 Java 版 SysMenuMapper.selectMenuTreeByUserId：
// 菜单和角色都必须是启用状态，按钮类型不参与。
func SelectMenuTreeByUserID(ctx context.Context, userID int64) ([]model.SysMenu, error) {
	var menus []model.SysMenu
	err := DB(ctx).
		Table("sys_menu m").
		Distinct("m.*").
		Joins("LEFT JOIN sys_role_menu rm ON m.menu_id = rm.menu_id").
		Joins("LEFT JOIN sys_user_role ur ON rm.role_id = ur.role_id").
		Joins("LEFT JOIN sys_role ro ON ur.role_id = ro.role_id").
		Where("ur.user_id = ?", userID).
		Where("m.menu_type IN ?", menuTreeTypes).
		Where("m.status = ?", model.StatusNormal).
		Where("ro.status = ?", model.StatusNormal).
		Order("m.parent_id, m.order_num").
		Find(&menus).Error
	if err != nil {
		return nil, fmt.Errorf("查询用户 %d 的菜单失败: %w", userID, err)
	}
	return menus, nil
}

// SelectMenuListAll 菜单管理页面用：查全部菜单（含按钮），支持条件过滤。
func SelectMenuListAll(ctx context.Context, query model.MenuQuery) ([]model.SysMenu, error) {
	db := DB(ctx).Model(&model.SysMenu{})
	if query.MenuName != "" {
		db = db.Where("menu_name LIKE ?", "%"+query.MenuName+"%")
	}
	if query.Visible != "" {
		db = db.Where("visible = ?", query.Visible)
	}
	if query.Status != "" {
		db = db.Where("status = ?", query.Status)
	}

	var menus []model.SysMenu
	if err := db.Order("parent_id, order_num").Find(&menus).Error; err != nil {
		return nil, fmt.Errorf("查询菜单列表失败: %w", err)
	}
	return menus, nil
}

// SelectMenuListByUserID 菜单管理页面用：查普通用户可见的全部菜单（含按钮）。
func SelectMenuListByUserID(ctx context.Context, userID int64, query model.MenuQuery) ([]model.SysMenu, error) {
	db := DB(ctx).
		Table("sys_menu m").
		Distinct("m.*").
		Joins("LEFT JOIN sys_role_menu rm ON m.menu_id = rm.menu_id").
		Joins("LEFT JOIN sys_user_role ur ON rm.role_id = ur.role_id").
		Joins("LEFT JOIN sys_role ro ON ur.role_id = ro.role_id").
		Where("ur.user_id = ?", userID)

	if query.MenuName != "" {
		db = db.Where("m.menu_name LIKE ?", "%"+query.MenuName+"%")
	}
	if query.Visible != "" {
		db = db.Where("m.visible = ?", query.Visible)
	}
	if query.Status != "" {
		db = db.Where("m.status = ?", query.Status)
	}

	var menus []model.SysMenu
	if err := db.Order("m.parent_id, m.order_num").Find(&menus).Error; err != nil {
		return nil, fmt.Errorf("查询用户 %d 的菜单列表失败: %w", userID, err)
	}
	return menus, nil
}

// SelectMenuByID 按 ID 查菜单，不存在返回 (nil, nil)。
func SelectMenuByID(ctx context.Context, menuID int64) (*model.SysMenu, error) {
	var menu model.SysMenu
	err := DB(ctx).Where("menu_id = ?", menuID).Take(&menu).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询菜单 %d 失败: %w", menuID, err)
	}
	return &menu, nil
}

// SelectMenuPermsByRoleID 查角色的权限标识。
//
// 注意这里只过滤菜单状态，不过滤角色状态 —— 角色是否启用由调用方判断。
func SelectMenuPermsByRoleID(ctx context.Context, roleID int64) ([]string, error) {
	var perms []string
	err := DB(ctx).
		Table("sys_menu m").
		Joins("LEFT JOIN sys_role_menu rm ON m.menu_id = rm.menu_id").
		Where("m.status = ?", model.StatusNormal).
		Where("rm.role_id = ?", roleID).
		Pluck("DISTINCT IFNULL(m.perms, '')", &perms).Error
	if err != nil {
		return nil, fmt.Errorf("查询角色 %d 的权限标识失败: %w", roleID, err)
	}
	return perms, nil
}

// SelectMenuPermsByUserID 查用户的权限标识。
//
// 用 IFNULL 兜底：目录类型的 perms 是 NULL，直接扫进 string 会报错。
// 返回值可能含空串和逗号分隔的多个标识，由 service 层清洗。
func SelectMenuPermsByUserID(ctx context.Context, userID int64) ([]string, error) {
	var perms []string
	err := DB(ctx).
		Table("sys_menu m").
		Joins("LEFT JOIN sys_role_menu rm ON m.menu_id = rm.menu_id").
		Joins("LEFT JOIN sys_user_role ur ON rm.role_id = ur.role_id").
		Joins("LEFT JOIN sys_role r ON r.role_id = ur.role_id").
		Where("m.status = ?", model.StatusNormal).
		Where("r.status = ?", model.StatusNormal).
		Where("ur.user_id = ?", userID).
		Pluck("DISTINCT IFNULL(m.perms, '')", &perms).Error
	if err != nil {
		return nil, fmt.Errorf("查询用户 %d 的权限标识失败: %w", userID, err)
	}
	return perms, nil
}

// CountMenuByNameAndParent 同一父菜单下的同名菜单数量。
func CountMenuByNameAndParent(ctx context.Context, menuName string, parentID, excludeID int64) (int64, error) {
	db := DB(ctx).Model(&model.SysMenu{}).
		Where("menu_name = ?", menuName).
		Where("parent_id = ?", parentID)
	if excludeID > 0 {
		db = db.Where("menu_id <> ?", excludeID)
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("校验菜单名称唯一性失败: %w", err)
	}
	return count, nil
}

// SelectMenusByPathOrRouteName 查路径或路由名相同的菜单，用于路由冲突检测。
//
// 【两个细节都不能少】
//
//  1. menu_type IN ('M','C') —— 按钮(F)的 path 和 route_name 都是空串，
//     不排除掉的话，库里只要已有一个按钮，再加第二个就会自己撞自己，
//     报"路由名称或地址已存在"，按钮功能直接不可用。
//  2. 四路交叉匹配 —— path 和 routeName 要互相比对，因为没配 route_name 的
//     菜单会用 path 兜底当路由名，两者处在同一个命名空间里。
func SelectMenusByPathOrRouteName(ctx context.Context, path, routeName string) ([]model.SysMenu, error) {
	var menus []model.SysMenu
	err := DB(ctx).
		Where("menu_type IN ?", menuTreeTypes).
		Where("path = ? OR path = ? OR route_name = ? OR route_name = ?",
			path, routeName, path, routeName).
		Find(&menus).Error
	if err != nil {
		return nil, fmt.Errorf("查询路由冲突失败: %w", err)
	}
	return menus, nil
}

// HasChildByMenuID 是否存在子菜单。
func HasChildByMenuID(ctx context.Context, menuID int64) (bool, error) {
	var count int64
	err := DB(ctx).Model(&model.SysMenu{}).Where("parent_id = ?", menuID).Limit(1).Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("检查菜单 %d 是否有子菜单失败: %w", menuID, err)
	}
	return count > 0, nil
}

// CheckMenuExistRole 菜单是否已被角色分配。
func CheckMenuExistRole(ctx context.Context, menuID int64) (bool, error) {
	var count int64
	err := DB(ctx).Table("sys_role_menu").Where("menu_id = ?", menuID).Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("检查菜单 %d 是否已分配失败: %w", menuID, err)
	}
	return count > 0, nil
}

// InsertMenu 新增菜单。
func InsertMenu(ctx context.Context, menu *model.SysMenu) error {
	if err := DB(ctx).Create(menu).Error; err != nil {
		return fmt.Errorf("新增菜单失败: %w", err)
	}
	return nil
}

// UpdateMenu 更新菜单。
func UpdateMenu(ctx context.Context, menu *model.SysMenu) error {
	updates := map[string]any{
		"menu_name":   menu.MenuName,
		"parent_id":   menu.ParentID,
		"order_num":   model.IntValue(menu.OrderNum),
		"path":        menu.Path,
		"component":   menu.Component,
		"query":       menu.Query,
		"route_name":  menu.RouteName,
		"is_frame":    menu.IsFrame,
		"is_cache":    menu.IsCache,
		"menu_type":   menu.MenuType,
		"visible":     menu.Visible,
		"status":      menu.Status,
		"perms":       menu.Perms,
		"icon":        menu.Icon,
		"update_by":   menu.UpdateBy,
		"update_time": menu.UpdateTime,
	}
	err := DB(ctx).Model(&model.SysMenu{}).
		Where("menu_id = ?", menu.MenuID).
		Updates(updates).Error
	if err != nil {
		return fmt.Errorf("更新菜单 %d 失败: %w", menu.MenuID, err)
	}
	return nil
}

// DeleteMenuByID 删除菜单（物理删除，sys_menu 没有 del_flag）。
func DeleteMenuByID(ctx context.Context, menuID int64) error {
	if err := DB(ctx).Where("menu_id = ?", menuID).Delete(&model.SysMenu{}).Error; err != nil {
		return fmt.Errorf("删除菜单 %d 失败: %w", menuID, err)
	}
	return nil
}

// UpdateMenuSort 批量保存菜单排序。
func UpdateMenuSort(ctx context.Context, sorts map[int64]int) error {
	if len(sorts) == 0 {
		return nil
	}
	return Transaction(ctx, func(tx *gorm.DB) error {
		for menuID, orderNum := range sorts {
			if err := tx.Model(&model.SysMenu{}).
				Where("menu_id = ?", menuID).
				Update("order_num", orderNum).Error; err != nil {
				return fmt.Errorf("更新菜单 %d 排序失败: %w", menuID, err)
			}
		}
		return nil
	})
}

// SelectMenuIDsByRoleID 查角色已选的菜单 ID。
//
// checkStrictly 为真时排除掉"有子节点被选中"的父节点 ——
// 前端的树组件用半选状态表达父节点，父节点回显成全选会导致勾选状态错乱。
func SelectMenuIDsByRoleID(ctx context.Context, roleID int64, checkStrictly bool) ([]int64, error) {
	db := DB(ctx).
		Table("sys_menu m").
		Joins("LEFT JOIN sys_role_menu rm ON m.menu_id = rm.menu_id").
		Where("rm.role_id = ?", roleID)
	if checkStrictly {
		db = db.Where(`m.menu_id NOT IN (
			SELECT m2.parent_id FROM sys_menu m2
			INNER JOIN sys_role_menu rm2 ON m2.menu_id = rm2.menu_id AND rm2.role_id = ?)`, roleID)
	}

	var ids []int64
	if err := db.Order("m.parent_id, m.order_num").Pluck("m.menu_id", &ids).Error; err != nil {
		return nil, fmt.Errorf("查询角色 %d 的菜单失败: %w", roleID, err)
	}
	return ids, nil
}
