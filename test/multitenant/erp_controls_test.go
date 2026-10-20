package multitenant_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/xuri/excelize/v2"
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

func TestERPControls(t *testing.T) {
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
		t.Fatal("invalid local configuration")
	}
	host, _, err := net.SplitHostPort(dsn.Addr)
	if err != nil || dsn.DBName != "ruoyi_go_mt" || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		t.Fatal("refusing original or remote database")
	}
	if err = repository.Init(cfg.MySQL); err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	cache := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	defer cache.Close()
	s := &suite{t: t, root: root, base: "http://127.0.0.1:8081", client: &http.Client{Timeout: 20 * time.Second}, redis: cache, passwords: loadTestAccounts(t, root)}
	platform := s.login("platform")
	t.Cleanup(func() { s.req("POST", "/logout", platform, nil) })
	a := newOperationsFixture(t, s, platform)
	b := newOperationsFixture(t, s, platform)
	denied := func(v map[string]any) {
		t.Helper()
		if v["code"] == float64(200) {
			t.Fatal("request must be denied")
		}
	}
	master := func(token, key, code string) int64 {
		t.Helper()
		body := map[string]any{"status": "0"}
		if key == "sku" {
			body["skuCode"] = code
			body["productName"] = "控制验收商品"
			body["unit"] = "件"
		} else {
			body["warehouseCode"] = code
			body["warehouseName"] = code
		}
		s.ok(s.req("POST", "/erp/"+key, token, body))
		v := s.req("GET", "/erp/"+key+"/list?"+key+"Code="+code, token, nil)
		s.ok(v)
		return int64(v["rows"].([]any)[0].(map[string]any)[key+"Id"].(float64))
	}
	wa := master(a.owner, "warehouse", "CONTROL-A")
	wb := master(a.owner, "warehouse", "CONTROL-B")
	foreign := master(b.owner, "warehouse", "CONTROL-A")
	sku := func() int64 { return master(a.owner, "sku", uuid.NewString()) }
	input := func(w, x, q int64) map[string]any {
		return map[string]any{"warehouseId": w, "requestKey": uuid.NewString(), "remark": "控制验收原因", "lines": []map[string]any{{"skuId": x, "quantity": q}}}
	}
	create := func(kind string, body map[string]any) int64 {
		t.Helper()
		v := s.req("POST", "/erp/"+kind, a.owner, body)
		s.ok(v)
		return int64(v["data"].(map[string]any)["documentId"].(float64))
	}
	detail := func(kind string, id int64) map[string]any {
		t.Helper()
		v := s.req("GET", fmt.Sprintf("/erp/%s/%d", kind, id), a.owner, nil)
		s.ok(v)
		return v["data"].(map[string]any)
	}
	action := func(kind string, id int64, op string) {
		t.Helper()
		s.ok(s.req("PUT", fmt.Sprintf("/erp/%s/%d/%s", kind, id, op), a.owner, nil))
	}
	receipt := func(w, x, q int64) int64 {
		id := create("receipt", input(w, x, q))
		action("receipt", id, "confirm")
		return id
	}
	sale := func(w, x, q int64) int64 {
		id := create("sales", input(w, x, q))
		action("sales", id, "confirm")
		action("sales", id, "ship")
		return id
	}
	balance := func(x int64) (int64, int64) {
		t.Helper()
		v := s.req("GET", fmt.Sprintf("/erp/stock/list?warehouseId=%d&skuId=%d", wa, x), a.owner, nil)
		s.ok(v)
		rows := v["rows"].([]any)
		if len(rows) == 0 {
			return 0, 0
		}
		r := rows[0].(map[string]any)
		return int64(r["onHand"].(float64)), int64(r["reserved"].(float64))
	}
	assertBalance := func(x, h, r int64) {
		t.Helper()
		ah, ar := balance(x)
		if ah != h || ar != r {
			t.Fatalf("stock got %d/%d want %d/%d", ah, ar, h, r)
		}
	}
	correction := func(kind string, original, x, q int64, reversal bool) int64 {
		body := input(wa, x, q)
		body["sourceDocumentId"] = original
		body["reversal"] = reversal
		return create(kind, body)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var menus []model.SysMenu
	if err = repository.DB(tenantctx.System(ctx)).Where("menu_id>=6000 AND menu_id<7000").Find(&menus).Error; err != nil {
		t.Fatal(err)
	}
	menuIDs := []int64{}
	for _, m := range menus {
		menuIDs = append(menuIDs, m.MenuID)
	}
	s.ok(s.req("POST", "/system/role", a.owner, map[string]any{"roleName": "ERP验收操作员", "roleKey": "erp_operator", "roleSort": 0, "status": "0", "dataScope": "1", "menuIds": menuIDs, "menuCheckStrictly": true, "deptCheckStrictly": true}))
	rv := s.req("GET", "/system/role/list?roleKey=erp_operator", a.owner, nil)
	s.ok(rv)
	roleID := rv["rows"].([]any)[0].(map[string]any)["roleId"]
	user := func(name string) (int64, string) {
		s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": name, "nickName": name, "password": operationsPassword, "status": "0", "roleIds": []any{roleID}}))
		v := s.req("GET", "/system/user/list?userName="+name, a.owner, nil)
		s.ok(v)
		id := int64(v["rows"].([]any)[0].(map[string]any)["userId"].(float64))
		return id, s.loginAs(a.code, name, operationsPassword)
	}
	aliceID, alice := user("alice")
	bobID, bob := user("bob")
	grant := func(id int64, ids []int64) {
		t.Helper()
		p := fmt.Sprintf("/erp/warehouse/access/%d", id)
		v := s.req("GET", p, a.owner, nil)
		s.ok(v)
		s.ok(s.req("PUT", p, a.owner, map[string]any{"version": v["data"].(map[string]any)["version"], "warehouseIds": ids}))
	}
	t.Run("warehouse_default_deny_and_grant", func(t *testing.T) {
		v := s.req("GET", "/erp/warehouse/list", alice, nil)
		s.ok(v)
		if v["total"] != float64(0) {
			t.Fatal("employee default must be empty")
		}
		denied(s.req("GET", fmt.Sprintf("/erp/warehouse/%d", wa), alice, nil))
		denied(s.req("GET", fmt.Sprintf("/erp/warehouse/access/%d", bobID), alice, nil))
		grant(aliceID, []int64{wa})
		grant(bobID, []int64{wa})
		v = s.req("GET", "/erp/warehouse/list", alice, nil)
		s.ok(v)
		if v["total"] != float64(1) {
			t.Fatal("warehouse scope did not filter")
		}
		denied(s.req("GET", fmt.Sprintf("/erp/warehouse/%d", wb), alice, nil))
		p := fmt.Sprintf("/erp/warehouse/access/%d", aliceID)
		v = s.req("GET", p, a.owner, nil)
		s.ok(v)
		body := map[string]any{"version": v["data"].(map[string]any)["version"], "warehouseIds": []int64{foreign}}
		denied(s.req("PUT", p, a.owner, body))
		s.ok(s.req("GET", fmt.Sprintf("/erp/warehouse/%d", wa), alice, nil))
	})
	t.Run("draft_edit_CAS_and_create_replay", func(t *testing.T) {
		x := sku()
		body := input(wa, x, 10)
		id := create("receipt", body)
		results := parallelJSON(s, 2, func(i int) (string, string, string, any) {
			token := a.owner
			if i == 1 {
				token = alice
			}
			v := input(wa, x, int64(11+i))
			delete(v, "requestKey")
			v["version"] = 1
			v["remark"] = "编辑原因"
			return "PUT", fmt.Sprintf("/erp/receipt/%d", id), token, v
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
			t.Fatal("exactly one same-version edit must succeed")
		}
		d := detail("receipt", id)
		if d["version"] != float64(2) || d["remark"] != "编辑原因" {
			t.Fatal("edit version or content lost")
		}
		replay := s.req("POST", "/erp/receipt", a.owner, body)
		s.ok(replay)
		if replay["data"].(map[string]any)["documentId"] != float64(id) {
			t.Fatal("edit broke creation idempotency")
		}
		action("receipt", id, "confirm")
		v := input(wa, x, 99)
		v["version"] = 3
		denied(s.req("PUT", fmt.Sprintf("/erp/receipt/%d", id), a.owner, v))
	})
	t.Run("partial_returns_and_immutable_original", func(t *testing.T) {
		x := sku()
		in := receipt(wa, x, 20)
		out := sale(wa, x, 10)
		ret := correction("salesReturn", out, x, 3, false)
		action("salesReturn", ret, "confirm")
		action("salesReturn", ret, "confirm")
		assertBalance(x, 13, 0)
		body := input(wa, x, 8)
		body["sourceDocumentId"] = out
		denied(s.req("POST", "/erp/salesReturn", a.owner, body))
		pr := correction("purchaseReturn", in, x, 4, false)
		action("purchaseReturn", pr, "confirm")
		assertBalance(x, 9, 0)
		if detail("sales", out)["status"] != "SHIPPED" || detail("receipt", in)["status"] != "POSTED" {
			t.Fatal("correction rewrote original state")
		}
		body = input(wa, x, 20)
		body["sourceDocumentId"] = in
		body["reversal"] = true
		denied(s.req("POST", "/erp/purchaseReturn", a.owner, body))
		denied(s.req("POST", "/erp/salesReturn", b.owner, input(wa, x, 1)))
	})
	t.Run("concurrent_returns_from_distinct_users", func(t *testing.T) {
		x := sku()
		receipt(wa, x, 10)
		out := sale(wa, x, 5)
		ids := []int64{correction("salesReturn", out, x, 4, false), correction("salesReturn", out, x, 4, false)}
		results := parallelJSON(s, 2, func(i int) (string, string, string, any) {
			token := alice
			if i == 1 {
				token = bob
			}
			return "PUT", fmt.Sprintf("/erp/salesReturn/%d/confirm", ids[i]), token, nil
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
			t.Fatal("original quantity oversold by concurrent returns")
		}
		assertBalance(x, 9, 0)
	})
	t.Run("full_reversal_and_reserved_stock_guard", func(t *testing.T) {
		x := sku()
		in := receipt(wa, x, 10)
		out := sale(wa, x, 3)
		sr := correction("salesReturn", out, x, 3, true)
		action("salesReturn", sr, "confirm")
		assertBalance(x, 10, 0)
		reserved := create("sales", input(wa, x, 2))
		action("sales", reserved, "confirm")
		pr := correction("purchaseReturn", in, x, 10, true)
		denied(s.req("PUT", fmt.Sprintf("/erp/purchaseReturn/%d/confirm", pr), a.owner, nil))
		assertBalance(x, 10, 2)
		action("sales", reserved, "cancel")
		action("purchaseReturn", pr, "confirm")
		action("purchaseReturn", pr, "confirm")
		assertBalance(x, 0, 0)
	})
	countBody := func(x, target int64) map[string]any {
		body := input(wa, x, 1)
		body["lines"] = []map[string]any{{"skuId": x, "countedQuantity": target}}
		return body
	}
	t.Run("count_zero_delta_snapshot_and_reservation", func(t *testing.T) {
		x := sku()
		receipt(wa, x, 10)
		id := create("count", countBody(x, 8))
		action("count", id, "confirm")
		action("count", id, "confirm")
		assertBalance(x, 8, 0)
		v := s.req("GET", fmt.Sprintf("/erp/ledger/list?documentId=%d", id), a.owner, nil)
		s.ok(v)
		row := v["rows"].([]any)[0].(map[string]any)
		if v["total"] != float64(1) || row["onHandDelta"] != float64(-2) {
			t.Fatal("count must append one delta ledger")
		}
		reserved := create("sales", input(wa, x, 2))
		action("sales", reserved, "confirm")
		denied(s.req("POST", "/erp/count", a.owner, countBody(x, 1)))
		action("sales", reserved, "cancel")
		zero := create("count", countBody(x, 0))
		action("count", zero, "confirm")
		assertBalance(x, 0, 0)
		stale := create("count", countBody(x, 1))
		in := receipt(wa, x, 1)
		pr := correction("purchaseReturn", in, x, 1, false)
		action("purchaseReturn", pr, "confirm")
		assertBalance(x, 0, 0)
		denied(s.req("PUT", fmt.Sprintf("/erp/count/%d/confirm", stale), a.owner, nil))
		update := countBody(x, 1)
		update["version"] = 1
		s.ok(s.req("PUT", fmt.Sprintf("/erp/count/%d", stale), a.owner, update))
		action("count", stale, "confirm")
		assertBalance(x, 1, 0)
	})
	t.Run("moving_draft_cannot_race_warehouse_authorization", func(t *testing.T) {
		for attempt := 0; attempt < 6; attempt++ {
			x := sku()
			id := create("receipt", input(wa, x, 1))
			results := parallelJSON(s, 2, func(i int) (string, string, string, any) {
				if i == 0 {
					v := input(wb, x, 2)
					v["version"] = 1
					return "PUT", fmt.Sprintf("/erp/receipt/%d", id), a.owner, v
				}
				return "PUT", fmt.Sprintf("/erp/receipt/%d/confirm", id), alice, nil
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
				t.Fatal("moving warehouse and unauthorized posting must not both succeed")
			}
			d := detail("receipt", id)
			if d["status"] == "POSTED" {
				if d["warehouseId"] != float64(wa) {
					t.Fatal("employee posted unauthorized moved draft")
				}
				assertBalance(x, 1, 0)
			} else if d["status"] != "DRAFT" || d["warehouseId"] != float64(wb) {
				t.Fatal("race left incoherent draft")
			}
		}
	})
	t.Run("warehouse_scope_lists_details_exports_and_revoke", func(t *testing.T) {
		x := sku()
		receipt(wa, x, 5)
		foreignDoc := receipt(wb, x, 9)
		for _, path := range []string{fmt.Sprintf("/erp/receipt/%d", foreignDoc), fmt.Sprintf("/erp/warehouse/%d", wb)} {
			denied(s.req("GET", path, alice, nil))
		}
		for _, key := range []string{"receipt", "sales", "salesReturn", "purchaseReturn", "count", "stock", "ledger"} {
			v := s.req("GET", fmt.Sprintf("/erp/%s/list?warehouseId=%d&tenantId=%d", key, wb, b.tenant.TenantID), alice, nil)
			s.ok(v)
			if v["total"] != float64(0) {
				t.Fatalf("warehouse leak: %s", key)
			}
		}
		denied(s.req("PUT", fmt.Sprintf("/erp/receipt/%d/confirm", foreignDoc), alice, nil))
		denied(s.req("POST", "/erp/receipt", alice, input(wb, x, 1)))
		req, _ := http.NewRequest("POST", s.base+"/erp/warehouse/export", nil)
		req.Header.Set("Authorization", "Bearer "+alice)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		res, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		book, err := excelize.OpenReader(bytes.NewReader(raw))
		if err != nil {
			t.Fatal("export not XLSX")
		}
		defer book.Close()
		rows, err := book.GetRows(book.GetSheetName(0))
		if err != nil || len(rows) != 2 {
			t.Fatal("export leaked other warehouse rows")
		}
		draft := create("receipt", input(wa, x, 1))
		p := fmt.Sprintf("/erp/warehouse/access/%d", aliceID)
		v := s.req("GET", p, a.owner, nil)
		s.ok(v)
		old := v["data"].(map[string]any)["version"]
		denied(s.req("PUT", p, a.owner, map[string]any{"version": old}))
		denied(s.req("PUT", p, a.owner, map[string]any{"version": old, "warehouseIds": nil}))
		s.ok(s.req("GET", fmt.Sprintf("/erp/warehouse/%d", wa), alice, nil))
		grant(aliceID, []int64{})
		denied(s.req("PUT", p, a.owner, map[string]any{"version": old, "warehouseIds": []int64{wa}}))
		denied(s.req("GET", fmt.Sprintf("/erp/receipt/%d", draft), alice, nil))
		denied(s.req("PUT", fmt.Sprintf("/erp/receipt/%d/confirm", draft), alice, nil))
		s.ok(s.req("GET", "/getInfo", alice, nil))
	})
	t.Run("warehouse_grant_audit_failure_rolls_back", func(t *testing.T) {
		p := fmt.Sprintf("/erp/warehouse/access/%d", bobID)
		before := s.req("GET", p, a.owner, nil)
		s.ok(before)
		version := int64(before["data"].(map[string]any)["version"].(float64))
		ctx, cancel := context.WithTimeout(erpctx.WithUser(tenantctx.With(context.Background(), a.tenant.TenantID, false), *a.tenant.OwnerUserID), 10*time.Second)
		defer cancel()
		cb := repository.DB(ctx).Callback().Create()
		failure := errors.New("injected warehouse audit failure")
		if err := cb.Before("gorm:create").Register("erp-controls:audit-failure", func(tx *gorm.DB) {
			if tx.Statement.Table == "sys_oper_log" {
				tx.AddError(failure)
			}
		}); err != nil {
			t.Fatal(err)
		}
		err := repository.ReplaceERPWarehouseAccess(ctx, bobID, model.ERPAccessInput{Version: version, WarehouseIDs: []int64{wb}}, "fault-test")
		cb.Remove("erp-controls:audit-failure")
		if !errors.Is(err, failure) {
			t.Fatal("audit fault was not reached")
		}
		after := s.req("GET", p, a.owner, nil)
		s.ok(after)
		if after["data"].(map[string]any)["version"] != float64(version) {
			t.Fatal("failed grant advanced version")
		}
		s.ok(s.req("GET", fmt.Sprintf("/erp/warehouse/%d", wa), bob, nil))
		denied(s.req("GET", fmt.Sprintf("/erp/warehouse/%d", wb), bob, nil))
	})
	t.Run("correction_and_count_ledger_failure_atomic", func(t *testing.T) {
		x := sku()
		in := receipt(wa, x, 5)
		pr := correction("purchaseReturn", in, x, 2, false)
		count := create("count", countBody(x, 3))
		ctx, cancel := context.WithTimeout(erpctx.WithUser(tenantctx.With(context.Background(), a.tenant.TenantID, false), *a.tenant.OwnerUserID), 10*time.Second)
		defer cancel()
		callback := repository.DB(ctx).Callback().Create()
		failure := errors.New("injected controls ledger failure")
		if err := callback.Before("gorm:create").Register("erp-controls:ledger-failure", func(tx *gorm.DB) {
			if tx.Statement.Table == "erp_stock_ledger" {
				tx.AddError(failure)
			}
		}); err != nil {
			t.Fatal(err)
		}
		defer callback.Remove("erp-controls:ledger-failure")
		for kind, id := range map[string]int64{"PR": pr, "COUNT": count} {
			if err := repository.TransitionERPDocument(ctx, kind, id, "confirm", "fault-test"); !errors.Is(err, failure) {
				t.Fatal("did not reach injected ledger failure")
			}
		}
		assertBalance(x, 5, 0)
		if detail("purchaseReturn", pr)["status"] != "DRAFT" || detail("count", count)["status"] != "DRAFT" {
			t.Fatal("failed ledger partially posted")
		}
	})
	t.Run("ledger_reconciles_all_balances", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var mismatches int64
		err := repository.DB(tenantctx.System(ctx)).Raw("SELECT COUNT(*) FROM erp_stock s LEFT JOIN (SELECT tenant_id,warehouse_id,sku_id,SUM(on_hand_delta) AS h,SUM(reserved_delta) AS r FROM erp_stock_ledger WHERE tenant_id=? GROUP BY tenant_id,warehouse_id,sku_id) l ON l.tenant_id=s.tenant_id AND l.warehouse_id=s.warehouse_id AND l.sku_id=s.sku_id WHERE s.tenant_id=? AND (s.on_hand<>COALESCE(l.h,0) OR s.reserved<>COALESCE(l.r,0) OR s.on_hand<s.reserved OR s.reserved<0)", a.tenant.TenantID, a.tenant.TenantID).Scan(&mismatches).Error
		if err != nil || mismatches != 0 {
			t.Fatal("inventory ledger and balance diverged")
		}
	})
}
