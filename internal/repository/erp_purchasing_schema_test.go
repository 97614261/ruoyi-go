package repository

import (
	"context"
	"strings"
	"testing"

	"ruoyi-go/pkg/tenantctx"
	erpsql "ruoyi-go/sql/erp"
)

func TestPurchasingCreateManifestAndFrozenPhase1(t *testing.T) {
	if ERPPhase1SHA256() != "ebc324c78ec32f39df3e0b36a4fee276bf86ff827dd1dc65f20fa880018db2eb" {
		t.Fatal("applied 003 resource must not change")
	}
	if ERPPurchasingSHA256() != "cba3c0146ca5161406e41af859c5eb95b664b0b4544414109619b6310a8533bd" {
		t.Fatal("applied 004 resource must not change; create a new version instead")
	}
	tables, err := purchasingManifest()
	if err != nil || len(tables) != 6 {
		t.Fatal("six independent purchasing tables required", err)
	}
	for _, table := range tables {
		cols := map[string]phase1Column{}
		for _, c := range table.Columns {
			cols[c.Name] = c
		}
		if c := cols["tenant_id"]; c.Type != "bigint" || c.Nullable || c.Default != nil {
			t.Fatal("invalid tenant field", table.Name)
		}
		for _, name := range []string{"create_by", "create_time", "update_by", "update_time", "remark"} {
			if _, ok := cols[name]; !ok {
				t.Fatal("missing audit field", table.Name, name)
			}
		}
		pk, ok := table.Indexes["PRIMARY"]
		if !ok || !pk.Unique || !cols[pk.Columns].Auto {
			t.Fatal("auto primary key required", table.Name)
		}
		for fk, ref := range table.ForeignKeys {
			left := strings.SplitN(ref, ">", 2)[0]
			if !strings.HasPrefix(left, "tenant_id") {
				t.Fatal("unscoped FK", fk)
			}
			covered := false
			for _, index := range table.Indexes {
				if index.Columns == left || strings.HasPrefix(index.Columns, left+",") {
					covered = true
				}
			}
			if !covered {
				t.Fatal("undeclared implicit FK index", table.Name, fk)
			}
		}
		for _, name := range []string{"ordered_amount", "unit_price", "line_amount"} {
			if c, exists := cols[name]; exists && (c.Type != "decimal(20,4)" || c.Nullable || c.Default != nil) {
				t.Fatal("amount drift", name)
			}
		}
	}
	for _, script := range []string{
		erpsql.Purchasing + "\nALTER TABLE sys_user ADD bad INT;",
		erpsql.Purchasing + "\nDROP TABLE erp_supplier;",
		erpsql.Purchasing + erpsql.Purchasing,
		strings.Replace(erpsql.Purchasing, "CREATE TABLE erp_supplier (", "CREATE TABLE erp_shop (", 1),
	} {
		if _, err := parseERPCreateManifest(script, purchasingBusinessTables); err == nil {
			t.Fatal("unsafe/duplicate/foreign CREATE accepted")
		}
	}
	if _, err := parsePhase1Manifest(erpsql.Purchasing); err == nil {
		t.Fatal("004 was accepted as 003")
	}
	if _, err := parseERPCreateManifest(erpsql.Phase1, purchasingBusinessTables); err == nil {
		t.Fatal("003 was accepted as 004")
	}
	for _, table := range tables {
		if table.Name == "erp_purchase_receipt_line" {
			if table.ForeignKeys["fk_p4_receipt_line_purchase"] != "tenant_id,purchase_id,purchase_line_id,sku_id>erp_purchase_order_line(tenant_id,purchase_id,line_id,sku_id)" {
				t.Fatal("wrong purchase/SKU boundary")
			}
			if table.ForeignKeys["fk_p4_receipt_line_receipt"] != "tenant_id,purchase_id,receipt_id>erp_purchase_receipt(tenant_id,purchase_id,receipt_id)" {
				t.Fatal("wrong receipt boundary")
			}
		}
	}
}

func TestPurchasingTablesAlwaysUseTenantGuard(t *testing.T) {
	for _, name := range purchasingBusinessTables {
		for _, tid := range []int64{17, 23} {
			q := guardDB(t).WithContext(tenantctx.With(context.Background(), tid, false)).Table(name).Count(new(int64))
			if q.Error != nil || !strings.Contains(q.Statement.SQL.String(), "tenant_id") {
				t.Fatal(name, q.Error)
			}
		}
		if q := guardDB(t).WithContext(context.Background()).Table(name).Count(new(int64)); q.Error == nil {
			t.Fatal("missing tenant accepted", name)
		}
	}
	names := ERPPurchasingTableNames()
	names[0] = "changed"
	if ERPPurchasingTableNames()[0] == "changed" {
		t.Fatal("exposed mutable manifest")
	}
}
