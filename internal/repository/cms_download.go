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

func cmsDownloadAccess(tx *gorm.DB, perm string) (cmsAccess, error) {
	a, e := cmsCurrentAccess(tx, cmsActor(tx.Statement.Context))
	if e == nil && !a.has(perm) {
		e = errs.ErrForbidden
	}
	return a, e
}
func cmsDownloadCategory(tx *gorm.DB, id int64) (model.CMSDownloadCategory, error) {
	var row model.CMSDownloadCategory
	e := tx.Where("category_id=? AND is_deleted=0", id).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = errs.New("资料分类不存在")
	}
	return row, e
}
func CMSDownloadCategories(ctx context.Context, q model.CMSDownloadCategoryQuery, pg page.Query, public bool) ([]model.CMSDownloadCategory, int64, error) {
	tx := cmsDB(ctx)
	if !public {
		if _, e := cmsDownloadAccess(tx, "cms:download:list"); e != nil {
			return nil, 0, e
		}
	}
	query := tx.Model(&model.CMSDownloadCategory{}).Where("is_deleted=0")
	if public {
		query = query.Where("enabled=1")
	}
	if q.Name != "" {
		query = query.Where("name LIKE ?", escapeLike(strings.TrimSpace(q.Name))+"%")
	}
	var n int64
	if e := query.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.CMSDownloadCategory{}
	e := query.Order("order_num,category_id").Offset(pg.Offset()).Limit(pg.PageSize).Find(&rows).Error
	return rows, n, e
}
func CMSSaveDownloadCategory(ctx context.Context, in model.CMSDownloadCategoryInput) (int64, error) {
	var id int64
	e := cmsLocked(ctx, func(tx *gorm.DB) error {
		a, e := cmsDownloadAccess(tx, "cms:download:category")
		if e != nil {
			return e
		}
		if in.CategoryId == 0 {
			row := model.CMSDownloadCategory{Name: in.Name, OrderNum: in.OrderNum, Enabled: in.Enabled, Version: 1, CMSWorkflowAudit: model.CMSWorkflowAudit{CreateBy: a.Name, CreateTime: types.Now()}}
			if e = tx.Create(&row).Error; e != nil {
				return e
			}
			id = row.CategoryId
			return nil
		}
		row, e := cmsDownloadCategory(tx, in.CategoryId)
		if e != nil {
			return e
		}
		if row.Version != in.Version {
			return errs.New("分类已变化，请刷新")
		}
		id = row.CategoryId
		return tx.Model(&model.CMSDownloadCategory{}).Where("category_id=? AND version=?", id, in.Version).Updates(map[string]any{"name": in.Name, "enabled": in.Enabled, "order_num": in.OrderNum, "version": in.Version + 1, "update_by": a.Name, "update_time": types.Now()}).Error
	})
	return id, e
}
func CMSRemoveDownloadCategory(ctx context.Context, id, version int64) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		a, e := cmsDownloadAccess(tx, "cms:download:category")
		if e != nil {
			return e
		}
		row, e := cmsDownloadCategory(tx, id)
		if e != nil {
			return e
		}
		if row.Version != version {
			return errs.New("分类已变化，请刷新")
		}
		var n int64
		if e = tx.Model(&model.CMSDownloadRevision{}).Where("category_id=?", id).Count(&n).Error; e != nil {
			return e
		}
		if n > 0 {
			return errs.New("分类仍被资料草稿或历史引用，请停用而不是删除")
		}
		return tx.Model(&model.CMSDownloadCategory{}).Where("category_id=?", id).Updates(map[string]any{"is_deleted": 1, "enabled": false, "version": version + 1, "update_by": a.Name, "update_time": types.Now()}).Error
	})
}
func CMSCreateDownloadFile(ctx context.Context, row model.CMSDownloadFile) (model.CMSDownloadFile, error) {
	e := cmsLocked(ctx, func(tx *gorm.DB) error {
		a, e := cmsDownloadAccess(tx, "cms:download:upload")
		if e != nil {
			return e
		}
		if !a.has("cms:download:query") {
			return errs.ErrForbidden
		}
		row.Version = 1
		row.CreateBy = a.Name
		row.CreateTime = types.Now()
		return tx.Create(&row).Error
	})
	return row, e
}
func cmsDownloadFile(tx *gorm.DB, id int64) (model.CMSDownloadFile, error) {
	var row model.CMSDownloadFile
	e := tx.Where("file_id=? AND is_deleted=0", id).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = errs.ErrNotFound
	}
	return row, e
}
func CMSDownloadFiles(ctx context.Context, q model.CMSDownloadFileQuery, pg page.Query) ([]model.CMSDownloadFile, int64, error) {
	tx := cmsDB(ctx)
	if _, e := cmsDownloadAccess(tx, "cms:download:query"); e != nil {
		return nil, 0, e
	}
	query := tx.Model(&model.CMSDownloadFile{}).Where("is_deleted=0")
	if q.Name != "" {
		query = query.Where("name LIKE ?", escapeLike(strings.TrimSpace(q.Name))+"%")
	}
	var n int64
	if e := query.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.CMSDownloadFile{}
	e := query.Order("file_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&rows).Error
	return rows, n, e
}
func CMSPrivateDownloadFile(ctx context.Context, id int64) (model.CMSDownloadFile, error) {
	tx := cmsDB(ctx)
	if _, e := cmsDownloadAccess(tx, "cms:download:query"); e != nil {
		return model.CMSDownloadFile{}, e
	}
	return cmsDownloadFile(tx, id)
}
func CMSDeleteDownloadFile(ctx context.Context, id, version int64) (model.CMSDownloadFile, error) {
	var row model.CMSDownloadFile
	e := cmsLocked(ctx, func(tx *gorm.DB) error {
		a, e := cmsDownloadAccess(tx, "cms:download:fileRemove")
		if e != nil {
			return e
		}
		if !a.has("cms:download:query") {
			return errs.ErrForbidden
		}
		row, e = cmsDownloadFile(tx, id)
		if e != nil {
			return e
		}
		if row.Version != version {
			return errs.New("附件已变化，请刷新")
		}
		var n int64
		if e = tx.Model(&model.CMSDownloadRevision{}).Where("file_id=?", id).Count(&n).Error; e != nil {
			return e
		}
		if n > 0 {
			return errs.New("附件被资料草稿或历史引用，不能物理删除")
		}
		return tx.Model(&model.CMSDownloadFile{}).Where("file_id=?", id).Updates(map[string]any{"is_deleted": 1, "version": version + 1, "update_by": a.Name, "update_time": types.Now()}).Error
	})
	return row, e
}
func cmsFindDownload(tx *gorm.DB, id int64) (model.CMSDownload, error) {
	var row model.CMSDownload
	e := tx.Where("download_id=? AND is_deleted=0", id).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = errs.ErrNotFound
	}
	return row, e
}
func CMSDownloadDetail(ctx context.Context, id int64) (model.CMSDownloadDetail, error) {
	out := model.CMSDownloadDetail{}
	tx := cmsDB(ctx)
	if _, e := cmsDownloadAccess(tx, "cms:download:query"); e != nil {
		return out, e
	}
	row, e := cmsFindDownload(tx, id)
	if e != nil {
		return out, e
	}
	out.CMSDownload = row
	e = tx.Where("download_id=? AND revision_id=?", id, row.WorkingRevisionId).Take(&out.Draft).Error
	return out, e
}
func cmsDownloadQuery(tx *gorm.DB, in model.CMSDownloadQuery, public bool) *gorm.DB {
	pointer := "d.working_revision_id"
	if public {
		pointer = "d.published_revision_id"
	}
	q := tx.Table("cms_download d").Joins("JOIN cms_download_revision r ON r.download_id=d.download_id AND r.revision_id=" + pointer).Joins("JOIN cms_download_category c ON c.category_id=r.category_id AND c.is_deleted=0").Joins("JOIN cms_download_file f ON f.file_id=r.file_id AND f.is_deleted=0").Where("d.is_deleted=0")
	if public {
		q = q.Where("d.is_published=1 AND c.enabled=1")
	} else if in.Published != nil {
		q = q.Where("d.is_published=?", *in.Published)
	}
	if in.Title != "" {
		q = q.Where("r.title LIKE ?", escapeLike(strings.TrimSpace(in.Title))+"%")
	}
	if in.CategoryId > 0 {
		q = q.Where("r.category_id=?", in.CategoryId)
	}
	return q
}
func CMSDownloadPage(ctx context.Context, in model.CMSDownloadQuery, pg page.Query) ([]model.CMSDownloadView, int64, error) {
	tx := cmsDB(ctx)
	if _, e := cmsDownloadAccess(tx, "cms:download:list"); e != nil {
		return nil, 0, e
	}
	q := cmsDownloadQuery(tx, in, false)
	var n int64
	if e := q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.CMSDownloadView{}
	e := q.Select("d.*,r.title,r.summary,r.category_id,c.name AS category_name,c.enabled AS category_enabled,r.file_id,f.name AS file_name,f.extension,f.byte_size").Order("d.download_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Scan(&rows).Error
	return rows, n, e
}
func CMSSaveDownload(ctx context.Context, in model.CMSDownloadInput) (int64, error) {
	var id int64
	e := cmsLocked(ctx, func(tx *gorm.DB) error {
		perm := "cms:download:add"
		if in.DownloadId > 0 {
			perm = "cms:download:edit"
		}
		a, e := cmsDownloadAccess(tx, perm)
		if e != nil {
			return e
		}
		if !a.has("cms:download:query") {
			return errs.ErrForbidden
		}
		if _, e = cmsDownloadCategory(tx, in.CategoryId); e != nil {
			return e
		}
		if _, e = cmsDownloadFile(tx, in.FileId); e != nil {
			return e
		}
		row := model.CMSDownload{Version: 1, CMSWorkflowAudit: model.CMSWorkflowAudit{CreateBy: a.Name, CreateTime: types.Now()}}
		if in.DownloadId > 0 {
			row, e = cmsFindDownload(tx, in.DownloadId)
			if e != nil {
				return e
			}
			if row.Version != in.Version {
				return errs.New("资料已变化，请刷新")
			}
		} else if e = tx.Create(&row).Error; e != nil {
			return e
		}
		rev := model.CMSDownloadRevision{DownloadId: row.DownloadId, RevisionNo: row.RevisionSeq + 1, CategoryId: in.CategoryId, FileId: in.FileId, Title: in.Title, Summary: in.Summary, CMSWorkflowAudit: model.CMSWorkflowAudit{CreateBy: a.Name, CreateTime: types.Now()}}
		if e = tx.Create(&rev).Error; e != nil {
			return e
		}
		id = row.DownloadId
		return tx.Model(&model.CMSDownload{}).Where("download_id=? AND version=?", id, row.Version).Updates(map[string]any{"working_revision_id": rev.RevisionId, "revision_seq": rev.RevisionNo, "version": row.Version + 1, "update_by": a.Name, "update_time": types.Now()}).Error
	})
	return id, e
}
func CMSDownloadHistory(ctx context.Context, id int64, pg page.Query) ([]model.CMSDownloadRevision, int64, error) {
	tx := cmsDB(ctx)
	if _, e := cmsDownloadAccess(tx, "cms:download:query"); e != nil {
		return nil, 0, e
	}
	if _, e := cmsFindDownload(tx, id); e != nil {
		return nil, 0, e
	}
	q := tx.Model(&model.CMSDownloadRevision{}).Where("download_id=?", id)
	var n int64
	if e := q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.CMSDownloadRevision{}
	e := q.Order("revision_no DESC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&rows).Error
	return rows, n, e
}

// DOWNLOAD has a separate publication permission. It never borrows PAGE approval.
func CMSChangeDownload(ctx context.Context, id, version int64, action string) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		perm := "cms:download:publish"
		if action == "remove" {
			perm = "cms:download:remove"
		}
		a, e := cmsDownloadAccess(tx, perm)
		if e != nil {
			return e
		}
		if !a.has("cms:download:query") {
			return errs.ErrForbidden
		}
		row, e := cmsFindDownload(tx, id)
		if e != nil {
			return e
		}
		if row.Version != version {
			return errs.New("资料已变化，请刷新")
		}
		changes := map[string]any{"version": version + 1, "update_by": a.Name, "update_time": types.Now()}
		switch action {
		case "publish":
			var rev model.CMSDownloadRevision
			if e = tx.Where("download_id=? AND revision_id=?", id, row.WorkingRevisionId).Take(&rev).Error; e != nil {
				return e
			}
			cat, e := cmsDownloadCategory(tx, rev.CategoryId)
			if e != nil {
				return e
			}
			if !cat.Enabled {
				return errs.New("请先启用资料分类")
			}
			if _, e = cmsDownloadFile(tx, rev.FileId); e != nil {
				return e
			}
			changes["published_revision_id"] = rev.RevisionId
			changes["published_at"] = types.Now()
			changes["is_published"] = 1
		case "unpublish":
			changes["is_published"] = 0
		case "remove":
			changes["is_published"] = 0
			changes["is_deleted"] = 1
		default:
			return errs.New("不支持的资料操作")
		}
		return tx.Model(&model.CMSDownload{}).Where("download_id=? AND version=?", id, version).Updates(changes).Error
	})
}
func PortalDownloadPage(ctx context.Context, in model.CMSDownloadQuery, offset, limit int) ([]model.PortalDownload, int64, error) {
	q := cmsDownloadQuery(cmsDB(ctx), in, true)
	var n int64
	if e := q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []model.PortalDownload{}
	e := q.Select("d.download_id,r.title,r.summary,r.category_id,c.name AS category_name,f.name AS file_name,f.extension,f.byte_size,f.sha256,d.published_at").Order("d.published_at DESC,d.download_id DESC").Offset(offset).Limit(limit).Scan(&rows).Error
	return rows, n, e
}
func PortalDownloadFile(ctx context.Context, id int64) (model.CMSDownloadFile, error) {
	var row model.CMSDownloadFile
	e := cmsDownloadQuery(cmsDB(ctx), model.CMSDownloadQuery{}, true).Where("d.download_id=?", id).Select("f.*").Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = errs.ErrNotFound
	}
	return row, e
}
