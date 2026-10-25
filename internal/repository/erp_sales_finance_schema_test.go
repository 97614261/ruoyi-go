package repository

import (
	"context"
	"ruoyi-go/pkg/tenantctx"
	"strings"
	"testing"
)

func TestSalesFinanceManifestTenantBoundary(t *testing.T) {
	if ERPSalesFinanceSHA256() != "26d9ed65f10246cb4207bae9617fcd529cdbdec66776eb5f70d083f91cd7449a" {
		t.Fatal("published009 changed")
	}
	tables, e := salesFinanceManifest()
	if e != nil || len(tables) != 6 {
		t.Fatal(e, len(tables))
	}
	for _, table := range tables {
		for name, ref := range table.ForeignKeys {
			if !strings.HasPrefix(ref, "tenant_id,") {
				t.Fatal(name)
			}
			columns := strings.SplitN(ref, ">", 2)[0]
			covered := false
			for _, index := range table.Indexes {
				if index.Columns == columns || strings.HasPrefix(index.Columns, columns+",") {
					covered = true
				}
			}
			if !covered {
				t.Fatalf("foreign key needs explicit supporting index (avoid implicit index drift): %s.%s", table.Name, name)
			}
		}
		q := guardDB(t).WithContext(tenantctx.With(context.Background(), 17, false)).Table(table.Name).Count(new(int64))
		if q.Error != nil || !strings.Contains(q.Statement.SQL.String(), "tenant_id") {
			t.Fatal(table.Name, q.Error)
		}
		if guardDB(t).WithContext(context.Background()).Table(table.Name).Count(new(int64)).Error == nil {
			t.Fatal("missing tenant accepted", table.Name)
		}
	}
}
