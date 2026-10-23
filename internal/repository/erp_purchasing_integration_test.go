package repository

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"ruoyi-go/internal/config"
	"ruoyi-go/pkg/tenantctx"
)

// Explicit verified restore ONLY. This test creates 004 there, rolls back FK
// fixtures and temporarily injects/repairs drift only on its own new tables.
// No original/source DSN, application jobs, Redis, passwords or grants are used.
func TestERPPurchasingRestoreIntegration(t *testing.T) {
	name := os.Getenv("ERP_PURCHASING_RESTORE_DB")
	if name == "" {
		t.Skip("explicit verified purchasing restore database required")
	}
	if name == TenantDatabaseName || !phase1DatabaseAllowed(name) {
		t.Fatal("restore namespace required")
	}
	root := os.Getenv("MT_TEST_ROOT")
	if root == "" {
		t.Fatal("MT_TEST_ROOT required")
	}
	cfg, err := config.Load(filepath.Join(root, "configs/multitenant.local.yml"))
	if err != nil {
		t.Fatal("local configuration unavailable")
	}
	dsn, err := mysql.ParseDSN(cfg.MySQL.DSN)
	if err != nil {
		t.Fatal("invalid local DSN")
	}
	host, _, err := net.SplitHostPort(dsn.Addr)
	if err != nil || dsn.Net != "tcp" || dsn.DBName != TenantDatabaseName || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		t.Fatal("nonlocal source configuration refused")
	}
	dsn.DBName = name
	dsn.MultiStatements = false
	cfg.MySQL.DSN = dsn.FormatDSN()
	if err := Init(cfg.MySQL); err != nil {
		t.Fatal("restore connection unavailable")
	}
	defer Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	q := DB(tenantctx.System(ctx))
	initial, err := InspectERPPurchasing(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if initial.InstalledTables == 0 && !initial.Recorded {
		t.Run("absent_optional_orphan_record_rejected", func(t *testing.T) {
			if err := VerifyERPPurchasingSchema(ctx); err != nil {
				t.Fatal(err)
			}
			if err := q.Exec("INSERT INTO erp_schema_migration(version_key,ddl_sha256) VALUES (?,?)", ERPPurchasingVersion, ERPPurchasingSHA256()).Error; err != nil {
				t.Fatal(err)
			}
			defer func() {
				if e := q.Session(&gorm.Session{NewDB: true}).Exec("DELETE FROM erp_schema_migration WHERE version_key=? AND ddl_sha256=?", ERPPurchasingVersion, ERPPurchasingSHA256()).Error; e != nil {
					t.Error(e)
				}
			}()
			if VerifyERPPurchasingSchema(ctx) == nil {
				t.Fatal("orphan marker bypassed startup")
			}
			if _, err := InstallERPPurchasing(ctx); err == nil {
				t.Fatal("published missing tables silently recreated")
			}
		})
		if t.Failed() {
			t.FailNow()
		}
		t.Run("partial_failure_retained_lock_released_retry", func(t *testing.T) {
			const callback = "purchasing:test-fail-third-create"
			if err := db.Callback().Raw().Before("gorm:raw").Register(callback, func(tx *gorm.DB) {
				if strings.HasPrefix(tx.Statement.SQL.String(), "CREATE TABLE erp_purchase_order_line (") {
					tx.AddError(errors.New("injected restore-only CREATE failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			_, installErr := InstallERPPurchasing(ctx)
			if err := db.Callback().Raw().Remove(callback); err != nil {
				t.Fatal(err)
			}
			if installErr == nil {
				t.Fatal("injected failure not observed")
			}
			s, err := InspectERPPurchasing(ctx)
			if err != nil || s.InstalledTables != 2 || s.Recorded {
				t.Fatal("unexpected partial state", s, err)
			}
			if VerifyERPPurchasingSchema(ctx) == nil {
				t.Fatal("partial startup accepted")
			}
			var free int
			if err := q.Raw("SELECT IS_FREE_LOCK(?)", name+":erp-phase1").Scan(&free).Error; err != nil || free != 1 {
				t.Fatal("failed CREATE leaked migration lock", err)
			}
			if _, err := InstallERPPurchasing(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
	if t.Failed() {
		t.FailNow()
	}
	t.Run("complete_idempotency_and_shared_lock", func(t *testing.T) {
		if _, err := InstallERPPurchasing(ctx); err != nil {
			t.Fatal(err)
		}
		marker := func() string {
			var value string
			if err := q.Raw("SELECT CONCAT(migration_id,':',UNIX_TIMESTAMP(create_time),':',ddl_sha256) FROM erp_schema_migration WHERE version_key=?", ERPPurchasingVersion).Scan(&value).Error; err != nil || value == "" {
				t.Fatal("missing marker", err)
			}
			return value
		}
		before := marker()
		if _, err := InstallERPPurchasing(ctx); err != nil {
			t.Fatal(err)
		}
		if marker() != before {
			t.Fatal("repeat changed marker")
		}
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		conn, err := sqlDB.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		var locked int
		if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?,0)", name+":erp-phase1").Scan(&locked); err != nil || locked != 1 {
			t.Fatal("lock unavailable", err)
		}
		defer conn.ExecContext(ctx, "DO RELEASE_LOCK(?)", name+":erp-phase1")
		if _, err := InstallERPPurchasing(ctx); err == nil {
			t.Fatal("concurrent migration accepted")
		}
	})
	t.Run("composite_foreign_keys_precision_unique", func(t *testing.T) { purchasingRestoreFKs(t, q) })
	t.Run("drift_rejected_not_repaired", func(t *testing.T) {
		for _, tc := range []struct{ name, alter, repair string }{
			{"column", "ALTER TABLE erp_supplier MODIFY supplier_code VARCHAR(65) COLLATE utf8mb4_bin NOT NULL", "ALTER TABLE erp_supplier MODIFY supplier_code VARCHAR(64) COLLATE utf8mb4_bin NOT NULL"},
			{"index", "ALTER TABLE erp_purchase_order DROP INDEX ix_p4_purchase_status", "ALTER TABLE erp_purchase_order ADD INDEX ix_p4_purchase_status (tenant_id,status,purchase_id)"},
			{"foreign_key", "ALTER TABLE erp_purchase_event DROP FOREIGN KEY fk_p4_event_receipt", "ALTER TABLE erp_purchase_event ADD CONSTRAINT fk_p4_event_receipt FOREIGN KEY (tenant_id,purchase_id,receipt_id) REFERENCES erp_purchase_receipt(tenant_id,purchase_id,receipt_id)"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if err := q.Exec(tc.alter).Error; err != nil {
					t.Fatal(err)
				}
				defer func() {
					if e := q.Session(&gorm.Session{NewDB: true}).Exec(tc.repair).Error; e != nil {
						t.Error("restore-only repair failed", e)
					}
				}()
				if VerifyERPPurchasingSchema(ctx) == nil {
					t.Fatal("drift accepted")
				}
				if _, err := InstallERPPurchasing(ctx); err == nil {
					t.Fatal("installer accepted or repaired drift")
				}
			})
			if t.Failed() {
				t.FailNow()
			}
			if err := VerifyERPPurchasingSchema(ctx); err != nil {
				t.Fatal(err)
			}
		}
	})
	if err := VerifyERPPhase1Schema(ctx); err != nil {
		t.Fatal("003 changed", err)
	}
	if err := VerifyERPPurchasingSchema(ctx); err != nil {
		t.Fatal(err)
	}
}

func purchasingRestoreFKs(t *testing.T, q *gorm.DB) {
	t.Helper()
	var tenants []struct{ TenantID, OwnerUserID int64 }
	if err := q.Raw("SELECT tenant_id,owner_user_id FROM sys_tenant WHERE tenant_kind='01' AND owner_user_id IS NOT NULL ORDER BY tenant_id LIMIT 2").Scan(&tenants).Error; err != nil || len(tenants) != 2 {
		t.Fatal("two restored customers required", err)
	}
	a, b := tenants[0], tenants[1]
	sentinel := errors.New("rollback only purchasing FK fixtures")
	err := q.Transaction(func(tx *gorm.DB) error {
		insert := func(sql string, args ...any) int64 {
			t.Helper()
			if err := tx.Session(&gorm.Session{NewDB: true}).Exec(sql, args...).Error; err != nil {
				t.Fatal("fixture insert failed", err)
			}
			var id int64
			if err := tx.Raw("SELECT LAST_INSERT_ID()").Scan(&id).Error; err != nil {
				t.Fatal(err)
			}
			return id
		}
		expect := func(number uint16, sql string, args ...any) {
			t.Helper()
			err := tx.Session(&gorm.Session{NewDB: true}).Exec(sql, args...).Error
			var me *mysql.MySQLError
			if !errors.As(err, &me) || me.Number != number {
				t.Fatalf("expected constraint %d, got %v", number, err)
			}
		}
		supplierSQL := "INSERT INTO erp_supplier(tenant_id,supplier_code,supplier_name) VALUES (?,'00001','restore test')"
		sa := insert(supplierSQL, a.TenantID)
		sb := insert(supplierSQL, b.TenantID)
		whSQL := "INSERT INTO erp_warehouse(tenant_id,warehouse_code,warehouse_name,status) VALUES (?,?,'test','0')"
		wa := insert(whSQL, a.TenantID, "p4-test-a")
		wa2 := insert(whSQL, a.TenantID, "p4-test-a2")
		wb := insert(whSQL, b.TenantID, "p4-test-b")
		skuSQL := "INSERT INTO erp_sku(tenant_id,sku_code,product_name,specification,unit,status) VALUES (?,?,'test','','piece','0')"
		ska := insert(skuSQL, a.TenantID, "p4-test-a")
		ska2 := insert(skuSQL, a.TenantID, "p4-test-a2")
		skb := insert(skuSQL, b.TenantID, "p4-test-b")
		poSQL := "INSERT INTO erp_purchase_order(tenant_id,purchase_no,supplier_id,warehouse_id,supplier_code,supplier_name,warehouse_name,currency,create_request_key,create_request_hash,ordered_amount) VALUES (?,?,?,?,'00001','test','test','CNY',?,REPEAT('a',64),'12.3400')"
		pa := insert(poSQL, a.TenantID, "00001", sa, wa, "p4-a")
		pa2 := insert(poSQL, a.TenantID, "00002", sa, wa, "p4-a2")
		pb := insert(poSQL, b.TenantID, "00001", sb, wb, "p4-b")
		expect(1452, poSQL, a.TenantID, "bad-supplier", sb, wa, "bad-supplier")
		expect(1452, poSQL, a.TenantID, "bad-warehouse", sa, wb, "bad-warehouse")
		expect(1062, poSQL, a.TenantID, "different-number", sa, wa, "p4-a")
		lineSQL := "INSERT INTO erp_purchase_order_line(tenant_id,purchase_id,line_no,sku_id,sku_code,product_name,specification,unit,ordered_quantity,unit_price,line_amount) VALUES (?,?,1,?,'00001','test','','piece',1,'12.3400','12.3400')"
		la := insert(lineSQL, a.TenantID, pa, ska)
		la2 := insert(lineSQL, a.TenantID, pa2, ska)
		_ = insert(lineSQL, b.TenantID, pb, skb)
		expect(1452, strings.Replace(lineSQL, "VALUES (?,?,1,?", "VALUES (?,?,2,?", 1), a.TenantID, pa, skb)
		docSQL := "INSERT INTO erp_document(tenant_id,document_no,kind,warehouse_id,warehouse_name,status,request_key,request_hash) VALUES (?,?,'IN',?,'test','POSTED',?,REPEAT('a',64))"
		da := insert(docSQL, a.TenantID, "p4-doc-a", wa, "p4-doc-a")
		da2 := insert(docSQL, a.TenantID, "p4-doc-a2", wa, "p4-doc-a2")
		dbb := insert(docSQL, b.TenantID, "p4-doc-b", wb, "p4-doc-b")
		rSQL := "INSERT INTO erp_purchase_receipt(tenant_id,receipt_no,purchase_id,warehouse_id,stock_document_id,actor_user_id,request_key,request_hash,posted_at) VALUES (?,?,?,?,?,?,?,REPEAT('a',64),NOW())"
		ra := insert(rSQL, a.TenantID, "p4-r-a", pa, wa, da, a.OwnerUserID, "p4-r-a")
		expect(1452, rSQL, a.TenantID, "wrong-wh", pa, wa2, da2, a.OwnerUserID, "wrong-wh")
		expect(1452, rSQL, a.TenantID, "foreign-doc", pa, wa, dbb, a.OwnerUserID, "foreign-doc")
		expect(1452, rSQL, a.TenantID, "foreign-actor", pa, wa, da2, b.OwnerUserID, "foreign-actor")
		expect(1062, rSQL, a.TenantID, "duplicate-doc", pa, wa, da, a.OwnerUserID, "duplicate-doc")
		rlSQL := "INSERT INTO erp_purchase_receipt_line(tenant_id,receipt_id,purchase_id,purchase_line_id,sku_id,quantity,sku_code,product_name,specification,unit) VALUES (?,?,?,?,?,1,'00001','test','','piece')"
		expect(1452, rlSQL, a.TenantID, ra, pa, la2, ska)
		expect(1452, rlSQL, a.TenantID, ra, pa2, la2, ska)
		expect(1452, rlSQL, a.TenantID, ra, pa, la, ska2)
		_ = insert(rlSQL, a.TenantID, ra, pa, la, ska)
		eSQL := "INSERT INTO erp_purchase_event(tenant_id,purchase_id,receipt_id,actor_user_id,action,request_key,request_hash,from_status,to_status,from_version,to_version,result_json) VALUES (?,?,?,?,'RECEIVE',?,REPEAT('a',64),'CONFIRMED','COMPLETED',1,2,'{}')"
		_ = insert(eSQL, a.TenantID, pa, ra, a.OwnerUserID, "p4-e")
		expect(1452, eSQL, a.TenantID, pa2, ra, a.OwnerUserID, "wrong-receipt")
		expect(1452, eSQL, a.TenantID, pa, ra, b.OwnerUserID, "wrong-actor")
		expect(1062, eSQL, a.TenantID, pa, ra, a.OwnerUserID, "p4-e")
		var saved struct{ PurchaseNo, Amount string }
		if err := tx.Raw("SELECT purchase_no,CAST(ordered_amount AS CHAR) AS amount FROM erp_purchase_order WHERE purchase_id=?", pa).Scan(&saved).Error; err != nil || saved.PurchaseNo != "00001" || saved.Amount != "12.3400" {
			return fmt.Errorf("exact text/decimal not retained")
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
}
