package repository

import (
	"context"
	"errors"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"net"
	"os"
	"path/filepath"
	"ruoyi-go/internal/config"
	"ruoyi-go/pkg/tenantctx"
	"strings"
	"testing"
	"time"
)

// Explicit new verified restore only. Never drop/alter an existing source table.
func TestERPPurchaseReturnRestoreIntegration(t *testing.T) {
	name := os.Getenv("ERP_PURCHASE_RETURN_RESTORE_DB")
	if name == "" {
		t.Skip("explicit verified return restore required")
	}
	if name == TenantDatabaseName || !phase1DatabaseAllowed(name) {
		t.Fatal("restore namespace required")
	}
	root := os.Getenv("MT_TEST_ROOT")
	if root == "" {
		t.Fatal("MT_TEST_ROOT required")
	}
	cfg, e := config.Load(filepath.Join(root, "configs/multitenant.local.yml"))
	if e != nil {
		t.Fatal("local config unavailable")
	}
	d, e := mysql.ParseDSN(cfg.MySQL.DSN)
	if e != nil {
		t.Fatal("invalid DSN")
	}
	h, _, e := net.SplitHostPort(d.Addr)
	if e != nil || d.Net != "tcp" || d.DBName != TenantDatabaseName || (h != "127.0.0.1" && h != "localhost" && h != "::1") {
		t.Fatal("nonlocal source refused")
	}
	d.DBName = name
	d.MultiStatements = false
	cfg.MySQL.DSN = d.FormatDSN()
	if Init(cfg.MySQL) != nil {
		t.Fatal("restore unavailable")
	}
	defer Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	q := DB(tenantctx.System(ctx))
	initial, e := InspectERPPurchaseReturn(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if initial.InstalledTables == 0 && !initial.Recorded {
		t.Run("absent_optional_orphan_rejected", func(t *testing.T) {
			if e := VerifyERPPurchaseReturnSchema(ctx); e != nil {
				t.Fatal(e)
			}
			if e := q.Exec("INSERT INTO erp_schema_migration(version_key,ddl_sha256) VALUES (?,?)", ERPPurchaseReturnVersion, ERPPurchaseReturnSHA256()).Error; e != nil {
				t.Fatal(e)
			}
			defer func() {
				if e := q.Session(&gorm.Session{NewDB: true}).Exec("DELETE FROM erp_schema_migration WHERE version_key=? AND ddl_sha256=?", ERPPurchaseReturnVersion, ERPPurchaseReturnSHA256()).Error; e != nil {
					t.Error(e)
				}
			}()
			if VerifyERPPurchaseReturnSchema(ctx) == nil {
				t.Fatal("orphan accepted")
			}
			if _, e := InstallERPPurchaseReturn(ctx); e == nil {
				t.Fatal("orphan repaired")
			}
		})
		if t.Failed() {
			t.FailNow()
		}
		t.Run("partial_failure_retained_lock_released_retry", func(t *testing.T) {
			const cb = "returns:fail-second-create"
			if e := db.Callback().Raw().Before("gorm:raw").Register(cb, func(tx *gorm.DB) {
				if strings.HasPrefix(tx.Statement.SQL.String(), "CREATE TABLE erp_purchase_return_line (") {
					tx.AddError(errors.New("injected restore CREATE failure"))
				}
			}); e != nil {
				t.Fatal(e)
			}
			_, installErr := InstallERPPurchaseReturn(ctx)
			if e := db.Callback().Raw().Remove(cb); e != nil {
				t.Fatal(e)
			}
			if installErr == nil {
				t.Fatal("injected failure absent")
			}
			s, e := InspectERPPurchaseReturn(ctx)
			if e != nil || s.InstalledTables != 1 || s.Recorded {
				t.Fatal("unexpected partial state", s, e)
			}
			if VerifyERPPurchaseReturnSchema(ctx) == nil {
				t.Fatal("partial startup accepted")
			}
			var free int
			if e := q.Raw("SELECT IS_FREE_LOCK(?)", name+":erp-phase1").Scan(&free).Error; e != nil || free != 1 {
				t.Fatal("lock leaked", e)
			}
			if _, e := InstallERPPurchaseReturn(ctx); e != nil {
				t.Fatal(e)
			}
		})
	}
	if t.Failed() {
		t.FailNow()
	}
	t.Run("idempotency_shared_lock", func(t *testing.T) {
		marker := func() string {
			var s string
			if e := q.Raw("SELECT CONCAT(migration_id,':',ddl_sha256,':',UNIX_TIMESTAMP(create_time)) FROM erp_schema_migration WHERE version_key=?", ERPPurchaseReturnVersion).Scan(&s).Error; e != nil || s == "" {
				t.Fatal("marker missing", e)
			}
			return s
		}
		before := marker()
		if _, e := InstallERPPurchaseReturn(ctx); e != nil {
			t.Fatal(e)
		}
		if before != marker() {
			t.Fatal("marker changed")
		}
		raw, e := db.DB()
		if e != nil {
			t.Fatal(e)
		}
		conn, e := raw.Conn(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer conn.Close()
		var locked int
		if e := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?,0)", name+":erp-phase1").Scan(&locked); e != nil || locked != 1 {
			t.Fatal("lock unavailable")
		}
		defer conn.ExecContext(ctx, "DO RELEASE_LOCK(?)", name+":erp-phase1")
		if _, e := InstallERPPurchaseReturn(ctx); e == nil {
			t.Fatal("competing installer accepted")
		}
	})
	t.Run("drift_rejected_not_repaired", func(t *testing.T) {
		for _, tc := range []struct{ name, alter, repair string }{
			{"column", "ALTER TABLE erp_purchase_return_order MODIFY reason VARCHAR(501) NOT NULL", "ALTER TABLE erp_purchase_return_order MODIFY reason VARCHAR(500) NOT NULL"},
			{"index", "ALTER TABLE erp_purchase_return_order DROP INDEX ix_p5_return_warehouse", "ALTER TABLE erp_purchase_return_order ADD INDEX ix_p5_return_warehouse(tenant_id,warehouse_id,return_id)"},
			{"fk", "ALTER TABLE erp_purchase_return_line DROP FOREIGN KEY fk_p5_line_receipt", "ALTER TABLE erp_purchase_return_line ADD CONSTRAINT fk_p5_line_receipt FOREIGN KEY(tenant_id,receipt_id,purchase_line_id) REFERENCES erp_purchase_receipt_line(tenant_id,receipt_id,purchase_line_id)"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if e := q.Exec(tc.alter).Error; e != nil {
					t.Fatal(e)
				}
				defer func() {
					if e := q.Session(&gorm.Session{NewDB: true}).Exec(tc.repair).Error; e != nil {
						t.Error(e)
					}
				}()
				if VerifyERPPurchaseReturnSchema(ctx) == nil {
					t.Fatal("drift accepted")
				}
				if _, e := InstallERPPurchaseReturn(ctx); e == nil {
					t.Fatal("drift repaired")
				}
			})
		}
	})
	if e := VerifyERPPurchaseReturnSchema(ctx); e != nil {
		t.Fatal(e)
	}
}
