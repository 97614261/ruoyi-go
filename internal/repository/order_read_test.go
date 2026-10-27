package repository

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/tenantctx"
)

func TestOrderPageScopesBeforeOffsetAndGuardsTheSummaryJoin(t *testing.T) {
	ctx := erpctx.WithUser(tenantctx.With(context.Background(), 417, false), 821)
	filter := model.ERPOrderQuery{ShopID: 37, Status: "CANCELLED", SourceOrderNo: "source-' OR 1=1"}
	var rows []model.ERPOrderSummary
	q := orderPageQuery(guardDB(t).WithContext(ctx), filter, page.Query{PageNum: 2000, PageSize: 20}).Find(&rows)
	if q.Error != nil {
		t.Fatal(q.Error)
	}
	sql := q.Statement.SQL.String()
	innerEnd := strings.Index(sql, ") AS order_page INNER JOIN")
	if innerEnd < 0 {
		t.Fatalf("page is not a single derived-table read: %s", sql)
	}
	inner, outer := sql[:innerEnd], sql[innerEnd:]
	for _, part := range []string{"SELECT tenant_id,order_id FROM `erp_order`", "shop_id=?", "status=?", "source_order_no=?", "erp_shop_access", "`erp_order`.`tenant_id` = ?", "ORDER BY order_id DESC LIMIT ? OFFSET ?"} {
		if !strings.Contains(inner, part) {
			t.Fatalf("page selection lost a predicate/ordering: %s", sql)
		}
	}
	for _, part := range []string{"erp_order.tenant_id=order_page.tenant_id", "erp_order.order_id=order_page.order_id", "erp_order.tenant_id = 417", "`order_page`.`tenant_id` = ?", "erp_shop_access", "ORDER BY erp_order.order_id DESC"} {
		if !strings.Contains(outer, part) {
			t.Fatalf("summary join lost tenant/current scope/order: %s", sql)
		}
	}
	if strings.Contains(sql, filter.SourceOrderNo) || strings.Contains(sql, "receiver_") || strings.Contains(sql, "SELECT *") {
		t.Fatal("input entered SQL or page fetched private/full order columns")
	}
	want := []any{int64(417), int64(821), int64(417), int64(821), filter.ShopID, filter.Status, filter.SourceOrderNo, int64(417), 20, 39980, int64(417), int64(821), int64(417), int64(821), int64(417)}
	if !reflect.DeepEqual(q.Statement.Vars, want) {
		t.Fatalf("page bindings drifted: %#v", q.Statement.Vars)
	}
}

func TestOrderPageRejectsMissingAndPlatformContext(t *testing.T) {
	for _, ctx := range []context.Context{context.Background(), tenantctx.With(context.Background(), 417, false), erpctx.WithUser(tenantctx.With(context.Background(), 1, true), 821)} {
		for _, n := range []int{1, 2000} {
			var rows []model.ERPOrderSummary
			if orderPageQuery(guardDB(t).WithContext(ctx), model.ERPOrderQuery{}, page.Query{PageNum: n, PageSize: 20}).Find(&rows).Error == nil {
				t.Fatal("missing employee/tenant identity or platform access was accepted")
			}
		}
	}
}

func TestOrderShallowPagesRetainOriginalQuery(t *testing.T) {
	ctx := erpctx.WithUser(tenantctx.With(context.Background(), 417, false), 821)
	filter := model.ERPOrderQuery{ShopID: 37, Status: "CANCELLED", SourceOrderNo: "bound-source"}
	for _, offset := range []int{0, 20, 100, 500, 1000, 5000, 9999} {
		var oldRows, newRows []model.ERPOrderSummary
		old := orderListQuery(guardDB(t).WithContext(ctx), filter).Select(orderSummaryColumns).Order("order_id DESC").Limit(1).Offset(offset).Find(&oldRows)
		got := orderPageQuery(guardDB(t).WithContext(ctx), filter, page.Query{PageNum: offset + 1, PageSize: 1}).Find(&newRows)
		if old.Error != nil || got.Error != nil || old.Statement.SQL.String() != got.Statement.SQL.String() || !reflect.DeepEqual(old.Statement.Vars, got.Statement.Vars) {
			t.Fatal("shallow page no longer uses the original scoped projection/query", offset)
		}
	}
}
