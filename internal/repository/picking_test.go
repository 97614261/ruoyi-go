package repository

import (
	"encoding/json"
	"math"
	"ruoyi-go/internal/model"
	"testing"
)

func TestPackingCountsRejectWrongMissingDuplicateAndExtra(t *testing.T) {
	items := []model.PickingItem{{SkuID: 1, ScanCode: "00001", Quantity: 2}, {SkuID: 2, ScanCode: "X", Quantity: 1}}
	if validatePackingCounts(items, []model.PickingScan{{ScanCode: "X", Quantity: 1}, {ScanCode: "00001", Quantity: 2}}) != nil {
		t.Fatal("valid exact scan rejected")
	}
	for _, scans := range [][]model.PickingScan{nil, {{ScanCode: "1", Quantity: 2}, {ScanCode: "X", Quantity: 1}}, {{ScanCode: "00001", Quantity: 1}, {ScanCode: "X", Quantity: 1}}, {{ScanCode: "00001", Quantity: 3}, {ScanCode: "X", Quantity: 1}}, {{ScanCode: "00001", Quantity: 2}, {ScanCode: "00001", Quantity: 2}}} {
		if validatePackingCounts(items, scans) == nil {
			t.Fatal("invalid scan accepted")
		}
	}
}
func TestPackingFingerprintChangesWithCurrentBarcodeAndKeepsCompactProof(t *testing.T) {
	p := model.ERPPackage{PackageID: 1, OrderID: 2, WarehouseID: 3}
	a := []model.PickingItem{{SkuID: 4, ScanCode: "00001", Quantity: 2, Unit: "箱"}}
	first := pickingFingerprint(p, a)
	a[0].ScanCode = "00002"
	if first == pickingFingerprint(p, a) {
		t.Fatal("barcode change did not invalidate proof")
	}
	id, v := int64(math.MaxInt64), int64(math.MaxInt64)
	raw, e := json.Marshal(packingProof{ERPFulfillmentResult: model.ERPFulfillmentResult{OrderID: id, Version: v, Status: "READY_TO_SHIP", PackageID: &id, PackageVersion: &v}, Fingerprint: first})
	if e != nil || len(raw) > 500 {
		t.Fatal("proof exceeds existing immutable event column")
	}
}
