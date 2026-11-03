package service

import (
	"context"
	"net/url"
	"regexp"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/portalview"
	"strings"
	"unicode/utf8"
)

func ValidateSite(cfg model.SiteConfig) (string, error) {
	if cfg.AnalyticsRetention < 0 || cfg.AnalyticsRetention > 90 {
		return "", errs.New("访问统计保留期0默认30天，最多90天")
	}
	for _, b := range cfg.Blocks {
		if len(b.Items) > 12 || utf8.RuneCountInString(b.Title) > 80 || strings.ContainsAny(b.Title, "<>\x00") || (b.Layout != "" && b.Layout != "grid" && b.Layout != "list") {
			return "", errs.New("首页组件标题、布局或条目错误")
		}
		for _, v := range b.Items {
			if b.Kind == "FAQ" && v.AssetId > 0 {
				return "", errs.New("常见问题不支持图片")
			}
			if strings.TrimSpace(v.Title) == "" || utf8.RuneCountInString(v.Title) > 80 || utf8.RuneCountInString(v.Text) > 500 || strings.ContainsAny(v.Title+v.Text, "<>\x00") || v.AssetId < 0 || v.ContentId < 0 {
				return "", errs.New("首页组件条目格式错误")
			}
		}
	}
	if cfg.ListingLayout != "" && cfg.ListingLayout != "grid" && cfg.ListingLayout != "list" {
		return "", errs.New("列表布局只支持网格或列表")
	}
	if len(cfg.ChannelLayouts) > 100 {
		return "", errs.New("栏目单独布局最多100项")
	}
	seenLayouts := map[int64]bool{}
	for _, o := range cfg.ChannelLayouts {
		if o.ChannelId < 1 || seenLayouts[o.ChannelId] || (o.Layout != "grid" && o.Layout != "list") {
			return "", errs.New("栏目布局ID重复、无效或布局不支持")
		}
		seenLayouts[o.ChannelId] = true
	}

	if (cfg.FontScale != "" && cfg.FontScale != "standard" && cfg.FontScale != "large") || (cfg.FrameWidth != "" && cfg.FrameWidth != "normal" && cfg.FrameWidth != "wide") || (cfg.Radius != "" && cfg.Radius != "square" && cfg.Radius != "soft") || cfg.IconAssetId < 0 {
		return "", errs.New("字号、圆角、宽度或图标不正确")
	}
	theme, known := portalview.FindBuiltinTheme(cfg.Theme)
	if !known {
		return "", errs.New("模板未注册")
	}
	if cfg.Palette != "earth" && cfg.Palette != "ocean" && cfg.Palette != "forest" {
		return "", errs.New("配色不受支持")
	}
	if cfg.Density != "normal" && cfg.Density != "compact" {
		return "", errs.New("密度不受支持")
	}
	for _, v := range []struct {
		s string
		n int
	}{{cfg.SiteName, 60}, {cfg.Slogan, 200}, {cfg.Footer, 500}, {cfg.Phone, 60}, {cfg.Email, 100}} {
		if utf8.RuneCountInString(v.s) > v.n || strings.ContainsAny(v.s, "<>\x00") {
			return "", errs.New("站点文本格式或长度不正确")
		}
	}
	if strings.TrimSpace(cfg.SiteName) == "" || cfg.LogoAssetId < 0 {
		return "", errs.New("站点名称或Logo不正确")
	}
	if len(cfg.Navigation) > 12 || len(cfg.Banners) > 5 || len(cfg.Blocks) > 15 {
		return "", errs.New("导航最多12项，轮播最多5项，区块最多15项")
	}
	for _, n := range cfg.Navigation {
		if n.Placement != "" && n.Placement != "header" && n.Placement != "footer" {
			return "", errs.New("导航位置不受支持")
		}
		if strings.TrimSpace(n.Label) == "" || utf8.RuneCountInString(n.Label) > 30 || strings.ContainsAny(n.Label, "<>\x00") || n.ContentId < 0 {
			return "", errs.New("导航文字或引用不正确")
		}
		if n.ContentId == 0 {
			u, e := url.Parse(n.URL)
			if e != nil || u.User != nil || len(n.URL) > 500 || !((u.Scheme == "https" && u.Host != "") || (u.Scheme == "" && u.Host == "" && strings.HasPrefix(n.URL, "/") && !strings.HasPrefix(n.URL, "//") && !strings.ContainsAny(n.URL, `\`))) {
				return "", errs.New("导航只允许站内路径、内容ID或HTTPS网址")
			}
		} else if n.URL != "" {
			return "", errs.New("内部导航请仅填写内容ID")
		}
	}
	for _, b := range cfg.Banners {
		if b.AssetId < 1 || b.ContentId < 0 || utf8.RuneCountInString(b.Title) > 100 || strings.ContainsAny(b.Title, "<>\x00") {
			return "", errs.New("轮播参数不正确")
		}
	}
	// Repeated content kinds are independent sections; the 15-section cap still applies.
	for _, b := range cfg.Blocks {
		if b.Kind == "SERVICE" || b.Kind == "ADVANTAGE" || b.Kind == "CASE" || b.Kind == "PARTNER" || b.Kind == "FAQ" {
			if b.ChannelId != 0 || b.FeaturedOnly {
				return "", errs.New("展示组件不能设置栏目或推荐筛选")
			}
			continue
		}
		if len(b.Items) > 0 {
			return "", errs.New("内容列表区块不能填写手动条目")
		}
		if !regexp.MustCompile(`^(ARTICLE|PRODUCT|PAGE)$`).MatchString(b.Kind) || b.Count < 1 || b.Count > 12 || b.ChannelId < 0 || (b.Kind == "PAGE" && b.ChannelId != 0) {
			return "", errs.New("首页区块类型、条数或栏目不正确")
		}
	}
	return theme.Version, nil
}
func CMSGetSite(ctx context.Context) (*model.SiteState, error) { return repository.CMSGetSite(ctx) }
func CMSSaveSite(ctx context.Context, in model.SiteInput, operator string, assetsAllowed bool) error {
	if _, err := ValidateSite(in.Config); err != nil {
		return err
	}
	if !assetsAllowed && siteHasAssets(in.Config) {
		return errs.ErrForbidden
	}
	return repository.CMSSaveSite(ctx, in, operator)
}
func CMSPublishSite(ctx context.Context, in model.SiteAction, operator string) error {
	return repository.CMSPublishSite(ctx, in, operator, ValidateSite)
}
func CMSSiteHistory(ctx context.Context, pg page.Query) ([]model.SiteRelease, int64, error) {
	return repository.CMSSiteHistory(ctx, pg)
}
func CMSPublicSite(ctx context.Context) (model.SiteConfig, error) {
	return repository.CMSPublicSite(ctx)
}
func CMSResolveSite(ctx context.Context, cfg model.SiteConfig) (model.SiteConfig, error) {
	return repository.CMSResolveSite(ctx, cfg)
}

func siteHasAssets(cfg model.SiteConfig) bool {
	if cfg.LogoAssetId > 0 || cfg.IconAssetId > 0 || len(cfg.Banners) > 0 {
		return true
	}
	for _, b := range cfg.Blocks {
		for _, v := range b.Items {
			if v.AssetId > 0 {
				return true
			}
		}
	}
	return false
}
