package service

import (
	"context"
	"fmt"
	"golang.org/x/net/html"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"strings"
)

func cmsIssue(code, msg string, id, asset int64) model.CMSIssue {
	return model.CMSIssue{Code: code, Message: msg, Severity: "error", ContentId: id, AssetId: asset}
}
func cmsRevisionAssets(r model.CMSRevision) ([]int64, error) {
	_, ids, err := CMSBody(r.BodyHTML)
	seen := map[int64]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	if r.CoverAssetId != nil && !seen[*r.CoverAssetId] {
		ids = append(ids, *r.CoverAssetId)
		seen[*r.CoverAssetId] = true
	}
	for _, id := range r.GalleryIds {
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	return ids, err
}
func cmsCheckAsset(id int64, assets map[int64]model.CmsAsset) []model.CMSIssue {
	a, ok := assets[id]
	if !ok {
		return []model.CMSIssue{cmsIssue("ASSET_MISSING", "素材未登记或已删除", 0, id)}
	}
	if cmsAssetRoot == "" || !cmsFileKey.MatchString(a.StorageKey) {
		return []model.CMSIssue{cmsIssue("ASSET_KEY_INVALID", "素材存储标识不正确", 0, id)}
	}
	info, err := os.Lstat(filepath.Join(cmsAssetRoot, a.StorageKey))
	if err != nil {
		return []model.CMSIssue{cmsIssue("FILE_MISSING", "素材文件缺失或不可读取", 0, id)}
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() != a.ByteSize {
		return []model.CMSIssue{cmsIssue("FILE_CHANGED", "素材不是普通文件或文件大小已变化", 0, id)}
	}
	return nil
}
func cmsChannels(rows []model.CmsChannel) map[int64]model.CmsChannel {
	out := map[int64]model.CmsChannel{}
	for _, r := range rows {
		out[r.ChannelId] = r
	}
	return out
}
func cmsActiveChannel(id int64, kind string, rows map[int64]model.CmsChannel) bool {
	seen := map[int64]bool{}
	for depth := 0; depth < 8; depth++ {
		r, ok := rows[id]
		if !ok || seen[id] || r.Kind != kind || r.Status != "0" {
			return false
		}
		if r.ParentId == 0 {
			return true
		}
		seen[id] = true
		id = r.ParentId
	}
	return false
}
func cmsCheckRevision(kind, slug string, version int64, r model.CMSRevision, assets map[int64]model.CmsAsset, channels map[int64]model.CmsChannel) model.CMSCheck {
	out := model.CMSCheck{ContentId: r.ContentId, Version: version, Issues: []model.CMSIssue{}}
	if strings.TrimSpace(r.Title) == "" || !cmsSlug.MatchString(slug) {
		out.Issues = append(out.Issues, cmsIssue("CONTENT_FIELDS", "标题或访问地址不正确", r.ContentId, 0))
	}
	if kind != "PAGE" && (r.ChannelId == nil || !cmsActiveChannel(*r.ChannelId, kind, channels)) {
		out.Issues = append(out.Issues, cmsIssue("CHANNEL_UNAVAILABLE", "栏目或上级不存在、停用或类型不匹配", r.ContentId, 0))
	}
	ids, err := cmsRevisionAssets(r)
	if err != nil {
		out.Issues = append(out.Issues, cmsIssue("BODY_INVALID", "正文为空、格式不正确或图片来源不合法", r.ContentId, 0))
	}
	if len(ids) > 32 {
		out.Issues = append(out.Issues, cmsIssue("ASSET_LIMIT", "素材数量超过限制", r.ContentId, 0))
	}
	for _, id := range ids {
		for _, issue := range cmsCheckAsset(id, assets) {
			issue.ContentId = r.ContentId
			out.Issues = append(out.Issues, issue)
		}
	}
	out.Ready = len(out.Issues) == 0
	return out
}
func CMSCheckPublication(ctx context.Context, kind string, id int64) (*model.CMSCheck, error) {
	d, err := repository.CMSContentDetail(ctx, kind, id)
	if err != nil {
		return nil, err
	}
	ids, _ := cmsRevisionAssets(d.Draft)
	rows, err := repository.CMSAssetsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	assets := map[int64]model.CmsAsset{}
	for _, r := range rows {
		assets[r.AssetId] = r
	}
	channels, err := repository.CMSInspectionChannels(ctx)
	if err != nil {
		return nil, err
	}
	out := cmsCheckRevision(kind, d.Slug, d.Version, d.Draft, assets, cmsChannels(channels))
	return &out, nil
}
func CMSBatchChange(ctx context.Context, kind string, in model.CMSBatchInput, operator string, canReadAssets bool) ([]model.CMSBatchResult, error) {
	if len(in.Items) < 1 || len(in.Items) > 20 || (in.Action != "publish" && in.Action != "unpublish") {
		return nil, errs.New("批量操作只支持1—20项发布或下架")
	}
	seen := map[int64]bool{}
	for _, i := range in.Items {
		if i.ContentId < 1 || i.Version < 1 || seen[i.ContentId] {
			return nil, errs.New("内容ID重复、版本无效或ID不正确")
		}
		seen[i.ContentId] = true
	}
	out := []model.CMSBatchResult{}
	for _, i := range in.Items {
		r := model.CMSBatchResult{ContentId: i.ContentId}
		if ctx.Err() != nil {
			r.Message = "请求已结束，未执行此项"
			out = append(out, r)
			continue
		}
		err := CMSChangeContent(ctx, kind, i.ContentId, model.CMSContentAction{Version: i.Version}, in.Action, operator, canReadAssets)
		if err == nil {
			r.Success = true
			r.Message = "操作成功"
		} else if e := errs.As(err); e != nil {
			r.Message = e.Msg
		} else {
			r.Message = "系统异常，请刷新核对执行结果"
			slog.Error("门户批量操作失败", "contentId", i.ContentId, "action", in.Action, "err", err)
		}
		out = append(out, r)
	}
	return out, nil
}
func CMSInspectSite(ctx context.Context, pg page.Query, publicOrigin string) (*model.CMSInspection, error) {
	rows, total, err := repository.CMSInspectionPage(ctx, pg)
	if err != nil {
		return nil, err
	}
	channels, err := repository.CMSInspectionChannels(ctx)
	if err != nil {
		return nil, err
	}
	byChannel := cmsChannels(channels)
	urls, err := repository.CMSInspectionURLs(ctx)
	if err != nil {
		return nil, err
	}
	for _, ch := range channels {
		if cmsActiveChannel(ch.ChannelId, ch.Kind, byChannel) {
			prefix := "/news/c/"
			if ch.Kind == "PRODUCT" {
				prefix = "/products/c/"
			}
			urls[prefix+ch.Slug] = true
		}
	}
	cfg, err := repository.CMSRawPublishedSite(ctx)
	if err != nil {
		return nil, err
	}
	assetIDs := []int64{}
	seen := map[int64]bool{}
	add := func(id int64) {
		if id > 0 && !seen[id] {
			assetIDs = append(assetIDs, id)
			seen[id] = true
		}
	}
	for _, r := range rows {
		ids, _ := cmsRevisionAssets(r.CMSRevision)
		for _, id := range ids {
			add(id)
		}
	}
	add(cfg.LogoAssetId)
	add(cfg.IconAssetId)
	for _, b := range cfg.Banners {
		add(b.AssetId)
	}
	assetsRows, err := repository.CMSAssetsByIDs(ctx, assetIDs)
	if err != nil {
		return nil, err
	}
	assets := map[int64]model.CmsAsset{}
	for _, r := range assetsRows {
		assets[r.AssetId] = r
	}
	out := &model.CMSInspection{Rows: []model.CMSCheck{}, Total: total, Issues: []model.CMSIssue{}, Scope: "当前页已标记发布内容 + 当前外观；仅站内逻辑/文件检查，不探测外站"}
	if expected, e := ValidateSite(cfg); e != nil {
		out.Issues = append(out.Issues, cmsIssue("SITE_CONFIG_INVALID", "站点设置存在不受支持的配置", 0, 0))
	} else if cfg.ThemeVersion != "" && cfg.ThemeVersion != expected {
		out.Issues = append(out.Issues, cmsIssue("SITE_RESOURCE_VERSION", "外观资源版本不匹配", 0, 0))
	}
	for _, id := range []int64{cfg.LogoAssetId, cfg.IconAssetId} {
		if id > 0 {
			out.Issues = append(out.Issues, cmsCheckAsset(id, assets)...)
		}
	}
	linkCheck := func(raw, base string, id int64) *model.CMSIssue {
		u, e := url.Parse(raw)
		if e != nil {
			issue := cmsIssue("LINK_INVALID", "链接格式不正确", id, 0)
			return &issue
		}
		if u.User != nil {
			issue := cmsIssue("LINK_CREDENTIALS", "链接不应包含账号信息", id, 0)
			return &issue
		}
		if u.IsAbs() || u.Host != "" {
			origin, e := url.Parse(publicOrigin)
			if e != nil || origin.Host == "" || !strings.EqualFold(u.Host, origin.Host) || (u.Scheme != "" && u.Scheme != origin.Scheme) {
				out.ExternalLinksSkipped++
				return nil
			}
			u.Host = ""
			u.Scheme = ""
		}
		if strings.HasPrefix(raw, "#") {
			return nil
		}
		baseURL, _ := url.Parse(base)
		path := baseURL.ResolveReference(u).Path
		if strings.HasPrefix(path, "/media/") {
			issue := cmsIssue("MEDIA_LINK_UNCHECKED", "媒体直链不做网络探测；请用素材预览或公开入口核验", id, 0)
			issue.Severity = "warning"
			issue.URL = cmsInspectionPath(path)
			return &issue
		}
		if !urls[path] {
			issue := cmsIssue("INTERNAL_LINK_BROKEN", "站内路径不存在、已下架或栏目不可用", id, 0)
			issue.URL = cmsInspectionPath(path)
			return &issue
		}
		return nil
	}
	for _, block := range cfg.Blocks {
		if block.Enabled && block.ChannelId > 0 && !cmsActiveChannel(block.ChannelId, block.Kind, byChannel) {
			out.Issues = append(out.Issues, cmsIssue("SITE_CHANNEL_UNAVAILABLE", "首页区块的栏目已停用或不存在", 0, 0))
		}
	}
	for _, n := range cfg.Navigation {
		if n.ContentId > 0 {
			if !urls[fmt.Sprintf("content:%d", n.ContentId)] {
				out.Issues = append(out.Issues, cmsIssue("SITE_CONTENT_UNAVAILABLE", "导航引用内容已失效", n.ContentId, 0))
			}
		} else if issue := linkCheck(n.URL, "/", 0); issue != nil {
			out.Issues = append(out.Issues, *issue)
		}
	}
	for _, b := range cfg.Banners {
		if !b.Enabled {
			continue
		}
		out.Issues = append(out.Issues, cmsCheckAsset(b.AssetId, assets)...)
		if b.ContentId > 0 {
			if !urls[fmt.Sprintf("content:%d", b.ContentId)] {
				out.Issues = append(out.Issues, cmsIssue("SITE_CONTENT_UNAVAILABLE", "轮播引用内容已失效", b.ContentId, b.AssetId))
			}
		}
	}
	for _, r := range rows {
		check := cmsCheckRevision(r.Kind, r.Slug, r.Version, r.CMSRevision, assets, byChannel)
		doc, e := html.Parse(strings.NewReader(r.BodyHTML))
		if e != nil {
			check.Issues = append(check.Issues, cmsIssue("BODY_INVALID", "正文无法解析", r.ContentId, 0))
		} else {
			links := 0
			var walk func(*html.Node)
			walk = func(n *html.Node) {
				if n.Type == html.ElementNode && n.Data == "a" {
					for _, a := range n.Attr {
						if a.Key == "href" {
							links++
							if links > 100 {
								continue
							}
							path := "/p/"
							if r.Kind == "ARTICLE" {
								path = "/news/"
							}
							if r.Kind == "PRODUCT" {
								path = "/products/"
							}
							if issue := linkCheck(a.Val, path+r.Slug, r.ContentId); issue != nil {
								check.Issues = append(check.Issues, *issue)
							}
						}
					}
				}
				for child := n.FirstChild; child != nil; child = child.NextSibling {
					walk(child)
				}
			}
			walk(doc)
			if links > 100 {
				check.Issues = append(check.Issues, cmsIssue("LINK_SCAN_LIMIT", "单篇链接超过100条，本页结果不完整，请拆分内容后复查", r.ContentId, 0))
			}
		}
		check.Ready = len(check.Issues) == 0
		out.Rows = append(out.Rows, check)
	}
	return out, nil
}

func cmsInspectionPath(path string) string {
	r := []rune(path)
	if len(r) > 500 {
		return string(r[:500]) + "（路径已截断）"
	}
	return path
}
