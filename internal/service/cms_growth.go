package service

import (
	"context"
	"net/url"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func CMSTags(ctx context.Context, q model.CMSTagQuery, pg page.Query) ([]model.CMSTag, int64, error) {
	return repository.CMSTags(ctx, q, pg)
}
func CMSSaveTag(ctx context.Context, in model.CMSTag, operator string) error {
	in.Name = strings.TrimSpace(in.Name)
	for _, ch := range in.Name {
		if unicode.IsControl(ch) {
			return errs.New("标签名称不能包含控制字符")
		}
	}
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 40 || strings.ContainsAny(in.Name, "<>\x00") {
		return errs.New("标签名称不正确")
	}
	return repository.CMSSaveTag(ctx, in, operator)
}
func CMSGetOps(ctx context.Context, k string, id int64) (*model.CMSOpsView, error) {
	return repository.CMSGetOps(ctx, k, id)
}
func ValidateContentOps(o model.CMSContentOps) error {
	if o.PinOrder < 0 || o.PinOrder > 100000 || o.ShareAssetId < 0 || len(o.TagIds) > 12 || len(o.RelatedIds) > 12 || utf8.RuneCountInString(o.ShareTitle) > 150 || utf8.RuneCountInString(o.ShareDescription) > 300 || strings.ContainsAny(o.ShareTitle+o.ShareDescription, "<>\x00") {
		return errs.New("运营与分享字段不正确或超限")
	}
	for _, ids := range [][]int64{o.TagIds, o.RelatedIds} {
		seen := map[int64]bool{}
		for _, id := range ids {
			if id < 1 || seen[id] {
				return errs.New("标签/关联编号必须为不重复的正数")
			}
			seen[id] = true
		}
	}
	return nil
}
func CMSSetOps(ctx context.Context, k string, id int64, in model.CMSOpsInput, operator string, assets bool) error {
	if e := ValidateContentOps(in.Settings); e != nil {
		return e
	}
	if in.Settings.ShareAssetId > 0 && !assets {
		return errs.ErrForbidden
	}
	if k == "PAGE" && (len(in.Settings.TagIds) > 0 || in.Settings.Featured || in.Settings.PinOrder > 0) {
		return errs.New("单页不支持标签、推荐或置顶")
	}
	d, e := repository.CMSContentDetail(ctx, k, id)
	if e != nil {
		return e
	}
	d.Draft.Ops = &in.Settings
	ids, e := cmsRevisionAssets(d.Draft)
	if e != nil {
		return e
	}
	return repository.CMSSetOps(ctx, k, id, in, ids, operator)
}
func CMSRecoverContent(ctx context.Context, k string, id, v int64, operator string) error {
	return repository.CMSRecoverContent(ctx, k, id, v, operator)
}
func CMSRelated(ctx context.Context, ids []int64) ([]model.PortalCard, error) {
	return repository.CMSRelated(ctx, ids)
}
func VisitDimensions(path, referrer, query string, cfg model.SiteConfig) []model.CMSVisit {
	if !cfg.AnalyticsEnabled {
		return nil
	}
	day := time.Now().In(types.Location).Format("2006-01-02")
	rows := []model.CMSVisit{}
	add := func(d, v string) {
		if utf8.RuneCountInString(v) > 160 {
			return
		}
		rows = append(rows, model.CMSVisit{VisitDay: day, Dimension: d, Value: v, Hits: 1, CreateBy: "portal", CreateTime: types.Now()})
	}
	add("total", "")
	add("page", path)
	source := "直接访问"
	if u, e := url.Parse(referrer); e == nil && u.User == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" {
		source = strings.ToLower(u.Hostname())
	}
	add("source", source)
	query = strings.TrimSpace(query)
	if cfg.AnalyticsKeywords && query != "" && utf8.RuneCountInString(query) <= 60 {
		valid := true
		for _, c := range query {
			if unicode.IsControl(c) {
				valid = false
			}
		}
		if valid {
			add("keyword", query)
		}
	}
	return rows
}
func CMSRecordVisit(ctx context.Context, path, referrer, query string, cfg model.SiteConfig) {
	rows := VisitDimensions(path, referrer, query, cfg)
	if len(rows) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	_ = repository.CMSRecordVisit(ctx, rows)
}
func NormalizeVisitQuery(q model.CMSVisitQuery) (model.CMSVisitQuery, error) {
	if q.Dimension != "" && q.Dimension != "day" && q.Dimension != "page" && q.Dimension != "source" && q.Dimension != "keyword" {
		return q, errs.New("统计维度错误")
	}
	today := time.Now().In(types.Location)
	if q.From == "" {
		q.From = today.AddDate(0, 0, -6).Format("2006-01-02")
	}
	if q.To == "" {
		q.To = today.Format("2006-01-02")
	}
	if q.Dimension == "" {
		q.Dimension = "day"
	}
	a, e := time.Parse("2006-01-02", q.From)
	if e != nil {
		return q, errs.New("开始日期格式错误")
	}
	b, e := time.Parse("2006-01-02", q.To)
	if e != nil || b.Before(a) || b.Sub(a) >= 90*24*time.Hour {
		return q, errs.New("日期范围最多90天且不能倒置")
	}
	return q, nil
}
func CMSVisits(ctx context.Context, q model.CMSVisitQuery, pg page.Query) ([]model.CMSVisitRow, int64, error) {
	q, e := NormalizeVisitQuery(q)
	if e != nil {
		return nil, 0, e
	}
	return repository.CMSVisits(ctx, q, pg)
}

func PortalFiltered(ctx context.Context, kind, channel, q string, tag int64, offset, limit int) (*model.PortalResults, error) {
	if kind == "" && q != "" {
		return portalFullText(ctx, q, tag, offset, limit)
	}
	return repository.PortalResults(repository.PortalFilteredContext(ctx, tag, false), kind, channel, q, offset, limit)
}

func CMSHomeRows(ctx context.Context, b []model.SiteBlock) (map[int][]model.PortalCard, error) {
	return repository.CMSHomeRows(ctx, b)
}

func CMSGetTag(ctx context.Context, id int64) (*model.CMSTag, error) {
	return repository.CMSGetTag(ctx, id)
}
