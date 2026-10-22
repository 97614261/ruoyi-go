package repository

import (
	"context"
	"errors"
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

// Opt-in ONLY for the operator-created, verified restore namespace. Never uses
// or changes the configured source database and never starts application jobs.
func TestERPPhase1RestoreIntegration(t *testing.T) {
	name := os.Getenv("ERP_PHASE1_RESTORE_DB")
	if name == "" {
		t.Skip("explicit verified restore database required")
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
		t.Fatal("invalid configuration")
	}
	host, _, err := net.SplitHostPort(dsn.Addr)
	if err != nil || dsn.Net != "tcp" || dsn.DBName != TenantDatabaseName || (host != "localhost" && host != "127.0.0.1" && host != "::1") {
		t.Fatal("refusing nonlocal source configuration")
	}
	dsn.DBName = name
	dsn.MultiStatements = false
	cfg.MySQL.DSN = dsn.FormatDSN()
	if err = Init(cfg.MySQL); err != nil {
		t.Fatal(err)
	}
	defer Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	q := DB(tenantctx.System(ctx))
	short, stop := context.WithTimeout(ctx, 5*time.Second)
	err = VerifyERPPhase1Schema(short)
	stop()
	if err != nil {
		t.Fatal(err)
	}
	var before, after string
	marker := func(dest *string) error {
		return q.Raw("SELECT CONCAT(migration_id,':',UNIX_TIMESTAMP(create_time),':',ddl_sha256) FROM erp_schema_migration WHERE version_key=?", ERPPhase1Version).Scan(dest).Error
	}
	if err = marker(&before); err != nil || before == "" {
		t.Fatal("verified complete restore installation required")
	}
	if _, err = InstallERPPhase1(ctx); err != nil {
		t.Fatal(err)
	}
	if err = marker(&after); err != nil || after != before {
		t.Fatal("idempotent installer mutated marker", err)
	}
	t.Run("tenant_foreign_key_and_leading_zero", func(t *testing.T) {
		var tids []int64
		if err = q.Raw("SELECT tenant_id FROM sys_tenant WHERE tenant_kind='01' ORDER BY tenant_id LIMIT 2").Scan(&tids).Error; err != nil || len(tids) != 2 {
			t.Fatal("two existing restored customer references required")
		}
		sentinel := errors.New("rollback fixture")
		err = q.Transaction(func(tx *gorm.DB) error {
			if e := tx.Exec("INSERT INTO erp_shop(tenant_id,shop_code,shop_name,channel_code,site_code,currency) VALUES (?,'p1-regression','test','manual','CN','CNY')", tids[0]).Error; e != nil {
				return e
			}
			var id int64
			if e := tx.Raw("SELECT shop_id FROM erp_shop WHERE tenant_id=? AND shop_code='p1-regression'", tids[0]).Scan(&id).Error; e != nil {
				return e
			}
			bad := tx.Session(&gorm.Session{NewDB: true}).Exec("INSERT INTO erp_shop_item(tenant_id,shop_id,shop_item_code,title) VALUES (?,?,'00001','test')", tids[1], id).Error
			var me *mysql.MySQLError
			if !errors.As(bad, &me) || me.Number != 1452 {
				return errors.New("cross-tenant reference accepted")
			}
			if e := tx.Exec("INSERT INTO erp_shop_item(tenant_id,shop_id,shop_item_code,title) VALUES (?,?,'00001','test')", tids[0], id).Error; e != nil {
				return e
			}
			var code string
			if e := tx.Raw("SELECT shop_item_code FROM erp_shop_item WHERE tenant_id=? AND shop_id=?", tids[0], id).Scan(&code).Error; e != nil {
				return e
			}
			if code != "00001" {
				return errors.New("external code lost leading zeros")
			}
			return sentinel
		})
		if !errors.Is(err, sentinel) {
			t.Fatal(err)
		}
	})
	t.Run("startup_rejects_drift_and_never_repairs", func(t *testing.T) {
		if err = q.Exec("ALTER TABLE erp_shop MODIFY shop_code VARCHAR(65) CHARACTER SET ascii COLLATE ascii_bin NOT NULL").Error; err != nil {
			t.Fatal(err)
		}
		repair := func() error {
			return q.Session(&gorm.Session{NewDB: true}).Exec("ALTER TABLE erp_shop MODIFY shop_code VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL").Error
		}
		defer func() {
			if e := repair(); e != nil {
				t.Error("restore-only drift cleanup failed", e)
			}
		}()
		if e := VerifyERPPhase1Schema(ctx); e == nil || !strings.Contains(e.Error(), "漂移") {
			t.Fatal("column drift not rejected", e)
		}
		if _, e := InstallERPPhase1(ctx); e == nil {
			t.Fatal("installer silently accepted/repaired drift")
		}
	})
	if err = VerifyERPPhase1Schema(ctx); err != nil {
		t.Fatal(err)
	}
}
