package repository

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"strings"
)

func portalURL(kind, slug string) string {
	if kind == "ARTICLE" {
		return "/news/" + slug
	}
	if kind == "PRODUCT" {
		return "/products/" + slug
	}
	return "/p/" + slug
}
func PortalResults(ctx context.Context, kind, channel, keyword string, offset, limit int) (*model.PortalResults, error) {
	tx := cmsDB(ctx)
	active, err := cmsActiveChannels(tx)
	if err != nil {
		return nil, err
	}
	all := []model.CmsChannel{}
	if err = tx.Select("channel_id,parent_id,kind,status,name,slug,seo_title,seo_description,order_num").Where("channel_id IN ?", active).Order("order_num,channel_id").Find(&all).Error; err != nil {
		return nil, err
	}
	query := cmsPublished(tx, active)
	if kind != "" {
		query = query.Where("c.kind=?", kind)
	}
	if channel != "" {
		var id int64
		for _, ch := range all {
			if ch.Kind == kind && ch.Slug == channel {
				id = ch.ChannelId
			}
		}
		if id == 0 {
			return nil, errs.ErrNotFound
		}
		query = query.Where("r.channel_id=?", id)
	}
	if keyword != "" {
		pattern := "%" + escapeLike(keyword) + "%"
		query = query.Where("r.title LIKE ? OR r.summary LIKE ?", pattern, pattern)
	}
	var total int64
	if err = query.Count(&total).Error; err != nil {
		return nil, err
	}
	rows := []model.PortalCard{}
	if err = query.Select("c.kind,c.slug,r.title,r.summary,c.published_at,r.cover_asset_id,r.channel_id").Order("c.published_at DESC,c.content_id DESC").Offset(offset).Limit(limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].URL = portalURL(rows[i].Kind, rows[i].Slug)
	}
	return &model.PortalResults{Rows: rows, Total: total, Channels: all}, nil
}

// Escape literal wildcard input; placeholders still own SQL quoting.
func escapeLike(s string) string { return likeEscaper.Replace(s) }

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
