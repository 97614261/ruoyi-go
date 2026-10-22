package service

import (
	"context"
	"ruoyi-go/internal/model"
	"testing"
)

func TestFulfillmentValidation(t *testing.T) {
	for _, ids := range [][2]int64{{0, 1}, {1, 0}, {-1, 1}} {
		if _, err := CheckERPOrderReadiness(context.Background(), ids[0], ids[1]); err == nil {
			t.Fatal("invalid readiness identifier accepted")
		}
	}
	if _, err := ERPOrderWorkbench(context.Background(), model.ERPOrderQuery{ShopID: -1}); err == nil {
		t.Fatal("invalid workbench filter accepted")
	}
	for _, in := range []model.ERPActionToken{{Version: 0, RequestKey: "valid-key"}, {Version: 1, RequestKey: "short"}, {Version: 1, RequestKey: "invalid key with spaces"}} {
		if validERPAction(in) == nil {
			t.Fatal("invalid action accepted")
		}
	}
	ctx := context.Background()
	if _, err := ReviewERPOrder(ctx, 1, model.ERPReviewInput{ERPActionToken: model.ERPActionToken{Version: 1, RequestKey: "valid-key"}}, "test"); err == nil {
		t.Fatal("missing warehouse accepted")
	}
	if _, err := CancelERPOrder(ctx, 1, model.ERPCancelInput{ERPActionToken: model.ERPActionToken{Version: 1, RequestKey: "valid-key"}}, "test"); err == nil {
		t.Fatal("empty reason accepted")
	}
	for _, tracking := range []string{"", " padded", "line\nbreak"} {
		if _, err := SetERPPackageLogistics(ctx, 1, model.ERPLogisticsInput{ERPActionToken: model.ERPActionToken{Version: 1, RequestKey: "valid-key"}, CarrierCode: "MANUAL", TrackingNo: tracking}, "test"); err == nil {
			t.Fatal("invalid tracking accepted")
		}
	}
}
