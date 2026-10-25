package multitenant_test

import (
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
	"reflect"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/tenantctx"
	"strings"
	"testing"
	"time"
)

func TestERPWarehouseOperations(t *testing.T) {
	root := os.Getenv("MT_TEST_ROOT")
	if root == "" {
		t.Skip("explicit local dedicated fixtures required")
	}
	cfg, e := config.Load(filepath.Join(root, "configs/multitenant.local.yml"))
	if e != nil {
		t.Fatal("local configuration unavailable")
	}
	d, e := mysql.ParseDSN(cfg.MySQL.DSN)
	if e != nil {
		t.Fatal("invalid configuration")
	}
	h, _, e := net.SplitHostPort(d.Addr)
	if e != nil || d.Net != "tcp" || d.DBName != "ruoyi_go_mt" || (h != "127.0.0.1" && h != "localhost" && h != "::1") {
		t.Fatal("nonlocal/original DB refused")
	}
	if repository.Init(cfg.MySQL) != nil {
		t.Fatal("database unavailable")
	}
	defer repository.Close()
	cache := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	defer cache.Close()
	s := &suite{t: t, root: root, base: "http://127.0.0.1:8081", client: &http.Client{Timeout: 20 * time.Second}, redis: cache, passwords: loadTestAccounts(t, root)}
	p := s.login("platform")
	t.Cleanup(func() { s.req("POST", "/logout", p, nil) })
	a, b := newOperationsFixture(t, s, p), newOperationsFixture(t, s, p)
	ctx, cancel := context.WithTimeout(erpctx.WithUser(tenantctx.With(context.Background(), a.tenant.TenantID, false), *a.tenant.OwnerUserID), 8*time.Minute)
	defer cancel()
	run := func(n string, f func(*testing.T)) {
		t.Run(n, func(t *testing.T) { old := s.t; s.t = t; defer func() { s.t = old }(); f(t) })
	}
	data := func(v map[string]any) map[string]any { s.ok(v); return v["data"].(map[string]any) }
	id := func(v map[string]any, k string) int64 { return int64(v[k].(float64)) }
	deny := func(v map[string]any) {
		s.t.Helper()
		if v["code"] == float64(200) {
			s.t.Fatal("invalid/unauthorized operation accepted")
		}
	}
	master := func(kind, code string) int64 {
		v := map[string]any{"status": "0"}
		if kind == "sku" {
			v["skuCode"] = code
			v["productName"] = "仓库增强商品"
			v["unit"] = "件"
		} else {
			v["warehouseCode"] = code
			v["warehouseName"] = code
		}
		s.ok(s.req("POST", "/erp/"+kind, a.owner, v))
		r := s.req("GET", "/erp/"+kind+"/list?"+kind+"Code="+code, a.owner, nil)
		s.ok(r)
		return id(r["rows"].([]any)[0].(map[string]any), kind+"Id")
	}
	wa, wb := master("warehouse", "W7-A"), master("warehouse", "W7-B")
	x, y := master("sku", "W7-X"), master("sku", "W7-Y")
	wh := func(w int64) map[string]any {
		return data(s.req("GET", fmt.Sprintf("/erp/warehouse/%d", w), a.owner, nil))
	}
	doc := func(kind string, w, x, q int64) int64 {
		v := data(s.req("POST", "/erp/"+kind, a.owner, map[string]any{"warehouseId": w, "requestKey": uuid.NewString(), "lines": []model.ERPLineInput{{SkuID: x, Quantity: q}}}))
		n := id(v, "documentId")
		s.ok(s.req("PUT", fmt.Sprintf("/erp/%s/%d/confirm", kind, n), a.owner, nil))
		return n
	}
	stock := func(w, x int64) (int64, int64) {
		v := s.req("GET", fmt.Sprintf("/erp/stock/list?warehouseId=%d&skuId=%d", w, x), a.owner, nil)
		s.ok(v)
		r := v["rows"].([]any)
		if len(r) == 0 {
			return 0, 0
		}
		z := r[0].(map[string]any)
		return id(z, "onHand"), id(z, "reserved")
	}
	assertStock := func(w, x, h, r int64) {
		s.t.Helper()
		a, b := stock(w, x)
		if a != h || b != r {
			s.t.Fatalf("stock=%d/%d want=%d/%d", a, b, h, r)
		}
	}
	makeInput := func(w, z, sku, q int64) model.ERPTransferInput {
		return model.ERPTransferInput{SourceWarehouseID: w, TargetWarehouseID: z, RequestKey: uuid.NewString(), Lines: []model.ERPLineInput{{SkuID: sku, Quantity: q}}}
	}
	create := func(in model.ERPTransferInput) int64 {
		return id(data(s.req("POST", "/erp/transfer", a.owner, in)), "transferId")
	}
	action := func(token string, tr, v int64, op, key string) map[string]any {
		return s.req("POST", fmt.Sprintf("/erp/transfer/%d/%s", tr, op), token, model.ERPTransferOperation{Version: v, RequestKey: key})
	}
	get := func(tr int64) map[string]any {
		return data(s.req("GET", fmt.Sprintf("/erp/transfer/%d", tr), a.owner, nil))
	}
	doc("receipt", wa, x, 20)
	doc("sales", wa, x, 3)
	run("warehouse_profile_CAS_privacy_and_status_filter", func(t *testing.T) {
		before := wh(wa)
		before["address"] = "仅资料权限可见地址"
		before["manager"] = "测试负责人"
		s.ok(s.req("PUT", "/erp/warehouse", a.owner, before))
		deny(s.req("PUT", "/erp/warehouse", a.owner, before))
		after := wh(wa)
		if after["address"] != before["address"] || id(after, "version") != id(before, "version")+1 {
			t.Fatal("profile not atomic")
		}
		r := s.req("GET", fmt.Sprintf("/erp/stock/list?warehouseId=%d", wa), a.owner, nil)
		s.ok(r)
		if strings.Contains(fmt.Sprint(r), "仅资料权限可见地址") {
			t.Fatal("private profile leaked")
		}
		after["status"] = "1"
		s.ok(s.req("PUT", "/erp/warehouse", a.owner, after))
		r = s.req("GET", "/erp/warehouse/list?status=1", a.owner, nil)
		s.ok(r)
		if r["total"] != float64(1) {
			t.Fatal("status filter")
		}
		after = wh(wa)
		after["status"] = "0"
		s.ok(s.req("PUT", "/erp/warehouse", a.owner, after))
	})
	run("alerts_zero_unset_CAS_ABA_and_filters", func(t *testing.T) {
		path := fmt.Sprintf("/erp/stock/alert?warehouseId=%d&skuId=%d", wa, x)
		r := data(s.req("GET", path, a.owner, nil))
		if r["thresholdQuantity"] != nil || r["version"] != float64(0) {
			t.Fatal("unset not null")
		}
		in := map[string]any{"warehouseId": wa, "skuId": x, "version": 0, "thresholdQuantity": 0}
		r = data(s.req("PUT", "/erp/stock/alert", a.owner, in))
		if r["thresholdQuantity"] != float64(0) {
			t.Fatal("zero lost")
		}
		deny(s.req("PUT", "/erp/stock/alert", a.owner, in))
		in["version"] = r["version"]
		in["thresholdQuantity"] = nil
		r = data(s.req("PUT", "/erp/stock/alert", a.owner, in))
		if r["thresholdQuantity"] != nil || r["version"] != float64(2) {
			t.Fatal("ABA tombstone lost")
		}
		in["version"] = r["version"]
		in["thresholdQuantity"] = 17
		s.ok(s.req("PUT", "/erp/stock/alert", a.owner, in))
		r = s.req("GET", fmt.Sprintf("/erp/stock/list?warehouseId=%d&stockState=LOW", wa), a.owner, nil)
		s.ok(r)
		if r["total"] != float64(1) {
			t.Fatal("low filter")
		}
		for _, value := range []any{-1, 1.2, "1"} {
			in["thresholdQuantity"] = value
			deny(s.req("PUT", "/erp/stock/alert", a.owner, in))
		}
	})
	var completed int64
	run("whole_transfer_in_transit_immutable_replay_and_reserved_conservation", func(t *testing.T) {
		in := makeInput(wa, wb, x, 4)
		completed = create(in)
		key := uuid.NewString()
		first := data(action(a.owner, completed, 1, "dispatch", key))
		assertStock(wa, x, 16, 3)
		assertStock(wb, x, 0, 0)
		if first["status"] != "IN_TRANSIT" {
			t.Fatal("no transit")
		}
		s.ok(action(a.owner, completed, 2, "receive", uuid.NewString()))
		assertStock(wa, x, 16, 3)
		assertStock(wb, x, 4, 0)
		replay := data(action(a.owner, completed, 1, "dispatch", key))
		if !reflect.DeepEqual(first, replay) {
			t.Fatal("replay returned latest not original")
		}
		deny(action(a.owner, completed, 2, "dispatch", key))
		original := data(s.req("POST", "/erp/transfer", a.owner, in))
		if original["status"] != "DRAFT" || original["version"] != float64(1) {
			t.Fatal("create original lost")
		}
		deny(action(a.owner, completed, 3, "cancel", uuid.NewString()))
		h := get(completed)
		if h["outDocumentId"] == nil || h["inDocumentId"] == nil {
			t.Fatal("missing vouchers")
		}
		for _, k := range []string{"outDocumentId", "inDocumentId"} {
			r := s.req("GET", fmt.Sprintf("/erp/ledger/list?documentId=%d", id(h, k)), a.owner, nil)
			s.ok(r)
			if r["total"] != float64(1) {
				t.Fatal("duplicate ledger")
			}
			l := r["rows"].([]any)[0].(map[string]any)
			if l["skuCode"] != "W7-X" || l["transferId"] != float64(completed) {
				t.Fatal("ledger trace missing")
			}
		}
	})
	run("invalid_inputs_and_reserved_protection", func(t *testing.T) {
		in := makeInput(wa, wb, x, 14)
		tr := create(in)
		deny(action(a.owner, tr, 1, "dispatch", uuid.NewString()))
		assertStock(wa, x, 16, 3)
		if get(tr)["status"] != "DRAFT" {
			t.Fatal("partial state")
		}
		in = makeInput(wa, wa, x, 1)
		deny(s.req("POST", "/erp/transfer", a.owner, in))
		in = makeInput(wa, wb, x, 0)
		deny(s.req("POST", "/erp/transfer", a.owner, in))
		in = makeInput(wa, wb, x, 1)
		in.Lines = append(in.Lines, in.Lines[0])
		deny(s.req("POST", "/erp/transfer", a.owner, in))
		deny(s.req("GET", "/erp/ledger/list?beginTime=bad", a.owner, nil))
		deny(s.req("GET", "/erp/stock/list?stockState=anything", a.owner, nil))
		h := get(completed)
		deny(s.req("PUT", fmt.Sprintf("/erp/receipt/%d/confirm", id(h, "inDocumentId")), a.owner, nil))
		deny(s.req("POST", "/erp/purchaseReturn", a.owner, map[string]any{"warehouseId": wb, "requestKey": uuid.NewString(), "sourceDocumentId": h["inDocumentId"], "remark": "invalid transfer return", "lines": []model.ERPLineInput{{SkuID: x, Quantity: 1}}}))
	})
	run("same_key_parallel_posts_once_different_key_only_one_version", func(t *testing.T) {
		tr := create(makeInput(wa, wb, x, 1))
		key := uuid.NewString()
		for _, r := range parallelJSON(s, 4, func(i int) (string, string, string, any) {
			return "POST", fmt.Sprintf("/erp/transfer/%d/dispatch", tr), a.owner, model.ERPTransferOperation{Version: 1, RequestKey: key}
		}) {
			if r.err != nil || r.code != 200 {
				t.Fatal("same-key replay", r.code, r.err)
			}
		}
		assertStock(wa, x, 15, 3)
		results := parallelJSON(s, 4, func(i int) (string, string, string, any) {
			return "POST", fmt.Sprintf("/erp/transfer/%d/receive", tr), a.owner, model.ERPTransferOperation{Version: 2, RequestKey: uuid.NewString()}
		})
		ok := 0
		for _, r := range results {
			if r.err != nil {
				t.Fatal(r.err)
			}
			if r.code == 200 {
				ok++
			}
		}
		if ok != 1 {
			t.Fatal("CAS not unique", ok)
		}
		assertStock(wb, x, 5, 0)
	})
	// Current-session source and target grants are intentionally distinct.
	var employees []int64
	grant := func(uid int64, ws []int64) {
		path := fmt.Sprintf("/erp/warehouse/access/%d", uid)
		r := data(s.req("GET", path, a.owner, nil))
		s.ok(s.req("PUT", path, a.owner, map[string]any{"version": r["version"], "warehouseIds": ws}))
	}
	t.Cleanup(func() {
		for _, uid := range employees {
			grant(uid, []int64{})
		}
	})
	run("directional_scope_revocation_and_cross_tenant", func(t *testing.T) {
		var menus []model.SysMenu
		if e := repository.DB(tenantctx.System(ctx)).Where("menu_id>=6000 AND menu_id<8300").Find(&menus).Error; e != nil {
			t.Fatal(e)
		}
		ids := []int64{}
		for _, m := range menus {
			ids = append(ids, m.MenuID)
		}
		s.ok(s.req("POST", "/system/role", a.owner, map[string]any{"roleName": "仓库增强测试员", "roleKey": "w7_operator", "roleSort": 0, "status": "0", "dataScope": "1", "menuIds": ids, "menuCheckStrictly": true, "deptCheckStrictly": true}))
		rv := s.req("GET", "/system/role/list?roleKey=w7_operator", a.owner, nil)
		s.ok(rv)
		role := rv["rows"].([]any)[0].(map[string]any)["roleId"]
		employee := func(name string) (int64, string) {
			s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": name, "nickName": name, "password": operationsPassword, "status": "0", "roleIds": []any{role}}))
			r := s.req("GET", "/system/user/list?userName="+name, a.owner, nil)
			s.ok(r)
			uid := id(r["rows"].([]any)[0].(map[string]any), "userId")
			employees = append(employees, uid)
			return uid, s.loginAs(a.code, name, operationsPassword)
		}
		su, source := employee("w7_source")
		tu, target := employee("w7_target")
		grant(su, []int64{wa})
		grant(tu, []int64{wb})
		tr := create(makeInput(wa, wb, x, 1))
		deny(action(target, tr, 1, "dispatch", uuid.NewString()))
		key := uuid.NewString()
		s.ok(action(source, tr, 1, "dispatch", key))
		deny(action(source, tr, 2, "receive", uuid.NewString()))
		s.ok(action(target, tr, 2, "receive", uuid.NewString()))
		deny(s.req("GET", fmt.Sprintf("/erp/transfer/%d", tr), b.owner, nil))
		deny(s.req("GET", fmt.Sprintf("/erp/transfer/%d", tr), p, nil))
		// Different actors eliminate the user-row serialization shortcut; two
		// opposite transfers really compete for both warehouse locks.
		grant(su, []int64{wa, wb})
		grant(tu, []int64{wa, wb})
		pair := []int64{create(makeInput(wa, wb, x, 1)), create(makeInput(wb, wa, x, 1))}
		for _, op := range []string{"dispatch", "receive"} {
			version := int64(1)
			if op == "receive" {
				version = 2
			}
			results := parallelJSON(s, 2, func(i int) (string, string, string, any) {
				token := source
				if i == 1 {
					token = target
				}
				return "POST", fmt.Sprintf("/erp/transfer/%d/%s", pair[i], op), token, model.ERPTransferOperation{Version: version, RequestKey: uuid.NewString()}
			})
			for _, r := range results {
				if r.err != nil || r.code != 200 {
					t.Fatal("opposite transfers failed", r.err, r.code)
				}
			}
		}
		grant(su, []int64{})
		deny(action(source, tr, 1, "dispatch", key))
		deny(s.req("GET", fmt.Sprintf("/erp/transfer/%d", tr), source, nil))
		r := s.req("GET", "/erp/transfer/list", source, nil)
		s.ok(r)
		if r["total"] != float64(0) {
			t.Fatal("revoked list visible")
		}
		grant(tu, []int64{})
		deny(s.req("GET", fmt.Sprintf("/erp/stock/alert?warehouseId=%d&skuId=%d", wb, x), target, nil))
	})
	run("transaction_faults_leave_all_stock_document_state_unchanged", func(t *testing.T) {
		doc("receipt", wa, y, 10)
		tr := create(makeInput(wa, wb, y, 2))
		tracked := []string{"erp_stock", "erp_stock_ledger", "erp_document", "erp_document_line", "erp_transfer", "erp_transfer_line", "erp_transfer_action", "sys_oper_log"}
		snapshot := func() map[string]any {
			out := map[string]any{}
			for _, table := range tracked {
				var rows []map[string]any
				if e := repository.TenantTable(repository.DB(ctx), table).Order("tenant_id").Find(&rows).Error; e != nil {
					t.Fatal(e)
				}
				out[table] = rows
			}
			return out
		}
		for _, op := range []string{"DISPATCH", "RECEIVE"} {
			version := int64(1)
			if op == "RECEIVE" {
				s.ok(action(a.owner, tr, 1, "dispatch", uuid.NewString()))
				version = 2
			}
			for _, table := range []string{"erp_document", "erp_document_line", "erp_stock_ledger", "erp_transfer_action", "sys_oper_log"} {
				before := snapshot()
				failure := errors.New("injected warehouse failure")
				cb := repository.DB(ctx).Callback().Create()
				if e := cb.Before("gorm:create").Register("warehouse:fault", func(tx *gorm.DB) {
					if tx.Statement.Table == table {
						tx.AddError(failure)
					}
				}); e != nil {
					t.Fatal(e)
				}
				_, e := service.ERPTransferChange(ctx, tr, op, model.ERPTransferInput{Version: version, RequestKey: uuid.NewString()}, "fault-test")
				cb.Remove("warehouse:fault")
				if !errors.Is(e, failure) {
					t.Fatal("fault not reached", table, e)
				}
				if !reflect.DeepEqual(before, snapshot()) {
					t.Fatal("partial transaction", table)
				}
			}
		}
		assertStock(wa, y, 8, 0)
		assertStock(wb, y, 0, 0)
		if get(tr)["status"] != "IN_TRANSIT" {
			t.Fatal("partial transfer")
		}
	})
	run("draft_reference_freezes_unit_and_requires_explicit_alert_fields", func(t *testing.T) {
		z := master("sku", "W7-DRAFT-UNIT")
		tr := create(makeInput(wa, wb, z, 1))
		v := data(s.req("GET", fmt.Sprintf("/erp/sku/%d", z), a.owner, nil))
		v["unit"] = "箱"
		deny(s.req("PUT", "/erp/sku", a.owner, v))
		s.ok(action(a.owner, tr, 1, "cancel", uuid.NewString()))
		deny(s.req("PUT", "/erp/stock/alert", a.owner, map[string]any{"warehouseId": wa, "skuId": z, "version": 0}))
		deny(s.req("PUT", "/erp/stock/alert", a.owner, map[string]any{"warehouseId": wa, "skuId": z, "thresholdQuantity": 0}))
	})
	run("readonly_reconciliation_and_filtered_XLSX_exports", func(t *testing.T) {
		// Deliberately corrupt only a newly created synthetic SKU: a zero-net
		// COUNT ledger whose balance is missing. Check must report, not repair it.
		z := master("sku", "W7-ORPHAN")
		voucher := model.ERPDocument{WarehouseID: wa, WarehouseName: "W7-A", Kind: "COUNT", Status: "POSTED", Version: 1, DocumentNo: "TEST-ORPHAN-" + uuid.NewString(), RequestKey: uuid.NewString(), RequestHash: strings.Repeat("0", 64)}
		if e := repository.DB(ctx).Transaction(func(tx *gorm.DB) error {
			if e := tx.Create(&voucher).Error; e != nil {
				return e
			}
			if e := tx.Create(&model.ERPDocumentLine{DocumentID: voucher.DocumentID, SkuID: z, SkuCode: "W7-ORPHAN", ProductName: "synthetic zero-count anomaly", Unit: "件"}).Error; e != nil {
				return e
			}
			return tx.Create(&model.ERPLedger{DocumentID: voucher.DocumentID, DocumentNo: voucher.DocumentNo, WarehouseID: wa, SkuID: z, Event: "COUNT"}).Error
		}); e != nil {
			t.Fatal(e)
		}
		orphan := s.req("GET", fmt.Sprintf("/erp/stock/reconcile?warehouseId=%d&skuId=%d", wa, z), a.owner, nil)
		s.ok(orphan)
		if orphan["total"] != float64(1) {
			t.Fatal("missing balance not counted")
		}
		anomaly := orphan["rows"].([]any)[0].(map[string]any)
		if anomaly["balanceMissing"] != true || anomaly["consistent"] != false {
			t.Fatal("zero-net orphan silently accepted")
		}
		var balanceCount int64
		if repository.DB(ctx).Model(&model.ERPStock{}).Where("warehouse_id=? AND sku_id=?", wa, z).Count(&balanceCount).Error != nil || balanceCount != 0 {
			t.Fatal("readonly checker repaired data")
		}
		// Restore only this injected test fixture to a consistent zero balance;
		// no anomaly rows are deleted, and normal business has no such API.
		if e := repository.DB(ctx).Create(&model.ERPStock{WarehouseID: wa, SkuID: z, Version: 1}).Error; e != nil {
			t.Fatal(e)
		}
		r := s.req("GET", "/erp/stock/reconcile?pageSize=100", a.owner, nil)
		s.ok(r)
		for _, v := range r["rows"].([]any) {
			if v.(map[string]any)["consistent"] != true {
				t.Fatal("ledger mismatch")
			}
		}
		for _, kind := range []string{"stock", "ledger"} {
			req, _ := http.NewRequest("POST", fmt.Sprintf("%s/erp/%s/export?warehouseId=%d&skuId=%d", s.base, kind, wa, x), nil)
			req.Header.Set("Authorization", "Bearer "+a.owner)
			res, e := s.client.Do(req)
			if e != nil {
				t.Fatal(e)
			}
			raw, e := io.ReadAll(res.Body)
			res.Body.Close()
			if e != nil {
				t.Fatal(e)
			}
			book, e := excelize.OpenReader(strings.NewReader(string(raw)))
			if e != nil {
				t.Fatal("not xlsx")
			}
			sheet := book.GetSheetName(0)
			rows, e := book.GetRows(sheet)
			book.Close()
			if e != nil || len(rows) < 2 {
				t.Fatal("missing export rows")
			}
			for _, line := range rows[1:] {
				if line[0] != "W7-A" || line[1] != "W7-X" {
					t.Fatal("export scope leak")
				}
			}
		}
	})
}
