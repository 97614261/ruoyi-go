package repository

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

// Do not expand draft HTML into slow/error SQL logs. Keep the normal logger and timings.
type cmsLogger struct{ logger.Interface }

func (l cmsLogger) LogMode(level logger.LogLevel) logger.Interface {
	return cmsLogger{l.Interface.LogMode(level)}
}
func (l cmsLogger) ParamsFilter(_ context.Context, sql string, _ ...interface{}) (string, []interface{}) {
	return sql, nil
}
func cmsDB(ctx context.Context) *gorm.DB {
	d := DB(ctx)
	return d.Session(&gorm.Session{Logger: cmsLogger{d.Logger}})
}

func cmsFindContent(tx *gorm.DB, kind string, id int64) (model.CMSContent, error) {
	var c model.CMSContent
	err := tx.Where("content_id=? AND kind=? AND is_deleted=0", id, kind).Take(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return c, errs.New("内容不存在或已删除")
	}
	return c, err
}
func CMSContentPage(ctx context.Context, kind string, q model.CMSContentQuery, pg page.Query) ([]model.CMSContentView, int64, error) {
	db := cmsDB(ctx).Table("cms_content c").Joins("JOIN cms_content_revision r ON r.revision_id=c.working_revision_id AND r.content_id=c.content_id").Where("c.kind=? AND c.is_deleted=0", kind)
	if q.Title != "" {
		db = db.Where("r.title LIKE ?", q.Title+"%")
	}
	if q.Published != nil {
		db = db.Where("c.is_published=?", *q.Published)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.CMSContentView{}
	err := db.Select("c.*,r.title,r.summary,r.channel_id").Order("c.content_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Scan(&rows).Error
	return rows, total, err
}
func CMSContentDetail(ctx context.Context, kind string, id int64) (*model.CMSContentDetail, error) {
	c, err := cmsFindContent(cmsDB(ctx), kind, id)
	if err != nil {
		return nil, err
	}
	var r model.CMSRevision
	if c.WorkingRevisionId == nil {
		return nil, errs.New("草稿版本缺失")
	}
	if err = cmsDB(ctx).Where("content_id=? AND revision_id=?", id, *c.WorkingRevisionId).Take(&r).Error; err != nil {
		return nil, err
	}
	return &model.CMSContentDetail{CMSContent: c, Draft: r}, nil
}
func cmsActiveChannels(tx *gorm.DB) ([]int64, error) {
	rows := []model.CmsChannel{}
	if err := tx.Select("channel_id,parent_id,kind,status").Limit(2001).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) > 2000 {
		return nil, errs.New("栏目数量超过限制")
	}
	byID := map[int64]model.CmsChannel{}
	for _, r := range rows {
		byID[r.ChannelId] = r
	}
	ids := []int64{}
	for _, r := range rows {
		good := false
		current := r
		for depth := 0; depth < 8; depth++ {
			if current.Kind != r.Kind || current.Status != "0" {
				break
			}
			if current.ParentId == 0 {
				good = true
				break
			}
			p, ok := byID[current.ParentId]
			if !ok {
				break
			}
			current = p
		}
		if good {
			ids = append(ids, r.ChannelId)
		}
	}
	return ids, nil
}
func cmsRevisionValid(tx *gorm.DB, kind string, r model.CMSRevision, ids []int64, publishing bool) error {
	if kind == "ARTICLE" || kind == "PRODUCT" {
		if r.ChannelId == nil {
			return errs.New("文章必须选择栏目")
		}
		var count int64
		if err := tx.Model(&model.CmsChannel{}).Where("channel_id=? AND kind=?", *r.ChannelId, kind).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return errs.New("文章栏目不存在或类型不符")
		}
		if publishing {
			active, err := cmsActiveChannels(tx)
			if err != nil {
				return err
			}
			ok := false
			for _, id := range active {
				if id == *r.ChannelId {
					ok = true
				}
			}
			if !ok {
				return errs.New("栏目及上级必须已启用才能发布")
			}
		}
	} else if r.ChannelId != nil {
		return errs.New("单页不能指定栏目")
	}
	if len(ids) > 32 {
		return errs.New("每个版本最多引用32张图片")
	}
	if len(ids) > 0 {
		var count int64
		if err := tx.Model(&model.CmsAsset{}).Where("asset_id IN ? AND is_deleted=0", ids).Count(&count).Error; err != nil {
			return err
		}
		if count != int64(len(ids)) {
			return errs.New("引用的素材不存在或已删除")
		}
	}
	return nil
}
func cmsAppendRevision(tx *gorm.DB, c *model.CMSContent, r model.CMSRevision, ids []int64, operator string) error {
	r.RevisionId = 0
	r.ContentId = c.ContentId
	r.RevisionNo = c.RevisionSeq + 1
	r.CreateBy = operator
	r.CreateTime = types.Now()
	r.UpdateBy = ""
	r.UpdateTime = types.Time{}
	if err := tx.Create(&r).Error; err != nil {
		return err
	}
	if len(ids) > 0 {
		refs := []map[string]any{}
		for _, id := range ids {
			refs = append(refs, map[string]any{"asset_id": id, "owner_type": "CONTENT_REVISION", "owner_id": r.RevisionId, "create_by": operator, "create_time": types.Now(), "update_by": "", "remark": "内容历史修订"})
		}
		if err := tx.Table("cms_asset_ref").Create(&refs).Error; err != nil {
			return err
		}
	}
	return tx.Model(&model.CMSContent{}).Where("content_id=? AND version=?", c.ContentId, c.Version).Updates(map[string]any{"working_revision_id": r.RevisionId, "revision_seq": r.RevisionNo, "version": c.Version + 1, "update_by": operator, "update_time": types.Now()}).Error
}
func CMSSaveDraft(ctx context.Context, kind string, in model.CMSDraftInput, r model.CMSRevision, ids []int64, operator string) (int64, error) {
	var id int64
	err := cmsLocked(ctx, func(tx *gorm.DB) error {
		if err := cmsRevisionValid(tx, kind, r, ids, false); err != nil {
			return err
		}
		c := model.CMSContent{Kind: kind, Slug: in.Slug, Version: 1, CreateBy: operator, CreateTime: types.Now()}
		if in.ContentId == 0 {
			if err := tx.Create(&c).Error; err != nil {
				return cmsContentDuplicate(err)
			}
		} else {
			var err error
			c, err = cmsFindContent(tx, kind, in.ContentId)
			if err != nil {
				return err
			}
			if c.Version != in.Version {
				return errs.New("内容已变化，请刷新后重试")
			}
			if c.EverPublished != 0 && c.Slug != in.Slug {
				return errs.New("首次发布后不能修改访问地址")
			}
			if c.Slug != in.Slug {
				if err = tx.Model(&model.CMSContent{}).Where("content_id=?", c.ContentId).Update("slug", in.Slug).Error; err != nil {
					return cmsContentDuplicate(err)
				}
			}
		}
		id = c.ContentId
		return cmsAppendRevision(tx, &c, r, ids, operator)
	})
	return id, err
}
func cmsContentDuplicate(err error) error {
	mapped := cmsDuplicate(err)
	if mapped != err {
		return errs.New("同类型内容地址已被使用（含历史删除内容）")
	}
	return err
}
func CMSHistory(ctx context.Context, kind string, id int64, pg page.Query) ([]model.CMSRevision, int64, error) {
	if _, err := cmsFindContent(cmsDB(ctx), kind, id); err != nil {
		return nil, 0, err
	}
	query := cmsDB(ctx).Model(&model.CMSRevision{}).Where("content_id=?", id)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.CMSRevision{}
	err := query.Omit("body_html").Order("revision_no DESC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&rows).Error
	return rows, total, err
}
func CMSChangeContent(ctx context.Context, kind string, id int64, in model.CMSContentAction, action, operator string, canReadAssets bool, normalize func(model.CMSRevision) (string, []int64, error)) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		return cmsChangeContentTx(tx, kind, id, in, action, operator, canReadAssets, normalize)
	})
}
func cmsChangeContentTx(tx *gorm.DB, kind string, id int64, in model.CMSContentAction, action, operator string, canReadAssets bool, normalize func(model.CMSRevision) (string, []int64, error)) error {
	c, err := cmsFindContent(tx, kind, id)
	if err != nil {
		return err
	}
	if c.Version != in.Version {
		return errs.New("内容已变化，请刷新后重试")
	}
	changes := map[string]any{"version": c.Version + 1, "update_by": operator, "update_time": types.Now()}
	switch action {
	case "publish", "restore":
		revision := c.WorkingRevisionId
		if action == "restore" {
			if in.RevisionId < 1 {
				return errs.New("请选择历史版本")
			}
			revision = &in.RevisionId
		}
		if revision == nil {
			return errs.New("草稿版本缺失")
		}
		var r model.CMSRevision
		if err = tx.Where("content_id=? AND revision_id=?", id, *revision).Take(&r).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return errs.New("历史版本不属于此内容")
		} else if err != nil {
			return err
		}
		body, ids, err := normalize(r)
		if err != nil {
			return err
		}
		r.BodyHTML = body
		if action == "restore" && len(ids) > 0 && !canReadAssets {
			return errs.WithCode(403, "恢复图片需要素材查询权限")
		}
		if err = cmsRevisionValid(tx, kind, r, ids, action == "publish"); err != nil {
			return err
		}
		if action == "restore" {
			return cmsAppendRevision(tx, &c, r, ids, operator)
		}
		changes["published_revision_id"] = *revision
		changes["is_published"] = 1
		changes["ever_published"] = 1
		changes["published_at"] = types.Now()
	case "unpublish":
		changes["is_published"] = 0
	case "remove":
		changes["is_published"] = 0
		changes["is_deleted"] = 1
	default:
		return errs.New("不支持的内容操作")
	}
	return tx.Model(&model.CMSContent{}).Where("content_id=? AND version=?", id, c.Version).Updates(changes).Error
}

func cmsPublished(tx *gorm.DB, active []int64) *gorm.DB {
	return tx.Table("cms_content c").Joins("JOIN cms_content_revision r ON r.content_id=c.content_id AND r.revision_id=c.published_revision_id").Where("c.is_published=1 AND c.is_deleted=0").Where("c.kind='PAGE' OR (c.kind IN ('ARTICLE','PRODUCT') AND r.channel_id IN ?)", active)
}
func CMSPublishedContent(ctx context.Context, kind, slug string) (*model.CMSRevision, error) {
	tx := cmsDB(ctx)
	active, err := cmsActiveChannels(tx)
	if err != nil {
		return nil, err
	}
	var row model.CMSRevision
	err = cmsPublished(tx, active).Select("r.*").Where("c.kind=? AND c.slug=?", kind, slug).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	return &row, err
}
func CMSPublicAssetAllowed(ctx context.Context, id int64) (bool, error) {
	tx := cmsDB(ctx)
	active, err := cmsActiveChannels(tx)
	if err != nil {
		return false, err
	}
	var count int64
	err = cmsPublished(tx, active).Joins("JOIN cms_asset_ref a ON a.owner_type='CONTENT_REVISION' AND a.owner_id=r.revision_id").Where("a.asset_id=?", id).Limit(1).Count(&count).Error
	if err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	return CMSPublicSiteAsset(ctx, id)
}

func CMSPublicPage(ctx context.Context, kind string, pg page.Query) ([]model.CMSPublicSummary, int64, error) {
	tx := cmsDB(ctx)
	active, err := cmsActiveChannels(tx)
	if err != nil {
		return nil, 0, err
	}
	query := cmsPublished(tx, active).Where("c.kind=?", kind)
	var total int64
	if err = query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []model.CMSPublicSummary{}
	err = query.Select("c.slug,r.title,r.summary,c.published_at,r.cover_asset_id").Order("c.published_at DESC,c.content_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Scan(&rows).Error
	return rows, total, err
}
