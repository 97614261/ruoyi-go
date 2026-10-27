package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type timingBook struct {
	mu       sync.Mutex
	Rows     map[string][]time.Duration
	Errors   int64
	Requests int64
}

var timings = timingBook{Rows: map[string][]time.Duration{}}

func measured(a api, method, path, token string, body any, kind string) map[string]any {
	start := time.Now()
	out, e := a.call(method, path, token, body)
	timings.mu.Lock()
	timings.Requests++
	timings.Rows[kind] = append(timings.Rows[kind], time.Since(start))
	bad := e != nil || out["code"] != float64(200)
	if bad {
		timings.Errors++
	}
	timings.mu.Unlock()
	if bad {
		panic(fail("measured HTTP/business failure: " + kind))
	}
	return out
}

type cycleReceipt struct {
	Phase                                                                                           string `json:"phase"`
	TenantID, Sequence, Purchase, Receipt, Payable, Payment, Order, Package, Receivable, Collection int64
	At                                                                                              time.Time `json:"atUtc"`
}

var journalMu sync.Mutex

func saveCycle(r cycleReceipt) {
	journalMu.Lock()
	defer journalMu.Unlock()
	must(appendEvidence(relRoot+"/cycles.jsonl", r))
}

func realCycle(a api, f *relFixture, sequence int64) {
	key := func(action string) string { return fmt.Sprintf("rel-%d-%d-%s", f.ID, sequence, action) }
	do := func(method, path string, body any, kind string) map[string]any {
		b, _ := json.Marshal(body)
		sum := sha256.Sum256(b)
		journalMu.Lock()
		err := appendEvidence(relRoot+"/request-intents.jsonl", map[string]any{"atUtc": time.Now().UTC(), "tenantId": f.ID, "sequence": sequence, "method": method, "path": path, "kind": kind, "bodySHA256": fmt.Sprintf("%x", sum)})
		journalMu.Unlock()
		must(err)
		return measured(a, method, path, f.Owner, body, kind)
	}
	post := func(path string, body any, kind string) map[string]any { return data(do("POST", path, body, kind)) }
	get := func(path, kind string) map[string]any { return data(do("GET", path, nil, kind)) }
	result := cycleReceipt{TenantID: f.ID, Sequence: sequence, Phase: phaseName()}
	result.Purchase = field(post("/erp/purchase", map[string]any{"supplierId": f.Supplier, "warehouseId": f.Warehouse, "currency": "CNY", "requestKey": key("purchase"), "lines": []map[string]any{{"skuId": f.SKU, "quantity": 5, "unitPrice": "2.0000"}}}, "purchase_create"), "purchaseId")
	do("PUT", fmt.Sprintf("/erp/purchase/%d/confirm", result.Purchase), map[string]any{"version": 1, "requestKey": key("confirm")}, "purchase_confirm")
	po := get(fmt.Sprintf("/erp/purchase/%d", result.Purchase), "purchase_query")
	line := field(po["lines"].([]any)[0].(map[string]any), "lineId")
	result.Receipt = field(post(fmt.Sprintf("/erp/purchase/%d/receipts", result.Purchase), map[string]any{"version": po["version"], "requestKey": key("receive"), "lines": []map[string]any{{"purchaseLineId": line, "quantity": 5}}}, "purchase_receive"), "receiptId")
	result.Payable = field(post("/erp/finance/payables", map[string]any{"receiptId": result.Receipt, "requestKey": key("payable")}, "payable_create"), "id")
	ap := get(fmt.Sprintf("/erp/finance/payables/%d", result.Payable), "payable_query")
	payment := map[string]any{"requestKey": key("payment"), "accountId": f.Account, "supplierId": f.Supplier, "currency": "CNY", "amount": "10.0000", "allocations": []map[string]any{{"payableId": result.Payable, "version": ap["version"], "amount": "10.0000"}}}
	result.Payment = field(post("/erp/finance/payments", payment, "payment_create"), "id")
	if replay := post("/erp/finance/payments", payment, "payment_replay"); replay["existing"] != true || field(replay, "id") != result.Payment {
		panic(fail("payment replay drift"))
	}
	result.Order = field(post("/erp/order", orderInput(f, key("order"), key("source"), 3), "order_create"), "orderId")
	review := map[string]any{"version": 1, "requestKey": key("review"), "warehouseId": f.Warehouse}
	reviewed := data(do("PUT", fmt.Sprintf("/erp/order/%d/review", result.Order), review, "order_review"))
	result.Package = field(reviewed, "packageId")
	do("PUT", fmt.Sprintf("/erp/package/%d/logistics", result.Package), map[string]any{"version": 1, "requestKey": key("logistics"), "carrierCode": "MANUAL", "trackingNo": key("tracking")}, "logistics_save")
	sheet := get(fmt.Sprintf("/erp/picking/%d", result.Package), "picking_query")
	checked := data(do("PUT", fmt.Sprintf("/erp/picking/%d/check", result.Package), map[string]any{"version": sheet["version"], "requestKey": key("packing"), "scans": []map[string]any{{"scanCode": "0009876543210", "quantity": 3}}}, "packing_check"))
	shipBody := map[string]any{"version": checked["packageVersion"], "requestKey": key("ship")}
	do("PUT", fmt.Sprintf("/erp/picking/%d/ship", result.Package), shipBody, "checked_ship")
	if replay := data(do("PUT", fmt.Sprintf("/erp/picking/%d/ship", result.Package), shipBody, "ship_replay")); replay["existing"] != true {
		panic(fail("shipment replay drift"))
	}
	result.Receivable = field(post("/erp/salesFinance/receivables", map[string]any{"orderId": result.Order, "requestKey": key("receivable")}, "receivable_create"), "id")
	ar := get(fmt.Sprintf("/erp/salesFinance/receivables/%d", result.Receivable), "receivable_query")
	collect := map[string]any{"requestKey": key("collection"), "accountId": f.Account, "shopId": f.Shop, "currency": "CNY", "amount": "9.0000", "allocations": []map[string]any{{"receivableId": result.Receivable, "version": ar["version"], "amount": "9.0000"}}}
	result.Collection = field(post("/erp/salesFinance/collections", collect, "collection_create"), "id")
	if replay := post("/erp/salesFinance/collections", collect, "collection_replay"); replay["existing"] != true || field(replay, "id") != result.Collection {
		panic(fail("collection replay drift"))
	}
	for _, id := range []int64{result.Purchase, result.Receipt, result.Payable, result.Payment, result.Order, result.Package, result.Receivable, result.Collection} {
		if id < 1 {
			panic(fail("cycle result missing ID"))
		}
	}
	result.At = time.Now().UTC()
	saveCycle(result)
}
func orderInput(f *relFixture, key, source string, quantity int64) map[string]any {
	return map[string]any{"shopId": f.Shop, "sourceOrderNo": source, "requestKey": key, "receiverName": "SYNTHETIC_LOCAL_TEST", "receiverCountry": "CN", "receiverAddress": "SYNTHETIC_LOCAL_TEST", "currency": "CNY", "originalAmount": "9.0000", "orderedAt": "2026-10-26 12:00:00", "lines": []map[string]any{{"sourceLineNo": "1", "shopItemCode": "REL-ITEM", "sourceTitle": "Synthetic item", "quantity": quantity}}}
}

func timingSummary() map[string]any {
	timings.mu.Lock()
	defer timings.mu.Unlock()
	out := map[string]any{}
	for kind, latencies := range timings.Rows {
		out[kind] = map[string]any{"requests": len(latencies), "p50Ms": relQuantile(latencies, 0.50), "p95Ms": relQuantile(latencies, 0.95), "p99Ms": relQuantile(latencies, 0.99)}
	}
	return map[string]any{"requests": timings.Requests, "errors": timings.Errors, "kinds": out}
}

func verifyFlowFacts(s relState, label string) map[string]any {
	_, d := sourceConfig()
	db := openDB(d)
	defer db.Close()
	ctx, c := context.WithTimeout(context.Background(), 30*time.Second)
	defer c()
	cycles := readCycles()
	out := map[string]any{}
	for _, f := range []*relFixture{s.Fixtures[0], s.Fixtures[2]} {
		var n int64
		for _, r := range cycles {
			if r.TenantID == f.ID {
				n++
			}
		}
		var on, res, delta, resDelta, ledgerCount int64
		must(db.QueryRowContext(ctx, "SELECT on_hand,reserved FROM erp_stock WHERE tenant_id=? AND warehouse_id=? AND sku_id=?", f.ID, f.Warehouse, f.SKU).Scan(&on, &res))
		must(db.QueryRowContext(ctx, "SELECT COALESCE(SUM(on_hand_delta),0),COALESCE(SUM(reserved_delta),0),COUNT(*) FROM erp_stock_ledger WHERE tenant_id=? AND warehouse_id=? AND sku_id=?", f.ID, f.Warehouse, f.SKU).Scan(&delta, &resDelta, &ledgerCount))
		if on != 100+n*2 || res != 0 || delta != on || resDelta != 0 || ledgerCount != 1+n*3 {
			panic(fail("inventory/ledger/cycle facts disagree"))
		}
		facts := map[string]any{"cycles": n, "onHand": on, "reserved": res, "ledgerOnHand": delta, "ledgerReserved": resDelta, "ledgerRows": ledgerCount}
		for _, q := range []struct{ Table, Amount, Paid, Expected string }{{"erp_fin_payable", "gross_amount", "paid_amount", fmt.Sprintf("%d.0000", n*10)}, {"erp_sf_receivable", "gross_amount", "received_amount", fmt.Sprintf("%d.0000", n*9)}} {
			var count int64
			var amount, paid string
			must(db.QueryRowContext(ctx, "SELECT COUNT(*),COALESCE(SUM("+q.Amount+"),0),COALESCE(SUM("+q.Paid+"),0) FROM "+q.Table+" WHERE tenant_id=?", f.ID).Scan(&count, &amount, &paid))
			if count != n || amount != q.Expected || paid != q.Expected {
				panic(fail("financial totals/cycle facts disagree"))
			}
			facts[q.Table] = map[string]any{"records": count, "gross": amount, "settled": paid}
		}
		for _, table := range []string{"erp_order", "erp_package", "erp_purchase_order", "erp_purchase_receipt", "erp_fin_payment", "erp_fin_allocation", "erp_sf_collection", "erp_sf_allocation"} {
			var count int64
			must(db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE tenant_id=?", f.ID).Scan(&count))
			if count != n {
				panic(fail("cycle cardinality mismatch: " + table))
			}
			facts[table] = count
		}
		var unshipped int64
		must(db.QueryRowContext(ctx, "SELECT COUNT(*) FROM erp_order WHERE tenant_id=? AND status<>'SHIPPED'", f.ID).Scan(&unshipped))
		if unshipped != 0 {
			panic(fail("partial order after cycle"))
		}
		out[fmt.Sprint(f.ID)] = facts
	}
	must(save(relRoot+"/facts-"+label+".json", out))
	return out
}
