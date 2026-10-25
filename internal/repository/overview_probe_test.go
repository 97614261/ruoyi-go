package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net"
	"os"
	"path/filepath"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
	"strings"
	"sync"
	"testing"
	"time"
)

type overviewProbeLogger struct {
	mu      sync.Mutex
	enabled bool
	queries []string
}

func (l *overviewProbeLogger) LogMode(logger.LogLevel) logger.Interface { return l }
func (l *overviewProbeLogger) Info(context.Context, string, ...any)     {}
func (l *overviewProbeLogger) Warn(context.Context, string, ...any)     {}
func (l *overviewProbeLogger) Error(context.Context, string, ...any)    {}
func (l *overviewProbeLogger) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.enabled {
		q, _ := fc()
		if strings.HasPrefix(strings.ToUpper(q), "SELECT ") && !strings.Contains(q, "information_schema") {
			l.queries = append(l.queries, q)
		}
	}
}

// Explicit read-only probe of THIS round's completed, disabled synthetic
// fixture. No DDL, mutation, service restart or old enterprise is involved.
func TestOverviewLocalReadProbe(t *testing.T) {
	if os.Getenv("ERP_OVERVIEW_READ_PROBE") != "1" {
		t.Skip("explicit local probe only")
	}
	root := os.Getenv("MT_TEST_ROOT")
	if root == "" {
		t.Fatal("explicit root required")
	}
	var receipt struct {
		ID    int64  `json:"tenantId"`
		Code  string `json:"tenantCode"`
		Owner int64  `json:"ownerUserId"`
		Test  string `json:"test"`
	}
	raw, e := os.ReadFile(filepath.Join(root, "logs/erp-overview-20261025/latest-fixtures.json"))
	if e != nil || json.Unmarshal(raw, &receipt) != nil || receipt.ID < 1 || receipt.Owner < 1 || receipt.Test != "TestERPOverview" || !strings.HasPrefix(receipt.Code, "ops-") {
		t.Fatal("owned current-round receipt required")
	}
	cfg, e := config.Load(filepath.Join(root, "configs/multitenant.local.yml"))
	if e != nil {
		t.Fatal("config")
	}
	d, e := mysql.ParseDSN(cfg.MySQL.DSN)
	if e != nil {
		t.Fatal("DSN")
	}
	host, _, e := net.SplitHostPort(d.Addr)
	if e != nil || d.Net != "tcp" || d.DBName != TenantDatabaseName || (host != "localhost" && host != "127.0.0.1" && host != "::1") {
		t.Fatal("local dedicated DB only")
	}
	if Init(cfg.MySQL) != nil {
		t.Fatal("repository")
	}
	defer Close()
	ctx, c := context.WithTimeout(context.Background(), 30*time.Second)
	defer c()
	var tenant model.SysTenant
	if DB(tenantctx.System(ctx)).Where("tenant_id=?", receipt.ID).Take(&tenant).Error != nil || tenant.TenantCode != receipt.Code || tenant.Status != "1" || tenant.OwnerUserID == nil || *tenant.OwnerUserID != receipt.Owner {
		t.Fatal("fixture must be exact disabled current-round enterprise")
	}
	ctx = erpctx.WithUser(tenantctx.With(ctx, receipt.ID, false), receipt.Owner)
	if yes, e := overviewRead(ctx, []string{"__overview_missing_module"}, func(*gorm.DB) error { t.Fatal("missing module read executed"); return nil }); yes || e != nil {
		t.Fatal("absent is not a database failure", e)
	}
	if _, e := overviewRead(ctx, []string{"erp_order", "__overview_missing_module"}, func(*gorm.DB) error { t.Fatal("partial module read executed"); return nil }); e == nil {
		t.Fatal("partial structure accepted")
	}
	stopped, cancel := context.WithCancel(ctx)
	cancel()
	if _, e := overviewRead(stopped, []string{"erp_order"}, func(*gorm.DB) error { return nil }); e == nil {
		t.Fatal("database failure fabricated absence")
	}
	capture := &overviewProbeLogger{enabled: true}
	db = db.Session(&gorm.Session{Logger: capture})
	if _, e := SelectOverviewOrders(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e := SelectOverviewStocks(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e := SelectOverviewPurchases(ctx); e != nil {
		t.Fatal(e)
	}
	now := time.Now().In(types.Location)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, types.Location)
	if _, e := SelectOverviewShipments(ctx, start.Format(types.Layout), start.AddDate(0, 0, -6).Format(types.Layout), start.AddDate(0, 0, 1).Format(types.Layout)); e != nil {
		t.Fatal(e)
	}
	for _, sales := range []bool{false, true} {
		if _, e := SelectOverviewMoney(ctx, "", page.Query{PageNum: 1, PageSize: 10}, sales); e != nil {
			t.Fatal(e)
		}
	}
	capture.enabled = false
	plans := []map[string]any{}
	seen := map[string]bool{}
	for _, q := range capture.queries {
		if seen[q] {
			continue
		}
		seen[q] = true
		// Diagnostic SQL is captured from fixed, already tenant-scoped reads.
		// Customer Raw is deliberately forbidden; do not relax that guard.
		rows, e := DB(tenantctx.System(ctx)).Raw("EXPLAIN " + q).Rows()
		if e != nil {
			t.Fatal("actual aggregate EXPLAIN failed")
		}
		cols, e := rows.Columns()
		if e != nil {
			t.Fatal(e)
		}
		details := []map[string]string{}
		for rows.Next() {
			v := make([]any, len(cols))
			p := make([]any, len(cols))
			for i := range v {
				p[i] = &v[i]
			}
			if rows.Scan(p...) != nil {
				t.Fatal("plan scan")
			}
			row := map[string]string{}
			for i, k := range cols {
				switch x := v[i].(type) {
				case []byte:
					row[k] = string(x)
				default:
					row[k] = fmt.Sprint(x)
				}
			}
			details = append(details, row)
		}
		if rows.Err() != nil {
			t.Fatal("plan rows")
		}
		rows.Close()
		plans = append(plans, map[string]any{"sql": q, "plan": details})
	}
	if len(plans) < 6 {
		t.Fatal("missing aggregate query plans")
	}
	out := map[string]any{"atUtc": time.Now().UTC(), "tenantId": receipt.ID, "readOnly": true, "absentModule": true, "partialModuleRejected": true, "databaseErrorNotZero": true, "plans": plans, "capacityClaim": false}
	raw, e = json.MarshalIndent(out, "", "  ")
	if e != nil || os.WriteFile(filepath.Join(root, "logs/erp-overview-20261025/query-plans.json"), raw, 0600) != nil {
		t.Fatal("probe evidence")
	}
	t.Logf("%d actual aggregate EXPLAIN plans; missing/partial/error branches verified without DDL", len(plans))
}
