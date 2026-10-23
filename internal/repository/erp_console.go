package repository

import (
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
)

func InstallERPConsole(ctx context.Context, grantDemo bool) error {
	return Transaction(tenantctx.System(ctx), func(tx *gorm.DB) error {
		var dbname string
		if err := tx.Raw("SELECT DATABASE()").Scan(&dbname).Error; err != nil {
			return err
		}
		if dbname != TenantDatabaseName {
			return fmt.Errorf("仅允许独立本地开发库")
		}
		menus := erpMenus()
		ids := make([]int64, len(menus))
		expected := map[int64]model.SysMenu{}
		for i, m := range menus {
			ids[i] = m.MenuID
			expected[m.MenuID] = m
		}
		var old []model.SysMenu
		if err := tx.Where("menu_id IN ?", ids).Find(&old).Error; err != nil {
			return err
		}
		for _, m := range old {
			want := expected[m.MenuID]
			if m.Path != want.Path || m.MenuType != want.MenuType || m.Perms == nil || *m.Perms != *want.Perms {
				return fmt.Errorf("ERP菜单ID已被占用，拒绝覆盖")
			}
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&menus).Error; err != nil {
			return err
		}
		if !grantDemo {
			return nil
		}
		var platform model.SysTenant
		if err := tx.Where("tenant_code='platform' AND tenant_kind='00' AND status='0'").Take(&platform).Error; err != nil || platform.OwnerUserID == nil {
			return fmt.Errorf("平台所有者未初始化，拒绝开通")
		}
		var owner model.SysUser
		if err := tx.Where("tenant_id=? AND user_id=? AND user_type='02' AND status='0' AND del_flag='0'", platform.TenantID, *platform.OwnerUserID).Take(&owner).Error; err != nil {
			return fmt.Errorf("平台所有者身份异常，拒绝开通")
		}
		actor := model.PlatformActor{TenantID: platform.TenantID, UserID: owner.UserID, UserName: owner.UserName, IP: "127.0.0.1"}
		var tenants []model.SysTenant
		// Serialize with platform grants/lifecycle and quota writers. Preserve all
		// preexisting grants, including historical nodes hidden by current trees.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_code IN ? AND tenant_kind='01'", []string{"demo", "demo-b"}).Order("tenant_id").Find(&tenants).Error; err != nil {
			return err
		}
		if len(tenants) != 2 {
			return fmt.Errorf("demo/demo-b必须都存在，拒绝部分开通")
		}
		for _, t := range tenants {
			if err := customerMutable(&t); err != nil {
				return err
			}
			var warehouse model.ErpWarehouse
			warehouseErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND warehouse_code='DEFAULT'", t.TenantID).Take(&warehouse).Error
			warehouseCreated := errors.Is(warehouseErr, gorm.ErrRecordNotFound)
			if warehouseErr != nil && !warehouseCreated {
				return warehouseErr
			}
			if warehouseCreated {
				warehouse = model.ErpWarehouse{TenantID: t.TenantID, WarehouseCode: "DEFAULT", WarehouseName: "默认仓库", Status: "0", CreateBy: "erp-console", CreateTime: types.Now()}
				if err := tx.Create(&warehouse).Error; err != nil {
					return err
				}
			}
			var existingGrants []model.SysTenantMenu
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND menu_id IN ?", t.TenantID, ids).Find(&existingGrants).Error; err != nil {
				return err
			}
			existingIDs := make(map[int64]bool, len(existingGrants))
			for _, g := range existingGrants {
				existingIDs[g.MenuID] = true
			}
			grants := []model.SysTenantMenu{}
			for _, id := range ids {
				if !existingIDs[id] {
					grants = append(grants, model.SysTenantMenu{TenantRecord: model.TenantRecord{TenantID: t.TenantID}, MenuID: id})
				}
			}
			// clientFoundRows=true makes ON DUPLICATE KEY no-ops report affected
			// rows too. Determine real additions under locks, not via RowsAffected.
			if len(grants) > 0 {
				if err := tx.Create(&grants).Error; err != nil {
					return err
				}
				if err := tx.Model(&model.SysTenant{}).Where("tenant_id=?", t.TenantID).Updates(map[string]any{"version": gorm.Expr("version+1"), "update_by": "erp-console", "update_time": types.Now()}).Error; err != nil {
					return err
				}
			}
			if len(grants) > 0 || warehouseCreated {
				if err := insertTenantActionAudit(tx, &t, actor, "ERP本地开通", "erpEnable", "用户明确确认：仅开通demo/demo-b的ERP并创建默认仓库；保留原授权", map[string]any{"addedMenuNodes": len(grants), "defaultWarehouseCreated": warehouseCreated}, model.BusinessTypeGrant); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
func erpMenus() []model.SysMenu {
	ptr := func(s string) *string { return &s }
	order := 0
	base := model.SysMenu{OrderNum: &order, IsFrame: "1", IsCache: "1", Visible: "0", Status: "0", CreateBy: "erp-console", CreateTime: types.Now(), Perms: ptr("")}
	root := base
	root.MenuID = 6000
	root.MenuName = "ERP工作台"
	root.Path = "erp"
	root.MenuType = "M"
	root.Icon = "shopping"
	menus := []model.SysMenu{root}
	specs := []struct {
		id        int64
		key, name string
		actions   []string
	}{{6100, "sku", "商品SKU", []string{"query", "add", "edit", "remove", "export"}}, {6200, "warehouse", "仓库", []string{"query", "add", "edit", "remove", "export", "grant"}}, {6300, "receipt", "采购入库", []string{"query", "add", "confirm", "cancel", "edit"}}, {6400, "sales", "销售出库", []string{"query", "add", "confirm", "cancel", "ship", "edit"}}, {6500, "stock", "库存余额", nil}, {6600, "ledger", "库存流水", nil}, {6700, "salesReturn", "销售退货/冲销", []string{"query", "add", "confirm", "cancel", "edit"}}, {6800, "purchaseReturn", "采购退货/冲销", []string{"query", "add", "confirm", "cancel", "edit"}}, {6900, "count", "库存盘点", []string{"query", "add", "confirm", "cancel", "edit"}}}
	for _, s := range specs {
		m := base
		m.MenuID = s.id
		m.ParentID = 6000
		m.MenuName = s.name
		m.Path = s.key
		m.MenuType = "C"
		m.Component = ptr("erp/" + s.key + "/index")
		m.Perms = ptr("erp:" + s.key + ":list")
		m.Icon = "list"
		menus = append(menus, m)
		for i, action := range s.actions {
			button := base
			button.MenuID = s.id + int64(i) + 1
			button.ParentID = s.id
			button.MenuName = s.name + "-" + action
			button.MenuType = "F"
			button.Perms = ptr("erp:" + s.key + ":" + action)
			menus = append(menus, button)
		}
	}
	// Definitions only. InstallERPConsole(false) never grants existing customers.
	for _, spec := range []struct {
		id        int64
		key, name string
		actions   []string
	}{
		{7000, "channel", "渠道目录", nil},
		{7100, "shop", "店铺管理", []string{"query", "add", "edit", "grant"}},
		{7200, "shopItem", "店铺商品配对", []string{"query", "add", "edit", "map"}},
		{7300, "order", "订单录入", []string{"query", "add", "edit", "import", "export", "review", "cancel"}},
		{7400, "importJob", "订单导入任务", []string{"query"}},
		{7500, "package", "包裹发货", []string{"query", "edit", "ship"}},
		{7600, "supplier", "供应商管理", []string{"query", "add", "edit"}},
		{7700, "purchase", "采购单", []string{"query", "add", "edit", "confirm", "cancel", "close"}},
		{7800, "purchaseReceipt", "采购收货记录", []string{"query"}},
		{7900, "purchaseReceive", "采购收货作业", nil},
		{8000, "purchaseReturnOrder", "采购退货", []string{"query", "return"}},
	} {
		m := base
		m.MenuID = spec.id
		m.ParentID = 6000
		m.MenuName = spec.name
		m.Path = spec.key
		m.MenuType = "C"
		m.Component = ptr("erp/" + spec.key + "/index")
		m.Perms = ptr("erp:" + spec.key + ":list")
		if spec.key == "purchaseReceive" {
			m.Perms = ptr("erp:purchase:receive")
		}
		m.Icon = "list"
		menus = append(menus, m)
		for i, action := range spec.actions {
			b := base
			b.MenuID = spec.id + int64(i) + 1
			b.ParentID = spec.id
			b.MenuName = spec.name + "-" + action
			b.MenuType = "F"
			b.Perms = ptr("erp:" + spec.key + ":" + action)
			menus = append(menus, b)
		}
	}
	return menus
}
