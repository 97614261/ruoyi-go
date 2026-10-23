package repository

import (
	"context"
	"ruoyi-go/pkg/tenantctx"
	erpsql "ruoyi-go/sql/erp"
	"strings"
	"testing"
)

func TestPurchaseReturnManifestAndTenantGuard(t *testing.T) {
	if ERPPurchaseReturnSHA256() != "8b94fd34e75644cf7d1e3c1fe2fedc9e580ccb0c7c69c49857fa3f9b86a10781" {
		t.Fatal("applied005 changed; use a new version")
	}
	if ERPPurchasingSHA256() != "cba3c0146ca5161406e41af859c5eb95b664b0b4544414109619b6310a8533bd" {
		t.Fatal("applied004 changed")
	}
	tables, e := purchaseReturnManifest()
	if e != nil || len(tables) != 2 {
		t.Fatal("two independent return tables required", e)
	}
	for _, table := range tables {
		columns := map[string]phase1Column{}
		for _, c := range table.Columns {
			columns[c.Name] = c
		}
		for _, name := range []string{"tenant_id", "create_by", "create_time", "update_by", "update_time", "remark"} {
			if _, ok := columns[name]; !ok {
				t.Fatal("missing audit/tenant", table.Name, name)
			}
		}
		for name, ref := range table.ForeignKeys {
			left := strings.SplitN(ref, ">", 2)[0]
			if !strings.HasPrefix(left, "tenant_id,") {
				t.Fatal("unscoped FK", name)
			}
			covered := false
			for _, ix := range table.Indexes {
				covered = covered || ix.Columns == left || strings.HasPrefix(ix.Columns, left+",")
			}
			if !covered {
				t.Fatal("implicit FK index", name)
			}
		}
		q := guardDB(t).WithContext(tenantctx.With(context.Background(), 17, false)).Table(table.Name).Count(new(int64))
		if q.Error != nil || !strings.Contains(q.Statement.SQL.String(), "tenant_id") {
			t.Fatal("missing tenant SQL", q.Error)
		}
		if q = guardDB(t).WithContext(context.Background()).Table(table.Name).Count(new(int64)); q.Error == nil {
			t.Fatal("missing tenant allowed")
		}
	}
	for _, script := range []string{erpsql.PurchaseReturns + "\nDROP TABLE erp_purchase_return_order;", erpsql.PurchaseReturns + erpsql.PurchaseReturns, erpsql.Purchasing} {
		if _, e := parseERPCreateManifest(script, purchaseReturnBusinessTables); e == nil {
			t.Fatal("unsafe manifest accepted")
		}
	}
	names := ERPPurchaseReturnTableNames()
	names[0] = "mutated"
	if ERPPurchaseReturnTableNames()[0] == "mutated" {
		t.Fatal("mutable whitelist")
	}
}
