package repository

import (
	"context"
	"ruoyi-go/pkg/tenantctx"
	"strings"
	"testing"
)

func TestLogisticsManifestBoundary(t *testing.T) {
	if ERPLogisticsSHA256() != "834668e64be8601122f07725b28d6139bf140c124dcf267a2e6fd15a0511614b" {
		t.Fatal("reviewed011 changed; publish a new version instead")
	}
	tables, e := logisticsManifest()
	if e != nil || len(tables) != 4 {
		t.Fatal(e, len(tables))
	}
	textChecked := false
	for _, table := range tables {
		for _, c := range table.Columns {
			if c.Name == "result_json" {
				textChecked = c.Type == "text" && c.Collation == "utf8mb4_unicode_ci"
			}
		}
		for name, ref := range table.ForeignKeys {
			left := strings.SplitN(ref, ">", 2)[0]
			if left != "tenant_id" && !strings.HasPrefix(left, "tenant_id,") {
				t.Fatal(name)
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
			t.Fatal(table.Name, q.Error)
		}
		if guardDB(t).Table(table.Name).Count(new(int64)).Error == nil {
			t.Fatal("unscoped read", table.Name)
		}
	}
	if !textChecked {
		t.Fatal("TEXT collation not parsed")
	}
}
