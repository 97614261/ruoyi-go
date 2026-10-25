package multitenant_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/tenantctx"
	"testing"
	"time"
)

func TestERPClosedLoop(t *testing.T) {
	root := os.Getenv("MT_TEST_ROOT")
	if root == "" {
		t.Skip("explicit dedicated local database only")
	}
	cfg, err := config.Load(filepath.Join(root, "configs/multitenant.local.yml"))
	if err != nil {
		t.Fatal("local config unavailable")
	}
	dsn, err := mysql.ParseDSN(cfg.MySQL.DSN)
	if err != nil {
		t.Fatal("invalid local DSN")
	}
	host, _, err := net.SplitHostPort(dsn.Addr)
	if err != nil || dsn.DBName != "ruoyi_go_mt" || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		t.Fatal("refusing original/remote database")
	}
	if err = repository.Init(cfg.MySQL); err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	passwords := loadTestAccounts(t, root)
	cache := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	defer cache.Close()
	s := &suite{t: t, base: "http://127.0.0.1:8081", root: root, client: &http.Client{Timeout: 20 * time.Second}, redis: cache, passwords: passwords}
	platform := s.login("platform")
	t.Cleanup(func() { s.req("POST", "/logout", platform, nil) })
	a := newOperationsFixture(t, s, platform)
	b := newOperationsFixture(t, s, platform)
	deny := func(v map[string]any) {
		t.Helper()
		if v["code"] == float64(200) {
			t.Fatal("expected rejected request")
		}
	}
	createMaster := func(token, key, code string) int64 {
		t.Helper()
		body := map[string]any{"status": "0"}
		if key == "sku" {
			body["skuCode"] = code
			body["productName"] = "闭环验收商品"
			body["unit"] = "件"
		} else {
			body["warehouseCode"] = code
			body["warehouseName"] = "闭环验收仓库"
		}
		s.ok(s.req("POST", "/erp/"+key, token, body))
		result := s.req("GET", "/erp/"+key+"/list?"+key+"Code="+code, token, nil)
		s.ok(result)
		rows := result["rows"].([]any)
		if len(rows) != 1 {
			t.Fatal("master lookup failed")
		}
		return int64(rows[0].(map[string]any)[key+"Id"].(float64))
	}
	wa := createMaster(a.owner, "warehouse", "DEFAULT")
	wb := createMaster(b.owner, "warehouse", "DEFAULT")
	x := createMaster(a.owner, "sku", "ITEM-001")
	y := createMaster(a.owner, "sku", "ITEM-002")
	bx := createMaster(b.owner, "sku", "ITEM-001")
	input := func(warehouse int64, lines ...model.ERPLineInput) map[string]any {
		return map[string]any{"warehouseId": warehouse, "requestKey": uuid.NewString(), "lines": lines, "remark": "ERP真实接口验收"}
	}
	createDoc := func(kind string, body map[string]any) int64 {
		t.Helper()
		r := s.req("POST", "/erp/"+kind, a.owner, body)
		s.ok(r)
		return int64(r["data"].(map[string]any)["documentId"].(float64))
	}
	action := func(kind string, id int64, op string) {
		t.Helper()
		s.ok(s.req("PUT", fmt.Sprintf("/erp/%s/%d/%s", kind, id, op), a.owner, nil))
	}
	state := func(kind string, id int64) map[string]any {
		t.Helper()
		r := s.req("GET", fmt.Sprintf("/erp/%s/%d", kind, id), a.owner, nil)
		s.ok(r)
		return r["data"].(map[string]any)
	}
	balance := func(sku int64) (int64, int64, int64) {
		t.Helper()
		r := s.req("GET", fmt.Sprintf("/erp/stock/list?warehouseId=%d&skuId=%d", wa, sku), a.owner, nil)
		s.ok(r)
		rows := r["rows"].([]any)
		if len(rows) == 0 {
			return 0, 0, 0
		}
		row := rows[0].(map[string]any)
		return int64(row["onHand"].(float64)), int64(row["reserved"].(float64)), int64(row["available"].(float64))
	}
	assertBalance := func(sku, h, r, v int64) {
		t.Helper()
		ah, ar, av := balance(sku)
		if ah != h || ar != r || av != v {
			t.Fatalf("balance got %d/%d/%d want %d/%d/%d", ah, ar, av, h, r, v)
		}
	}
	ledgerCount := func(id int64) int {
		t.Helper()
		r := s.req("GET", fmt.Sprintf("/erp/ledger/list?documentId=%d", id), a.owner, nil)
		s.ok(r)
		return int(r["total"].(float64))
	}

	t.Run("inbound_reserve_ship_and_cancel", func(t *testing.T) {
		inbound := createDoc("receipt", input(wa, model.ERPLineInput{SkuID: x, Quantity: 100}))
		assertBalance(x, 0, 0, 0)
		action("receipt", inbound, "confirm")
		action("receipt", inbound, "confirm")
		assertBalance(x, 100, 0, 100)
		if ledgerCount(inbound) != 1 {
			t.Fatal("duplicate receipt created ledger")
		}
		deny(s.req("PUT", fmt.Sprintf("/erp/receipt/%d/cancel", inbound), a.owner, nil))
		sale := createDoc("sales", input(wa, model.ERPLineInput{SkuID: x, Quantity: 3}))
		action("sales", sale, "confirm")
		action("sales", sale, "confirm")
		assertBalance(x, 100, 3, 97)
		action("sales", sale, "ship")
		action("sales", sale, "ship")
		assertBalance(x, 97, 0, 97)
		if ledgerCount(sale) != 2 {
			t.Fatal("duplicate shipment created ledger")
		}
		deny(s.req("PUT", fmt.Sprintf("/erp/sales/%d/cancel", sale), a.owner, nil))
		cancel := createDoc("sales", input(wa, model.ERPLineInput{SkuID: x, Quantity: 3}))
		action("sales", cancel, "confirm")
		assertBalance(x, 97, 3, 94)
		action("sales", cancel, "cancel")
		action("sales", cancel, "cancel")
		assertBalance(x, 97, 0, 97)
		draft := createDoc("sales", input(wa, model.ERPLineInput{SkuID: x, Quantity: 1}))
		deny(s.req("PUT", fmt.Sprintf("/erp/sales/%d/ship", draft), a.owner, nil))
		action("sales", draft, "cancel")
		if ledgerCount(draft) != 0 {
			t.Fatal("draft cancellation changes stock")
		}
	})
	t.Run("validation_and_atomic_multi_line_failure", func(t *testing.T) {
		for _, lines := range []any{[]any{}, []model.ERPLineInput{{SkuID: x, Quantity: 0}}, []model.ERPLineInput{{SkuID: x, Quantity: -1}}, []model.ERPLineInput{{SkuID: x, Quantity: 1}, {SkuID: x, Quantity: 2}}, []map[string]any{{"skuId": x, "quantity": 1.5}}} {
			body := input(wa)
			body["lines"] = lines
			deny(s.req("POST", "/erp/sales", a.owner, body))
		}
		tooMany := make([]model.ERPLineInput, 101)
		for i := range tooMany {
			tooMany[i] = model.ERPLineInput{SkuID: x, Quantity: 1}
		}
		deny(s.req("POST", "/erp/sales", a.owner, input(wa, tooMany...)))
		sale := createDoc("sales", input(wa, model.ERPLineInput{SkuID: x, Quantity: 1}, model.ERPLineInput{SkuID: y, Quantity: 1}))
		deny(s.req("PUT", fmt.Sprintf("/erp/sales/%d/confirm", sale), a.owner, nil))
		assertBalance(x, 97, 0, 97)
		assertBalance(y, 0, 0, 0)
		if state("sales", sale)["status"] != "DRAFT" || ledgerCount(sale) != 0 {
			t.Fatal("multi-line failed transaction partially committed")
		}
	})
	t.Run("concurrent_create_and_confirm_are_idempotent", func(t *testing.T) {
		body := input(wa, model.ERPLineInput{SkuID: x, Quantity: 1})
		results := parallelJSON(s, 8, func(i int) (string, string, string, any) { return "POST", "/erp/sales", a.owner, body })
		for _, r := range results {
			if r.err != nil || r.code != 200 {
				t.Fatalf("idempotent create failed: %v code=%v", r.err, r.code)
			}
		}
		r := s.req("POST", "/erp/sales", a.owner, body)
		s.ok(r)
		id := int64(r["data"].(map[string]any)["documentId"].(float64))
		ctx := tenantctx.With(context.Background(), a.tenant.TenantID, false)
		ctx = erpctx.WithUser(ctx, *a.tenant.OwnerUserID)
		var count int64
		if err := repository.DB(ctx).Model(&model.ERPDocument{}).Where("request_key=?", body["requestKey"]).Count(&count).Error; err != nil || count != 1 {
			t.Fatal("idempotency key created multiple documents")
		}
		changed := input(wa, model.ERPLineInput{SkuID: x, Quantity: 2})
		changed["requestKey"] = body["requestKey"]
		deny(s.req("POST", "/erp/sales", a.owner, changed))
		results = parallelJSON(s, 8, func(i int) (string, string, string, any) {
			return "PUT", fmt.Sprintf("/erp/sales/%d/confirm", id), a.owner, nil
		})
		for _, r := range results {
			if r.err != nil || r.code != 200 {
				t.Fatal("repeated confirmation must be idempotent")
			}
		}
		assertBalance(x, 97, 1, 96)
		if ledgerCount(id) != 1 {
			t.Fatal("confirmation duplicated ledger")
		}
		action("sales", id, "cancel")
		assertBalance(x, 97, 0, 97)
	})
	t.Run("last_unit_has_one_concurrent_winner", func(t *testing.T) {
		race := createMaster(a.owner, "sku", "LAST-ONE")
		inbound := createDoc("receipt", input(wa, model.ERPLineInput{SkuID: race, Quantity: 1}))
		action("receipt", inbound, "confirm")
		ids := make([]int64, 8)
		for i := range ids {
			ids[i] = createDoc("sales", input(wa, model.ERPLineInput{SkuID: race, Quantity: 1}))
		}
		results := parallelJSON(s, len(ids), func(i int) (string, string, string, any) {
			return "PUT", fmt.Sprintf("/erp/sales/%d/confirm", ids[i]), a.owner, nil
		})
		winners := 0
		for _, r := range results {
			if r.err != nil {
				t.Fatal(r.err)
			}
			if r.code == 200 {
				winners++
			}
		}
		if winners != 1 {
			t.Fatalf("last unit winners=%d", winners)
		}
		assertBalance(race, 1, 1, 0)
	})
	t.Run("concurrent_ship_and_cancel_have_one_outcome", func(t *testing.T) {
		race := createMaster(a.owner, "sku", "SHIP-CANCEL")
		inbound := createDoc("receipt", input(wa, model.ERPLineInput{SkuID: race, Quantity: 1}))
		action("receipt", inbound, "confirm")
		sale := createDoc("sales", input(wa, model.ERPLineInput{SkuID: race, Quantity: 1}))
		action("sales", sale, "confirm")
		results := parallelJSON(s, 2, func(i int) (string, string, string, any) {
			op := "ship"
			if i == 1 {
				op = "cancel"
			}
			return "PUT", fmt.Sprintf("/erp/sales/%d/%s", sale, op), a.owner, nil
		})
		wins := 0
		for _, r := range results {
			if r.err != nil {
				t.Fatal(r.err)
			}
			if r.code == 200 {
				wins++
			}
		}
		if wins != 1 {
			t.Fatal("shipment and cancellation must not both succeed")
		}
		switch state("sales", sale)["status"] {
		case "SHIPPED":
			assertBalance(race, 0, 0, 0)
		case "CANCELLED":
			assertBalance(race, 1, 0, 1)
		default:
			t.Fatal("race left an intermediate state")
		}
		if ledgerCount(sale) != 2 {
			t.Fatal("race must create exactly one terminal ledger event")
		}
	})
	t.Run("generated_master_crud_and_snapshot", func(t *testing.T) {
		previous := s.t
		s.t = t
		defer func() { s.t = previous }()
		original := createDoc("sales", input(wa, model.ERPLineInput{SkuID: x, Quantity: 1}))
		for _, bad := range []struct{ key, value string }{{"skuCode", "CHANGED"}, {"unit", "箱"}, {"specification", "更新规格"}} {
			v := map[string]any{"skuId": x, "skuCode": "ITEM-001", "productName": "修改后的商品名", "specification": "", "unit": "件", "status": "0"}
			v[bad.key] = bad.value
			deny(s.req("PUT", "/erp/sku", a.owner, v))
		}
		// Catalog010 deliberately freezes referenced identity/spec/unit. Names
		// remain editable, without rewriting immutable historical line snapshots.
		s.ok(s.req("PUT", "/erp/sku", a.owner, map[string]any{"skuId": x, "skuCode": "ITEM-001", "productName": "修改后的商品名", "specification": "", "unit": "件", "status": "0"}))
		detail := s.req("GET", fmt.Sprintf("/erp/sku/%d", x), a.owner, nil)
		s.ok(detail)
		data := detail["data"].(map[string]any)
		if data["updateBy"] != "owner" || data["productName"] != "修改后的商品名" || data["specification"] != "" || data["unit"] != "件" || data["skuCode"] != "ITEM-001" {
			t.Fatal("generated edit/audit not persisted")
		}
		line := state("sales", original)["lines"].([]any)[0].(map[string]any)
		if line["productName"] != "闭环验收商品" || line["unit"] != "件" {
			t.Fatal("historical document snapshot changed with master data")
		}
		unused := createMaster(a.owner, "sku", "UNUSED")
		s.ok(s.req("PUT", "/erp/sku", a.owner, map[string]any{"skuId": unused, "skuCode": "UNUSED", "productName": "未引用商品", "specification": "允许修改的规格", "unit": "包", "status": "0"}))
		deny(s.req("DELETE", fmt.Sprintf("/erp/sku/%d,%d", unused, bx), a.owner, nil))
		s.ok(s.req("GET", fmt.Sprintf("/erp/sku/%d", unused), a.owner, nil))
		s.ok(s.req("DELETE", fmt.Sprintf("/erp/sku/%d", unused), a.owner, nil))
		deny(s.req("GET", fmt.Sprintf("/erp/sku/%d", unused), a.owner, nil))
		for _, key := range []string{"sku", "warehouse"} {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			req, err := http.NewRequestWithContext(ctx, "POST", s.base+"/erp/"+key+"/export", nil)
			if err != nil {
				cancel()
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+a.owner)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			res, err := s.client.Do(req)
			if err != nil {
				cancel()
				t.Fatal(err)
			}
			var magic [2]byte
			_, err = io.ReadFull(res.Body, magic[:])
			res.Body.Close()
			cancel()
			if err != nil || magic != [2]byte{'P', 'K'} {
				t.Fatal("generated export did not return XLSX")
			}
		}
	})
	t.Run("tenant_and_permission_boundaries", func(t *testing.T) {
		deny(s.req("GET", "/erp/sku/list", platform, nil))
		var containsERP func(any) bool
		containsERP = func(value any) bool {
			switch v := value.(type) {
			case []any:
				for _, item := range v {
					if containsERP(item) {
						return true
					}
				}
			case map[string]any:
				if v["path"] == "erp" || v["path"] == "/erp" {
					return true
				}
				return containsERP(v["children"])
			}
			return false
		}
		routes := s.req("GET", "/getRouters", platform, nil)
		s.ok(routes)
		if containsERP(routes["data"]) {
			t.Fatal("platform navigation exposed customer-only ERP")
		}
		routes = s.req("GET", "/getRouters", a.owner, nil)
		s.ok(routes)
		if !containsERP(routes["data"]) {
			t.Fatal("enabled customer missing ERP navigation")
		}
		deny(s.req("POST", "/erp/sales", a.owner, input(wb, model.ERPLineInput{SkuID: x, Quantity: 1})))
		deny(s.req("POST", "/erp/sales", a.owner, input(wa, model.ERPLineInput{SkuID: bx, Quantity: 1})))
		doc := createDoc("sales", input(wa, model.ERPLineInput{SkuID: x, Quantity: 1}))
		deny(s.req("GET", fmt.Sprintf("/erp/sales/%d", doc), b.owner, nil))
		deny(s.req("PUT", fmt.Sprintf("/erp/sales/%d/confirm", doc), b.owner, nil))
		deny(s.req("DELETE", fmt.Sprintf("/erp/sku/%d", bx), a.owner, nil))
		deny(s.req("DELETE", fmt.Sprintf("/erp/warehouse/%d", wb), a.owner, nil))
		deny(s.req("DELETE", fmt.Sprintf("/erp/sku/%d", x), a.owner, nil))
		deny(s.req("DELETE", fmt.Sprintf("/erp/warehouse/%d", wa), a.owner, nil))
		list := s.req("GET", fmt.Sprintf("/erp/stock/list?tenantId=%d", a.tenant.TenantID), b.owner, nil)
		s.ok(list)
		if list["total"] != float64(0) {
			t.Fatal("foreign tenant injection leaked balances")
		}
		s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": "employee", "nickName": "无权限员工", "password": operationsPassword, "status": "0", "sex": "0"}))
		employee := s.loginAs(a.code, "employee", operationsPassword)
		deny(s.req("GET", "/erp/stock/list", employee, nil))
		deny(s.req("POST", "/erp/sales", employee, input(wa, model.ERPLineInput{SkuID: x, Quantity: 1})))
		direct := &model.ERPDocument{ERPRecord: model.ERPRecord{TenantRecord: model.TenantRecord{TenantID: a.tenant.TenantID}}, WarehouseID: wb, WarehouseName: "foreign", Kind: "SALE", Status: "DRAFT", DocumentNo: uuid.NewString(), RequestKey: uuid.NewString(), RequestHash: "bad"}
		err := repository.DB(tenantctx.System(context.Background())).Create(direct).Error
		var my *mysql.MySQLError
		if !errors.As(err, &my) || my.Number != 1452 {
			t.Fatal("composite foreign key did not reject cross-tenant warehouse")
		}
	})
	t.Run("ledger_failure_rolls_back_balance_and_status", func(t *testing.T) {
		doc := createDoc("receipt", input(wa, model.ERPLineInput{SkuID: x, Quantity: 5}))
		ctx := tenantctx.With(context.Background(), a.tenant.TenantID, false)
		callback := repository.DB(ctx).Callback().Create()
		ctx = erpctx.WithUser(ctx, *a.tenant.OwnerUserID)
		failure := errors.New("injected inventory ledger failure")
		if err := callback.Before("gorm:create").Register("erp-test:ledger-failure", func(tx *gorm.DB) {
			if tx.Statement.Table == "erp_stock_ledger" {
				tx.AddError(failure)
			}
		}); err != nil {
			t.Fatal(err)
		}
		defer callback.Remove("erp-test:ledger-failure")
		if err := repository.TransitionERPDocument(ctx, "IN", doc, "confirm", "rollback-test"); !errors.Is(err, failure) {
			t.Fatal("injected failure ignored")
		}
		assertBalance(x, 97, 0, 97)
		if state("receipt", doc)["status"] != "DRAFT" || ledgerCount(doc) != 0 {
			t.Fatal("ledger failure partially committed")
		}
	})
	t.Run("pagination_has_hard_cap_and_stable_order", func(t *testing.T) {
		r := s.req("GET", "/erp/sales/list?pageSize=100000&orderByColumn=badSql", a.owner, nil)
		s.ok(r)
		rows := r["rows"].([]any)
		if len(rows) > 100 {
			t.Fatal("page cap bypass")
		}
		last := int64(1 << 62)
		for _, item := range rows {
			id := int64(item.(map[string]any)["documentId"].(float64))
			if id >= last {
				t.Fatal("unstable document order")
			}
			last = id
		}
	})
}
