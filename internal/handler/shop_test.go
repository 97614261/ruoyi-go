package handler

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"net/http/httptest"
	"ruoyi-go/internal/model"
	"strings"
	"testing"
)

func TestERPShopAccessExplicitSetBinding(t *testing.T) {
	for _, raw := range []string{`{"version":1}`, `{"version":1,"shopIds":null}`, `{"version":1,"shopIds":[0]}`, `{"version":1,"shopIds":[1.1]}`, `{"version":0,"shopIds":[]}`} {
		var input model.ERPShopAccessRequest
		if err := binding.JSON.Bind(httptest.NewRequest("PUT", "/", strings.NewReader(raw)), &input); err == nil {
			t.Fatalf("invalid authorization accepted: %s", raw)
		}
	}
	var input model.ERPShopAccessRequest
	if err := binding.JSON.Bind(httptest.NewRequest("PUT", "/", strings.NewReader(`{"version":1,"shopIds":[]}`)), &input); err != nil || input.ShopIDs == nil || len(*input.ShopIDs) != 0 {
		t.Fatal("explicit revoke rejected", err)
	}
}
func TestERPChannelResponseShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/list", ERPChannelList)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/list?pageSize=1&pageNum=1", nil))
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != float64(200) || body["total"] != float64(6) || body["data"] != nil {
		t.Fatal("table contract", body)
	}
	rows := body["rows"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["supportsApi"] != false {
		t.Fatal("capability contract")
	}
}
