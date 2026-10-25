package repository

import (
	"context"
	"ruoyi-go/pkg/tenantctx"
	"strings"
	"testing"
)

func TestCatalogManifestBoundary(t *testing.T) {
	if ERPCatalogSHA256() != "62534663d51de9422cb60654276fa638c3f17dc690e3f98a6d8c98db7b1b2687" {
		t.Fatal("reviewed010 changed; publish a new version instead")
	}
	tables, e := catalogManifest()
	if e != nil || len(tables) != 5 {
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
