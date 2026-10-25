package service

import (
	"context"
	"reflect"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"testing"
)

func TestFinancePaymentExactAndCanonical(t *testing.T) {
	valid := func() model.FinPaymentWrite {
		return model.FinPaymentWrite{RequestKey: "finance-test-key", AccountID: 1, SupplierID: 2, Currency: "usd", Amount: "0.3", Allocations: []model.FinAllocationInput{{PayableID: 4, Version: 2, Amount: "0.2"}, {PayableID: 3, Version: 1, Amount: "0.1"}}}
	}
	in := valid()
	original := append([]model.FinAllocationInput(nil), in.Allocations...)
	if e := prepareFinPayment(&in); e != nil {
		t.Fatal(e)
	}
	if in.Amount != "0.3000" || in.Currency != "USD" || in.Allocations[0].PayableID != 3 || !reflect.DeepEqual(original, valid().Allocations) {
		t.Fatal(in)
	}
	for _, bad := range []string{"0", "-1", "1e2", "01", "1.00001", "", "99999999999999999"} {
		p := valid()
		p.Amount = bad
		if prepareFinPayment(&p) == nil {
			t.Fatal(bad)
		}
	}
	for _, f := range []func(*model.FinPaymentWrite){func(p *model.FinPaymentWrite) { p.Allocations[0].Amount = "0" }, func(p *model.FinPaymentWrite) { p.Allocations[0].PayableID = 3 }, func(p *model.FinPaymentWrite) { p.Allocations[0].Version = 0 }, func(p *model.FinPaymentWrite) { p.Amount = "1" }, func(p *model.FinPaymentWrite) { p.Currency = "1AB" }, func(p *model.FinPaymentWrite) { p.Allocations = nil }} {
		p := valid()
		f(&p)
		if prepareFinPayment(&p) == nil {
			t.Fatal(p)
		}
	}
	a := erpctx.WithUser(context.Background(), 10)
	b := erpctx.WithUser(context.Background(), 11)
	if purchaseHash(a, "PAYMENT_CREATE", 0, in) == purchaseHash(b, "PAYMENT_CREATE", 0, in) {
		t.Fatal("actor missing from fingerprint")
	}
}
func TestFinanceAccountAndAdjustValidation(t *testing.T) {
	a := model.FinAccountWrite{RequestKey: "account-test-key", AccountCode: "BANK-A", AccountName: "内部记账", AccountType: "BANK", Currency: "cny", Status: "0"}
	if prepareFinAccount(&a, 0) != nil || a.Currency != "CNY" {
		t.Fatal(a)
	}
	if prepareFinAccount(&a, 1) == nil {
		t.Fatal("missing edit version")
	}
	in := model.FinAdjust{FinToken: model.FinToken{Version: 1, RequestKey: "adjust-test-key"}, ReturnIDs: []int64{3, 2}}
	if prepareFinAdjust(&in) != nil || in.ReturnIDs[0] != 2 {
		t.Fatal(in)
	}
	in.ReturnIDs = []int64{2, 2}
	if prepareFinAdjust(&in) == nil {
		t.Fatal("duplicate returns")
	}
}
