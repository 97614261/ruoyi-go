package repository

import (
	"context"
	"database/sql"
	"fmt"
	"gorm.io/gorm"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
	"strings"
)

type setupSpec struct {
	key, permission string
	tables          []string
	exists          string
}

// SQL fragments are compile-time constants. Every subquery binds the same
// authenticated tenant, including all joined entities; no client scope is read.
func setupSpecs() []setupSpec {
	return []setupSpec{
		{"employees", "system:user:list", []string{"sys_user"}, "SELECT 1 FROM sys_user a WHERE a.tenant_id=? AND a.user_type='00' AND a.status='0' AND a.del_flag='0'"},
		{"roles", "system:role:list", []string{"sys_role"}, "SELECT 1 FROM sys_role a WHERE a.tenant_id=? AND a.role_key NOT IN ('tenant_admin','platform_admin','admin') AND a.status='0' AND a.del_flag='0'"},
		{"warehouses", "erp:warehouse:list", []string{"erp_warehouse"}, "SELECT 1 FROM erp_warehouse a WHERE a.tenant_id=? AND a.status='0'"},
		{"products", "erp:sku:list", []string{"erp_sku"}, "SELECT 1 FROM erp_sku a WHERE a.tenant_id=? AND a.status='0'"},
		{"shops", "erp:shop:list", []string{"erp_shop"}, "SELECT 1 FROM erp_shop a WHERE a.tenant_id=? AND a.status='0'"},
		{"suppliers", "erp:supplier:list", []string{"erp_supplier"}, "SELECT 1 FROM erp_supplier a WHERE a.tenant_id=? AND a.status='0'"},
		{"pairings", "erp:shopItem:list", []string{"erp_shop_sku_mapping", "erp_shop_item", "erp_shop", "erp_sku"}, "SELECT 1 FROM erp_shop_sku_mapping a JOIN erp_shop_item i ON i.tenant_id=a.tenant_id AND i.item_id=a.shop_item_id JOIN erp_shop s ON s.tenant_id=a.tenant_id AND s.shop_id=a.shop_id JOIN erp_sku k ON k.tenant_id=a.tenant_id AND k.sku_id=a.sku_id WHERE a.tenant_id=? AND a.status='0' AND i.status='0' AND s.status='0' AND k.status='0'"},
		{"warehouseScopes", "erp:warehouse:grant", []string{"erp_warehouse_access", "sys_user", "erp_warehouse"}, "SELECT 1 FROM erp_warehouse_access a JOIN sys_user u ON u.tenant_id=a.tenant_id AND u.user_id=a.user_id JOIN erp_warehouse w ON w.tenant_id=a.tenant_id AND w.warehouse_id=a.warehouse_id WHERE a.tenant_id=? AND u.user_type='00' AND u.status='0' AND u.del_flag='0' AND w.status='0'"},
		{"shopScopes", "erp:shop:grant", []string{"erp_shop_access", "sys_user", "erp_shop"}, "SELECT 1 FROM erp_shop_access a JOIN sys_user u ON u.tenant_id=a.tenant_id AND u.user_id=a.user_id JOIN erp_shop s ON s.tenant_id=a.tenant_id AND s.shop_id=a.shop_id WHERE a.tenant_id=? AND u.user_type='00' AND u.status='0' AND u.del_flag='0' AND s.status='0'"},
		{"logistics", "erp:logistics:list", []string{"erp_shipping_method", "erp_carrier"}, "SELECT 1 FROM erp_shipping_method a JOIN erp_carrier c ON c.tenant_id=a.tenant_id AND c.carrier_id=a.carrier_id WHERE a.tenant_id=? AND a.status='0' AND c.status='0'"},
		{"accounts", "erp:financeAccount:list", []string{"erp_fin_account"}, "SELECT 1 FROM erp_fin_account a WHERE a.tenant_id=? AND a.status='0'"},
		{"stock", "erp:stock:list", []string{"erp_stock"}, "SELECT 1 FROM erp_stock a WHERE a.tenant_id=? AND a.on_hand>0"},
		{"receipts", "erp:purchaseReceipt:list", []string{"erp_purchase_receipt"}, "SELECT 1 FROM erp_purchase_receipt a WHERE a.tenant_id=?"},
		{"shipments", "erp:shipment:list", []string{"erp_package"}, "SELECT 1 FROM erp_package a WHERE a.tenant_id=? AND a.status='SHIPPED'"},
		{"payables", "erp:payable:list", []string{"erp_fin_payable"}, "SELECT 1 FROM erp_fin_payable a WHERE a.tenant_id=?"},
		{"receivables", "erp:receivable:list", []string{"erp_sf_receivable"}, "SELECT 1 FROM erp_sf_receivable a WHERE a.tenant_id=?"},
	}
}

func SelectERPSetup(ctx context.Context, allowed map[string]bool) (*model.ERPSetup, error) {
	tid, uid := tenantctx.ID(ctx), erpctx.UserID(ctx)
	if tid < 1 || uid < 1 || tenantctx.Get(ctx).Platform {
		return nil, errs.New("仅企业所有者可读取启用检查")
	}
	out := &model.ERPSetup{Checks: []model.ERPSetupCheck{}}
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		var owner model.SysTenant
		if e := tx.Where("tenant_id=? AND tenant_kind='01' AND owner_user_id=?", tid, uid).Take(&owner).Error; e != nil {
			if e == gorm.ErrRecordNotFound {
				return errs.New("仅企业所有者可读取启用检查")
			}
			return e
		}
		specs := setupSpecs()
		tables := []string{}
		seen := map[string]bool{}
		for _, s := range specs {
			for _, t := range s.tables {
				if !seen[t] {
					tables = append(tables, t)
					seen[t] = true
				}
			}
		}
		var installed []string
		if e := tx.WithContext(tenantctx.System(ctx)).Table("information_schema.tables").Where("table_schema=DATABASE() AND table_name IN ?", tables).Pluck("table_name", &installed).Error; e != nil {
			return e
		}
		have := map[string]bool{}
		for _, t := range installed {
			have[t] = true
		}
		columns := []string{}
		args := []any{}
		indices := []int{}
		for i, s := range specs {
			check := model.ERPSetupCheck{Key: s.key, State: "NOT_ENABLED"}
			if allowed[s.permission] {
				n := 0
				for _, t := range s.tables {
					if have[t] {
						n++
					}
				}
				// Some dependencies (SKU/users) predate the optional module. If its
				// primary table is absent, report not installed; startup guards still
				// reject partially installed versioned modules. Missing prerequisites
				// of an installed primary table are errors, never a missing/zero flag.
				check.State = "NOT_INSTALLED"
				if have[s.tables[0]] {
					if n != len(s.tables) {
						return errs.New("启用检查依赖结构不完整")
					}
					check.State = "MISSING"
					columns = append(columns, "EXISTS ("+s.exists+") AS "+s.key)
					args = append(args, tid)
					indices = append(indices, i)
				}
			}
			out.Checks = append(out.Checks, check)
		}
		if len(columns) == 0 {
			return nil
		}
		flags := map[string]any{}
		if e := tx.Table("sys_tenant").Select(strings.Join(columns, ","), args...).Where("tenant_id=? AND owner_user_id=?", tid, uid).Take(&flags).Error; e != nil {
			return e
		}
		for _, i := range indices {
			n, ok := flags[specs[i].key].(int64)
			if !ok || (n != 0 && n != 1) {
				return fmt.Errorf("启用检查结果类型异常")
			}
			if n == 1 {
				out.Checks[i].State = "READY"
			}
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	out.CheckedAt = types.Now()
	return out, nil
}
