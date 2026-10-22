package repository

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/tenantctx"
	"strings"
	"testing"
)

func TestERPShopLiveScopeGuard(t *testing.T) {
	for _, ctx := range []context.Context{context.Background(), tenantctx.With(context.Background(), 10, false), erpctx.WithUser(tenantctx.With(context.Background(), 10, true), 7)} {
		if err := erpShopScope(guardDB(t).WithContext(ctx).Model(&model.ErpShop{})).Error; err == nil {
			t.Fatal("missing or platform identity accepted")
		}
	}
	ctx := erpctx.WithUser(tenantctx.With(context.Background(), 10, false), 7)
	var rows []model.ErpShop
	q := erpShopScope(guardDB(t).WithContext(ctx).Model(&model.ErpShop{})).Where("shop_id=?", 8).Find(&rows)
	sql := q.Statement.SQL.String()
	if q.Error != nil || !strings.Contains(sql, "erp_shop_access WHERE tenant_id=? AND user_id=?") || !strings.Contains(sql, "`erp_shop`.`tenant_id` = ?") || !strings.Contains(sql, "owner_user_id=?") {
		t.Fatalf("missing live or tenant scope: %s %v", sql, q.Error)
	}
}
func TestERPShopMenuDefinitions(t *testing.T) {
	wanted := map[string]bool{"erp:channel:list": false, "erp:shop:list": false, "erp:shop:query": false, "erp:shop:add": false, "erp:shop:edit": false, "erp:shop:grant": false}
	ids := map[int64]bool{}
	for _, m := range erpMenus() {
		if ids[m.MenuID] {
			t.Fatal("duplicate ID")
		}
		ids[m.MenuID] = true
		if m.Perms != nil {
			if _, ok := wanted[*m.Perms]; ok {
				wanted[*m.Perms] = true
			}
			if *m.Perms == "erp:shop:remove" || *m.Perms == "erp:channel:add" {
				t.Fatal("forbidden CRUD")
			}
		}
	}
	for p, found := range wanted {
		if !found {
			t.Fatal("missing menu", p)
		}
	}
}
