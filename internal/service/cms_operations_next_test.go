package service

import (
	"ruoyi-go/internal/model"
	"strings"
	"testing"
)

func TestInquiryDateFilterBounds(t *testing.T) {
	f, e := cmsNormalizeInquiryFilter(model.CMSInquiryFilter{From: "2024-02-29", To: "2024-02-29"})
	if e != nil || f.From != "2024-02-29 00:00:00" || f.To != "2024-03-01 00:00:00" {
		t.Fatal("inclusive date boundary", e)
	}
	for _, f := range []model.CMSInquiryFilter{{From: "2023-02-29"}, {To: "2000-13-01"}, {From: "2025-01-02", To: "2025-01-01"}} {
		if _, e = cmsNormalizeInquiryFilter(f); e == nil {
			t.Fatal("invalid date accepted")
		}
	}
}
func TestComparisonDoesNotFlattenAwayMarkupOrNestedFields(t *testing.T) {
	s := model.CMSReviewSnapshot{Content: &model.CMSContentDetail{Draft: model.CMSRevision{Title: "标题", BodyHTML: "<p>文字&lt;img&gt;</p>"}}}
	f := cmsComparisonFields(s)
	if f["正文文字"] != "文字<img>" || !strings.Contains(f["正文HTML与图片标记"], "<p>") {
		t.Fatal("plain and source comparison must remain distinct")
	}
}
