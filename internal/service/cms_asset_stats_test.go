package service

import (
	"ruoyi-go/internal/model"
	"strings"
	"testing"
)

func TestAssetGroupsAndRanges(t *testing.T) {
	for _, s := range []string{"", " 产品图 ", strings.Repeat("图", 40)} {
		if _, e := cmsAssetGroup(s); e != nil {
			t.Fatal(e)
		}
	}
	for _, s := range []string{"../x", "x/y", `x\y`, "x\ny", "<b>", strings.Repeat("图", 41)} {
		if _, e := cmsAssetGroup(s); e == nil {
			t.Fatal("bad group accepted", s)
		}
	}
	if got, _ := cmsAssetGroup(" 产品图 "); got != "产品图" {
		t.Fatal("group not normalized")
	}
	for _, q := range []model.CMSAssetQuery{{MinWidth: 2, MaxWidth: 1}, {MinBytes: 3, MaxBytes: 2}} {
		if _, e := cmsAssetQuery(q); e == nil {
			t.Fatal("inverted range")
		}
	}
	if _, e := cmsAssetQuery(model.CMSAssetQuery{MinWidth: 200, MaxWidth: 0}); e != nil {
		t.Fatal("zero should mean unbounded")
	}
}
