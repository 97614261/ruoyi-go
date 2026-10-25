package service

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"testing"
)

func TestTransferValidationAndFingerprint(t *testing.T) {
	ctx := erpctx.WithUser(context.Background(), 7)
	in := model.ERPTransferInput{SourceWarehouseID: 1, TargetWarehouseID: 2, RequestKey: "request-key-7", Lines: []model.ERPLineInput{{SkuID: 2, Quantity: 3}, {SkuID: 1, Quantity: 1}}}
	sorted, h, e := prepareTransfer(ctx, 0, "CREATE", in)
	if e != nil || sorted.Lines[0].SkuID != 1 {
		t.Fatal(e)
	}
	if in.Lines[0].SkuID != 2 {
		t.Fatal("mutated caller")
	}
	_, other, e := prepareTransfer(erpctx.WithUser(ctx, 8), 0, "CREATE", in)
	if e != nil || h == other {
		t.Fatal("actor not fingerprinted")
	}
	in.Lines = []model.ERPLineInput{{SkuID: 1, Quantity: 1}, {SkuID: 1, Quantity: 2}}
	if _, _, e = prepareTransfer(ctx, 0, "CREATE", in); e == nil {
		t.Fatal("duplicate")
	}
	in.Lines = []model.ERPLineInput{{SkuID: 1, Quantity: 0}}
	if _, _, e = prepareTransfer(ctx, 0, "CREATE", in); e == nil {
		t.Fatal("zero")
	}
	in.TargetWarehouseID = 1
	if _, _, e = prepareTransfer(ctx, 0, "CREATE", in); e == nil {
		t.Fatal("same warehouse")
	}
	for _, q := range []model.ERPQuery{{BeginTime: "2026-02-30 00:00:00"}, {BeginTime: "2026-10-24 00:00:00", EndTime: "2026-10-23 00:00:00"}, {StockState: "inject"}} {
		if ValidateERPStockQuery(q) == nil {
			t.Fatal("invalid query")
		}
	}
}
