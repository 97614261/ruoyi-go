package multitenant_test

import (
	"context"
	"encoding/json"
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

func TestERPFinance(t *testing.T) {
	root := os.Getenv("MT_TEST_ROOT")
	if root == "" {
		t.Skip("explicit dedicated local fixtures required")
	}
	cfg, e := config.Load(filepath.Join(root, "configs/multitenant.local.yml"))
	if e != nil {
		t.Fatal("local config unavailable")
	}
	d, e := mysql.ParseDSN(cfg.MySQL.DSN)
	if e != nil {
		t.Fatal("DSN")
	}
	h, _, e := net.SplitHostPort(d.Addr)
	if e != nil || d.Net != "tcp" || d.DBName != "ruoyi_go_mt" || (h != "127.0.0.1" && h != "localhost" && h != "::1") {
		t.Fatal("local dedicated DB only")
	}
	if repository.Init(cfg.MySQL) != nil {
		t.Fatal("DB unavailable")
	}
	defer repository.Close()
	cache := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	defer cache.Close()
	s := &suite{t: t, root: root, base: "http://127.0.0.1:8081", client: &http.Client{Timeout: 20 * time.Second}, redis: cache, passwords: loadTestAccounts(t, root)}
	platform := s.login("platform")
	t.Cleanup(func() { s.req("POST", "/logout", platform, nil) })
	a, b := newOperationsFixture(t, s, platform), newOperationsFixture(t, s, platform)
	ctx, cancel := context.WithTimeout(erpctx.WithUser(tenantctx.With(context.Background(), a.tenant.TenantID, false), *a.tenant.OwnerUserID), 10*time.Minute)
	defer cancel()
	data := func(r map[string]any) map[string]any { s.ok(r); return r["data"].(map[string]any) }
	id := func(r map[string]any, k string) int64 { return int64(r[k].(float64)) }
	deny := func(r map[string]any) {
		s.t.Helper()
		if r["code"] == float64(200) {
			s.t.Fatal("unexpected financial success")
		}
	}
	run := func(name string, f func(*testing.T)) {
		t.Run(name, func(t *testing.T) { old := s.t; s.t = t; defer func() { s.t = old }(); f(t) })
	}
	master := func(kind, code string) int64 {
		v := map[string]any{"status": "0"}
		if kind == "sku" {
			v["skuCode"] = code
			v["productName"] = "财务测试商品"
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
	wa, wb, sku := master("warehouse", "F8-A"), master("warehouse", "F8-B"), master("sku", "F8-X")
	supplier := id(data(s.req("POST", "/erp/supplier", a.owner, map[string]any{"supplierCode": "F8-S", "supplierName": "财务验收供应商", "status": "0", "phone": "FINANCE_PRIVATE_CONTACT"})), "supplierId")
	accountInput := func(code, currency string) model.FinAccountWrite {
		return model.FinAccountWrite{RequestKey: uuid.NewString(), AccountCode: code, AccountName: "内部记账账户", AccountType: "BANK", Currency: currency, Status: "0"}
	}
	account := id(data(s.req("POST", "/erp/finance/accounts", a.owner, accountInput("F8-BANK", "USD"))), "id")
	type batch struct{ purchase, receipt, line int64 }
	newBatch := func(w, ordered, received int64, price, currency string) batch {
		p := id(data(s.req("POST", "/erp/purchase", a.owner, model.ERPPurchaseWrite{SupplierID: supplier, WarehouseID: w, Currency: currency, RequestKey: uuid.NewString(), Lines: []model.ERPPurchaseLineInput{{SkuID: sku, Quantity: ordered, UnitPrice: price}}})), "purchaseId")
		s.ok(s.req("PUT", fmt.Sprintf("/erp/purchase/%d/confirm", p), a.owner, map[string]any{"version": 1, "requestKey": uuid.NewString()}))
		v := data(s.req("GET", fmt.Sprintf("/erp/purchase/%d", p), a.owner, nil))
		line := id(v["lines"].([]any)[0].(map[string]any), "lineId")
		r := data(s.req("POST", fmt.Sprintf("/erp/purchase/%d/receipts", p), a.owner, model.ERPPurchaseReceiveInput{ERPActionToken: model.ERPActionToken{Version: 2, RequestKey: uuid.NewString()}, Lines: []model.ERPPurchaseReceiveLineInput{{PurchaseLineID: line, Quantity: received}}}))
		receipt := id(r, "receiptId")
		rd := data(s.req("GET", fmt.Sprintf("/erp/purchaseReceipt/%d", receipt), a.owner, nil))
		return batch{p, receipt, id(rd["lines"].([]any)[0].(map[string]any), "lineId")}
	}
	register := func(r batch) int64 {
		return id(data(s.req("POST", "/erp/finance/payables", a.owner, model.FinPayableCreate{ReceiptID: r.receipt, RequestKey: uuid.NewString()})), "id")
	}
	payable := func(p int64) map[string]any {
		return data(s.req("GET", fmt.Sprintf("/erp/finance/payables/%d", p), a.owner, nil))
	}
	payment := func(p int64) map[string]any {
		return data(s.req("GET", fmt.Sprintf("/erp/finance/payments/%d", p), a.owner, nil))
	}
	makePayment := func(ps []int64, amounts []string) model.FinPaymentWrite {
		v := model.FinPaymentWrite{RequestKey: uuid.NewString(), AccountID: account, SupplierID: supplier, Currency: "USD", Amount: "0.0000"}
		for i, p := range ps {
			v.Allocations = append(v.Allocations, model.FinAllocationInput{PayableID: p, Version: id(payable(p), "version"), Amount: amounts[i]})
		}
		return v
	}
	// Amount totals in fixture construction are explicit decimal strings, never
	// floating point. The service verifies the equality independently.
	pay := func(v model.FinPaymentWrite) map[string]any {
		return data(s.req("POST", "/erp/finance/payments", a.owner, v))
	}
	returnBatch := func(r batch, q int64) int64 {
		v := data(s.req("GET", fmt.Sprintf("/erp/purchaseReceipt/%d/returnable", r.receipt), a.owner, nil))
		return id(data(s.req("POST", fmt.Sprintf("/erp/purchaseReceipt/%d/returns", r.receipt), a.owner, model.ERPPurchaseReturnInput{ERPActionToken: model.ERPActionToken{Version: id(v, "version"), RequestKey: uuid.NewString()}, Reason: "财务验收原批退货", Lines: []model.ERPPurchaseReturnLineInput{{ReceiptLineID: r.line, Quantity: q}}})), "returnId")
	}
	adjust := func(p int64, returns ...int64) model.FinAdjust {
		return model.FinAdjust{FinToken: model.FinToken{Version: id(payable(p), "version"), RequestKey: uuid.NewString()}, ReturnIDs: returns}
	}
	assertMoney := func(p int64, gross, credit, paid, due, over string) {
		s.t.Helper()
		v := payable(p)
		for k, want := range map[string]string{"grossAmount": gross, "creditAmount": credit, "paidAmount": paid, "dueAmount": due, "overpaidAmount": over} {
			if v[k] != want {
				s.t.Fatalf("%s=%v want=%s", k, v[k], want)
			}
		}
	}
	run("explicit_receipt_registration_frozen_prices_partial_receipts_and_replay", func(t *testing.T) {
		r := newBatch(wa, 10, 4, "0.1000", "USD")
		before := data(s.req("GET", fmt.Sprintf("/erp/purchase/%d", r.purchase), a.owner, nil))
		sources := s.req("GET", fmt.Sprintf("/erp/finance/sources?receiptId=%d", r.receipt), a.owner, nil)
		s.ok(sources)
		if sources["total"] != float64(1) {
			t.Fatal("source missing / auto posted")
		}
		preview := data(s.req("GET", fmt.Sprintf("/erp/finance/sources/%d", r.receipt), a.owner, nil))
		if preview["grossAmount"] != "0.4000" {
			t.Fatal("ordered rather than received amount")
		}
		in := model.FinPayableCreate{ReceiptID: r.receipt, RequestKey: uuid.NewString()}
		first := data(s.req("POST", "/erp/finance/payables", a.owner, in))
		p := id(first, "id")
		assertMoney(p, "0.4000", "0.0000", "0.0000", "0.4000", "0.0000")
		replay := data(s.req("POST", "/erp/finance/payables", a.owner, in))
		if replay["existing"] != true || id(replay, "id") != p {
			t.Fatal("replay")
		}
		deny(s.req("POST", "/erp/finance/payables", a.owner, model.FinPayableCreate{ReceiptID: r.receipt, RequestKey: uuid.NewString()}))
		after := data(s.req("GET", fmt.Sprintf("/erp/purchase/%d", r.purchase), a.owner, nil))
		if !reflect.DeepEqual(before, after) {
			t.Fatal("finance changed purchase facts")
		}
		body, _ := json.Marshal(preview)
		if strings.Contains(string(body), "FINANCE_PRIVATE_CONTACT") {
			t.Fatal("supplier contact leak")
		}
	})
	run("account_CAS_currency_immutable_zero_and_numeric_amount_rejection", func(t *testing.T) {
		v := data(s.req("GET", fmt.Sprintf("/erp/finance/accounts/%d", account), a.owner, nil))
		in := accountInput("F8-BANK", "USD")
		in.Version = id(v, "version")
		in.AccountName = "改名账户"
		s.ok(s.req("PUT", fmt.Sprintf("/erp/finance/accounts/%d", account), a.owner, in))
		in.RequestKey = uuid.NewString()
		deny(s.req("PUT", fmt.Sprintf("/erp/finance/accounts/%d", account), a.owner, in))
		in.Version++
		in.Currency = "CNY"
		deny(s.req("PUT", fmt.Sprintf("/erp/finance/accounts/%d", account), a.owner, in))
		p := register(newBatch(wa, 1, 1, "0", "USD"))
		assertMoney(p, "0.0000", "0.0000", "0.0000", "0.0000", "0.0000")
		vpay := makePayment([]int64{p}, []string{"0"})
		vpay.Amount = "0"
		deny(s.req("POST", "/erp/finance/payments", a.owner, vpay))
		deny(s.req("POST", "/erp/finance/payments", a.owner, map[string]any{"requestKey": uuid.NewString(), "accountId": account, "supplierId": supplier, "currency": "USD", "amount": 0.1, "allocations": []any{map[string]any{"payableId": p, "version": 1, "amount": "0.1"}}}))
	})
	var spanning, spanningPayableA, spanningPayableB int64
	run("partial_and_multi_bill_payment_reverse_and_original_results", func(t *testing.T) {
		p, q := register(newBatch(wa, 10, 10, "10", "USD")), register(newBatch(wb, 10, 10, "10", "USD"))
		spanningPayableA, spanningPayableB = p, q
		firstIn := makePayment([]int64{p, q}, []string{"20", "30"})
		firstIn.Amount = "50"
		first := pay(firstIn)
		spanning = id(first, "id")
		second := makePayment([]int64{p, q}, []string{"60", "10"})
		second.Amount = "70"
		pay(second)
		assertMoney(p, "100.0000", "0.0000", "80.0000", "20.0000", "0.0000")
		assertMoney(q, "100.0000", "0.0000", "40.0000", "60.0000", "0.0000")
		rev := model.FinReverse{FinToken: model.FinToken{Version: 1, RequestKey: uuid.NewString()}, Reason: "错误付款记账冲销"}
		s.ok(s.req("POST", fmt.Sprintf("/erp/finance/payments/%d/reverse", spanning), a.owner, rev))
		assertMoney(p, "100.0000", "0.0000", "60.0000", "40.0000", "0.0000")
		assertMoney(q, "100.0000", "0.0000", "10.0000", "90.0000", "0.0000")
		replay := pay(firstIn)
		if replay["status"] != "POSTED" || replay["existing"] != true || id(replay, "version") != 1 || payment(spanning)["status"] != "REVERSED" {
			t.Fatal("not original result")
		}
		firstIn.Amount = "51"
		deny(s.req("POST", "/erp/finance/payments", a.owner, firstIn))
		rev.RequestKey = uuid.NewString()
		rev.Version = 2
		deny(s.req("POST", fmt.Sprintf("/erp/finance/payments/%d/reverse", spanning), a.owner, rev))
	})
	run("disabled_account_denies_new_payment_but_preserves_original_replay_and_reversal", func(t *testing.T) {
		ac := id(data(s.req("POST", "/erp/finance/accounts", a.owner, accountInput("F8-DIS", "USD"))), "id")
		p := register(newBatch(wa, 2, 2, "10", "USD"))
		in := makePayment([]int64{p}, []string{"5"})
		in.AccountID, in.Amount = ac, "5"
		posted := pay(in)
		edit := accountInput("F8-DIS", "USD")
		edit.Version, edit.Status = 1, "1"
		s.ok(s.req("PUT", fmt.Sprintf("/erp/finance/accounts/%d", ac), a.owner, edit))
		replay := pay(in)
		if replay["existing"] != true || id(replay, "id") != id(posted, "id") || replay["status"] != "POSTED" {
			t.Fatal("disabled account changed original payment result")
		}
		fresh := makePayment([]int64{p}, []string{"1"})
		fresh.AccountID, fresh.Amount = ac, "1"
		deny(s.req("POST", "/erp/finance/payments", a.owner, fresh))
		assertMoney(p, "20.0000", "0.0000", "5.0000", "15.0000", "0.0000")
		reverse := model.FinReverse{FinToken: model.FinToken{Version: 1, RequestKey: uuid.NewString()}, Reason: "停用账户旧记账冲销"}
		path := fmt.Sprintf("/erp/finance/payments/%d/reverse", id(posted, "id"))
		s.ok(s.req("POST", path, a.owner, reverse))
		s.ok(s.req("POST", path, a.owner, reverse))
		assertMoney(p, "20.0000", "0.0000", "0.0000", "20.0000", "0.0000")
	})
	run("original_batch_credit_blocks_stale_payment_and_exposes_overpayment", func(t *testing.T) {
		r := newBatch(wa, 10, 10, "10", "USD")
		p := register(r)
		v := makePayment([]int64{p}, []string{"80"})
		v.Amount = "80"
		paid := id(pay(v), "id")
		ret := returnBatch(r, 5)
		if payable(p)["pendingReturnCount"] != float64(1) {
			t.Fatal("unregistered return not exposed")
		}
		v = makePayment([]int64{p}, []string{"10"})
		v.Amount = "10"
		deny(s.req("POST", "/erp/finance/payments", a.owner, v))
		in := adjust(p, ret)
		s.ok(s.req("POST", fmt.Sprintf("/erp/finance/payables/%d/adjust", p), a.owner, in))
		assertMoney(p, "100.0000", "50.0000", "80.0000", "0.0000", "30.0000")
		duplicate := adjust(p, ret)
		deny(s.req("POST", fmt.Sprintf("/erp/finance/payables/%d/adjust", p), a.owner, duplicate))
		s.ok(s.req("POST", fmt.Sprintf("/erp/finance/payments/%d/reverse", paid), a.owner, model.FinReverse{FinToken: model.FinToken{Version: 1, RequestKey: uuid.NewString()}, Reason: "测试撤销记账"}))
		old := data(s.req("POST", fmt.Sprintf("/erp/finance/payables/%d/adjust", p), a.owner, in))
		if old["existing"] != true || id(old, "version") != 3 {
			t.Fatal("credit original result")
		}
		assertMoney(p, "100.0000", "50.0000", "0.0000", "50.0000", "0.0000")
	})
	run("same_key_parallel_once_different_keys_single_version_winner", func(t *testing.T) {
		p := register(newBatch(wa, 10, 10, "10", "USD"))
		in := makePayment([]int64{p}, []string{"10"})
		in.Amount = "10"
		for _, r := range parallelJSON(s, 4, func(i int) (string, string, string, any) { return "POST", "/erp/finance/payments", a.owner, in }) {
			if r.err != nil || r.code != 200 {
				t.Fatal(r)
			}
		}
		assertMoney(p, "100.0000", "0.0000", "10.0000", "90.0000", "0.0000")
		in = makePayment([]int64{p}, []string{"10"})
		in.Amount = "10"
		winners := 0
		for _, r := range parallelJSON(s, 4, func(i int) (string, string, string, any) {
			v := in
			v.RequestKey = uuid.NewString()
			return "POST", "/erp/finance/payments", a.owner, v
		}) {
			if r.err != nil {
				t.Fatal(r.err)
			}
			if r.code == 200 {
				winners++
			}
		}
		if winners != 1 {
			t.Fatal("multiple CAS winners", winners)
		}
	})
	employees := []int64{}
	grant := func(uid int64, warehouses []int64) {
		path := fmt.Sprintf("/erp/warehouse/access/%d", uid)
		v := data(s.req("GET", path, a.owner, nil))
		s.ok(s.req("PUT", path, a.owner, map[string]any{"version": v["version"], "warehouseIds": warehouses}))
	}
	t.Cleanup(func() {
		for _, uid := range employees {
			grant(uid, []int64{})
		}
	})
	run("independent_finance_permission_all_warehouses_cross_tenant_and_live_revocation", func(t *testing.T) {
		menuIDs := []int64{6000, 8300, 8301, 8302, 8303, 8400, 8401, 8402, 8403, 8404, 8500, 8501, 8502, 8503, 8600, 8601}
		s.ok(s.req("POST", "/system/role", a.owner, map[string]any{"roleName": "财务验收员", "roleKey": "f8_finance", "roleSort": 0, "status": "0", "dataScope": "1", "menuIds": menuIDs}))
		roles := s.req("GET", "/system/role/list?roleKey=f8_finance", a.owner, nil)
		s.ok(roles)
		rid := id(roles["rows"].([]any)[0].(map[string]any), "roleId")
		makeUser := func(name string, role int64) (int64, string) {
			s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": name, "nickName": name, "password": operationsPassword, "status": "0", "roleIds": []int64{role}}))
			users := s.req("GET", "/system/user/list?userName="+name, a.owner, nil)
			s.ok(users)
			uid := id(users["rows"].([]any)[0].(map[string]any), "userId")
			employees = append(employees, uid)
			return uid, s.loginAs(a.code, name, operationsPassword)
		}
		u, one := makeUser("finone", rid)
		w, two := makeUser("fintwo", rid)
		grant(u, []int64{wa})
		grant(w, []int64{wa, wb})
		deny(s.req("GET", fmt.Sprintf("/erp/finance/payments/%d", spanning), one, nil))
		list := s.req("GET", "/erp/finance/payments?pageSize=100", one, nil)
		s.ok(list)
		for _, x := range list["rows"].([]any) {
			if id(x.(map[string]any), "paymentId") == spanning {
				t.Fatal("partial payment total leak")
			}
		}
		grant(u, []int64{wa, wb})
		s.ok(s.req("GET", fmt.Sprintf("/erp/finance/payments/%d", spanning), one, nil))
		p := register(newBatch(wa, 10, 10, "10", "USD"))
		v := makePayment([]int64{p}, []string{"10"})
		v.Amount = "10"
		tokens := []string{one, two}
		requests := []model.FinPaymentWrite{v, v}
		for i := range requests {
			requests[i].RequestKey = uuid.NewString()
		}
		win := 0
		winner := -1
		for _, r := range parallelJSON(s, 2, func(i int) (string, string, string, any) {
			return "POST", "/erp/finance/payments", tokens[i], requests[i]
		}) {
			if r.err != nil {
				t.Fatal(r.err)
			}
			if r.code == 200 {
				win++
				winner = r.index
			}
		}
		if win != 1 {
			t.Fatal("cross-employee double allocation", win)
		}
		// A physical return and finance allocation use different employees but
		// the same supplier lock. Either allocation wins before the return, or
		// is refused because that return is now awaiting recognition.
		raceBatch := newBatch(wa, 10, 10, "10", "USD")
		racePayable := register(raceBatch)
		racePayment := makePayment([]int64{racePayable}, []string{"100"})
		racePayment.Amount = "100"
		raceReturn := model.ERPPurchaseReturnInput{ERPActionToken: model.ERPActionToken{Version: 1, RequestKey: uuid.NewString()}, Reason: "并发退货", Lines: []model.ERPPurchaseReturnLineInput{{ReceiptLineID: raceBatch.line, Quantity: 1}}}
		for _, r := range parallelJSON(s, 2, func(i int) (string, string, string, any) {
			if i == 0 {
				return "POST", "/erp/finance/payments", one, racePayment
			}
			return "POST", fmt.Sprintf("/erp/purchaseReceipt/%d/returns", raceBatch.receipt), a.owner, raceReturn
		}) {
			if r.err != nil || r.index == 1 && r.code != 200 {
				t.Fatal("return/payment race", r)
			}
		}
		raceState := payable(racePayable)
		if raceState["pendingReturnCount"] != float64(1) || (raceState["paidAmount"] != "0.0000" && raceState["paidAmount"] != "100.0000") {
			t.Fatal("nonserial return/payment state")
		}
		grant([]int64{u, w}[winner], []int64{})
		deny(s.req("POST", "/erp/finance/payments", tokens[winner], requests[winner]))
		grant(u, []int64{})
		deny(s.req("GET", fmt.Sprintf("/erp/finance/payables/%d", p), one, nil))
		deny(s.req("GET", fmt.Sprintf("/erp/finance/payments/%d", spanning), one, nil))
		for _, kind := range []string{"payables", "payments", "statements"} {
			r := s.req("GET", "/erp/finance/"+kind, one, nil)
			s.ok(r)
			if r["total"] != float64(0) {
				t.Fatal("revoked financial list visible", kind)
			}
		}
		for _, token := range []string{b.owner, platform} {
			deny(s.req("GET", fmt.Sprintf("/erp/finance/payables/%d", p), token, nil))
			deny(s.req("GET", fmt.Sprintf("/erp/finance/accounts/%d", account), token, nil))
			deny(s.req("POST", "/erp/finance/payments", token, v))
		}
		s.ok(s.req("POST", "/system/role", a.owner, map[string]any{"roleName": "仅收货财务隔离", "roleKey": "f8_receive", "roleSort": 0, "status": "0", "dataScope": "1", "menuIds": []int64{6000, 7900}}))
		rr := s.req("GET", "/system/role/list?roleKey=f8_receive", a.owner, nil)
		s.ok(rr)
		receiverID, receiver := makeUser("finreceiver", id(rr["rows"].([]any)[0].(map[string]any), "roleId"))
		grant(receiverID, []int64{wa})
		for _, path := range []string{"/erp/finance/payables", "/erp/finance/accounts", "/erp/finance/sources", "/erp/finance/statements"} {
			deny(s.req("GET", path, receiver, nil))
		}
		grant(w, []int64{})
	})
	run("create_payment_credit_reverse_faults_are_atomic", func(t *testing.T) {
		tracked := map[string]string{"erp_fin_account": "account_id", "erp_fin_payable": "payable_id", "erp_fin_payable_line": "line_id", "erp_fin_credit": "credit_id", "erp_fin_payment": "payment_id", "erp_fin_allocation": "allocation_id", "erp_fin_action": "action_id", "sys_oper_log": "oper_id", "erp_stock": "stock_id", "erp_stock_ledger": "ledger_id"}
		snapshot := func() map[string]any {
			out := map[string]any{}
			for table, pk := range tracked {
				var rows []map[string]any
				q := repository.TenantTable(repository.DB(ctx), table)
				if table == "sys_oper_log" {
					q = q.Where("title='ERP采购财务'")
				}
				if e := q.Order(pk).Find(&rows).Error; e != nil {
					t.Fatal(e)
				}
				out[table] = rows
			}
			return out
		}
		r := newBatch(wa, 10, 10, "10", "USD")
		p := register(newBatch(wa, 10, 10, "10", "USD"))
		rcredit := newBatch(wa, 10, 10, "10", "USD")
		pc := register(rcredit)
		ret := returnBatch(rcredit, 1)
		basePayment := makePayment([]int64{p}, []string{"10"})
		basePayment.Amount = "10"
		posted := id(pay(basePayment), "id")
		type scenario struct {
			name   string
			tables []string
			call   func() error
		}
		cases := []scenario{
			{"payable", []string{"erp_fin_payable", "erp_fin_payable_line", "erp_fin_action", "sys_oper_log"}, func() error {
				_, e := service.CreateFinancePayable(ctx, model.FinPayableCreate{ReceiptID: r.receipt, RequestKey: uuid.NewString()}, "fault-test")
				return e
			}},
			{"payment", []string{"erp_fin_payment", "erp_fin_allocation", "erp_fin_action", "sys_oper_log"}, func() error {
				v := makePayment([]int64{p}, []string{"10"})
				v.Amount = "10"
				_, e := service.CreateFinancePayment(ctx, v, "fault-test")
				return e
			}},
			{"credit", []string{"erp_fin_credit", "erp_fin_action", "sys_oper_log"}, func() error { _, e := service.AdjustFinancePayable(ctx, pc, adjust(pc, ret), "fault-test"); return e }},
			{"reverse", []string{"erp_fin_action", "sys_oper_log"}, func() error {
				_, e := service.ReverseFinancePayment(ctx, posted, model.FinReverse{FinToken: model.FinToken{Version: 1, RequestKey: uuid.NewString()}, Reason: "故障回滚测试"}, "fault-test")
				return e
			}},
		}
		for _, c := range cases {
			for _, table := range c.tables {
				before := snapshot()
				failure := errors.New("injected financial fault")
				cb := repository.DB(ctx).Callback().Create()
				if e := cb.Before("gorm:create").Register("finance:fault", func(tx *gorm.DB) {
					if tx.Statement.Table == table {
						tx.AddError(failure)
					}
				}); e != nil {
					t.Fatal(e)
				}
				e := c.call()
				cb.Remove("finance:fault")
				if !errors.Is(e, failure) {
					t.Fatal(c.name, table, "fault not reached", e)
				}
				if !reflect.DeepEqual(before, snapshot()) {
					t.Fatal("partial financial transaction", c.name, table)
				}
			}
		}
	})
	run("scoped_statement_currency_separation_exports_and_private_DTOs", func(t *testing.T) {
		cny := register(newBatch(wa, 2, 2, "0.1", "CNY"))
		bad := makePayment([]int64{cny}, []string{"0.1"})
		bad.Amount = "0.1"
		deny(s.req("POST", "/erp/finance/payments", a.owner, bad))
		st := s.req("GET", "/erp/finance/statements?pageSize=100", a.owner, nil)
		s.ok(st)
		currencies := map[string]bool{}
		for _, r := range st["rows"].([]any) {
			currencies[r.(map[string]any)["currency"].(string)] = true
		}
		if !currencies["USD"] || !currencies["CNY"] {
			t.Fatal("currencies mixed", st["total"])
		}
		for _, path := range []string{"payables", "statements"} {
			req, _ := http.NewRequest("POST", s.base+"/erp/finance/"+path+fmt.Sprintf("/export?warehouseId=%d", wa), nil)
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
			if res.Header.Get("Cache-Control") != "no-store" {
				t.Fatal("cache header")
			}
			f, e := excelize.OpenReader(strings.NewReader(string(raw)))
			if e != nil {
				t.Fatal("not XLSX", res.Header.Get("Content-Type"))
			}
			rows, e := f.GetRows(f.GetSheetList()[0])
			f.Close()
			if e != nil || len(rows) < 2 {
				t.Fatal("empty export", e)
			}
		}
		raw, _ := json.Marshal(s.req("GET", fmt.Sprintf("/erp/finance/payables/%d", spanningPayableA), a.owner, nil))
		for _, secret := range []string{"requestHash", "resultJson", "FINANCE_PRIVATE_CONTACT"} {
			if strings.Contains(string(raw), secret) {
				t.Fatal("private payload leak", secret)
			}
		}
		if payable(spanningPayableB)["warehouseId"] != float64(wb) {
			t.Fatal("wrong source warehouse")
		}
	})
}
