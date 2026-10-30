package service

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/net/html"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/htmlx"
	"ruoyi-go/pkg/page"
	"strconv"
	"strings"
)

// CMSBody accepts local material identifiers only. Run on both write and read boundaries.
func CMSBody(raw string) (string, []int64, error) {
	if len(raw) > 256<<10 {
		return "", nil, errs.New("正文最多256KiB")
	}
	doc, err := html.Parse(strings.NewReader(htmlx.SanitizeQuill(raw)))
	if err != nil {
		return "", nil, errs.New("正文格式错误")
	}
	ids := []int64{}
	seen := map[int64]bool{}
	hasText := false
	var body *html.Node
	var walk func(*html.Node) error
	walk = func(n *html.Node) error {
		if n.Type == html.ElementNode && n.Data == "body" {
			body = n
		}
		if n.Type == html.ElementNode && n.Data == "iframe" {
			return errs.New("正文暂不支持内嵌视频")
		}
		if n.Type == html.TextNode && strings.TrimSpace(n.Data) != "" {
			hasText = true
		}
		if n.Type == html.ElementNode && n.Data == "img" {
			src := ""
			sourceCount := 0
			for _, a := range n.Attr {
				if a.Key == "src" {
					sourceCount++
					src = a.Val
				}
			}
			if sourceCount != 1 {
				return errs.New("图片地址必须唯一")
			}
			id, e := strconv.ParseInt(strings.TrimPrefix(src, "/media/"), 10, 64)
			if e != nil || id < 1 || src != "/media/"+strconv.FormatInt(id, 10) {
				return errs.New("正文图片必须从门户素材库选择")
			}
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
			if len(ids) > 32 {
				return errs.New("每个版本最多引用32张图片")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if e := walk(c); e != nil {
				return e
			}
		}
		return nil
	}
	if err = walk(doc); err != nil {
		return "", nil, err
	}
	if !hasText && len(ids) == 0 {
		return "", nil, errs.New("请输入正文")
	}
	var out strings.Builder
	if body == nil {
		return "", nil, errs.New("正文格式错误")
	}
	for n := body.FirstChild; n != nil; n = n.NextSibling {
		if err = html.Render(&out, n); err != nil {
			return "", nil, err
		}
	}
	return out.String(), ids, nil
}
func cmsRevision(in model.CMSDraftInput) (model.CMSRevision, []int64, error) {
	body, ids, err := CMSBody(in.BodyHTML)
	if err != nil {
		return model.CMSRevision{}, nil, err
	}
	r := model.CMSRevision{ProductCode: in.ProductCode, ProductParams: in.ProductParams, GalleryIds: in.GalleryIds, Title: strings.TrimSpace(in.Title), Summary: in.Summary, BodyHTML: body, Author: in.Author, CoverAltText: in.CoverAltText, SeoTitle: in.SeoTitle, SeoDescription: in.SeoDescription, Remark: in.Remark}
	if in.ChannelId > 0 {
		r.ChannelId = &in.ChannelId
	}
	if in.CoverAssetId > 0 {
		r.CoverAssetId = &in.CoverAssetId
		found := false
		for _, id := range ids {
			if id == in.CoverAssetId {
				found = true
			}
		}
		if !found {
			ids = append(ids, in.CoverAssetId)
		}
	}
	for _, galleryID := range in.GalleryIds {
		found := false
		for _, id := range ids {
			if id == galleryID {
				found = true
			}
		}
		if !found {
			ids = append(ids, galleryID)
		}
	}
	if len(ids) > 32 {
		return r, nil, errs.New("每个版本最多引用32张图片（含封面）")
	}
	return r, ids, nil
}
func CMSSaveDraft(ctx context.Context, kind string, in model.CMSDraftInput, operator string, canReadAssets bool) (int64, error) {
	if !cmsSlug.MatchString(in.Slug) {
		return 0, errs.New("地址仅支持小写字母开头、数字和连字符")
	}
	if (kind == "ARTICLE" || kind == "PRODUCT") && in.ChannelId < 1 {
		return 0, errs.New("文章必须选择文章栏目")
	}
	if kind == "PAGE" && in.ChannelId != 0 {
		return 0, errs.New("单页不能指定栏目")
	}
	if in.ContentId > 0 && in.Version < 1 {
		return 0, errs.New("请提交当前版本")
	}
	if kind != "PRODUCT" && (in.ProductCode != "" || in.ProductParams != "" || len(in.GalleryIds) > 0) {
		return 0, errs.New("产品字段只能用于产品展示")
	}
	r, ids, err := cmsRevision(in)
	if err != nil {
		return 0, err
	}
	if len(ids) > 0 && !canReadAssets {
		return 0, errs.WithCode(403, "引用图片需要素材查询权限")
	}
	return repository.CMSSaveDraft(ctx, kind, in, r, ids, operator)
}
func CMSContentPage(ctx context.Context, kind string, q model.CMSContentQuery, pg page.Query) ([]model.CMSContentView, int64, error) {
	return repository.CMSContentPage(ctx, kind, q, pg)
}
func CMSContentDetail(ctx context.Context, kind string, id int64) (*model.CMSContentDetail, error) {
	d, err := repository.CMSContentDetail(ctx, kind, id)
	if err != nil {
		return nil, err
	}
	body, _, err := CMSBody(d.Draft.BodyHTML)
	if err != nil {
		return nil, err
	}
	d.Draft.BodyHTML = body
	return d, nil
}
func CMSHistory(ctx context.Context, kind string, id int64, pg page.Query) ([]model.CMSRevision, int64, error) {
	return repository.CMSHistory(ctx, kind, id, pg)
}
func CMSChangeContent(ctx context.Context, kind string, id int64, in model.CMSContentAction, action, operator string, canReadAssets bool) error {
	if action == "publish" {
		check, err := CMSCheckPublication(ctx, kind, id)
		if err != nil {
			return err
		}
		if check.Version != in.Version {
			return errs.New("内容已变化，请刷新后重试")
		}
		if !check.Ready {
			return errs.New("发布前检查未通过：" + check.Issues[0].Message)
		}
	}
	return repository.CMSChangeContent(ctx, kind, id, in, action, operator, canReadAssets, func(r model.CMSRevision) (string, []int64, error) {
		body, ids, err := CMSBody(r.BodyHTML)
		if r.CoverAssetId != nil {
			found := false
			for _, id := range ids {
				if id == *r.CoverAssetId {
					found = true
				}
			}
			if !found {
				ids = append(ids, *r.CoverAssetId)
			}
		}
		for _, galleryID := range r.GalleryIds {
			found := false
			for _, id := range ids {
				if id == galleryID {
					found = true
				}
			}
			if !found {
				ids = append(ids, galleryID)
			}
		}
		return body, ids, err
	})
}
func CMSPublicContent(ctx context.Context, kind, slug string) (*model.CMSRevision, error) {
	if !cmsSlug.MatchString(slug) {
		return nil, errs.ErrNotFound
	}
	r, err := repository.CMSPublishedContent(ctx, kind, slug)
	if err != nil {
		return nil, err
	}
	body, _, err := CMSBody(r.BodyHTML)
	if err != nil {
		return nil, fmt.Errorf("published revision invalid: %w", err)
	}
	r.BodyHTML = body
	return r, nil
}
func CMSPublicAssetAllowed(ctx context.Context, id int64) (bool, error) {
	return repository.CMSPublicAssetAllowed(ctx, id)
}

func IsCMSNotFound(err error) bool { return errors.Is(err, errs.ErrNotFound) }

func CMSPublicPage(ctx context.Context, kind string, pg page.Query) ([]model.CMSPublicSummary, int64, error) {
	return repository.CMSPublicPage(ctx, kind, pg)
}
