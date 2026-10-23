package handler

import (
	"encoding/json"
	"github.com/gin-gonic/gin/binding"
	"net/http/httptest"
	"ruoyi-go/internal/model"
	"strings"
	"testing"
)

func TestSupplierBindingAndServerControlledFields(t *testing.T) {
	for _, extra := range []string{``, `,"version":0`, `,"version":-1`, `,"version":"1"`, `,"version":1.5`} {
		var in model.ERPSupplierEdit
		raw := `{"supplierCode":"00001","supplierName":"test","status":"0"` + extra + `}`
		if binding.JSON.Bind(httptest.NewRequest("PUT", "/", strings.NewReader(raw)), &in) == nil {
			t.Fatal("bad edit version accepted")
		}
	}
	var in model.ERPSupplierWrite
	raw := `{"supplierCode":"00001","supplierName":"test","status":"0","tenantId":99,"supplierId":88,"createBy":"forged","version":77}`
	if err := binding.JSON.Bind(httptest.NewRequest("POST", "/", strings.NewReader(raw)), &in); err != nil {
		t.Fatal("optional blank fields rejected", err)
	}
	b, _ := json.Marshal(in)
	for _, field := range []string{"tenantId", "supplierId", "createBy", "version"} {
		if strings.Contains(string(b), `"`+field+`"`) {
			t.Fatal("request controls protected field", field)
		}
	}
}
