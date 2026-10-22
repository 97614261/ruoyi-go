package repository

import (
	"context"
	"math"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/tenantctx"
	"strings"
	"testing"
)

func TestShopItemScopeAndJoins(t *testing.T) {
	ctx := erpctx.WithUser(tenantctx.With(context.Background(), 10, false), 7)
	var rows []model.ERPShopItemView
	q := shopItemViewQuery(guardDB(t).WithContext(ctx)).Select(shopItemViewColumns).Find(&rows)
	sql := q.Statement.SQL.String()
	if q.Error != nil {
		t.Fatal(q.Error)
	}
	for _, s := range []string{"erp_shop_item.shop_id IN", "erp_shop_access WHERE tenant_id=? AND user_id=?", "s.tenant_id = 10", "m.tenant_id = 10", "k.tenant_id = 10", "`erp_shop_item`.`tenant_id` = ?"} {
		if !strings.Contains(sql, s) {
			t.Fatalf("missing scope %s in %s", s, sql)
		}
	}
	if err := erpShopItemScope(guardDB(t).WithContext(context.Background())).Error; err == nil {
		t.Fatal("missing actor accepted")
	}
}
func TestShopItemVersionsAndMenu(t *testing.T) {
	for _, v := range []int64{0, 1, math.MaxInt64} {
		item := model.ErpShopItem{Version: 2}
		if v == math.MaxInt64 {
			item.Version = v
		}
		if err := checkShopItemVersion(item, v); err == nil {
			t.Fatal("unsafe version", v)
		}
	}
	if err := checkShopItemVersion(model.ErpShopItem{Version: 2}, 2); err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, m := range erpMenus() {
		if m.Perms != nil {
			found[*m.Perms] = true
		}
	}
	for _, s := range []string{"list", "query", "add", "edit", "map"} {
		if !found["erp:shopItem:"+s] {
			t.Fatal("missing menu", s)
		}
	}
	if found["erp:shopItem:remove"] {
		t.Fatal("physical delete exposed")
	}
}
