package service

import (
	"context"
	"ruoyi-go/internal/model"
	"testing"
)

func TestBatchValidationBeforeAnyMutation(t *testing.T) {
	cases := []model.CMSBatchInput{{Action: "publish"}, {Action: "remove", Items: []model.CMSBatchItem{{ContentId: 1, Version: 1}}}, {Action: "publish", Items: []model.CMSBatchItem{{ContentId: 1, Version: 1}, {ContentId: 1, Version: 2}}}, {Action: "publish", Items: []model.CMSBatchItem{{ContentId: 1, Version: 0}}}}
	many := model.CMSBatchInput{Action: "publish"}
	for i := 0; i < 21; i++ {
		many.Items = append(many.Items, model.CMSBatchItem{ContentId: int64(i + 1), Version: 1})
	}
	cases = append(cases, many)
	for _, in := range cases {
		if _, err := CMSBatchChange(context.Background(), "ARTICLE", in, "test", false); err == nil {
			t.Fatal("invalid batch allowed")
		}
	}
}
func TestPreflightAncestorScope(t *testing.T) {
	rows := cmsChannels([]model.CmsChannel{{ChannelId: 1, ParentId: 0, Kind: "ARTICLE", Status: "1"}, {ChannelId: 2, ParentId: 1, Kind: "ARTICLE", Status: "0"}})
	if cmsActiveChannel(2, "ARTICLE", rows) {
		t.Fatal("disabled ancestor allowed")
	}
	r := rows[1]
	r.Status = "0"
	rows[1] = r
	if !cmsActiveChannel(2, "ARTICLE", rows) {
		t.Fatal("valid ancestor refused")
	}
	if cmsActiveChannel(2, "PRODUCT", rows) {
		t.Fatal("wrong channel kind allowed")
	}
}
