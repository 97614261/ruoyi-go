package repository

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/tenantctx"
)

func TestOrderCreateCandidatesRetainAllTenantAndLiveShopBoundaries(t *testing.T) {
	ctx := erpctx.WithUser(tenantctx.With(context.Background(), 417, false), 821)
	in := model.ERPOrderWrite{RequestKey: "bound-key-' OR 1=1", ShopID: 37, SourceOrderNo: "source-' OR 1=1"}
	var rows []model.ERPOrder
	q := orderCreateCandidates(guardDB(t).WithContext(ctx), in).Find(&rows)
	if q.Error != nil {
		t.Fatal(q.Error)
	}
	sql := q.Statement.SQL.String()
	if strings.Count(sql, "tenant_id=?") != 4 || !strings.Contains(sql, "`erp_order`.`tenant_id` = ?") || !strings.Contains(sql, "erp_order.shop_id IN (SELECT shop_id FROM erp_shop_access") {
		t.Fatalf("missing explicit base tenant predicates or outer current guards: %s", sql)
	}
	if !strings.Contains(sql, "UNION SELECT") || strings.Contains(sql, "UNION ALL") || strings.Contains(sql, in.RequestKey) || strings.Contains(sql, in.SourceOrderNo) {
		t.Fatal("union does not deduplicate or request input entered SQL text")
	}
	want := []any{int64(417), in.RequestKey, int64(417), in.ShopID, in.SourceOrderNo, int64(417), int64(821), int64(417), int64(821), int64(417)}
	if !reflect.DeepEqual(q.Statement.Vars, want) {
		t.Fatalf("tenant or input bindings drifted: %#v", q.Statement.Vars)
	}
}

func TestOrderCreateCandidatesRejectMissingAndPlatformContext(t *testing.T) {
	for _, ctx := range []context.Context{context.Background(), tenantctx.With(context.Background(), 417, false), erpctx.WithUser(tenantctx.With(context.Background(), 1, true), 821)} {
		var rows []model.ERPOrder
		if orderCreateCandidates(guardDB(t).WithContext(ctx), model.ERPOrderWrite{ShopID: 37, RequestKey: "original-key", SourceOrderNo: "source"}).Find(&rows).Error == nil {
			t.Fatal("missing employee/tenant identity or platform access was accepted")
		}
	}
}
