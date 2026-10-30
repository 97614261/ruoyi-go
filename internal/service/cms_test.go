package service

import (
	"ruoyi-go/internal/model"
	"testing"
)

func TestCMSGraphProtectsCyclesKindsDepthAndActiveParents(t *testing.T) {
	rows := []model.CmsChannel{{ChannelId: 1, ParentId: 0, Kind: "ARTICLE", Status: "0"}, {ChannelId: 2, ParentId: 1, Kind: "ARTICLE", Status: "0"}}
	for _, in := range []model.CMSChannelInput{
		{ChannelId: 1, ParentId: 2, Kind: "ARTICLE", Status: "0"},
		{ChannelId: 1, ParentId: 0, Kind: "PRODUCT", Status: "0"},
		{ChannelId: 1, ParentId: 0, Kind: "ARTICLE", Status: "1"},
		{ParentId: 99, Kind: "ARTICLE", Status: "0"},
	} {
		if err := validateCMSGraph(rows, in); err == nil {
			t.Fatal("invalid graph was accepted")
		}
	}
	if err := validateCMSGraph(rows, model.CMSChannelInput{ParentId: 2, Kind: "ARTICLE", Status: "0"}); err != nil {
		t.Fatal(err)
	}
	deep := []model.CmsChannel{}
	for i := int64(1); i <= 8; i++ {
		deep = append(deep, model.CmsChannel{ChannelId: i, ParentId: i - 1, Kind: "ARTICLE", Status: "0"})
	}
	if err := validateCMSGraph(deep, model.CMSChannelInput{ParentId: 8, Kind: "ARTICLE", Status: "0"}); err == nil {
		t.Fatal("ninth level accepted")
	}
}
