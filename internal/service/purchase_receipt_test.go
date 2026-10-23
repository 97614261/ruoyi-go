package service

import (
	"encoding/json"
	"ruoyi-go/internal/model"
	"strings"
	"testing"
)

func TestPurchaseReceiveValidationAndPrivateDTO(t *testing.T) {
	good := model.ERPPurchaseReceiveInput{ERPActionToken: model.ERPActionToken{Version: 1, RequestKey: "receive-key-01"}, Lines: []model.ERPPurchaseReceiveLineInput{{PurchaseLineID: 2, Quantity: 4}, {PurchaseLineID: 1, Quantity: 6}}}
	if e := normalizePurchaseReceive(&good); e != nil {
		t.Fatal(e)
	}
	if good.Lines[0].PurchaseLineID != 1 {
		t.Fatal("not canonical")
	}
	for _, bad := range []model.ERPPurchaseReceiveInput{{}, {ERPActionToken: good.ERPActionToken}, {ERPActionToken: good.ERPActionToken, Lines: []model.ERPPurchaseReceiveLineInput{{PurchaseLineID: 1, Quantity: 0}}}, {ERPActionToken: good.ERPActionToken, Lines: []model.ERPPurchaseReceiveLineInput{{PurchaseLineID: 1, Quantity: 1}, {PurchaseLineID: 1, Quantity: 2}}}} {
		if e := normalizePurchaseReceive(&bad); e == nil {
			t.Fatal("bad receive accepted")
		}
	}
	for _, v := range []any{model.ERPPurchaseReceivable{}, model.ERPPurchaseReceipt{}, model.ERPPurchaseReceiveResult{}} {
		raw, _ := json.Marshal(v)
		for _, f := range []string{"unitPrice", "lineAmount", "orderedAmount", "contactName", "phone", "email", "address", "requestHash"} {
			if strings.Contains(string(raw), `"`+f+`"`) {
				t.Fatal("private receive field", f)
			}
		}
	}
}
