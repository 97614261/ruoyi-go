package service

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
	"strings"
	"testing"
)

func TestERPShopValidation(t *testing.T) {
	good := model.ERPShopWrite{ShopCode: "SHOP-1", ShopName: "店铺", ChannelCode: " manual ", SiteCode: "cn", Currency: "cny", Status: "0"}
	v, err := prepareERPShop(good)
	if err != nil || v.ChannelCode != "MANUAL" || v.SiteCode != "CN" || v.Currency != "CNY" || v.TenantID != 0 {
		t.Fatal("canonical validation failed", err)
	}
	for _, test := range []struct {
		name   string
		change func(*model.ERPShopWrite)
	}{
		{"blankCode", func(x *model.ERPShopWrite) { x.ShopCode = "  " }}, {"unicodeCode", func(x *model.ERPShopWrite) { x.ShopCode = "中文" }},
		{"longCode", func(x *model.ERPShopWrite) { x.ShopCode = strings.Repeat("x", 65) }}, {"unsafeCode", func(x *model.ERPShopWrite) { x.ShopCode = "a' OR 1=1" }},
		{"blankName", func(x *model.ERPShopWrite) { x.ShopName = "  " }}, {"longName", func(x *model.ERPShopWrite) { x.ShopName = strings.Repeat("中", 121) }},
		{"channel", func(x *model.ERPShopWrite) { x.ChannelCode = "UNKNOWN" }}, {"site", func(x *model.ERPShopWrite) { x.SiteCode = "US/EAST" }},
		{"currency", func(x *model.ERPShopWrite) { x.Currency = "C1Y" }}, {"status", func(x *model.ERPShopWrite) { x.Status = "2" }},
		{"remark", func(x *model.ERPShopWrite) { x.Remark = strings.Repeat("中", 501) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := good
			test.change(&input)
			if _, err := prepareERPShop(input); err == nil {
				t.Fatal("invalid input accepted")
			}
		})
	}
	if err = UpdateErpShop(context.Background(), 1, good, "test"); err == nil {
		t.Fatal("missing version accepted")
	}
	for _, ids := range [][]int64{nil, {1, 1}, {0}, {-1}, make([]int64, 201)} {
		if err = SetERPShopAccess(context.Background(), 1, 1, ids, "test"); err == nil {
			t.Fatal("invalid scope accepted")
		}
	}
}
func TestERPChannelPaginationAndCapabilities(t *testing.T) {
	ctx := context.Background()
	seen := map[string]bool{}
	for i := 1; i <= 6; i++ {
		rows, total, err := ListERPChannels(ctx, page.Query{PageNum: i, PageSize: 1})
		if err != nil || total != 6 || len(rows) != 1 {
			t.Fatal("pagination", err)
		}
		v := rows[0]
		if seen[v.ChannelCode] || v.SupportsAPI || len(v.Sources) != 2 {
			t.Fatal("invalid capability")
		}
		seen[v.ChannelCode] = true
	}
	rows, total, err := ListERPChannels(ctx, page.Query{PageNum: 100, PageSize: 100})
	if err != nil || total != 6 || len(rows) != 0 || rows == nil {
		t.Fatal("empty page contract")
	}
	rows, _, _ = ListERPChannels(ctx, page.Query{PageNum: 1, PageSize: 100})
	rows[0].Sources[0] = "mutated"
	again, _, _ := ListERPChannels(ctx, page.Query{PageNum: 1, PageSize: 100})
	if again[0].Sources[0] != "MANUAL" {
		t.Fatal("catalog shared mutable slice")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err = ListERPChannels(canceled, page.Query{}); err == nil {
		t.Fatal("canceled request accepted")
	}
}
