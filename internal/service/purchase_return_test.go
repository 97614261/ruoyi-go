package service

import (
	"encoding/json"
	"ruoyi-go/internal/model"
	"strings"
	"testing"
)

func TestNormalizePurchaseReturn(t *testing.T) {
	valid := func() model.ERPPurchaseReturnInput {
		return model.ERPPurchaseReturnInput{ERPActionToken: model.ERPActionToken{Version: 1, RequestKey: "return-test"}, Reason: " 原包装破损 ", Lines: []model.ERPPurchaseReturnLineInput{{ReceiptLineID: 2, Quantity: 3}, {ReceiptLineID: 1, Quantity: 1}}}
	}
	in := valid()
	original := in.Lines
	if e := normalizePurchaseReturn(&in); e != nil {
		t.Fatal(e)
	}
	if in.Reason != "原包装破损" || in.Lines[0].ReceiptLineID != 1 || original[0].ReceiptLineID != 2 {
		t.Fatal("normalization mutated caller or failed")
	}
	for _, change := range []func(*model.ERPPurchaseReturnInput){func(v *model.ERPPurchaseReturnInput) { v.Version = 0 }, func(v *model.ERPPurchaseReturnInput) { v.RequestKey = "" }, func(v *model.ERPPurchaseReturnInput) { v.Reason = "  " }, func(v *model.ERPPurchaseReturnInput) { v.Reason = "x\ny" }, func(v *model.ERPPurchaseReturnInput) { v.Reason = strings.Repeat("退", 501) }, func(v *model.ERPPurchaseReturnInput) { v.Lines = nil }, func(v *model.ERPPurchaseReturnInput) { v.Lines = make([]model.ERPPurchaseReturnLineInput, 101) }, func(v *model.ERPPurchaseReturnInput) { v.Lines[0].Quantity = 0 }, func(v *model.ERPPurchaseReturnInput) { v.Lines[0].Quantity = 1000000001 }, func(v *model.ERPPurchaseReturnInput) { v.Lines[0].ReceiptLineID = 0 }, func(v *model.ERPPurchaseReturnInput) { v.Lines[0].ReceiptLineID = 1 }} {
		v := valid()
		change(&v)
		if normalizePurchaseReturn(&v) == nil {
			t.Fatal("invalid return accepted", v)
		}
	}
}
func TestPurchaseReturnResponsesNeverExposeSecrets(t *testing.T) {
	raw, _ := json.Marshal(model.ERPPurchaseReturn{RequestKey: "PRIVATE_KEY", RequestHash: "PRIVATE_HASH", ResultJSON: "PRIVATE_RESULT", Lines: []model.ERPPurchaseReturnLine{{Quantity: 1}}})
	for _, bad := range []string{"PRIVATE_", "requestKey", "requestHash", "resultJson", "unitPrice", "orderedAmount", "contactName"} {
		if strings.Contains(string(raw), bad) {
			t.Fatal("private field", bad)
		}
	}
}
