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

// Only the fresh pre009 restored test copy created in this round; never source.
func TestERPSalesFinanceRestoreIntegration(t *testing.T) {
	name := os.Getenv("ERP_SALES_FINANCE_RESTORE_DB")
	if name == "" {
		t.Skip("explicit new recovery copy required")
	}
	if name == TenantDatabaseName || !phase1DatabaseAllowed(name) {
		t.Fatal("explicit disposable recovery namespace required")
	}
	root := os.Getenv("MT_TEST_ROOT")
	cfg, e := config.Load(filepath.Join(root, "configs/multitenant.local.yml"))
	if e != nil {
		t.Fatal("local config")
	}
	d, e := mysql.ParseDSN(cfg.MySQL.DSN)
	if e != nil {
		t.Fatal("DSN")
	}
	h, _, e := net.SplitHostPort(d.Addr)
	if e != nil || d.Net != "tcp" || d.DBName != TenantDatabaseName || (h != "127.0.0.1" && h != "localhost" && h != "::1") {
		t.Fatal("local source only")
	}
	d.DBName = name
	cfg.MySQL.DSN = d.FormatDSN()
	if Init(cfg.MySQL) != nil {
		t.Fatal("restore unavailable")
	}
	defer Close()
	ctx, c := context.WithTimeout(context.Background(), 90*time.Second)
	defer c()
	q := DB(tenantctx.System(ctx))
	s, e := InspectERPSalesFinance(ctx)
	if e != nil || s.InstalledTables != 0 || s.Recorded {
		t.Fatal("fresh pre009 copy required", e)
	}
	t.Run("optional_absent_orphan_rejected", func(t *testing.T) {
		if VerifyERPSalesFinanceSchema(ctx) != nil {
			t.Fatal("absent optional")
		}
		if q.Exec("INSERT INTO erp_schema_migration(version_key,ddl_sha256) VALUES (?,?)", ERPSalesFinanceVersion, ERPSalesFinanceSHA256()).Error != nil {
			t.Fatal("orphan fixture")
		}
		defer q.Session(&gorm.Session{NewDB: true}).Exec("DELETE FROM erp_schema_migration WHERE version_key=?", ERPSalesFinanceVersion)
		if VerifyERPSalesFinanceSchema(ctx) == nil {
			t.Fatal("orphan allowed")
		}
		if _, e := InstallERPSalesFinance(ctx); e == nil {
			t.Fatal("orphan repaired")
		}
	})
	if t.Failed() {
		return
	}
	t.Run("partial_DDL_retained_unrecorded_retry_and_drift", func(t *testing.T) {
		cb := db.Callback().Raw()
		if cb.Before("gorm:raw").Register("sales-finance:create-fault", func(tx *gorm.DB) {
			if strings.HasPrefix(tx.Statement.SQL.String(), "CREATE TABLE erp_sf_credit (") {
				tx.AddError(errors.New("injected create interruption"))
			}
		}) != nil {
			t.Fatal("callback")
		}
		_, installErr := InstallERPSalesFinance(ctx)
		cb.Remove("sales-finance:create-fault")
		if installErr == nil {
			t.Fatal("fault missing")
		}
		s, e := InspectERPSalesFinance(ctx)
		if e != nil || s.InstalledTables != 2 || s.Recorded {
			t.Fatal("partial state", e)
		}
		if VerifyERPSalesFinanceSchema(ctx) == nil {
			t.Fatal("partial accepted")
		}
		if _, e = InstallERPSalesFinance(ctx); e != nil {
			t.Fatal("retry", e)
		}
		if VerifyERPSalesFinanceSchema(ctx) != nil {
			t.Fatal("installed invalid")
		}
		if e = q.Exec("ALTER TABLE erp_sf_receivable MODIFY version BIGINT NULL").Error; e != nil {
			t.Fatal(e)
		}
		if VerifyERPSalesFinanceSchema(ctx) == nil {
			t.Fatal("drift accepted")
		}
		if _, e = InstallERPSalesFinance(ctx); e == nil {
			t.Fatal("drift repaired")
		}
		if q.Exec("ALTER TABLE erp_sf_receivable MODIFY version BIGINT NOT NULL").Error != nil {
			t.Fatal("restore test shape")
		}
		if VerifyERPSalesFinanceSchema(ctx) != nil {
			t.Fatal("restored verifier")
		}
	})
}
