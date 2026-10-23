package handler

import (
	"github.com/gin-gonic/gin/binding"
	"net/http/httptest"
	"ruoyi-go/internal/model"
	"strings"
	"testing"
)

func TestPurchaseBindingTypes(t *testing.T) {
	for _, price := range []string{`0`, `null`, `true`, `[]`, `{}`} {
		var in model.ERPPurchaseWrite
		raw := `{"supplierId":1,"warehouseId":2,"currency":"USD","requestKey":"test-key","lines":[{"skuId":3,"quantity":1,"unitPrice":` + price + `}]}`
		if binding.JSON.Bind(httptest.NewRequest("POST", "/", strings.NewReader(raw)), &in) == nil {
			t.Fatal("non-string price accepted", price)
		}
	}
	for _, quantity := range []string{`0`, `-1`, `"1"`, `1.5`, `null`} {
		var in model.ERPPurchaseWrite
		raw := `{"supplierId":1,"warehouseId":2,"currency":"USD","requestKey":"test-key","lines":[{"skuId":3,"quantity":` + quantity + `,"unitPrice":"0"}]}`
		if binding.JSON.Bind(httptest.NewRequest("POST", "/", strings.NewReader(raw)), &in) == nil {
			t.Fatal("bad quantity", quantity)
		}
	}
	var in model.ERPPurchaseWrite
	if e := binding.JSON.Bind(httptest.NewRequest("POST", "/", strings.NewReader(`{"supplierId":1,"warehouseId":2,"currency":"USD","requestKey":"test-key","orderedAmount":"999","status":"CLOSED","version":888,"lines":[{"skuId":3,"quantity":1,"unitPrice":"0","lineAmount":"999"}]}`)), &in); e != nil {
		t.Fatal(e)
	}
	if in.OrderedAmount != "" || in.Lines[0].LineAmount != "" {
		t.Fatal("server amounts accepted")
	}
}
