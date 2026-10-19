package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
)

// EnsurePlatformConsole seeds presentation/ACL data only in the dedicated local
// multitenant database. It reuses the existing platform owner, never elevates a
// customer account or changes a password. All changes are atomic/idempotent.
func EnsurePlatformConsole(ctx context.Context) error {
	return Transaction(tenantctx.System(ctx), func(tx *gorm.DB) error {
		var database string
		if err := tx.Raw("SELECT DATABASE()").Scan(&database).Error; err != nil {
			return err
		}
		if database != TenantDatabaseName {
			return fmt.Errorf("拒绝修改非独立多租户开发库")
		}
		var tenant model.SysTenant
		if err := tx.Where("tenant_code=? AND tenant_kind=?", "platform", "00").Take(&tenant).Error; err != nil || tenant.OwnerUserID == nil {
			return fmt.Errorf("平台所有者未初始化")
		}
		var owner model.SysUser
		if err := tx.Where("tenant_id=? AND user_id=? AND user_type='02' AND del_flag='0' AND status='0'", tenant.TenantID, *tenant.OwnerUserID).Take(&owner).Error; err != nil {
			return fmt.Errorf("平台所有者身份异常，拒绝自动提权")
		}
		var role model.SysRole
		if err := tx.Where("tenant_id=? AND role_key='platform_admin' AND del_flag='0' AND status='0'", tenant.TenantID).Take(&role).Error; err != nil {
			return fmt.Errorf("平台管理员角色未初始化")
		}
		menus := platformConsoleMenus()
		ids := make([]int64, 0, len(menus))
		expected := make(map[int64]model.SysMenu, len(menus))
		for _, menu := range menus {
			ids = append(ids, menu.MenuID)
			expected[menu.MenuID] = menu
		}
		var existing []model.SysMenu
		if err := tx.Where("menu_id IN ?", ids).Find(&existing).Error; err != nil {
			return err
		}
		for _, old := range existing {
			want := expected[old.MenuID]
			if old.Path != want.Path || old.MenuType != want.MenuType || old.Perms == nil || *old.Perms != *want.Perms {
				return fmt.Errorf("菜单ID %d已被其他功能占用，拒绝覆盖", old.MenuID)
			}
		}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "menu_id"}}, DoUpdates: clause.AssignmentColumns([]string{
			"menu_name", "parent_id", "order_num", "component", "route_name", "is_frame", "is_cache", "visible", "status", "icon", "update_by", "update_time",
		})}).Create(&menus).Error; err != nil {
			return err
		}
		grants := make([]model.SysTenantMenu, 0, len(ids))
		roleMenus := make([]model.SysRoleMenu, 0, len(ids))
		for _, id := range ids {
			grants = append(grants, model.SysTenantMenu{TenantRecord: model.TenantRecord{TenantID: tenant.TenantID}, MenuID: id})
			roleMenus = append(roleMenus, model.SysRoleMenu{TenantRecord: model.TenantRecord{TenantID: tenant.TenantID}, RoleID: role.RoleID, MenuID: id})
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&grants).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&roleMenus).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.SysUserRole{TenantRecord: model.TenantRecord{TenantID: tenant.TenantID}, UserID: owner.UserID, RoleID: role.RoleID}).Error; err != nil {
			return err
		}
		return tx.Model(&model.SysRole{}).Where("tenant_id=? AND role_id=?", tenant.TenantID, role.RoleID).
			Updates(map[string]any{"role_name": "平台超级管理员", "update_by": "platform-console", "update_time": types.Now()}).Error
	})
}

func platformConsoleMenus() []model.SysMenu {
	ptr := func(v string) *string { return &v }
	order := 0
	base := model.SysMenu{OrderNum: &order, IsFrame: "1", IsCache: "1", Visible: "0", Status: "0", CreateBy: "platform-console", CreateTime: types.Now(), UpdateBy: "platform-console", UpdateTime: types.Now()}
	root, list, add, edit, grant, reset := base, base, base, base, base, base
	root.MenuID, root.MenuName, root.Path, root.MenuType, root.Icon, root.Perms = 5100, "平台管理", "platform", "M", "tree", ptr("platform:tenant:list")
	list.MenuID, list.ParentID, list.MenuName, list.Path, list.MenuType = 5000, 5100, "租户管理", "tenant", "C"
	list.Component, list.Perms, list.Icon = ptr("platform/tenant/index"), ptr("platform:tenant:list"), "peoples"
	add.MenuID, add.ParentID, add.MenuName, add.Path, add.MenuType, add.Perms = 5001, 5000, "开通企业", "", "F", ptr("platform:tenant:add")
	edit.MenuID, edit.ParentID, edit.MenuName, edit.Path, edit.MenuType, edit.Perms = 5002, 5000, "管理企业", "", "F", ptr("platform:tenant:edit")
	grant.MenuID, grant.ParentID, grant.MenuName, grant.Path, grant.MenuType, grant.Perms = 5003, 5000, "企业功能授权", "", "F", ptr("platform:tenant:grant")
	reset.MenuID, reset.ParentID, reset.MenuName, reset.Path, reset.MenuType, reset.Perms = 5004, 5000, "重置企业管理员密码", "", "F", ptr("platform:tenant:resetPwd")
	detail, quota, lifecycle := base, base, base
	detail.MenuID, detail.ParentID, detail.MenuName, detail.Path, detail.MenuType, detail.Perms = 5005, 5000, "企业详情", "", "F", ptr("platform:tenant:query")
	quota.MenuID, quota.ParentID, quota.MenuName, quota.Path, quota.MenuType, quota.Perms = 5006, 5000, "企业配额", "", "F", ptr("platform:tenant:quota")
	lifecycle.MenuID, lifecycle.ParentID, lifecycle.MenuName, lifecycle.Path, lifecycle.MenuType, lifecycle.Perms = 5007, 5000, "企业生命周期", "", "F", ptr("platform:tenant:lifecycle")
	return []model.SysMenu{root, list, add, edit, grant, reset, detail, quota, lifecycle}
}
