package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
)

func cmsCompareSide(tx *gorm.DB, a cmsAccess, kind string, id int64, ref model.CMSVersionRef) (model.CMSCompareSide, error) {
	out := model.CMSCompareSide{}
	switch ref.Source {
	case "DRAFT":
		key, v, s, e := cmsSnapshot(tx, kind, id)
		if e != nil {
			return out, e
		}
		if ref.Version < 1 || v != ref.Version {
			return out, errs.New("当前草稿已变化，请刷新后比较")
		}
		out.Snapshot = s
		out.Key = key
		out.Label = fmt.Sprintf("当前草稿（工作版本%d）", v)
	case "REVISION":
		if kind == "SITE" || ref.Id < 1 {
			return out, errs.New("修订参数错误")
		}
		c, e := cmsFindContent(tx, kind, id)
		if e != nil {
			return out, e
		}
		var r model.CMSRevision
		if e = tx.Where("revision_id=? AND content_id=?", ref.Id, id).Take(&r).Error; e != nil {
			return out, errs.New("历史修订不属于此内容")
		}
		if e = cmsAttachOps(tx, &r); e != nil {
			return out, e
		}
		out.Snapshot = model.CMSReviewSnapshot{Content: &model.CMSContentDetail{CMSContent: c, Draft: r}, Operations: r.Ops}
		out.Key = fmt.Sprint(r.RevisionId)
		out.Label = fmt.Sprintf("历史修订%d（#%d）", r.RevisionNo, r.RevisionId)
	case "RELEASE":
		if kind != "SITE" || id != 1 || ref.Id < 1 {
			return out, errs.New("外观版本参数错误")
		}
		var r model.SiteRelease
		if e := tx.Where("site_id=1 AND release_id=?", ref.Id).Take(&r).Error; e != nil {
			return out, errs.New("外观历史版本不存在")
		}
		var cfg model.SiteConfig
		if e := json.Unmarshal([]byte(r.ConfigJSON), &cfg); e != nil {
			return out, e
		}
		out.Snapshot.Site = &cfg
		out.Key = fmt.Sprint(ref.Id)
		out.Label = fmt.Sprintf("外观发布版 #%d", ref.Id)
	case "REVIEW":
		r, e := cmsReviewByID(tx, a, ref.Id)
		if e != nil {
			return out, e
		}
		if r.Kind != kind || r.ObjectId != id {
			return out, errs.New("送审快照不属于此对象")
		}
		if e = json.Unmarshal([]byte(r.SnapshotJSON), &out.Snapshot); e != nil {
			return out, e
		}
		out.Key = r.SnapshotKey
		out.Label = fmt.Sprintf("送审快照 #%d（工作版本%d）", r.ReviewId, r.ContentVersion)
	default:
		return out, errs.New("不支持的比较来源")
	}
	return out, nil
}
func CMSCompareSnapshots(ctx context.Context, in model.CMSCompareInput) (model.CMSCompareSide, model.CMSCompareSide, error) {
	var left, right model.CMSCompareSide
	e := cmsDB(ctx).Transaction(func(tx *gorm.DB) error {
		a, e := cmsCurrentAccess(tx, cmsActor(ctx))
		if e != nil {
			return e
		}
		if !a.source(in.Kind, "query") {
			return errs.ErrForbidden
		}
		left, e = cmsCompareSide(tx, a, in.Kind, in.ObjectId, in.Left)
		if e != nil {
			return e
		}
		right, e = cmsCompareSide(tx, a, in.Kind, in.ObjectId, in.Right)
		return e
	})
	return left, right, e
}
