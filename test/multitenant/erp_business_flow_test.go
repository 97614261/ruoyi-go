package multitenant_test

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
)

// TestERPBusinessFlow keeps one catalog SKU, warehouse, purchase receipt and
// sales order through both stock and financial lifecycles. Per-module suites
// remain responsible for broader permission, concurrency and fault coverage.
func TestERPBusinessFlow(t *testing.T) {
	runERPBusinessFlow(t, false)
}

func runERPBusinessFlow(t *testing.T, loseResponses bool) {
	t.Helper()
	root := os.Getenv("MT_TEST_ROOT")
	if root == "" {
		t.Skip("explicit dedicated local acceptance required")
	}
	cfg, err := config.Load(filepath.Join(root, "configs/multitenant.local.yml"))
	if err != nil {
		t.Fatal("local config unavailable")
	}
	d, err := mysql.ParseDSN(cfg.MySQL.DSN)
	if err != nil {
		t.Fatal("local DSN unavailable")
	}
	host, _, err := net.SplitHostPort(d.Addr)
	local := func(h string) bool { return h == "localhost" || h == "127.0.0.1" || h == "::1" }
	if err != nil || d.Net != "tcp" || d.DBName != repository.TenantDatabaseName || !local(host) {
		t.Fatal("dedicated local DB only")
	}
	rh, _, err := net.SplitHostPort(cfg.Redis.Addr)
	if err != nil || !local(rh) || cfg.Redis.DB != 13 {
		t.Fatal("dedicated local Redis only")
	}
	if err = repository.Init(cfg.MySQL); err != nil {
		t.Fatal("repository unavailable")
	}
	t.Cleanup(func() { _ = repository.Close() })
	cache := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	t.Cleanup(func() { _ = cache.Close() })
	s := &suite{t: t, root: root, base: erpEnablementTestBase(t, cfg), client: &http.Client{Timeout: 20 * time.Second}, redis: cache, passwords: loadTestAccounts(t, root)}
	p := s.login("platform")
	t.Cleanup(func() { s.req("POST", "/logout", p, nil) })
	a, b := newOperationsFixture(t, s, p), newOperationsFixture(t, s, p)
	t.Cleanup(func() { s.req("POST", "/logout", a.owner, nil); s.req("POST", "/logout", b.owner, nil) })
	t.Logf("business flow fixtures tenantIds=%d,%d", a.tenant.TenantID, b.tenant.TenantID)

	object := func(v any) map[string]any {
		s.t.Helper()
		m, ok := v.(map[string]any)
		if !ok {
			s.t.Fatal("expected object")
		}
		return m
	}
	data := func(v map[string]any) map[string]any { s.t.Helper(); s.ok(v); return object(v["data"]) }
	get := func(path string) map[string]any { return data(s.req("GET", path, a.owner, nil)) }
	beforeRetry := map[string]func(){}
	losses := 0
	expectLoss := func(method, path string, verifyCommitted func()) {
		if loseResponses {
			beforeRetry[method+" "+path] = verifyCommitted
		}
	}
	request := func(method, path string, body any) map[string]any {
		s.t.Helper()
		if verify, ok := beforeRetry[method+" "+path]; ok {
			delete(beforeRetry, method+" "+path)
			loseERPResponseAfterCommit(s.t, s.base, a.owner, method, path, body)
			// Verify via separate reads BEFORE the retry. The retry must return
			// the persisted original result, never execute the business again.
			verify()
			result := s.req(method, path, a.owner, body)
			s.ok(result)
			if object(result["data"])["existing"] != true {
				s.t.Fatal("lost response retry executed instead of replaying")
			}
			losses++
			s.t.Logf("committed effect verified before original-key retry: %s %s", method, path)
			return result
		}
		return s.req(method, path, a.owner, body)
	}
	post := func(path string, body any) map[string]any { return data(request("POST", path, body)) }
	number := func(m map[string]any, k string) int64 {
		s.t.Helper()
		v, ok := m[k].(float64)
		if !ok || v != float64(int64(v)) {
			s.t.Fatalf("%s must be an integer", k)
		}
		return int64(v)
	}
	id := func(m map[string]any, k string) int64 {
		s.t.Helper()
		v := number(m, k)
		if v < 1 {
			s.t.Fatalf("%s must be positive", k)
		}
		return v
	}
	rows := func(path string) []any {
		s.t.Helper()
		v := s.req("GET", path, a.owner, nil)
		s.ok(v)
		r, ok := v["rows"].([]any)
		if !ok {
			s.t.Fatal("expected table rows")
		}
		if number(v, "total") != int64(len(r)) {
			s.t.Fatal("unexpected pagination or hidden rows")
		}
		return r
	}
	one := func(path string) map[string]any {
		s.t.Helper()
		r := rows(path)
		if len(r) != 1 {
			s.t.Fatalf("expected one row: %s", path)
		}
		return object(r[0])
	}
	line := func(m map[string]any) map[string]any {
		s.t.Helper()
		r, ok := m["lines"].([]any)
		if !ok || len(r) != 1 {
			s.t.Fatal("expected one source line")
		}
		return object(r[0])
	}
	deny := func(v map[string]any) {
		s.t.Helper()
		if v["code"] == float64(200) {
			s.t.Fatal("invalid/foreign operation accepted")
		}
	}
	money := func(v map[string]any, want map[string]string) {
		s.t.Helper()
		for k, w := range want {
			if value, ok := v[k].(string); !ok || value != w {
				s.t.Fatalf("%s=%v, want decimal string %s", k, v[k], w)
			}
		}
	}
	replay := func(v map[string]any, k string, want int64) {
		s.t.Helper()
		if v["existing"] != true || id(v, k) != want {
			s.t.Fatal("original request did not replay its original identity")
		}
	}
	stage := func(name string, fn func(*testing.T)) bool {
		return t.Run(name, func(t *testing.T) { old := s.t; s.t = t; defer func() { s.t = old }(); fn(t) })
	}
	var productID, sku, warehouse, supplier, shop, item, account, purchase, receipt, receiptLine, payable, order, pkg, packageLine, receivable int64
	var purchaseCreate model.ERPPurchaseWrite
	var receive model.ERPPurchaseReceiveInput
	var purchaseReturn model.ERPPurchaseReturnInput
	var salesReturn model.ERPSalesReturnInput
	var firstPayment, lastPayment model.FinPaymentWrite
	var firstCollection, lastCollection model.SalesFinCollectionWrite
	var purchaseReturnID, salesReturnID, firstPaymentID, lastPaymentID, firstCollectionID, lastCollectionID int64
	var review, ship map[string]any
	var originalPurchaseLine, originalReceiptLine map[string]any
	var originalOrder, originalPackage map[string]any
	var catalogCreate model.CatalogWrite
	stockQuery := func() string { return fmt.Sprintf("warehouseId=%d&skuId=%d&pageSize=100", warehouse, sku) }
	inventory := func() map[string]any {
		return map[string]any{"stock": rows("/erp/stock/list?" + stockQuery()), "ledger": rows("/erp/ledger/list?" + stockQuery())}
	}
	balance := func(hand, reserved int64) {
		s.t.Helper()
		v := one("/erp/stock/list?" + stockQuery())
		if number(v, "onHand") != hand || number(v, "reserved") != reserved || number(v, "available") != hand-reserved {
			s.t.Fatalf("stock must be hand=%d reserved=%d available=%d", hand, reserved, hand-reserved)
		}
		r := one("/erp/stock/reconcile?" + stockQuery())
		if r["consistent"] != true || number(r, "ledgerOnHand") != hand || number(r, "ledgerReserved") != reserved || number(r, "onHandDifference") != 0 || number(r, "reservedDifference") != 0 {
			s.t.Fatal("balance and immutable ledger disagree")
		}
	}
	assertInventoryUnchanged := func(before map[string]any) {
		s.t.Helper()
		if !reflect.DeepEqual(before, inventory()) {
			s.t.Fatal("bookkeeping/rejected/replayed operation changed inventory")
		}
	}
	getAP := func() map[string]any { return get(fmt.Sprintf("/erp/finance/payables/%d", payable)) }
	getAR := func() map[string]any { return get(fmt.Sprintf("/erp/salesFinance/receivables/%d", receivable)) }
	pay := func(amount string) model.FinPaymentWrite {
		return model.FinPaymentWrite{RequestKey: uuid.NewString(), AccountID: account, SupplierID: supplier, Currency: "CNY", Amount: amount, Allocations: []model.FinAllocationInput{{PayableID: payable, Version: id(getAP(), "version"), Amount: amount}}}
	}
	collect := func(amount string) model.SalesFinCollectionWrite {
		return model.SalesFinCollectionWrite{RequestKey: uuid.NewString(), AccountID: account, ShopID: shop, Currency: "CNY", Amount: amount, Allocations: []model.SalesFinAllocationInput{{ReceivableID: receivable, Version: id(getAR(), "version"), Amount: amount}}}
	}

	if !stage("catalog_SKU_is_shared_by_purchase_and_shop_mapping", func(t *testing.T) {
		barcode := "0001234567890"
		catalogCreate = model.CatalogWrite{RequestKey: uuid.NewString(), ProductCode: "FLOW-P", ProductName: "联动原商品名称", Variants: []model.CatalogVariant{{SkuCode: "FLOW-SKU", Specification: "蓝色", Unit: "件", Status: "0", Barcode: &barcode}}}
		created := post("/erp/product", catalogCreate)
		ids, ok := created["productIds"].([]any)
		if !ok || len(ids) != 1 {
			t.Fatal("one product required")
		}
		productID = id(map[string]any{"id": ids[0]}, "id")
		v := get(fmt.Sprintf("/erp/product/%d", productID))
		variants, ok := v["variants"].([]any)
		if !ok || len(variants) != 1 {
			t.Fatal("one catalog SKU required")
		}
		variant := object(variants[0])
		sku = id(variant, "skuId")
		if variant["barcode"] != barcode {
			t.Fatal("barcode lost leading zeros")
		}
		replayed := post("/erp/product", catalogCreate)
		if replayed["existing"] != true || !reflect.DeepEqual(replayed["productIds"], created["productIds"]) {
			t.Fatal("catalog duplicate created")
		}
		s.ok(s.req("POST", "/erp/warehouse", a.owner, map[string]any{"warehouseCode": "FLOW-W", "warehouseName": "联动仓", "status": "0"}))
		warehouse = id(one("/erp/warehouse/list?warehouseCode=FLOW-W"), "warehouseId")
		supplier = id(post("/erp/supplier", map[string]any{"supplierCode": "FLOW-SUP", "supplierName": "联动供应商", "status": "0"}), "supplierId")
		shop = id(post("/erp/shop", map[string]any{"shopCode": "FLOW-SHOP", "shopName": "联动店铺", "channelCode": "MANUAL", "siteCode": "CN", "currency": "CNY", "status": "0"}), "shopId")
		item = id(post("/erp/shopItem", map[string]any{"shopId": shop, "shopItemCode": "FLOW-ITEM", "title": "联动店铺变种", "status": "0"}), "itemId")
		s.ok(s.req("PUT", fmt.Sprintf("/erp/shopItem/%d/mapping", item), a.owner, map[string]any{"version": 1, "skuId": sku}))
		account = id(post("/erp/finance/accounts", model.FinAccountWrite{RequestKey: uuid.NewString(), AccountCode: "FLOW-ACCOUNT", AccountName: "内部联动记账账户", AccountType: "BANK", Currency: "CNY", Status: "0"}), "id")
	}) {
		return
	}
	if !stage("purchase_receipt_creates_ten_units_and_exact_payable", func(t *testing.T) {
		purchaseCreate = model.ERPPurchaseWrite{SupplierID: supplier, WarehouseID: warehouse, Currency: "CNY", RequestKey: uuid.NewString(), Lines: []model.ERPPurchaseLineInput{{SkuID: sku, Quantity: 10, UnitPrice: "2.3500"}}}
		purchase = id(post("/erp/purchase", purchaseCreate), "purchaseId")
		s.ok(s.req("PUT", fmt.Sprintf("/erp/purchase/%d/confirm", purchase), a.owner, model.ERPActionToken{Version: 1, RequestKey: uuid.NewString()}))
		p := get(fmt.Sprintf("/erp/purchase/%d", purchase))
		originalPurchaseLine = line(p)
		if id(originalPurchaseLine, "skuId") != sku {
			t.Fatal("purchase allocated another SKU")
		}
		receive = model.ERPPurchaseReceiveInput{ERPActionToken: model.ERPActionToken{Version: id(p, "version"), RequestKey: uuid.NewString()}, Lines: []model.ERPPurchaseReceiveLineInput{{PurchaseLineID: id(originalPurchaseLine, "lineId"), Quantity: 10}}}
		receipt = id(post(fmt.Sprintf("/erp/purchase/%d/receipts", purchase), receive), "receiptId")
		originalPurchaseLine = line(get(fmt.Sprintf("/erp/purchase/%d", purchase)))
		originalReceiptLine = line(get(fmt.Sprintf("/erp/purchaseReceipt/%d", receipt)))
		receiptLine = id(originalReceiptLine, "lineId")
		balance(10, 0)
		before := inventory()
		payable = id(post("/erp/finance/payables", model.FinPayableCreate{ReceiptID: receipt, RequestKey: uuid.NewString()}), "id")
		money(getAP(), map[string]string{"grossAmount": "23.5000", "creditAmount": "0.0000", "paidAmount": "0.0000", "dueAmount": "23.5000"})
		firstPayment = pay("10.0000")
		expectLoss("POST", "/erp/finance/payments", func() {
			money(getAP(), map[string]string{"paidAmount": "10.0000", "dueAmount": "13.5000"})
			if len(rows("/erp/finance/payments?pageSize=100")) != 1 {
				s.t.Fatal("lost response payment did not commit exactly one record")
			}
			balance(10, 0)
		})
		firstPaymentID = id(post("/erp/finance/payments", firstPayment), "id")
		money(getAP(), map[string]string{"paidAmount": "10.0000", "dueAmount": "13.5000"})
		assertInventoryUnchanged(before)
		// Rename through010, then prove old purchase/receipt facts do not drift.
		catalog := get(fmt.Sprintf("/erp/product/%d", productID))
		raw, e := json.Marshal(catalog)
		if e != nil {
			t.Fatal(e)
		}
		var edit model.CatalogWrite
		if e = json.Unmarshal(raw, &edit); e != nil {
			t.Fatal(e)
		}
		edit.RequestKey = uuid.NewString()
		edit.ProductName = "联动改名后的商品"
		s.ok(s.req("PUT", fmt.Sprintf("/erp/product/%d", productID), a.owner, edit))
		if !reflect.DeepEqual(originalPurchaseLine, line(get(fmt.Sprintf("/erp/purchase/%d", purchase)))) || !reflect.DeepEqual(originalReceiptLine, line(get(fmt.Sprintf("/erp/purchaseReceipt/%d", receipt)))) {
			t.Fatal("catalog rename rewrote historical purchase snapshots")
		}
	}) {
		return
	}
	if !stage("purchase_return_blocks_payment_until_credit_and_preserves_gross_receipt", func(t *testing.T) {
		v := get(fmt.Sprintf("/erp/purchaseReceipt/%d/returnable", receipt))
		purchaseReturn = model.ERPPurchaseReturnInput{ERPActionToken: model.ERPActionToken{Version: id(v, "version"), RequestKey: uuid.NewString()}, Reason: "联动采购退回两件", Lines: []model.ERPPurchaseReturnLineInput{{ReceiptLineID: receiptLine, Quantity: 2}}}
		expectLoss("POST", fmt.Sprintf("/erp/purchaseReceipt/%d/returns", receipt), func() {
			balance(8, 0)
			if number(get(fmt.Sprintf("/erp/purchaseReceipt/%d/returnable", receipt)), "returnedQuantity") != 2 {
				s.t.Fatal("lost response purchase return did not commit")
			}
		})
		purchaseReturnID = id(post(fmt.Sprintf("/erp/purchaseReceipt/%d/returns", receipt), purchaseReturn), "returnId")
		balance(8, 0)
		before := inventory()
		unchangedAP := getAP()
		if number(unchangedAP, "pendingReturnCount") != 1 {
			t.Fatal("purchase return must remain pending finance confirmation")
		}
		deny(s.req("POST", "/erp/finance/payments", a.owner, pay("1.0000")))
		if !reflect.DeepEqual(unchangedAP, getAP()) {
			t.Fatal("rejected payment changed payable")
		}
		post(fmt.Sprintf("/erp/finance/payables/%d/adjust", payable), model.FinAdjust{FinToken: model.FinToken{Version: id(getAP(), "version"), RequestKey: uuid.NewString()}, ReturnIDs: []int64{purchaseReturnID}})
		money(getAP(), map[string]string{"grossAmount": "23.5000", "creditAmount": "4.7000", "paidAmount": "10.0000", "dueAmount": "8.8000"})
		lastPayment = pay("8.8000")
		lastPaymentID = id(post("/erp/finance/payments", lastPayment), "id")
		money(getAP(), map[string]string{"netAmount": "18.8000", "paidAmount": "18.8000", "dueAmount": "0.0000", "overpaidAmount": "0.0000"})
		assertInventoryUnchanged(before)
		v = get(fmt.Sprintf("/erp/purchaseReceipt/%d/returnable", receipt))
		if number(v, "receivedQuantity") != 10 || number(v, "returnedQuantity") != 2 || number(v, "netReceivedQuantity") != 8 {
			t.Fatal("purchase gross/return/net facts drifted")
		}
	}) {
		return
	}
	if !stage("shop_order_reserves_and_ships_the_same_catalog_SKU", func(t *testing.T) {
		order = id(post("/erp/order", map[string]any{"shopId": shop, "sourceOrderNo": "FLOW-ORDER", "requestKey": uuid.NewString(), "receiverName": "FLOW_PRIVATE_NAME", "receiverPhone": "FLOW_PRIVATE_PHONE", "receiverCountry": "CN", "receiverAddress": "FLOW_PRIVATE_ADDRESS", "orderedAt": "2026-10-25 08:00:00", "originalAmount": "59.9700", "currency": "CNY", "lines": []model.ERPOrderLineInput{{SourceLineNo: "1", ShopItemCode: "FLOW-ITEM", SourceTitle: "店铺原商品标题", Quantity: 3}}}), "orderId")
		balance(8, 0)
		review = map[string]any{"version": 1, "requestKey": uuid.NewString(), "warehouseId": warehouse}
		v := data(s.req("PUT", fmt.Sprintf("/erp/order/%d/review", order), a.owner, review))
		pkg = id(v, "packageId")
		balance(8, 3)
		packageView := get(fmt.Sprintf("/erp/package/%d", pkg))
		packageLine = id(line(packageView), "lineId")
		if id(line(packageView), "skuId") != sku {
			t.Fatal("mapping resolved a different SKU")
		}
		s.ok(s.req("PUT", fmt.Sprintf("/erp/package/%d/logistics", pkg), a.owner, map[string]any{"version": 1, "requestKey": uuid.NewString(), "carrierCode": "MANUAL", "trackingNo": "FLOW-TRACKING"}))
		ship = map[string]any{"version": 2, "requestKey": uuid.NewString()}
		expectLoss("PUT", fmt.Sprintf("/erp/package/%d/ship", pkg), func() {
			balance(5, 0)
			if get(fmt.Sprintf("/erp/order/%d", order))["status"] != "SHIPPED" {
				s.t.Fatal("lost response shipment did not commit")
			}
		})
		s.ok(request("PUT", fmt.Sprintf("/erp/package/%d/ship", pkg), ship))
		balance(5, 0)
		originalOrder = get(fmt.Sprintf("/erp/order/%d", order))
		originalPackage = get(fmt.Sprintf("/erp/package/%d", pkg))
		if originalOrder["status"] != "SHIPPED" || originalPackage["status"] != "SHIPPED" {
			t.Fatal("shipment facts missing")
		}
		before := inventory()
		receivable = id(post("/erp/salesFinance/receivables", model.SalesFinReceivableCreate{OrderID: order, RequestKey: uuid.NewString()}), "id")
		money(getAR(), map[string]string{"grossAmount": "59.9700", "creditAmount": "0.0000", "receivedAmount": "0.0000", "dueAmount": "59.9700"})
		firstCollection = collect("20.0000")
		expectLoss("POST", "/erp/salesFinance/collections", func() {
			money(getAR(), map[string]string{"receivedAmount": "20.0000", "dueAmount": "39.9700"})
			if len(rows("/erp/salesFinance/collections?pageSize=100")) != 1 {
				s.t.Fatal("lost response collection did not commit exactly one record")
			}
			balance(5, 0)
		})
		firstCollectionID = id(post("/erp/salesFinance/collections", firstCollection), "id")
		money(getAR(), map[string]string{"receivedAmount": "20.0000", "dueAmount": "39.9700"})
		assertInventoryUnchanged(before)
	}) {
		return
	}
	if !stage("sales_return_blocks_collection_until_manual_credit_without_rewriting_shipment", func(t *testing.T) {
		v := get(fmt.Sprintf("/erp/package/%d/returnable", pkg))
		salesReturn = model.ERPSalesReturnInput{ERPActionToken: model.ERPActionToken{Version: id(v, "version"), RequestKey: uuid.NewString()}, Reason: "联动销售退回一件", Lines: []model.ERPSalesReturnLineInput{{PackageLineID: packageLine, Quantity: 1}}}
		expectLoss("POST", fmt.Sprintf("/erp/package/%d/returns", pkg), func() {
			balance(6, 0)
			if number(get(fmt.Sprintf("/erp/package/%d/returnable", pkg)), "returnedQuantity") != 1 {
				s.t.Fatal("lost response sales return did not commit")
			}
		})
		salesReturnID = id(post(fmt.Sprintf("/erp/package/%d/returns", pkg), salesReturn), "returnId")
		balance(6, 0)
		before := inventory()
		unchangedAR := getAR()
		if number(unchangedAR, "pendingReturnCount") != 1 {
			t.Fatal("physical return must await manual financial confirmation")
		}
		deny(s.req("POST", "/erp/salesFinance/collections", a.owner, collect("1.0000")))
		if !reflect.DeepEqual(unchangedAR, getAR()) {
			t.Fatal("rejected collection changed receivable")
		}
		post(fmt.Sprintf("/erp/salesFinance/receivables/%d/adjust", receivable), model.SalesFinAdjust{FinToken: model.FinToken{Version: id(getAR(), "version"), RequestKey: uuid.NewString()}, ReturnID: salesReturnID, Amount: "19.9900", Reason: "本单人工核对金额，非系统按数量自动推算"})
		money(getAR(), map[string]string{"grossAmount": "59.9700", "creditAmount": "19.9900", "receivedAmount": "20.0000", "dueAmount": "19.9800"})
		lastCollection = collect("19.9800")
		lastCollectionID = id(post("/erp/salesFinance/collections", lastCollection), "id")
		money(getAR(), map[string]string{"netAmount": "39.9800", "receivedAmount": "39.9800", "dueAmount": "0.0000", "overreceivedAmount": "0.0000"})
		assertInventoryUnchanged(before)
		if !reflect.DeepEqual(originalOrder, get(fmt.Sprintf("/erp/order/%d", order))) || !reflect.DeepEqual(originalPackage, get(fmt.Sprintf("/erp/package/%d", pkg))) {
			t.Fatal("return/finance rewrote original shipment")
		}
		v = get(fmt.Sprintf("/erp/package/%d/returnable", pkg))
		if number(v, "shippedQuantity") != 3 || number(v, "returnedQuantity") != 1 || number(v, "netShippedQuantity") != 2 {
			t.Fatal("sales gross/return/net facts drifted")
		}
	}) {
		return
	}
	if !stage("replay_and_invalid_writes_cannot_double_stock_or_money", func(t *testing.T) {
		before := inventory()
		ap, ar := getAP(), getAR()
		replay(post("/erp/purchase", purchaseCreate), "purchaseId", purchase)
		replay(post(fmt.Sprintf("/erp/purchase/%d/receipts", purchase), receive), "receiptId", receipt)
		replay(post(fmt.Sprintf("/erp/purchaseReceipt/%d/returns", receipt), purchaseReturn), "returnId", purchaseReturnID)
		replay(post(fmt.Sprintf("/erp/package/%d/returns", pkg), salesReturn), "returnId", salesReturnID)
		s.ok(s.req("PUT", fmt.Sprintf("/erp/order/%d/review", order), a.owner, review))
		s.ok(s.req("PUT", fmt.Sprintf("/erp/package/%d/ship", pkg), a.owner, ship))
		replay(post("/erp/finance/payments", firstPayment), "id", firstPaymentID)
		replay(post("/erp/finance/payments", lastPayment), "id", lastPaymentID)
		replay(post("/erp/salesFinance/collections", firstCollection), "id", firstCollectionID)
		replay(post("/erp/salesFinance/collections", lastCollection), "id", lastCollectionID)
		deny(s.req("POST", "/erp/finance/payments", a.owner, pay("0.0001")))
		deny(s.req("POST", "/erp/salesFinance/collections", a.owner, collect("0.0001")))
		v := get(fmt.Sprintf("/erp/product/%d", productID))
		raw, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		var edit model.CatalogWrite
		if e = json.Unmarshal(raw, &edit); e != nil {
			t.Fatal(e)
		}
		edit.RequestKey = uuid.NewString()
		edit.Variants[0].Unit = "箱"
		deny(s.req("PUT", fmt.Sprintf("/erp/product/%d", productID), a.owner, edit))
		deny(s.req("PUT", "/erp/sku", a.owner, get(fmt.Sprintf("/erp/sku/%d", sku))))
		if !reflect.DeepEqual(ap, getAP()) || !reflect.DeepEqual(ar, getAR()) {
			t.Fatal("replay or rejected write changed financial totals/version")
		}
		assertInventoryUnchanged(before)
		balance(6, 0)
	}) {
		return
	}
	if !stage("final_statements_ledger_and_cross_tenant_reads_agree", func(t *testing.T) {
		money(one(fmt.Sprintf("/erp/finance/statements?supplierId=%d&currency=CNY", supplier)), map[string]string{"grossAmount": "23.5000", "creditAmount": "4.7000", "paidAmount": "18.8000", "dueAmount": "0.0000", "overpaidAmount": "0.0000"})
		money(one(fmt.Sprintf("/erp/salesFinance/statements?shopId=%d&currency=CNY", shop)), map[string]string{"grossAmount": "59.9700", "creditAmount": "19.9900", "receivedAmount": "39.9800", "dueAmount": "0.0000", "overreceivedAmount": "0.0000"})
		ledger := rows("/erp/ledger/list?" + stockQuery())
		var hand, reserved int64
		for _, row := range ledger {
			v := object(row)
			hand += number(v, "onHandDelta")
			reserved += number(v, "reservedDelta")
		}
		if len(ledger) != 5 || hand != 6 || reserved != 0 {
			t.Fatal("unexpected duplicate/missing stock movement")
		}
		balance(6, 0)
		for _, path := range []string{fmt.Sprintf("/erp/product/%d", productID), fmt.Sprintf("/erp/purchase/%d", purchase), fmt.Sprintf("/erp/finance/payables/%d", payable), fmt.Sprintf("/erp/order/%d", order), fmt.Sprintf("/erp/package/%d", pkg), fmt.Sprintf("/erp/salesFinance/receivables/%d", receivable)} {
			deny(s.req("GET", path, b.owner, nil))
		}
		for _, path := range []string{"/erp/stock/list?" + stockQuery(), "/erp/ledger/list?" + stockQuery(), fmt.Sprintf("/erp/finance/statements?supplierId=%d", supplier), fmt.Sprintf("/erp/salesFinance/statements?shopId=%d", shop)} {
			v := s.req("GET", path, b.owner, nil)
			if v["code"] == float64(200) && number(v, "total") != 0 {
				t.Fatal("foreign resource list leaked")
			}
		}
		t.Log("final facts: stock=6 reserved=0 ledger=5; AP gross=23.5000 credit=4.7000 paid=18.8000 due=0; AR gross=59.9700 credit=19.9900 received=39.9800 due=0")
		setup := get("/erp/setup")
		states := map[string]string{}
		for _, item := range setup["checks"].([]any) {
			row := object(item)
			states[row["key"].(string)] = row["state"].(string)
		}
		for _, key := range []string{"warehouses", "products", "shops", "suppliers", "pairings", "stock", "receipts", "shipments", "payables", "receivables"} {
			if states[key] != "READY" {
				t.Fatalf("completed business facts missing from enablement: %s=%s", key, states[key])
			}
		}
		if loseResponses {
			if losses != 5 || len(beforeRetry) != 0 {
				t.Fatal("all five committed-response losses must be exercised")
			}
			t.Log("all five lost responses verified committed before original-key retry")
		}
	}) {
		return
	}
}
