// Generated CRUD specialized for global CMS transaction locking and publication checks.
package repository

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
	"strings"
)

type cmsRedirectRow struct {
	model.CMSRedirect
	TargetKind string
	TargetSlug string
}

func redirectView(d *gorm.DB) *gorm.DB {
	return d.Table("cms_redirect d").Select("d.*,c.kind AS target_kind,c.slug AS target_slug").Joins("JOIN cms_content c ON c.content_id=d.target_id")
}
func CMSRedirectPage(ctx context.Context, q model.CMSRedirectQuery, pg page.Query) ([]model.CMSRedirectView, int64, error) {
	d := cmsDB(ctx).Table("cms_redirect d")
	if q.SourcePath != "" {
		d = d.Where("d.source_path LIKE ?", escapeLike(q.SourcePath)+"%")
	}
	if q.Status != "" {
		d = d.Where("d.status=?", q.Status)
	}
	var n int64
	if e := d.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []cmsRedirectRow{}
	if e := redirectView(d).Order("d.redirect_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Scan(&rows).Error; e != nil {
		return nil, 0, e
	}
	out := make([]model.CMSRedirectView, 0, len(rows))
	for _, v := range rows {
		out = append(out, model.CMSRedirectView{CMSRedirect: v.CMSRedirect, TargetPath: portalURL(v.TargetKind, v.TargetSlug)})
	}
	return out, n, nil
}
func CMSRedirectDetail(ctx context.Context, id int64) (*model.CMSRedirectView, error) {
	var row cmsRedirectRow
	e := redirectView(cmsDB(ctx)).Where("d.redirect_id=?", id).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, errs.ErrNotFound
	}
	if e != nil {
		return nil, e
	}
	return &model.CMSRedirectView{CMSRedirect: row.CMSRedirect, TargetPath: portalURL(row.TargetKind, row.TargetSlug)}, nil
}

// Sources are reserved even while disabled. Both this function and content creation use cmsLocked.
func cmsContentPathAvailable(tx *gorm.DB, kind, slug string) error {
	var n int64
	if e := tx.Model(&model.CMSRedirect{}).Where("source_path=?", portalURL(kind, slug)).Count(&n).Error; e != nil {
		return e
	}
	if n != 0 {
		return errs.New("访问地址已登记为旧路径，请先修改跳转映射")
	}
	return nil
}
func cmsRedirectSourceAvailable(tx *gorm.DB, source string, id int64) error {
	var n int64
	if e := tx.Model(&model.CMSRedirect{}).Where("source_path=? AND redirect_id<>?", source, id).Count(&n).Error; e != nil {
		return e
	}
	if n != 0 {
		return errs.New("旧路径已经配置，请编辑现有映射")
	}
	parts := strings.Split(strings.TrimPrefix(source, "/"), "/")
	if len(parts) == 2 {
		kind := ""
		switch parts[0] {
		case "news":
			kind = "ARTICLE"
		case "products":
			kind = "PRODUCT"
		case "p":
			kind = "PAGE"
		}
		if kind != "" {
			if e := tx.Model(&model.CMSContent{}).Where("kind=? AND slug=?", kind, parts[1]).Count(&n).Error; e != nil {
				return e
			}
			if n > 0 {
				return errs.New("旧路径与已有内容地址冲突（含草稿、下架及历史删除内容）")
			}
		}
	}
	return nil
}
func cmsRedirectTarget(tx *gorm.DB, id int64) (string, error) {
	active, e := cmsActiveChannels(tx)
	if e != nil {
		return "", e
	}
	var c model.CMSContent
	e = cmsPublished(tx, active).Select("c.content_id,c.kind,c.slug").Where("c.content_id=?", id).Take(&c).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return "", errs.ErrNotFound
	}
	if e != nil {
		return "", e
	}
	return portalURL(c.Kind, c.Slug), nil
}
func CMSSaveRedirect(ctx context.Context, in model.CMSRedirectInput, operator string) (int64, error) {
	id := in.RedirectId
	e := cmsLocked(ctx, func(tx *gorm.DB) error {
		if id > 0 {
			var old model.CMSRedirect
			if e := tx.Where("redirect_id=?", id).Take(&old).Error; errors.Is(e, gorm.ErrRecordNotFound) {
				return errs.ErrNotFound
			} else if e != nil {
				return e
			}
			if old.Version != in.Version {
				return errs.New("映射已变化，请刷新")
			}
			if old.Status != "1" {
				return errs.New("请先停用映射，再修改旧路径或目标")
			}
		}
		if e := cmsRedirectSourceAvailable(tx, in.SourcePath, id); e != nil {
			return e
		}
		if _, e := cmsRedirectTarget(tx, in.TargetId); errors.Is(e, errs.ErrNotFound) {
			return errs.New("目标必须是当前公开且栏目可用的内容")
		} else if e != nil {
			return e
		}
		if id == 0 {
			row := model.CMSRedirect{SourcePath: in.SourcePath, TargetId: in.TargetId, Status: "1", Version: 1, CreateBy: operator, CreateTime: types.Now(), Remark: in.Remark}
			if e := tx.Create(&row).Error; e != nil {
				return e
			}
			id = row.RedirectId
			return nil
		}
		return tx.Model(&model.CMSRedirect{}).Where("redirect_id=? AND version=?", id, in.Version).Updates(map[string]any{"source_path": in.SourcePath, "target_id": in.TargetId, "remark": in.Remark, "version": in.Version + 1, "update_by": operator, "update_time": types.Now()}).Error
	})
	return id, e
}
func CMSSetRedirectStatus(ctx context.Context, id int64, in model.CMSRedirectStatus, operator string) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		var row model.CMSRedirect
		if e := tx.Where("redirect_id=?", id).Take(&row).Error; errors.Is(e, gorm.ErrRecordNotFound) {
			return errs.ErrNotFound
		} else if e != nil {
			return e
		}
		if row.Version != in.Version {
			return errs.New("映射已变化，请刷新")
		}
		if in.Status == "0" {
			if e := cmsRedirectSourceAvailable(tx, row.SourcePath, id); e != nil {
				return e
			}
			if _, e := cmsRedirectTarget(tx, row.TargetId); errors.Is(e, errs.ErrNotFound) {
				return errs.New("目标已下架、删除或栏目已停用，不能启用跳转")
			} else if e != nil {
				return e
			}
		}
		return tx.Model(&model.CMSRedirect{}).Where("redirect_id=? AND version=?", id, in.Version).Updates(map[string]any{"status": in.Status, "version": in.Version + 1, "update_by": operator, "update_time": types.Now()}).Error
	})
}
func CMSResolveRedirect(ctx context.Context, source string) (string, error) {
	var row model.CMSRedirect
	e := cmsDB(ctx).Select("target_id").Where("source_path=? AND status='0'", source).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return "", errs.ErrNotFound
	}
	if e != nil {
		return "", e
	}
	return cmsRedirectTarget(cmsDB(ctx), row.TargetId)
}
