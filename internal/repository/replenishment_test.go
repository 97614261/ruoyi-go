package repository

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/tenantctx"
	"strings"
	"testing"
)

func TestReplenishmentQuantitiesAndBasis(t *testing.T) {
	r := model.ERPReplenishment{WarehouseID: 1, SkuID: 2, Unit: "件", OnHand: 10, Reserved: 7, ThresholdQuantity: 10, AlertVersion: 1, DraftQuantity: 2, IncomingQuantity: 3}
	if e := finishReplenishment(1, &r); e != nil {
		t.Fatal(e)
	}
	if r.Available != 3 || r.TargetQuantity != 11 || r.NeededQuantity != 3 || r.SuggestedQuantity != 3 || len(r.Basis) != 64 {
		t.Fatal(r)
	}
	before := r.Basis
	r.DraftQuantity += 3
	if e := finishReplenishment(1, &r); e != nil || r.NeededQuantity != 0 || r.Basis == before {
		t.Fatal(r, e)
	}
	r = model.ERPReplenishment{ThresholdQuantity: 0}
	if e := finishReplenishment(1, &r); e != nil || r.NeededQuantity != 1 {
		t.Fatal(r, e)
	}
	r.ThresholdQuantity = 1000000000000
	if e := finishReplenishment(1, &r); e != nil || r.NeededQuantity != 1000000000001 || r.SuggestedQuantity != 1000000000 {
		t.Fatal(r, e)
	}
	r.Reserved = 1
	if finishReplenishment(1, &r) == nil {
		t.Fatal("invalid negative availability accepted")
	}
}
func TestReplenishmentReadScope(t *testing.T) {
	for _, ctx := range []context.Context{context.Background(), tenantctx.With(context.Background(), 17, false), erpctx.WithUser(tenantctx.With(context.Background(), 17, true), 8)} {
		if replenishmentQuery(guardDB(t).WithContext(ctx), 3).Error == nil {
			t.Fatal("unscoped read accepted")
		}
	}
	ctx := erpctx.WithUser(tenantctx.With(context.Background(), 17, false), 8)
	q := replenishmentQuery(guardDB(t).WithContext(ctx), 3).Select(replenishmentColumns).Find(&[]model.ERPReplenishment{})
	if q.Error != nil {
		t.Fatal(q.Error)
	}
	s := q.Statement.SQL.String()
	for _, fragment := range []string{"tenant_id", "erp_warehouse_access", "threshold_quantity IS NOT NULL", "s.status='0'", "w.status='0'"} {
		if !strings.Contains(s, fragment) {
			t.Fatal(s)
		}
	}
}
