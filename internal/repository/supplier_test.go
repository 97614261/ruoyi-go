package repository

import (
	"context"
	"encoding/json"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/tenantctx"
	"strings"
	"testing"
)

func TestSupplierReadScopeAndProjection(t *testing.T) {
	for _, ctx := range []context.Context{context.Background(), tenantctx.With(context.Background(), 17, false), erpctx.WithUser(tenantctx.With(context.Background(), 17, true), 8)} {
		if supplierScope(guardDB(t).WithContext(ctx).Table("erp_supplier")).Error == nil {
			t.Fatal("missing/platform identity accepted")
		}
	}
	ctx := erpctx.WithUser(tenantctx.With(context.Background(), 17, false), 8)
	var rows []model.ErpSupplierListRow
	q := supplierScope(TenantTable(guardDB(t).WithContext(ctx), "erp_supplier")).Where("supplier_id=?", 1).Find(&rows)
	if q.Error != nil || !strings.Contains(q.Statement.SQL.String(), "tenant_id") {
		t.Fatal("missing tenant filter", q.Error)
	}
	raw, err := json.Marshal(model.ErpSupplierListRow{})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"contactName", "phone", "email", "address", "remark", "tenantId"} {
		if strings.Contains(string(raw), `"`+field+`"`) {
			t.Fatal("list leaks private field", field)
		}
	}
	result, _ := json.Marshal(model.ERPSupplierResult{SupplierId: 1, Version: 1})
	if string(result) != `{"supplierId":1,"version":1}` {
		t.Fatal("write response must stay minimal")
	}
}
func TestSupplierMenusHaveNoDeleteOrExport(t *testing.T) {
	wanted := map[string]bool{"erp:supplier:list": false, "erp:supplier:query": false, "erp:supplier:add": false, "erp:supplier:edit": false}
	ids := map[int64]bool{}
	for _, m := range erpMenus() {
		if ids[m.MenuID] {
			t.Fatal("duplicate menu ID")
		}
		ids[m.MenuID] = true
		if m.Perms == nil {
			continue
		}
		p := *m.Perms
		if _, ok := wanted[p]; ok {
			wanted[p] = true
		}
		if strings.HasPrefix(p, "erp:supplier:") {
			if _, ok := wanted[p]; !ok {
				t.Fatal("unexpected supplier permission", p)
			}
		}
	}
	for p, found := range wanted {
		if !found {
			t.Fatal("missing", p)
		}
	}
}
