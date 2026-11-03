package service

import (
	"context"
	"encoding/json"
	"fmt"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/portalsearch"
	"ruoyi-go/pkg/textdiff"
	"sort"
	"strings"
)

func cmsComparisonFields(s model.CMSReviewSnapshot) map[string]string {
	out := map[string]string{}
	j := func(v any) string { b, _ := json.MarshalIndent(v, "", "  "); return string(b) }
	if s.Content != nil {
		r := s.Content.Draft
		out = map[string]string{"标题": r.Title, "访问地址": s.Content.Slug, "摘要": r.Summary, "正文文字": portalsearch.Plain(r.BodyHTML), "正文HTML与图片标记": r.BodyHTML, "作者": r.Author, "栏目ID": j(r.ChannelId), "封面ID": j(r.CoverAssetId), "封面说明": r.CoverAltText, "SEO标题": r.SeoTitle, "SEO描述": r.SeoDescription, "产品编号": r.ProductCode, "产品参数": r.ProductParams, "产品图集": j(r.GalleryIds), "修订备注": r.Remark}
	}
	var walk func(string, any)
	walk = func(path string, v any) {
		switch x := v.(type) {
		case map[string]any:
			if len(x) == 0 {
				out[path] = "{}"
			}
			for k, v := range x {
				walk(strings.TrimPrefix(path+"."+k, "."), v)
			}
		case []any:
			if len(x) == 0 {
				out[path] = "[]"
			}
			for i, v := range x {
				walk(fmt.Sprintf("%s[%d]", path, i), v)
			}
		case string:
			out[path] = x
		default:
			out[path] = j(v)
		}
	}
	if s.Operations != nil {
		b, _ := json.Marshal(s.Operations)
		var v any
		_ = json.Unmarshal(b, &v)
		walk("运营/分享", v)
	}
	if s.Site != nil {
		b, _ := json.Marshal(s.Site)
		var v any
		_ = json.Unmarshal(b, &v)
		walk("外观", v)
	}
	return out
}
func CMSCompare(ctx context.Context, in model.CMSCompareInput) (model.CMSComparison, error) {
	left, right, e := repository.CMSCompareSnapshots(ctx, in)
	if e != nil {
		return model.CMSComparison{}, e
	}
	a, b := cmsComparisonFields(left.Snapshot), cmsComparisonFields(right.Snapshot)
	notices := []string{}
	// Old content revisions never stored the historical slug. Do not present the
	// current address as though it were a saved historical value.
	if in.Left.Source == "REVISION" || in.Right.Source == "REVISION" {
		delete(a, "访问地址")
		delete(b, "访问地址")
		notices = append(notices, "历史内容修订未记录当时的访问地址，本次不比较地址；首次发布后地址仍锁定。")
	}

	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	ordered := []string{}
	for k := range keys {
		ordered = append(ordered, k)
	}
	sort.Strings(ordered)
	out := model.CMSComparison{Notices: notices, LeftLabel: left.Label, RightLabel: right.Label, Fields: []model.CMSDiffField{}}
	for _, k := range ordered {
		f := model.CMSDiffField{Path: k, Label: k, Left: a[k], Right: b[k], Changed: a[k] != b[k], Parts: []model.CMSDiffPart{}}
		parts, coarse := textdiff.Diff(a[k], b[k])
		f.Coarse = coarse
		for _, p := range parts {
			f.Parts = append(f.Parts, model.CMSDiffPart{Text: p.Text, Op: p.Op})
		}
		if f.Changed {
			out.ChangedCount++
		}
		out.Fields = append(out.Fields, f)
	}
	return out, nil
}
