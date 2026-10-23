package handler

import (
	"github.com/gin-gonic/gin/binding"
	"net/http/httptest"
	"ruoyi-go/internal/model"
	"strings"
	"testing"
)

func TestPurchaseReturnBinding(t *testing.T) {
	for _, v := range []string{`0`, `-1`, `1.5`, `"1"`, `null`, `true`, `1000000001`} {
		var in model.ERPPurchaseReturnInput
		raw := `{"version":1,"requestKey":"return-test","reason":"broken","lines":[{"receiptLineId":1,"quantity":` + v + `}]}`
		if binding.JSON.Bind(httptest.NewRequest("POST", "/", strings.NewReader(raw)), &in) == nil {
			t.Fatal("invalid quantity", v)
		}
	}
	var in model.ERPPurchaseReturnInput
	if e := binding.JSON.Bind(httptest.NewRequest("POST", "/", strings.NewReader(`{"version":1,"requestKey":"return-test","reason":"broken","tenantId":999,"warehouseId":999,"stockDocumentId":999,"lines":[{"receiptLineId":1,"quantity":1,"skuId":999,"unitPrice":"99"}]}`)), &in); e != nil {
		t.Fatal(e)
	}
	if in.Lines[0].ReceiptLineID != 1 || in.Lines[0].Quantity != 1 {
		t.Fatal("binding drift")
	}
}
