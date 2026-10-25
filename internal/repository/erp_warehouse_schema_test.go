package repository

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/tenantctx"
	"strings"
	"testing"
)

func TestWarehouseManifestAndScopes(t *testing.T) {
	if ERPWarehouseSHA256() != "a3301879926643a297cf4626c3ecc27382ebd5578a5147e5b460bfe5b03a0a7f" {
		t.Fatal("published007 changed")
	}
	tables, e := warehouseManifest()
	if e != nil || len(tables) != 5 {
		t.Fatal(e)
	}
	for _, table := range tables {
		for name, ref := range table.ForeignKeys {
			if !strings.HasPrefix(ref, "tenant_id,") {
				t.Fatal(name)
			}
		}
		q := guardDB(t).WithContext(tenantctx.With(context.Background(), 17, false)).Table(table.Name).Count(new(int64))
		if q.Error != nil || !strings.Contains(q.Statement.SQL.String(), "tenant_id") {
			t.Fatal("guard", q.Error)
		}
		if guardDB(t).WithContext(context.Background()).Table(table.Name).Count(new(int64)).Error == nil {
			t.Fatal("missing context allowed")
		}
	}
	q := transferVisible(guardDB(t).WithContext(erpctx.WithUser(tenantctx.With(context.Background(), 17, false), 23)).Model(&model.ERPTransfer{})).Find(&[]model.ERPTransfer{})
	if q.Error != nil || !strings.Contains(q.Statement.SQL.String(), "target_warehouse_id") {
		t.Fatal(q.Error)
	}
	if transferVisible(guardDB(t).Model(&model.ERPTransfer{})).Find(&[]model.ERPTransfer{}).Error == nil {
		t.Fatal("missing ERP identity")
	}
	for _, event := range []string{"TIN", "TOUT"} {
		h, r, e := erpApplyBalance(10, 7, 3, event)
		if e != nil || r != 7 || (event == "TIN" && h != 13) || (event == "TOUT" && h != 7) {
			t.Fatal(event, h, r, e)
		}
	}
	if _, _, e := erpApplyBalance(10, 7, 4, "TOUT"); e == nil {
		t.Fatal("stole reserved")
	}
}
