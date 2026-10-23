package repository

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/tenantctx"
	"strings"
	"testing"
)

func TestPurchaseScopeAndMenus(t *testing.T) {
	for _, ctx := range []context.Context{context.Background(), tenantctx.With(context.Background(), 17, false), erpctx.WithUser(tenantctx.With(context.Background(), 17, true), 8)} {
		if purchaseScope(guardDB(t).WithContext(ctx).Model(&model.ERPPurchase{})).Error == nil {
			t.Fatal("unscoped purchase accepted")
		}
	}
	ctx := erpctx.WithUser(tenantctx.With(context.Background(), 17, false), 8)
	q := purchaseScope(guardDB(t).WithContext(ctx).Model(&model.ERPPurchase{})).Find(&[]model.ERPPurchase{})
	sql := q.Statement.SQL.String()
	if q.Error != nil || !strings.Contains(sql, "tenant_id") || !strings.Contains(sql, "warehouse_id") {
		t.Fatal(sql, q.Error)
	}
	wanted := map[string]bool{}
	for _, p := range []string{"list", "query", "add", "edit", "confirm", "cancel", "close", "receive"} {
		wanted["erp:purchase:"+p] = false
	}
	for _, m := range erpMenus() {
		if m.Perms == nil {
			continue
		}
		p := *m.Perms
		if strings.HasPrefix(p, "erp:purchase:") {
			if _, ok := wanted[p]; !ok {
				t.Fatal("unexpected purchase permission", p)
			}
			wanted[p] = true
		}
	}
	for p, found := range wanted {
		if !found {
			t.Fatal("missing", p)
		}
	}
}
