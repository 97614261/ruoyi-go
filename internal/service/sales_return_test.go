package service

import (
	"encoding/json"
	"ruoyi-go/internal/model"
	"strings"
	"testing"
)

func TestNormalizeSalesReturn(t *testing.T) {
	valid := func() model.ERPSalesReturnInput {
		return model.ERPSalesReturnInput{ERPActionToken: model.ERPActionToken{Version: 1, RequestKey: "return-test"}, Reason: " 原包装破损 ", Lines: []model.ERPSalesReturnLineInput{{PackageLineID: 2, Quantity: 3}, {PackageLineID: 1, Quantity: 1}}}
	}
	in := valid()
	original := in.Lines
	if e := normalizeSalesReturn(&in); e != nil {
		t.Fatal(e)
	}
	if in.Reason != "原包装破损" || in.Lines[0].PackageLineID != 1 || original[0].PackageLineID != 2 {
		t.Fatal("normalization mutated caller or failed")
	}
	for _, change := range []func(*model.ERPSalesReturnInput){func(v *model.ERPSalesReturnInput) { v.Version = 0 }, func(v *model.ERPSalesReturnInput) { v.RequestKey = "" }, func(v *model.ERPSalesReturnInput) { v.Reason = "  " }, func(v *model.ERPSalesReturnInput) { v.Reason = "x\ny" }, func(v *model.ERPSalesReturnInput) { v.Reason = strings.Repeat("退", 501) }, func(v *model.ERPSalesReturnInput) { v.Lines = nil }, func(v *model.ERPSalesReturnInput) { v.Lines = make([]model.ERPSalesReturnLineInput, 101) }, func(v *model.ERPSalesReturnInput) { v.Lines[0].Quantity = 0 }, func(v *model.ERPSalesReturnInput) { v.Lines[0].Quantity = 1000000001 }, func(v *model.ERPSalesReturnInput) { v.Lines[0].PackageLineID = 0 }, func(v *model.ERPSalesReturnInput) { v.Lines[0].PackageLineID = 1 }} {
		v := valid()
		change(&v)
		if normalizeSalesReturn(&v) == nil {
			t.Fatal("invalid return accepted", v)
		}
	}
}
func TestSalesReturnResponsesNeverExposeSecrets(t *testing.T) {
	raw, _ := json.Marshal(model.ERPSalesReturn{RequestKey: "PRIVATE_KEY", RequestHash: "PRIVATE_HASH", ResultJSON: "PRIVATE_RESULT", Lines: []model.ERPSalesReturnLine{{Quantity: 1}}})
	for _, bad := range []string{"PRIVATE_", "requestKey", "requestHash", "resultJson", "unitPrice", "orderedAmount", "contactName"} {
		if strings.Contains(string(raw), bad) {
			t.Fatal("private field", bad)
		}
	}
}
