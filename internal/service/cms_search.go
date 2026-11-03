package service

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/portalsearch"
	"ruoyi-go/pkg/types"
	"strings"
)

func CMSRunSearchJobs(ctx context.Context) error {
	jobs, e := repository.CMSFetchSearchJobs(ctx)
	if e != nil {
		return e
	}
	built := []model.CMSSearchBuilt{}
	for _, j := range jobs {
		if e = ctx.Err(); e != nil {
			return e
		}
		b := model.CMSSearchBuilt{Job: j}
		if j.IsPublished == 1 && j.IsDeleted == 0 && j.PublishedRevisionId != nil {
			if len(j.BodyHTML) > 256<<10 {
				b.Error = "limit"
			} else {
				plain := portalsearch.Plain(j.BodyHTML)
				if j.ProductCode != "" {
					plain += "\n" + j.ProductCode
				}
				if j.ProductParams != "" {
					plain += "\n" + j.ProductParams
				}
				audit := model.CMSWorkflowAudit{CreateBy: "indexer", CreateTime: types.Now(), UpdateBy: "indexer", UpdateTime: types.Now()}
				b.Doc = model.CMSSearchDocument{ContentId: j.ContentId, RevisionId: *j.PublishedRevisionId, TitleText: portalsearch.Normalize(j.Title), SummaryText: portalsearch.Normalize(j.Summary), BodyText: plain, SearchText: portalsearch.Normalize(j.Title + "\n" + j.Summary + "\n" + plain), CMSWorkflowAudit: audit}
				for _, p := range portalsearch.Build(j.Title, j.Summary, plain) {
					b.Postings = append(b.Postings, model.CMSSearchPosting{TokenHash: p.Hash, ContentId: j.ContentId, Weight: p.Weight, CMSWorkflowAudit: audit})
				}
			}
		}
		built = append(built, b)
	}
	return repository.CMSCommitSearch(ctx, built)
}
func CMSRebuildSearch(ctx context.Context) (int64, error) { return repository.CMSRebuildSearch(ctx) }
func CMSSearchStatus(ctx context.Context) (model.CMSSearchOverview, error) {
	return repository.CMSSearchStatus(ctx)
}
func CMSSearchTasks(ctx context.Context, q model.CMSSearchQuery, pg page.Query) ([]model.CMSSearchTaskView, int64, error) {
	return repository.CMSSearchTasks(ctx, q, pg)
}
func portalFullText(ctx context.Context, q string, tag int64, offset, limit int) (*model.PortalResults, error) {
	rows, n, channels, e := repository.PortalFullTextResults(ctx, q, tag, offset, limit)
	if e != nil {
		return nil, e
	}
	out := &model.PortalResults{Rows: []model.PortalCard{}, Total: n, Channels: channels}
	convert := func(parts []portalsearch.Part) []model.PortalTextPart {
		r := []model.PortalTextPart{}
		for _, p := range parts {
			r = append(r, model.PortalTextPart{Text: p.Text, Match: p.Match})
		}
		return r
	}
	for _, row := range rows {
		c := row.PortalCard
		c.TitleParts = convert(portalsearch.Highlight(c.Title, q, 0))
		text := row.BodyText
		if text == "" {
			text = c.Summary
		} else {
			for _, term := range portalsearch.Terms(q) {
				if strings.Contains(portalsearch.Normalize(c.Summary), term) {
					text = c.Summary
					break
				}
			}
		}
		c.SummaryParts = convert(portalsearch.Highlight(text, q, 180))
		out.Rows = append(out.Rows, c)
	}
	return out, nil
}
