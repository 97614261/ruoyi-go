package service

import (
	"context"
	"ruoyi-go/internal/model"
	"strings"
	"testing"
)

func TestShopItemIdentifiersAndValidation(t *testing.T) {
	ptr := func(v string) *string { return &v }
	good := model.ERPShopItemWrite{ShopID: 1, ShopItemCode: "00001-Ab中", ExternalProductID: ptr("000001"), ExternalVariantID: ptr("00002"), Title: " 商品 ", Status: "0"}
	item, err := prepareERPShopItem(good)
	if err != nil || item.ShopItemCode != good.ShopItemCode || *item.ExternalProductID != "000001" || item.Title != "商品" || item.TenantID != 0 {
		t.Fatal("identifier contract", err)
	}
	for _, v := range []string{" x", "x ", "x\n", "x\tY", strings.Repeat("中", 129)} {
		x := good
		x.ShopItemCode = v
		if _, err = prepareERPShopItem(x); err == nil {
			t.Fatal("invalid identifier accepted", v)
		}
	}
	for _, v := range []string{"  ", strings.Repeat("x", 129), "x\x00y"} {
		x := good
		x.ExternalVariantID = ptr(v)
		if _, err = prepareERPShopItem(x); err == nil {
			t.Fatal("invalid external ID accepted")
		}
	}
	x := good
	x.ExternalVariantID = ptr("")
	item, err = prepareERPShopItem(x)
	if err != nil || item.ExternalVariantID != nil {
		t.Fatal("optional empty must become SQL NULL", err)
	}
	for _, mutate := range []func(*model.ERPShopItemWrite){func(x *model.ERPShopItemWrite) { x.Title = " " }, func(x *model.ERPShopItemWrite) { x.Title = strings.Repeat("中", 201) }, func(x *model.ERPShopItemWrite) { x.ShopID = 0 }, func(x *model.ERPShopItemWrite) { x.Status = "2" }, func(x *model.ERPShopItemWrite) { x.Remark = strings.Repeat("中", 501) }} {
		x := good
		mutate(&x)
		if _, err = prepareERPShopItem(x); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
	if err = EditERPShopItem(context.Background(), 1, good, "test"); err == nil {
		t.Fatal("missing version accepted")
	}
	for _, input := range []model.ERPMappingWrite{{Version: 0, SkuID: 1}, {Version: 1, SkuID: 0}, {Version: 1, SkuID: 1, Remark: strings.Repeat("x", 501)}} {
		if _, err = SetERPShopItemMapping(context.Background(), 1, input, "test"); err == nil {
			t.Fatal("invalid mapping accepted")
		}
	}
	if _, err = CancelERPShopItemMapping(context.Background(), 1, 0, "test"); err == nil {
		t.Fatal("missing cancel version")
	}
}
