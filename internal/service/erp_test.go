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
func TestERPCorrectionAndCountInput(t *testing.T) {
	zero := int64(0)
	original := int64(8)
	in := model.ERPDocumentInput{WarehouseID: 1, RequestKey: "request-12345", Remark: "盘点原因", Lines: []model.ERPLineInput{{SkuID: 1, CountedQuantity: &zero}}}
	doc, lines, err := prepareERPDocument("COUNT", in, "tester")
	if err != nil || doc.Version != 1 || lines[0].Quantity != 0 {
		t.Fatal("explicit zero count must be accepted")
	}
	in.Lines[0].CountedQuantity = nil
	if _, _, err = prepareERPDocument("COUNT", in, "tester"); err == nil {
		t.Fatal("missing actual count accepted")
	}
	in.Lines[0].Quantity = 1
	if _, _, err = prepareERPDocument("SR", in, "tester"); err == nil {
		t.Fatal("return without source accepted")
	}
	in.SourceDocumentID = &original
	if _, _, err = prepareERPDocument("SR", in, "tester"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = prepareERPDocument("IN", in, "tester"); err == nil {
		t.Fatal("source injected into receipt")
	}
	in.Remark = " "
	if _, _, err = prepareERPDocument("PR", in, "tester"); err == nil {
		t.Fatal("correction without reason accepted")
	}
}
