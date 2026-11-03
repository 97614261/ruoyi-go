package service

import (
	"ruoyi-go/internal/model"
	"strings"
	"testing"
)

func TestNormalizeReplenishment(t *testing.T) {
	in := model.ERPReplenishmentWrite{ERPPurchaseWrite: model.ERPPurchaseWrite{SupplierID: 1, WarehouseID: 2, Currency: "cny", RequestKey: "replenish-test", Lines: []model.ERPPurchaseLineInput{{SkuID: 3, Quantity: 2, UnitPrice: "0"}}}, Basis: []model.ERPReplenishmentBasis{{SkuID: 3, Basis: strings.Repeat("a", 64)}}}
	if e := normalizeReplenishment(&in); e != nil {
		t.Fatal(e)
	}
	if in.Currency != "CNY" || in.Lines[0].UnitPrice != "0.0000" {
		t.Fatal(in)
	}
	for _, basis := range [][]model.ERPReplenishmentBasis{nil, {{SkuID: 4, Basis: strings.Repeat("a", 64)}}, {{SkuID: 3, Basis: strings.Repeat("x", 64)}}, {{SkuID: 3, Basis: "a"}}} {
		bad := in
		bad.Basis = basis
		if normalizeReplenishment(&bad) == nil {
			t.Fatal("invalid recommendation accepted")
		}
	}
}
