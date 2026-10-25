package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"net"
	"os"
	"path/filepath"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/tenantctx"
	"testing"
	"time"
)

// Diagnostic only: current-run receipt, exact disabled synthetic enterprise,
// fixed generated SQL, no DDL or business mutation. Default unit runs skip it.
func TestERPEnablementReadProbe(t *testing.T) {
	if os.Getenv("ERP_SETUP_READ_PROBE") != "1" {
		t.Skip("explicit owned local diagnostic only")
	}
	root := os.Getenv("MT_TEST_ROOT")
	dir := filepath.Join(root, "logs/erp-enablement-20261025")
	var f struct {
		ID    int64  `json:"tenantId"`
		Code  string `json:"tenantCode"`
		Owner int64  `json:"ownerUserId"`
		Test  string `json:"test"`
	}
	raw, e := os.ReadFile(filepath.Join(dir, "latest-fixtures.json"))
	if e != nil || json.Unmarshal(raw, &f) != nil || f.Test != "TestERPEnablement" || f.ID < 1 || f.Owner < 1 {
		t.Fatal("owned receipt required")
	}
	var b struct {
		Watermark int64 `json:"watermarkTenantId"`
	}
	raw, e = os.ReadFile(filepath.Join(dir, "pre-round-audit.json"))
	if e != nil || json.Unmarshal(raw, &b) != nil || f.ID <= b.Watermark {
		t.Fatal("old fixture refused")
	}
	cfg, e := config.Load(filepath.Join(root, "configs/multitenant.local.yml"))
	if e != nil {
		t.Fatal("config")
	}
	dsn, e := mysql.ParseDSN(cfg.MySQL.DSN)
	if e != nil {
		t.Fatal("DSN")
	}
	host, _, e := net.SplitHostPort(dsn.Addr)
	if e != nil || dsn.DBName != TenantDatabaseName || (host != "localhost" && host != "127.0.0.1") {
		t.Fatal("dedicated local database only")
	}
	if Init(cfg.MySQL) != nil {
		t.Fatal("repository")
	}
	defer Close()
	ctx, c := context.WithTimeout(context.Background(), 25*time.Second)
	defer c()
	var tenant model.SysTenant
	if DB(tenantctx.System(ctx)).Where("tenant_id=?", f.ID).Take(&tenant).Error != nil || tenant.Status != "1" || tenant.TenantCode != f.Code || tenant.OwnerUserID == nil || *tenant.OwnerUserID != f.Owner {
		t.Fatal("exact disabled fixture required")
	}
	ctx = erpctx.WithUser(tenantctx.With(ctx, f.ID, false), f.Owner)
	allowed := map[string]bool{}
	for _, s := range setupSpecs() {
		allowed[s.permission] = true
	}
	capture := &overviewProbeLogger{enabled: true}
	db = db.Session(&gorm.Session{Logger: capture})
	if _, e := SelectERPSetup(ctx, allowed); e != nil {
		t.Fatal("setup query")
	}
	capture.enabled = false
	plans := []map[string]any{}
	for _, q := range capture.queries {
		rows, e := DB(tenantctx.System(ctx)).Raw("EXPLAIN " + q).Rows()
		if e != nil {
			t.Fatal("EXPLAIN fixed captured SQL")
		}
		cols, e := rows.Columns()
		if e != nil {
			t.Fatal(e)
		}
		detail := []map[string]string{}
		for rows.Next() {
			v := make([]any, len(cols))
			p := make([]any, len(cols))
			for i := range v {
				p[i] = &v[i]
			}
			if rows.Scan(p...) != nil {
				t.Fatal("scan")
			}
			r := map[string]string{}
			for i, k := range cols {
				if b, ok := v[i].([]byte); ok {
					r[k] = string(b)
				} else {
					r[k] = fmt.Sprint(v[i])
				}
			}
			detail = append(detail, r)
		}
		if rows.Err() != nil {
			t.Fatal("rows")
		}
		rows.Close()
		plans = append(plans, map[string]any{"sql": q, "plan": detail})
	}
	if len(plans) != 2 {
		t.Fatalf("expected two fixed queries, got %d", len(plans))
	}
	stopped, cancel := context.WithCancel(ctx)
	cancel()
	if _, e := SelectERPSetup(stopped, allowed); e == nil {
		t.Fatal("DB cancellation fabricated ready")
	}
	out := map[string]any{"atUtc": time.Now().UTC(), "tenantId": f.ID, "readOnly": true, "databaseErrorNotReady": true, "plans": plans, "capacityClaim": false}
	raw, e = json.MarshalIndent(out, "", "  ")
	if e != nil || os.WriteFile(filepath.Join(dir, "setup-query-plans.json"), raw, 0600) != nil {
		t.Fatal("evidence")
	}
}
