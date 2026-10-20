package service

import (
	"ruoyi-go/internal/model"
	"testing"
)

func TestERPDocumentValidationAndCanonicalIdempotency(t *testing.T) {
	input := model.ERPDocumentInput{WarehouseID: 1, RequestKey: "request-123456", Lines: []model.ERPLineInput{{SkuID: 2, Quantity: 3}, {SkuID: 1, Quantity: 4}}}
	a, _, err := prepareERPDocument("SALE", input, "owner")
	if err != nil {
		t.Fatal(err)
	}
	input.Lines[0], input.Lines[1] = input.Lines[1], input.Lines[0]
	b, _, err := prepareERPDocument("SALE", input, "owner")
	if err != nil || a.RequestHash != b.RequestHash {
		t.Fatal("order-independent idempotency")
	}
	input.Lines[0].Quantity = 5
	c, _, _ := prepareERPDocument("SALE", input, "owner")
	if c.RequestHash == a.RequestHash {
		t.Fatal("content must affect hash")
	}
	input.Lines = []model.ERPLineInput{{SkuID: 1, Quantity: 1}, {SkuID: 1, Quantity: 2}}
	if _, _, err = prepareERPDocument("SALE", input, "owner"); err == nil {
		t.Fatal("duplicate SKU accepted")
	}
	input.Lines = []model.ERPLineInput{{SkuID: 1, Quantity: 0}}
	if _, _, err = prepareERPDocument("IN", input, "owner"); err == nil {
		t.Fatal("zero accepted")
	}
	input.RequestKey = "<bad>"
	if _, _, err = prepareERPDocument("IN", input, "owner"); err == nil {
		t.Fatal("invalid key accepted")
	}
}
