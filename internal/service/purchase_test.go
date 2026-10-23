package service

import (
	"context"
	"encoding/json"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"strings"
	"testing"
)

func purchaseInput() model.ERPPurchaseWrite {
	return model.ERPPurchaseWrite{SupplierID: 1, WarehouseID: 2, Currency: "usd", RequestKey: "purchase-key-01", Lines: []model.ERPPurchaseLineInput{{SkuID: 4, Quantity: 3, UnitPrice: "0.1001"}, {SkuID: 3, Quantity: 1, UnitPrice: "0"}}}
}
func TestPurchaseNormalizeAmountsAndCanonicalHash(t *testing.T) {
	original := purchaseInput()
	in := original
	if e := normalizePurchase(&in); e != nil {
		t.Fatal(e)
	}
	if in.OrderedAmount != "0.3003" || in.Currency != "USD" || in.Lines[0].SkuID != 3 || in.Lines[0].UnitPrice != "0.0000" {
		t.Fatal(in)
	}
	if original.Lines[0].SkuID != 4 || original.Lines[0].UnitPrice != "0.1001" {
		t.Fatal("caller slice mutated")
	}
	second := purchaseInput()
	second.Currency = "USD"
	second.Lines = []model.ERPPurchaseLineInput{second.Lines[1], second.Lines[0]}
	second.Lines[0].UnitPrice = "0.0000"
	if e := normalizePurchase(&second); e != nil {
		t.Fatal(e)
	}
	ctx := erpctx.WithUser(context.Background(), 1)
	if purchaseHash(ctx, "CREATE", 0, in) != purchaseHash(ctx, "CREATE", 0, second) {
		t.Fatal("canonical hash changed")
	}
	if purchaseHash(ctx, "CREATE", 0, in) == purchaseHash(erpctx.WithUser(ctx, 2), "CREATE", 0, in) {
		t.Fatal("actor missing from hash")
	}
}
func TestPurchaseBadInputs(t *testing.T) {
	for _, change := range []func(*model.ERPPurchaseWrite){func(x *model.ERPPurchaseWrite) { x.Lines = nil }, func(x *model.ERPPurchaseWrite) { x.Lines[0].Quantity = 0 }, func(x *model.ERPPurchaseWrite) { x.Lines[0].Quantity = 1000000001 }, func(x *model.ERPPurchaseWrite) { x.Lines[0].SkuID = x.Lines[1].SkuID }, func(x *model.ERPPurchaseWrite) { x.Lines[0].UnitPrice = "1e2" }, func(x *model.ERPPurchaseWrite) { x.Lines[0].UnitPrice = "9999999999999999.9999" }, func(x *model.ERPPurchaseWrite) { x.Currency = "US1" }, func(x *model.ERPPurchaseWrite) { x.RequestKey = "bad" }, func(x *model.ERPPurchaseWrite) { x.Remark = "bad\n" }} {
		in := purchaseInput()
		change(&in)
		if e := normalizePurchase(&in); e == nil {
			t.Fatal("bad input accepted", in)
		}
	}
	in := purchaseInput()
	in.Lines = []model.ERPPurchaseLineInput{{SkuID: 1, Quantity: 1, UnitPrice: "9999999999999999.9999"}, {SkuID: 2, Quantity: 1, UnitPrice: "0.0001"}}
	if e := normalizePurchase(&in); e == nil {
		t.Fatal("sum overflow accepted")
	}
}
func TestPurchasePrivateProjection(t *testing.T) {
	raw, _ := json.Marshal(model.ERPPurchaseSummary{})
	for _, f := range []string{"orderedAmount", "unitPrice", "remark", "closeReason", "requestKey", "tenantId"} {
		if strings.Contains(string(raw), `"`+f+`"`) {
			t.Fatal("private summary field", f)
		}
	}
	raw, _ = json.Marshal(model.ERPPurchaseEvent{})
	if strings.Contains(string(raw), "resultJSON") || strings.Contains(string(raw), "requestHash") {
		t.Fatal("event replay details leaked")
	}
}
