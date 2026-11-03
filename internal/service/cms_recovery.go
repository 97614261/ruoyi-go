package service

import (
	"context"
	"encoding/json"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
)

const CMSRecoveryLimit = 384 << 10

func recoveryIdentity(kind string, id, user int64) error {
	if user < 1 || id < 0 || (kind != "ARTICLE" && kind != "PRODUCT" && kind != "PAGE") {
		return errs.New("恢复副本参数错误")
	}
	return nil
}
func CMSGetRecovery(ctx context.Context, kind string, id, user int64) (model.CMSRecoveryView, error) {
	if e := recoveryIdentity(kind, id, user); e != nil {
		return model.CMSRecoveryView{}, e
	}
	return repository.CMSGetRecovery(ctx, kind, id, user)
}
func cmsRecoveryNormalize(kind string, d model.CMSRecoveryDraft, canAssets bool) (model.CMSRecoveryDraft, []int64, error) {
	body, ids, e := cmsBody(d.BodyHTML, true)
	if e != nil {
		return d, nil, e
	}
	d.BodyHTML = body
	if kind != "PRODUCT" && (d.ProductCode != "" || d.ProductParams != "" || len(d.GalleryIds) > 0) {
		return d, nil, errs.New("产品字段只能用于产品")
	}
	if kind == "PAGE" && d.ChannelId != 0 {
		return d, nil, errs.New("单页不能指定栏目")
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	for _, id := range append(append([]int64{}, d.GalleryIds...), d.CoverAssetId) {
		if id > 0 && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) > 32 {
		return d, nil, errs.New("恢复副本最多引用32张图片")
	}
	if len(ids) > 0 && !canAssets {
		return d, nil, errs.WithCode(403, "引用图片需要素材查询权限")
	}
	return d, ids, nil
}
func CMSPutRecovery(ctx context.Context, kind string, id, user int64, in model.CMSRecoveryInput, operator string, canAssets bool) (model.CMSRecoveryView, error) {
	if e := recoveryIdentity(kind, id, user); e != nil {
		return model.CMSRecoveryView{}, e
	}
	d, ids, e := cmsRecoveryNormalize(kind, in.Draft, canAssets)
	if e != nil {
		return model.CMSRecoveryView{}, e
	}
	in.Draft = d
	b, e := json.Marshal(d)
	if e != nil {
		return model.CMSRecoveryView{}, e
	}
	if len(b) > CMSRecoveryLimit {
		return model.CMSRecoveryView{}, errs.New("恢复副本过大")
	}
	return repository.CMSPutRecovery(ctx, kind, id, user, in, ids, string(b), operator)
}
func CMSClearRecovery(ctx context.Context, kind string, id, user int64, in model.CMSRecoveryToken, operator string) (model.CMSRecoveryView, error) {
	if e := recoveryIdentity(kind, id, user); e != nil {
		return model.CMSRecoveryView{}, e
	}
	return repository.CMSClearRecovery(ctx, kind, id, user, in, operator)
}
func CMSCommitRecovery(ctx context.Context, kind string, id, user int64, in model.CMSRecoveryCommit, operator string, canAssets bool) (model.CMSRecoveryCommitted, error) {
	if e := recoveryIdentity(kind, id, user); e != nil {
		return model.CMSRecoveryCommitted{}, e
	}
	in.Draft.ContentId = id
	in.Draft.Version = in.BaseVersion
	r, ids, e := cmsPrepareDraft(kind, in.Draft, canAssets)
	if e != nil {
		return model.CMSRecoveryCommitted{}, e
	}
	return repository.CMSCommitRecovery(ctx, kind, id, user, in, r, ids, operator)
}
