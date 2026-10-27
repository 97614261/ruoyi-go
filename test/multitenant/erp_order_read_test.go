package multitenant_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"sort"
	"testing"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
)

// Isolated read fixtures intentionally include a historical unknown status and
// no workflow lines. They test read projection/counting, not valid transitions.
func checkOrderReadPages(t *testing.T, parent *suite, platform string) {
	local := *parent
	local.t = t
	s := &local
	a, b := newOperationsFixture(t, s, platform), newOperationsFixture(t, s, platform)
	ctx := erpctx.WithUser(tenantctx.With(context.Background(), a.tenant.TenantID, false), *a.tenant.OwnerUserID)
	shop := func(f operationsFixture, code string) int64 {
		r := s.req("POST", "/erp/shop", f.owner, map[string]any{"shopCode": code, "shopName": code, "channelCode": "MANUAL", "siteCode": "CN", "currency": "CNY", "status": "0"})
		s.ok(r)
		return int64(r["data"].(map[string]any)["shopId"].(float64))
	}
	shops := []int64{shop(a, "READ-A"), shop(a, "READ-B")}
	foreign := shop(b, "READ-A")
	statuses := []string{"PENDING_REVIEW", "READY_TO_SHIP", "SHIPPED", "CANCELLED", "LEGACY"}
	orders := make([]model.ERPOrder, 10047)
	for i := range orders {
		no := fmt.Sprintf("READ-%03d", i)
		if i < 2 {
			no = "READ-SAME"
		}
		shopID := shops[0]
		if i == 1 || i >= 10028 {
			shopID = shops[1]
		}
		orders[i] = model.ERPOrder{ERPOrderFields: model.ERPOrderFields{ReceiverName: "PRIVATE_READ_FIXTURE", ReceiverAddress: "PRIVATE_READ_FIXTURE", Currency: "CNY", OrderedAt: types.Now()}, ShopID: shopID, OrderNo: fmt.Sprintf("READ-NO-%03d", i), SourceType: "MANUAL", SourceOrderNo: no, CreateRequestKey: fmt.Sprintf("read-seed-%03d", i), CreateRequestHash: "read-only-synthetic-fixture", Status: statuses[i%len(statuses)], Version: int64(i + 1)}
		if i%2 == 0 {
			amount := "0.0000"
			orders[i].OriginalAmount = &amount
		}
	}
	if err := repository.DB(ctx).CreateInBatches(&orders, 200).Error; err != nil {
		t.Fatal(err)
	}
	// Reload timestamps at stored precision before building an independent oracle.
	if err := repository.DB(ctx).Order("order_id DESC").Find(&orders).Error; err != nil {
		t.Fatal(err)
	}
	sort.Slice(orders, func(i, j int) bool { return orders[i].OrderID > orders[j].OrderID })
	s.ok(s.req("POST", "/system/role", a.owner, map[string]any{"roleName": "订单读取验收", "roleKey": "read_pages", "roleSort": 0, "status": "0", "dataScope": "1", "menuIds": []int64{6000, 7100, 7300, 7301, 7302, 9400}}))
	r := s.req("GET", "/system/role/list?roleKey=read_pages", a.owner, nil)
	s.ok(r)
	role := r["rows"].([]any)[0].(map[string]any)["roleId"]
	s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": "reademployee", "nickName": "读取验收", "password": operationsPassword, "status": "0", "roleIds": []any{role}}))
	r = s.req("GET", "/system/user/list?userName=reademployee", a.owner, nil)
	s.ok(r)
	uid := int64(r["rows"].([]any)[0].(map[string]any)["userId"].(float64))
	employee := s.loginAs(a.code, "reademployee", operationsPassword)
	t.Cleanup(func() { s.req("POST", "/logout", employee, nil) })
	grant := func(ids []int64) {
		p := fmt.Sprintf("/erp/shop/access/%d", uid)
		v := s.req("GET", p, a.owner, nil)
		s.ok(v)
		s.ok(s.req("PUT", p, a.owner, map[string]any{"version": v["data"].(map[string]any)["version"], "shopIds": ids}))
	}
	t.Cleanup(func() { grant([]int64{}) })
	jsonValue := func(v any) any {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var out any
		if err = json.Unmarshal(b, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	counts := func(rows []model.ERPOrder) any {
		out := model.ERPOrderWorkbench{Total: int64(len(rows))}
		for _, o := range rows {
			switch o.Status {
			case "PENDING_REVIEW":
				out.PendingReview++
			case "READY_TO_SHIP":
				out.ReadyToShip++
			case "SHIPPED":
				out.Shipped++
			case "CANCELLED":
				out.Cancelled++
			}
		}
		return jsonValue(out)
	}
	check := func(token string, visible []int64) {
		t.Helper()
		allowed := map[int64]bool{}
		for _, id := range visible {
			allowed[id] = true
		}
		for _, filter := range []model.ERPOrderQuery{{}, {ShopID: shops[0]}, {ShopID: shops[1]}, {ShopID: foreign}, {Status: "CANCELLED"}, {SourceOrderNo: "READ-SAME"}, {SourceOrderNo: "missing-' OR 1=1"}, {ShopID: shops[0], Status: "SHIPPED", SourceOrderNo: "READ-SAME"}} {
			params := url.Values{}
			if filter.ShopID > 0 {
				params.Set("shopId", fmt.Sprint(filter.ShopID))
			}
			if filter.Status != "" {
				params.Set("status", filter.Status)
			}
			if filter.SourceOrderNo != "" {
				params.Set("sourceOrderNo", filter.SourceOrderNo)
			}
			selected, allStatuses := []model.ERPOrder{}, []model.ERPOrder{}
			for _, o := range orders {
				if !allowed[o.ShopID] || (filter.ShopID > 0 && filter.ShopID != o.ShopID) || (filter.SourceOrderNo != "" && filter.SourceOrderNo != o.SourceOrderNo) {
					continue
				}
				allStatuses = append(allStatuses, o)
				if filter.Status == "" || filter.Status == o.Status {
					selected = append(selected, o)
				}
			}
			stats := s.req("GET", "/erp/order/workbench?"+params.Encode(), token, nil)
			s.ok(stats)
			if !reflect.DeepEqual(stats["data"], counts(allStatuses)) {
				t.Fatal("tab counts changed scope/status semantics", filter, stats)
			}
			for _, p := range [][2]int{{1, 7}, {2, 7}, {4, 7}, {7, 7}, {999, 7}, {1, 100}, {501, 20}, {502, 20}, {503, 20}, {9999, 7}} {
				params.Set("pageNum", fmt.Sprint(p[0]))
				params.Set("pageSize", fmt.Sprint(p[1]))
				got := s.req("GET", "/erp/order/list?"+params.Encode(), token, nil)
				s.ok(got)
				start, end := (p[0]-1)*p[1], p[0]*p[1]
				if start > len(selected) {
					start = len(selected)
				}
				if end > len(selected) {
					end = len(selected)
				}
				want := []model.ERPOrderSummary{}
				for _, o := range selected[start:end] {
					want = append(want, model.ERPOrderSummary{OrderID: o.OrderID, ShopID: o.ShopID, OrderNo: o.OrderNo, SourceType: o.SourceType, SourceOrderNo: o.SourceOrderNo, Status: o.Status, Version: o.Version, Currency: o.Currency, OriginalAmount: o.OriginalAmount, OrderedAt: o.OrderedAt})
				}
				if got["total"] != float64(len(selected)) || !reflect.DeepEqual(got["rows"], jsonValue(want)) {
					t.Fatal("page/count/order/projection drift", filter, p, got)
				}
			}
		}
		visibleOrders := []model.ERPOrder{}
		for _, o := range orders {
			if allowed[o.ShopID] {
				visibleOrders = append(visibleOrders, o)
			}
		}
		got := s.req("GET", "/erp/overview/orders", token, nil)
		s.ok(got)
		data := got["data"].(map[string]any)
		if data["available"] != true || !reflect.DeepEqual(data["counts"], counts(visibleOrders)) {
			t.Fatal("overview counts drift", got)
		}
	}
	check(a.owner, shops)
	check(employee, nil)
	grant(shops[:1])
	check(employee, shops[:1])
	grant(shops)
	check(employee, shops)
	// Disabled shops still contribute historical orders. Use the same old token.
	r = s.req("GET", fmt.Sprintf("/erp/shop/%d", shops[1]), a.owner, nil)
	s.ok(r)
	input := r["data"].(map[string]any)
	input["status"] = "1"
	s.ok(s.req("PUT", fmt.Sprintf("/erp/shop/%d", shops[1]), a.owner, input))
	check(a.owner, shops)
	check(employee, shops)
	grant([]int64{})
	check(employee, nil)
	// SQL comparison is case-insensitive; the original grouped Go switch is
	// spelling-sensitive. Preserve both workbench and overview's distinct old
	// behavior for historical spellings, without permitting such new writes.
	caseShop := shop(a, "READ-CASE")
	before := s.req("GET", "/erp/overview/orders", a.owner, nil)
	s.ok(before)
	baseline := before["data"].(map[string]any)["counts"].(map[string]any)
	for i, status := range []string{"pending_review", "PENDING_REVIEW ", "PÉNDING_REVIEW", "PENDING_REVIEW"} {
		o := model.ERPOrder{ERPOrderFields: model.ERPOrderFields{Currency: "CNY", OrderedAt: types.Now()}, ShopID: caseShop, OrderNo: fmt.Sprintf("CASE-%d", i), SourceType: "MANUAL", SourceOrderNo: fmt.Sprintf("CASE-%d", i), CreateRequestKey: fmt.Sprintf("case-seed-%d", i), CreateRequestHash: "synthetic-read-fixture", Status: status, Version: 1}
		if err := repository.DB(ctx).Create(&o).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 4; i++ {
		var expected model.ERPOrderWorkbench
		expected.Total = 1
		if i == 3 {
			expected.PendingReview = 1
		}
		got := s.req("GET", fmt.Sprintf("/erp/order/workbench?shopId=%d&sourceOrderNo=CASE-%d&status=CANCELLED", caseShop, i), a.owner, nil)
		s.ok(got)
		if !reflect.DeepEqual(got["data"], jsonValue(expected)) {
			t.Fatal("historical spelling changed legacy workbench", i, got)
		}
	}
	var legacy []struct {
		Status string
		Total  int64
	}
	if err := repository.DB(ctx).Model(&model.ERPOrder{}).Where("shop_id=?", caseShop).Select("status,COUNT(*) AS total").Group("status").Scan(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	var expected model.ERPOrderWorkbench
	for _, g := range legacy {
		expected.Total += g.Total
		if g.Status == "PENDING_REVIEW" {
			expected.PendingReview = g.Total
		}
	}
	got := s.req("GET", fmt.Sprintf("/erp/order/workbench?shopId=%d", caseShop), a.owner, nil)
	s.ok(got)
	if !reflect.DeepEqual(got["data"], jsonValue(expected)) {
		t.Fatal("collation-colliding group changed", got)
	}
	after := s.req("GET", "/erp/overview/orders", a.owner, nil)
	s.ok(after)
	actual := after["data"].(map[string]any)["counts"].(map[string]any)
	if actual["total"] != baseline["total"].(float64)+4 || actual["pendingReview"] != baseline["pendingReview"].(float64)+4 {
		t.Fatal("overview's original SQL collation semantics changed", after)
	}
}
