package service

import (
	"ruoyi-go/internal/model"
	"testing"
)

func TestPickingInputPreservesCodesRejectsBadCountsAndCanonicalizesRetry(t *testing.T) {
	good := model.PickingCheckInput{ERPActionToken: model.ERPActionToken{Version: 1, RequestKey: "12345678"}, Scans: []model.PickingScan{{ScanCode: "Z", Quantity: 1}, {ScanCode: "00001", Quantity: 2}}}
	if validatePickingInput(&good) != nil || good.Scans[0].ScanCode != "00001" {
		t.Fatal("codes were not preserved/canonicalized")
	}
	for _, scans := range [][]model.PickingScan{nil, {{ScanCode: "", Quantity: 1}}, {{ScanCode: " X", Quantity: 1}}, {{ScanCode: "X", Quantity: 0}}, {{ScanCode: "X", Quantity: 1000000001}}, {{ScanCode: "X", Quantity: 1}, {ScanCode: "X", Quantity: 1}}} {
		bad := good
		bad.Scans = scans
		if validatePickingInput(&bad) == nil {
			t.Fatal("bad scan accepted")
		}
	}
}
