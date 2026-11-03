package repository

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
	"strconv"
	"strings"
)

func cmsPresetActor(tx *gorm.DB, apply bool, makePages bool) (cmsAccess, error) {
	a, e := cmsCurrentAccess(tx, cmsActor(tx.Statement.Context))
	if e != nil {
		return a, e
	}
	if !a.has("cms:preset:list") || !a.has("cms:site:query") {
		return a, errs.ErrForbidden
	}
	if makePages && !a.has("cms:page:query") {
		return a, errs.ErrForbidden
	}
	if apply && (!a.has("cms:preset:apply") || !a.has("cms:site:edit") || (makePages && (!a.has("cms:page:add") || !a.has("cms:page:query")))) {
		return a, errs.ErrForbidden
	}
	return a, nil
}
func CMSPresetAccess(ctx context.Context, apply bool) error {
	_, e := cmsPresetActor(cmsDB(ctx), apply, false)
	return e
}
func cmsPresetState(tx *gorm.DB) (model.SiteState, string, error) {
	var row model.CMSSite
	var state model.SiteState
	if e := tx.Where("site_id=1").Take(&row).Error; e != nil {
		return state, "", e
	}
	state.Version = row.Version
	state.PublishedReleaseId = row.PublishedReleaseId
	e := json.Unmarshal([]byte(row.DraftJSON), &state.Config)
	return state, row.DraftJSON, e
}
func cmsPresetConflicts(tx *gorm.DB, pages []model.CMSPresetPage) ([]string, error) {
	out := []string{}
	if len(pages) == 0 {
		return out, nil
	}
	slugs, paths := []string{}, []string{}
	for _, p := range pages {
		slugs = append(slugs, p.Slug)
		paths = append(paths, "/p/"+p.Slug)
	}
	var existing []string
	if e := tx.Model(&model.CMSContent{}).Where("kind='PAGE' AND slug IN ?", slugs).Pluck("slug", &existing).Error; e != nil {
		return nil, e
	}
	for _, s := range existing {
		out = append(out, "页面地址已使用（含已删除历史）："+s)
	}
	var redirects []string
	if e := tx.Model(&model.CMSRedirect{}).Where("source_path IN ?", paths).Pluck("source_path", &redirects).Error; e != nil {
		return nil, e
	}
	for _, s := range redirects {
		out = append(out, "地址已有旧路径映射："+s)
	}
	return out, nil
}
func CMSPreviewPreset(ctx context.Context, in model.CMSPresetInput, build func(model.SiteState) (model.CMSPresetPlan, error)) (model.CMSPresetPlan, error) {
	var out model.CMSPresetPlan
	e := cmsDB(ctx).Transaction(func(tx *gorm.DB) error {
		if _, e := cmsPresetActor(tx, false, len(in.PageKeys) > 0); e != nil {
			return e
		}
		state, _, e := cmsPresetState(tx)
		if e != nil {
			return e
		}
		out, e = build(state)
		if e != nil {
			return e
		}
		out.Conflicts, e = cmsPresetConflicts(tx, out.Pages)
		return e
	})
	return out, e
}

// Fixed maximum three new pages. Every lookup above is batched; these loops only
// build rows and one CASE update. Existing pages are never edited or reused.
func cmsCreatePresetPages(tx *gorm.DB, pages []model.CMSPresetPage, a cmsAccess) (map[string]int64, []int64, error) {
	byPath := map[string]int64{}
	ids := []int64{}
	if len(pages) == 0 {
		return byPath, ids, nil
	}
	contents := []model.CMSContent{}
	now := types.Now()
	for _, p := range pages {
		contents = append(contents, model.CMSContent{Kind: "PAGE", Slug: p.Slug, Version: 1})
	}
	// CMSContent uses explicit audit fields, unlike the newer extension models.
	for i := range contents {
		contents[i].CreateBy = a.Name
		contents[i].CreateTime = now
	}
	if e := tx.Create(&contents).Error; e != nil {
		return nil, nil, e
	}
	revisions := []model.CMSRevision{}
	for i, p := range pages {
		revisions = append(revisions, model.CMSRevision{ContentId: contents[i].ContentId, RevisionNo: 1, Title: p.Title, Summary: p.Summary, BodyHTML: p.BodyHTML, GalleryIds: []int64{}, CreateBy: a.Name, CreateTime: now, Remark: "行业预设新建草稿"})
	}
	if e := tx.Create(&revisions).Error; e != nil {
		return nil, nil, e
	}
	actors := []model.CMSDraftActor{}
	ops := []model.CMSRevisionOps{}
	var cases strings.Builder
	args := []any{}
	for i, rev := range revisions {
		c := contents[i]
		ids = append(ids, c.ContentId)
		byPath["/p/"+c.Slug] = c.ContentId
		actors = append(actors, model.CMSDraftActor{Kind: "PAGE", SnapshotKey: strconv.FormatInt(rev.RevisionId, 10), ObjectId: c.ContentId, AuthorUserId: a.ID, CMSWorkflowAudit: model.CMSWorkflowAudit{CreateBy: a.Name, CreateTime: now}})
		ops = append(ops, model.CMSRevisionOps{RevisionId: rev.RevisionId, Payload: "{}", CreateBy: a.Name, CreateTime: now})
		cases.WriteString(" WHEN ? THEN ?")
		args = append(args, c.ContentId, rev.RevisionId)
	}
	if e := tx.Create(&actors).Error; e != nil {
		return nil, nil, e
	}
	if e := tx.Create(&ops).Error; e != nil {
		return nil, nil, e
	}
	args = append(args, a.Name, ids)
	if e := tx.Exec("UPDATE cms_content SET working_revision_id=CASE content_id"+cases.String()+" END,revision_seq=1,version=2,update_by=?,update_time=NOW() WHERE content_id IN ?", args...).Error; e != nil {
		return nil, nil, e
	}
	return byPath, ids, nil
}
func CMSApplyPreset(ctx context.Context, in model.CMSPresetApply, requestHash string, build func(model.SiteState) (model.CMSPresetPlan, error)) (model.CMSPresetApplication, error) {
	var out model.CMSPresetApplication
	e := cmsLocked(ctx, func(tx *gorm.DB) error {
		a, e := cmsPresetActor(tx, true, len(in.PageKeys) > 0)
		if e != nil {
			return e
		}
		e = tx.Where("actor_user_id=? AND request_id=?", a.ID, in.RequestId).Take(&out).Error
		if e == nil {
			if out.RequestSHA256 != requestHash {
				return errs.New("该请求ID已用于其他参数，请重新预览")
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		state, before, e := cmsPresetState(tx)
		if e != nil {
			return e
		}
		plan, e := build(state)
		if e != nil {
			return e
		}
		if plan.PlanSHA256 != in.PlanSHA256 {
			return errs.New("预设或计划已变化，请重新预览")
		}
		conflicts, e := cmsPresetConflicts(tx, plan.Pages)
		if e != nil {
			return e
		}
		if len(conflicts) > 0 {
			return errs.New("页面地址有冲突，请更换前缀后重新预览")
		}
		assets, _ := siteReferences(plan.Config)
		if len(assets) > 0 && !a.has("cms:asset:query") {
			return errs.ErrForbidden
		}
		byPath, ids, e := cmsCreatePresetPages(tx, plan.Pages, a)
		if e != nil {
			return e
		}
		for i, n := range plan.Config.Navigation {
			if id, ok := byPath[n.URL]; ok {
				plan.Config.Navigation[i].ContentId = id
				plan.Config.Navigation[i].URL = ""
			}
		}
		if e = siteValidateRefs(tx, plan.Config, false); e != nil {
			return e
		}
		if e = siteSaveRefs(tx, plan.Config, "SITE_DRAFT", 1, a.Name); e != nil {
			return e
		}
		if e = cmsRecordDraftActor(tx, "SITE", 1, strconv.FormatInt(state.Version+1, 10), a.Name); e != nil {
			return e
		}
		body, e := json.Marshal(plan.Config)
		if e != nil {
			return e
		}
		if e = tx.Model(&model.CMSSite{}).Where("site_id=1 AND version=?", in.SiteVersion).Updates(map[string]any{"draft_json": string(body), "version": in.SiteVersion + 1, "update_by": a.Name, "update_time": types.Now()}).Error; e != nil {
			return e
		}
		out = model.CMSPresetApplication{ActorUserId: a.ID, RequestId: in.RequestId, RequestSHA256: requestHash, PresetId: in.PresetId, PresetVersion: plan.PresetVersion, SiteVersionBefore: in.SiteVersion, SiteVersionAfter: in.SiteVersion + 1, PageIds: ids, BeforeConfigJSON: before, CMSWorkflowAudit: model.CMSWorkflowAudit{CreateBy: a.Name, CreateTime: types.Now(), Remark: in.Reason}}
		return tx.Create(&out).Error
	})
	return out, e
}
func CMSPresetHistory(ctx context.Context, pg page.Query) ([]model.CMSPresetApplication, int64, error) {
	tx := cmsDB(ctx)
	if _, e := cmsPresetActor(tx, false, false); e != nil {
		return nil, 0, e
	}
	q := tx.Model(&model.CMSPresetApplication{})
	var n int64
	if e := q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	out := []model.CMSPresetApplication{}
	e := q.Omit("before_config_json", "request_sha256").Order("application_id DESC").Offset(pg.Offset()).Limit(pg.PageSize).Find(&out).Error
	return out, n, e
}
