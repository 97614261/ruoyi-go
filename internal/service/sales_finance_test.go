package service

import (
	"context"
	"reflect"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"testing"
)

func TestSalesFinanceCollectionExactAndCanonical(t *testing.T) {
	valid := func() model.SalesFinCollectionWrite {
		return model.SalesFinCollectionWrite{RequestKey: "salesFinance-test-key", AccountID: 1, ShopID: 2, Currency: "usd", Amount: "0.3", Allocations: []model.SalesFinAllocationInput{{ReceivableID: 4, Version: 2, Amount: "0.2"}, {ReceivableID: 3, Version: 1, Amount: "0.1"}}}
	}
	in := valid()
	original := append([]model.SalesFinAllocationInput(nil), in.Allocations...)
	if e := prepareSalesFinCollection(&in); e != nil {
		t.Fatal(e)
	}
	if in.Amount != "0.3000" || in.Currency != "USD" || in.Allocations[0].ReceivableID != 3 || !reflect.DeepEqual(original, valid().Allocations) {
		t.Fatal(in)
	}
	for _, bad := range []string{"0", "-1", "1e2", "01", "1.00001", "", "99999999999999999"} {
		p := valid()
		p.Amount = bad
		if prepareSalesFinCollection(&p) == nil {
			t.Fatal(bad)
		}
	}
	for _, f := range []func(*model.SalesFinCollectionWrite){func(p *model.SalesFinCollectionWrite) { p.Allocations[0].Amount = "0" }, func(p *model.SalesFinCollectionWrite) { p.Allocations[0].ReceivableID = 3 }, func(p *model.SalesFinCollectionWrite) { p.Allocations[0].Version = 0 }, func(p *model.SalesFinCollectionWrite) { p.Amount = "1" }, func(p *model.SalesFinCollectionWrite) { p.Currency = "1AB" }, func(p *model.SalesFinCollectionWrite) { p.Allocations = nil }} {
		p := valid()
		f(&p)
		if prepareSalesFinCollection(&p) == nil {
			t.Fatal(p)
		}
	}
	a := erpctx.WithUser(context.Background(), 10)
	b := erpctx.WithUser(context.Background(), 11)
	if purchaseHash(a, "COLLECTION_CREATE", 0, in) == purchaseHash(b, "COLLECTION_CREATE", 0, in) {
		t.Fatal("actor missing from fingerprint")
	}
}
func TestSalesFinanceManualCredit(t *testing.T) {
	in := model.SalesFinAdjust{FinToken: model.FinToken{Version: 1, RequestKey: "sales-credit-test"}, ReturnID: 1, Amount: "0", Reason: "确认无货款抵减"}
	if prepareSalesFinAdjust(&in) != nil || in.Amount != "0.0000" {
		t.Fatal(in)
	}
	for _, bad := range []string{"", "1e2", "-1", "01", "1.00001"} {
		x := in
		x.Amount = bad
		if prepareSalesFinAdjust(&x) == nil {
			t.Fatal(bad)
		}
	}
	in.Reason = ""
	if prepareSalesFinAdjust(&in) == nil {
		t.Fatal("reason missing")
	}
}
