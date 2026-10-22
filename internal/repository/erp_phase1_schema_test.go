package repository

import (
	"context"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"ruoyi-go/pkg/tenantctx"
	erpsql "ruoyi-go/sql/erp"
	"strings"
	"testing"
)

func TestERPPhase1ManifestIsCreateOnlyAndScoped(t *testing.T) {
	tables, err := phase1Manifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 13 {
		t.Fatal("expected 12 business tables and technical migration metadata")
	}
	for _, table := range tables {
		cols := map[string]phase1Column{}
		for _, c := range table.Columns {
			cols[c.Name] = c
		}
		for _, name := range []string{"create_by", "create_time", "update_by", "update_time", "remark"} {
			if _, ok := cols[name]; !ok {
				t.Fatal("missing audit field", table.Name, name)
			}
		}
		if table.Name != "erp_schema_migration" {
			if c, ok := cols["tenant_id"]; !ok || c.Nullable || c.Default != nil || c.Type != "bigint" {
				t.Fatal("invalid tenant boundary", table.Name)
			}
		}
		if _, ok := table.Indexes["PRIMARY"]; !ok {
			t.Fatal("missing primary key")
		}
		for name, reference := range table.ForeignKeys {
			left := strings.SplitN(reference, ">", 2)[0]
			covered := false
			for _, index := range table.Indexes {
				if index.Columns == left || strings.HasPrefix(index.Columns, left+",") {
					covered = true
				}
			}
			if !covered {
				t.Fatal("foreign key would create an implicit index not in the manifest", table.Name, name)
			}
		}
	}
	for _, script := range []string{erpsql.Phase1 + "\nDROP TABLE sys_user;", strings.Replace(erpsql.Phase1, "CREATE TABLE erp_shop (", "CREATE TABLE sys_user (", 1), erpsql.Phase1 + "\n" + erpsql.Phase1} {
		if _, err := parsePhase1Manifest(script); err == nil {
			t.Fatal("unsafe/incomplete/duplicate script accepted")
		}
	}
}
func TestERPPhase1ColumnDriftComparison(t *testing.T) {
	zero := "0"
	coll := "ascii_bin"
	want := phase1Column{Name: "id", Type: "bigint", Auto: true}
	got := phase1ActualColumn{ColumnName: "id", ColumnType: "bigint(20)", IsNullable: "NO", Extra: "auto_increment"}
	if !equalPhase1Column(want, got) {
		t.Fatal("integer display width should be equivalent")
	}
	got.ColumnType = "bigint(20) unsigned"
	if equalPhase1Column(want, got) {
		t.Fatal("unsigned drift accepted")
	}
	got.ColumnType = "bigint(20)"
	got.ColumnDefault = &zero
	if equalPhase1Column(want, got) {
		t.Fatal("default drift accepted")
	}
	got.ColumnDefault = nil
	got.IsNullable = "YES"
	if equalPhase1Column(want, got) {
		t.Fatal("nullable drift accepted")
	}
	want = phase1Column{Name: "code", Type: "varchar(64)", Collation: coll}
	got = phase1ActualColumn{ColumnName: "code", ColumnType: "varchar(64)", IsNullable: "NO", CollationName: &coll}
	if !equalPhase1Column(want, got) {
		t.Fatal("correct collation rejected")
	}
	got.CollationName = nil
	if equalPhase1Column(want, got) {
		t.Fatal("collation drift accepted")
	}
}
func TestERPPhase1NewTablesUseTenantGuard(t *testing.T) {
	for _, name := range phase1BusinessTables {
		q := guardDB(t).WithContext(tenantctx.With(context.Background(), 17, false)).Table(name).Where("id=1").Count(new(int64))
		if q.Error != nil || !strings.Contains(q.Statement.SQL.String(), "tenant_id") {
			t.Fatal("unscoped new table", name, q.Error)
		}
	}
	if err := TenantTable(guardDB(t), "erp_schema_migration").Error; err == nil {
		t.Fatal("technical metadata accepted as generated business table")
	}
}
func TestERPPhase1TargetNamespace(t *testing.T) {
	for _, name := range []string{"ry", "mysql", "ruoyi_go_mt2", "ruoyi_go_mt_restore_", strings.Repeat("x", 80)} {
		if phase1DatabaseAllowed(name) {
			t.Fatal("unsafe database accepted")
		}
	}
	if !phase1DatabaseAllowed(TenantDatabaseName) || !phase1DatabaseAllowed("ruoyi_go_mt_restore_test_12345678") {
		t.Fatal("valid test target refused")
	}
}

func TestERPPhase1ConnectionGuardBeforeAnySQL(t *testing.T) {
	for _, tc := range []struct {
		dsn     string
		allowed bool
	}{
		{"u@tcp(127.0.0.1:3306)/ruoyi_go_mt", true},
		{"u@tcp(localhost:3306)/ruoyi_go_mt_restore_test_12345678", true},
		{"u@tcp(192.0.2.1:3306)/ruoyi_go_mt", false},
		{"u@tcp(127.0.0.1:3306)/ry", false},
		{"u@unix(/mysql.sock)/ruoyi_go_mt", false},
	} {
		g, err := gorm.Open(mysql.New(mysql.Config{DSN: tc.dsn, SkipInitializeWithVersion: true}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
		if err != nil {
			t.Fatal(err)
		}
		if (phase1LocalConnection(g) == nil) != tc.allowed {
			t.Fatal("unexpected connection guard result", tc.allowed)
		}
	}
}
