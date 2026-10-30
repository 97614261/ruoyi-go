package repository

import (
	"context"
	"encoding/json"
	"gorm.io/gorm"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

func CMSGetSite(ctx context.Context) (*model.SiteState, error) {
	var row model.CMSSite
	if err := cmsDB(ctx).Where("site_id=1").Take(&row).Error; err != nil {
		return nil, err
	}
	out := &model.SiteState{Version: row.Version, PublishedReleaseId: row.PublishedReleaseId}
	err := json.Unmarshal([]byte(row.DraftJSON), &out.Config)
	return out, err
}
func siteReferences(cfg model.SiteConfig) ([]int64, []int64) {
	assets, contents := []int64{}, []int64{}
	seenA, seenC := map[int64]bool{}, map[int64]bool{}
	addA := func(id int64) {
		if id > 0 && !seenA[id] {
			assets = append(assets, id)
			seenA[id] = true
		}
	}
	addC := func(id int64) {
		if id > 0 && !seenC[id] {
			contents = append(contents, id)
			seenC[id] = true
		}
	}
	addA(cfg.LogoAssetId)
	addA(cfg.IconAssetId)
	for _, b := range cfg.Banners {
		addA(b.AssetId)
		addC(b.ContentId)
	}
	for _, n := range cfg.Navigation {
		addC(n.ContentId)
	}
	return assets, contents
}
func siteValidateRefs(tx *gorm.DB, cfg model.SiteConfig, publish bool) error {
	assets, contents := siteReferences(cfg)
	if len(assets) > 0 {
		var count int64
		if err := tx.Model(&model.CmsAsset{}).Where("asset_id IN ? AND is_deleted=0", assets).Count(&count).Error; err != nil {
			return err
		}
		if count != int64(len(assets)) {
			return errs.New("站点素材不存在或已删除")
		}
	}
	active, err := cmsActiveChannels(tx)
	if err != nil {
		return err
	}
	if len(contents) > 0 {
		var count int64
		query := tx.Model(&model.CMSContent{}).Where("content_id IN ? AND is_deleted=0", contents)
		if publish {
			query = cmsPublished(tx, active).Where("c.content_id IN ?", contents)
		}
		if err = query.Count(&count).Error; err != nil {
			return err
		}
		if count != int64(len(contents)) {
			return errs.New("站点引用内容已删除、下架或栏目停用，请先处理引用")
		}
	}
	channelIDs := []int64{}
	for _, b := range cfg.Blocks {
		if b.ChannelId > 0 {
			channelIDs = append(channelIDs, b.ChannelId)
		}
	}
	if len(channelIDs) > 0 {
		rows := []model.CmsChannel{}
		if err = tx.Where("channel_id IN ?", channelIDs).Find(&rows).Error; err != nil {
			return err
		}
		by := map[int64]model.CmsChannel{}
		for _, r := range rows {
			by[r.ChannelId] = r
		}
		for _, b := range cfg.Blocks {
			if b.ChannelId > 0 {
				r, ok := by[b.ChannelId]
				if !ok || r.Kind != b.Kind {
					return errs.New("首页区块栏目类型不匹配")
				}
				if publish {
					valid := false
					for _, id := range active {
						if id == b.ChannelId {
							valid = true
						}
					}
					if !valid {
						return errs.New("首页区块栏目已停用")
					}
				}
			}
		}
	}
	return nil
}
func siteSaveRefs(tx *gorm.DB, cfg model.SiteConfig, owner string, id int64, operator string) error {
	assets, _ := siteReferences(cfg)
	if owner == "SITE_DRAFT" {
		if err := tx.Table("cms_asset_ref").Where("owner_type=? AND owner_id=?", owner, id).Delete(nil).Error; err != nil {
			return err
		}
	}
	if len(assets) == 0 {
		return nil
	}
	refs := []map[string]any{}
	for _, asset := range assets {
		refs = append(refs, map[string]any{"asset_id": asset, "owner_type": owner, "owner_id": id, "create_by": operator, "create_time": types.Now(), "update_by": "", "remark": "站点外观引用"})
	}
	return tx.Table("cms_asset_ref").Create(&refs).Error
}
func CMSSaveSite(ctx context.Context, in model.SiteInput, operator string) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		var row model.CMSSite
		if err := tx.Where("site_id=1").Take(&row).Error; err != nil {
			return err
		}
		if row.Version != in.Version {
			return errs.New("站点已变化，请刷新后重试")
		}
		if err := siteValidateRefs(tx, in.Config, false); err != nil {
			return err
		}
		body, err := json.Marshal(in.Config)
		if err != nil {
			return err
		}
		if err = siteSaveRefs(tx, in.Config, "SITE_DRAFT", 1, operator); err != nil {
			return err
		}
		return tx.Model(&model.CMSSite{}).Where("site_id=1").Updates(map[string]any{"draft_json": string(body), "version": row.Version + 1, "update_by": operator, "update_time": types.Now()}).Error
	})
}
func CMSPublishSite(ctx context.Context, in model.SiteAction, operator string, check func(model.SiteConfig) (string, error)) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		var site model.CMSSite
		if err := tx.Where("site_id=1").Take(&site).Error; err != nil {
			return err
		}
		if site.Version != in.Version {
			return errs.New("站点已变化，请刷新后重试")
		}
		body := site.DraftJSON
		if in.ReleaseId > 0 {
			var old model.SiteRelease
			if err := tx.Where("release_id=? AND site_id=1", in.ReleaseId).Take(&old).Error; err != nil {
				return errs.New("外观历史版本不存在")
			}
			body = old.ConfigJSON
		}
		var cfg model.SiteConfig
		if err := json.Unmarshal([]byte(body), &cfg); err != nil {
			return err
		}
		themeVersion, err := check(cfg)
		if err != nil {
			return err
		}
		if err = siteValidateRefs(tx, cfg, true); err != nil {
			return err
		}
		release := model.SiteRelease{SiteId: 1, ConfigJSON: body, ThemeVersion: themeVersion, CreateBy: operator, CreateTime: types.Now()}
		if err = tx.Create(&release).Error; err != nil {
			return err
		}
		if err = siteSaveRefs(tx, cfg, "SITE_RELEASE", release.ReleaseId, operator); err != nil {
			return err
		}
		if err = siteSaveRefs(tx, cfg, "SITE_DRAFT", 1, operator); err != nil {
			return err
		}
		return tx.Model(&model.CMSSite{}).Where("site_id=1").Updates(map[string]any{"draft_json": body, "published_release_id": release.ReleaseId, "version": site.Version + 1, "update_by": operator, "update_time": types.Now()}).Error
	})
}
func CMSSiteHistory(ctx context.Context, pg page.Query) ([]model.SiteRelease, int64, error) {
	var total int64
	q := cmsDB(ctx).Model(&model.SiteRelease{}).Where("site_id=1")
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.SiteRelease{}
	err := q.Omit("config_json").Order("release_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&rows).Error
	return rows, total, err
}
func CMSRawPublishedSite(ctx context.Context) (model.SiteConfig, error) {
	var row struct {
		ConfigJSON   string
		ThemeVersion string
	}
	err := cmsDB(ctx).Table("cms_site s").Select("r.config_json,r.theme_version").Joins("JOIN cms_site_release r ON r.site_id=s.site_id AND r.release_id=s.published_release_id").Where("s.site_id=1").Take(&row).Error
	if err != nil {
		return model.SiteConfig{}, err
	}
	var cfg model.SiteConfig
	err = json.Unmarshal([]byte(row.ConfigJSON), &cfg)
	if err != nil {
		return cfg, err
	}
	cfg.ThemeVersion = row.ThemeVersion
	return cfg, nil
}
func CMSResolveSite(ctx context.Context, cfg model.SiteConfig) (model.SiteConfig, error) {
	_, ids := siteReferences(cfg)
	links := map[int64]string{}
	if len(ids) > 0 {
		tx := cmsDB(ctx)
		active, err := cmsActiveChannels(tx)
		if err != nil {
			return cfg, err
		}
		var rows []struct {
			ContentId int64
			Kind      string
			Slug      string
		}
		if err = cmsPublished(tx, active).Select("c.content_id,c.kind,c.slug").Where("c.content_id IN ?", ids).Scan(&rows).Error; err != nil {
			return cfg, err
		}
		for _, row := range rows {
			links[row.ContentId] = portalURL(row.Kind, row.Slug)
		}
	}
	nav := []model.SiteLink{}
	for _, n := range cfg.Navigation {
		if n.ContentId > 0 {
			n.URL = links[n.ContentId]
			if n.URL == "" {
				continue
			}
		}
		nav = append(nav, n)
	}
	cfg.Navigation = nav
	banners := []model.SiteBanner{}
	for _, b := range cfg.Banners {
		if !b.Enabled {
			continue
		}
		if b.ContentId > 0 {
			b.URL = links[b.ContentId]
			if b.URL == "" {
				continue
			}
		}
		banners = append(banners, b)
	}
	cfg.Banners = banners
	return cfg, nil
}
func CMSPublicSiteAsset(ctx context.Context, id int64) (bool, error) {
	cfg, err := CMSPublicSite(ctx)
	if err != nil {
		return false, err
	}
	if (cfg.LogoAssetId == id || cfg.IconAssetId == id) && id > 0 {
		return true, nil
	}
	for _, b := range cfg.Banners {
		if b.AssetId == id {
			return true, nil
		}
	}
	return false, nil
}

func CMSPublicSite(ctx context.Context) (model.SiteConfig, error) {
	cfg, err := CMSRawPublishedSite(ctx)
	if err != nil {
		return cfg, err
	}
	return CMSResolveSite(ctx, cfg)
}
