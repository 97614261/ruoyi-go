package service

import (
	"ruoyi-go/internal/model"
	"testing"
)

func TestLogisticsValidation(t *testing.T) {
	valid := model.LogisticsWrite{RequestKey: "request-123", Code: "SF_1", Name: "标准物流", Status: "0"}
	if validateLogisticsWrite(0, &valid, false) != nil {
		t.Fatal("valid carrier")
	}
	for _, change := range []func(*model.LogisticsWrite){func(v *model.LogisticsWrite) { v.Code = "bad/code" }, func(v *model.LogisticsWrite) { v.Code = "中文" }, func(v *model.LogisticsWrite) { v.Name = "" }, func(v *model.LogisticsWrite) { v.Name = "bad\nname" }, func(v *model.LogisticsWrite) { v.Status = "2" }, func(v *model.LogisticsWrite) { v.Version = 1 }, func(v *model.LogisticsWrite) { v.CarrierID = 1 }, func(v *model.LogisticsWrite) { v.RequestKey = "short" }} {
		v := valid
		change(&v)
		if validateLogisticsWrite(0, &v, false) == nil {
			t.Fatal("invalid carrier allowed")
		}
	}
	method := valid
	method.CarrierID = 1
	if validateLogisticsWrite(0, &method, true) != nil {
		t.Fatal("valid method")
	}
	method.CarrierID = 0
	if validateLogisticsWrite(0, &method, true) == nil {
		t.Fatal("method without carrier")
	}
	if validateLogisticsQuery(model.LogisticsQuery{Status: "2"}) == nil {
		t.Fatal("bad query status")
	}
	for _, q := range []model.ShipmentQuery{{Mode: "BAD"}, {TrackingNo: "x\n"}, {CarrierCode: "bad/code"}, {ShippedFrom: "2026-99-01 00:00:00"}, {ShippedFrom: "2026-10-25 12:00:00", ShippedTo: "2026-10-25 11:00:00"}, {ShopID: -1}} {
		if validateShipmentQuery(q) == nil {
			t.Fatal("invalid shipment filter")
		}
	}
	if validateShipmentQuery(model.ShipmentQuery{Mode: "MANUAL", TrackingNo: "000123", ShippedFrom: "2026-10-25 00:00:00", ShippedTo: "2026-10-25 23:59:59"}) != nil {
		t.Fatal("valid filter")
	}
}
