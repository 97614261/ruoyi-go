package service

import (
	"context"
	"encoding/json"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/validate"
)

const CMSTransferLimit = 1 << 20
const cmsTransferFormat = "portal-content-v1"

func CMSTransferTemplate(kind string) model.CMSTransfer {
	return model.CMSTransfer{Format: cmsTransferFormat, Kind: kind, Rows: []model.CMSDraftInput{{Title: "请填写标题", Slug: "replace-this-address", BodyHTML: "<p>请填写正文</p>"}}}
}
func CMSCheckTransferEnvelope(kind string, in model.CMSTransfer) error {
	if in.Format != cmsTransferFormat || in.Kind != kind || len(in.Rows) < 1 || len(in.Rows) > 20 || len(in.Source) > 20 {
		return errs.New("格式须为portal-content-v1，类型须对应当前模块，每批1—20行")
	}
	b, e := json.Marshal(in)
	if e != nil || len(b) > CMSTransferLimit {
		return errs.New("导入JSON最多1MiB")
	}
	return nil
}
func cmsTransferMessage(e error) string {
	if b := errs.As(e); b != nil {
		return b.Msg
	}
	return "处理失败，请刷新后重试"
}
func CMSImportDrafts(ctx context.Context, kind string, in model.CMSTransfer, operator string, canAssets, apply bool) (*model.CMSTransferReport, error) {
	if e := CMSCheckTransferEnvelope(kind, in); e != nil {
		return nil, e
	}
	slugs := []string{}
	counts := map[string]int{}
	allIDs := []int64{}
	prepared := make([]model.CMSRevision, len(in.Rows))
	failures := make([]error, len(in.Rows))
	for i, row := range in.Rows {
		slugs = append(slugs, row.Slug)
		counts[row.Slug]++
		if row.ContentId != 0 || row.Version != 0 {
			failures[i] = errs.New("导入只新增草稿，contentId和version必须为0")
			continue
		}
		if e := validate.Struct(row); e != nil {
			failures[i] = errs.New("字段校验失败，请检查必填、长度、类型和文本格式")
			continue
		}
		revision, ids, e := cmsPrepareDraft(kind, row, canAssets)
		prepared[i] = revision
		failures[i] = e
		allIDs = append(allIDs, ids...)
	}
	used, e := repository.CMSUsedSlugs(ctx, kind, slugs)
	if e != nil {
		return nil, e
	}
	channels, e := repository.CMSInspectionChannels(ctx)
	if e != nil {
		return nil, e
	}
	byChannel := cmsChannels(channels)
	assetRows, e := repository.CMSAssetsByIDs(ctx, allIDs)
	if e != nil {
		return nil, e
	}
	assets := map[int64]model.CmsAsset{}
	for _, a := range assetRows {
		assets[a.AssetId] = a
	}
	report := &model.CMSTransferReport{CheckOnly: !apply, Rows: []model.CMSTransferRow{}}
	for i, row := range in.Rows {
		result := model.CMSTransferRow{Row: i + 1, Slug: row.Slug}
		err := failures[i]
		if err == nil && (used[row.Slug] || counts[row.Slug] > 1) {
			err = errs.New("地址已被使用（含历史删除内容）或在本文件重复")
		}
		revision := prepared[i]
		if err == nil && revision.ChannelId != nil {
			ch, ok := byChannel[*revision.ChannelId]
			if !ok || ch.Kind != kind {
				err = errs.New("栏目不存在或类型不符")
			}
		}
		if err == nil {
			ids, _ := cmsRevisionAssets(revision)
			for _, id := range ids {
				if issues := cmsCheckAsset(id, assets); len(issues) > 0 {
					err = errs.New(issues[0].Message)
					break
				}
			}
		}
		result.Ready = err == nil
		// Applying rows is deliberately a bounded sequence of independent write transactions, like batch publish.
		if apply && err == nil {
			if ctx.Err() != nil {
				err = errs.New("请求已中断，此行未执行")
			} else {
				result.ContentId, err = CMSSaveDraft(ctx, kind, row, operator, canAssets)
				result.Success = err == nil
			}
		}
		if err != nil {
			result.Message = cmsTransferMessage(err)
			report.FailureCount++
		} else if apply {
			result.Message = "已新增草稿，尚未发布"
			report.SuccessCount++
		} else {
			result.Message = "预检查通过，尚未写入"
		}
		report.Rows = append(report.Rows, result)
	}
	return report, nil
}
func CMSExportDrafts(ctx context.Context, kind string, items []model.CMSBatchItem, canAssets bool) (*model.CMSTransfer, error) {
	if len(items) < 1 || len(items) > 20 {
		return nil, errs.New("每次导出1—20条")
	}
	ids := []int64{}
	seen := map[int64]bool{}
	for _, item := range items {
		if item.ContentId < 1 || item.Version < 1 || seen[item.ContentId] {
			return nil, errs.New("ID或版本错误、ID重复")
		}
		seen[item.ContentId] = true
		ids = append(ids, item.ContentId)
	}
	rows, e := repository.CMSTransferDrafts(ctx, kind, ids)
	if e != nil {
		return nil, e
	}
	byID := map[int64]model.CMSInspectionRow{}
	for _, row := range rows {
		byID[row.ContentId] = row
	}
	out := &model.CMSTransfer{Format: cmsTransferFormat, Kind: kind, Rows: []model.CMSDraftInput{}, Source: []model.CMSTransferSource{}}
	for _, item := range items {
		row, ok := byID[item.ContentId]
		if !ok || row.Version != item.Version {
			return nil, errs.New("内容已变化或不存在，请刷新后导出")
		}
		r := row.CMSRevision
		normalized, assets, e := cmsPrepareDraft(kind, model.CMSDraftInput{Title: r.Title, Slug: row.Slug, BodyHTML: r.BodyHTML, ChannelId: valueOrZero(r.ChannelId), CoverAssetId: valueOrZero(r.CoverAssetId), GalleryIds: r.GalleryIds, ProductCode: r.ProductCode, ProductParams: r.ProductParams}, canAssets)
		if e != nil {
			return nil, e
		}
		if len(assets) > 0 && !canAssets {
			return nil, errs.WithCode(403, "导出含图内容需要素材查询权限")
		}
		r.BodyHTML = normalized.BodyHTML
		out.Rows = append(out.Rows, model.CMSDraftInput{Title: r.Title, Slug: row.Slug, ChannelId: valueOrZero(r.ChannelId), Summary: r.Summary, BodyHTML: r.BodyHTML, Author: r.Author, CoverAssetId: valueOrZero(r.CoverAssetId), CoverAltText: r.CoverAltText, SeoTitle: r.SeoTitle, SeoDescription: r.SeoDescription, Remark: r.Remark, ProductCode: r.ProductCode, ProductParams: r.ProductParams, GalleryIds: r.GalleryIds})
		out.Source = append(out.Source, model.CMSTransferSource{ContentId: item.ContentId, Version: item.Version})
	}
	b, e := json.Marshal(out)
	if e != nil || len(b) > CMSTransferLimit {
		return nil, errs.New("选中内容超过1MiB，请减少导出条数")
	}
	return out, nil
}
func valueOrZero(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}
