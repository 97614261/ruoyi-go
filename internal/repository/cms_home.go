package repository

import (
	"context"
	"fmt"
	"ruoyi-go/internal/model"
	"strings"
)

// Bounded UNION: one batched query for all content blocks, not a query per card/block.
func CMSHomeRows(ctx context.Context, blocks []model.SiteBlock) (map[int][]model.PortalCard, error) {
	tx := cmsDB(ctx)
	active, e := cmsActiveChannels(tx)
	if e != nil {
		return nil, e
	}
	parts := []string{}
	args := []any{}
	out := map[int][]model.PortalCard{}
	for i, b := range blocks {
		if !b.Enabled || (b.Kind != "ARTICLE" && b.Kind != "PRODUCT" && b.Kind != "PAGE") {
			continue
		}
		cond := ""
		if b.ChannelId > 0 {
			cond += " AND r.channel_id=?"
		}
		if b.FeaturedOnly {
			cond += " AND ro.featured=1"
		}
		parts = append(parts, fmt.Sprintf("(SELECT %d section_index,c.content_id,UNIX_TIMESTAMP(c.published_at) sort_time,COALESCE(ro.pin_order,0) sort_order,c.kind,c.slug,r.title,r.summary,r.cover_asset_id FROM cms_content c JOIN cms_content_revision r ON r.revision_id=c.published_revision_id AND r.content_id=c.content_id LEFT JOIN cms_revision_ops ro ON ro.revision_id=r.revision_id WHERE c.is_deleted=0 AND c.is_published=1 AND c.kind=? AND (c.kind='PAGE' OR r.channel_id IN ?)"+cond+" ORDER BY COALESCE(ro.pin_order,0) DESC,c.published_at DESC,c.content_id DESC LIMIT ?)", i))
		args = append(args, b.Kind, active)
		if b.ChannelId > 0 {
			args = append(args, b.ChannelId)
		}
		args = append(args, b.Count)
	}
	if len(parts) == 0 {
		return out, nil
	}
	var rows []struct {
		model.PortalCard
		SectionIndex int
	}
	if e = tx.Raw(strings.Join(parts, " UNION ALL ")+" ORDER BY section_index,sort_order DESC,sort_time DESC,content_id DESC", args...).Scan(&rows).Error; e != nil {
		return nil, e
	}
	for _, v := range rows {
		v.URL = portalURL(v.Kind, v.Slug)
		out[v.SectionIndex] = append(out[v.SectionIndex], v.PortalCard)
	}
	return out, nil
}
