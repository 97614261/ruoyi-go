package service

import (
	"context"
	"net/url"
	"regexp"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"strings"
	"unicode/utf8"
)

func ValidateSite(cfg model.SiteConfig) (string, error) {
	if (cfg.FontScale != "" && cfg.FontScale != "standard" && cfg.FontScale != "large") || (cfg.FrameWidth != "" && cfg.FrameWidth != "normal" && cfg.FrameWidth != "wide") || (cfg.Radius != "" && cfg.Radius != "square" && cfg.Radius != "soft") || cfg.IconAssetId < 0 {
		return "", errs.New("字号、圆角、宽度或图标不正确")
	}
	if cfg.Theme != "business" && cfg.Theme != "editorial" {
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
	if len(cfg.Navigation) > 12 || len(cfg.Banners) > 5 || len(cfg.Blocks) > 3 {
		return "", errs.New("导航最多12项，轮播最多5项，区块最多3项")
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
	seen := map[string]bool{}
	for _, b := range cfg.Blocks {
		if !regexp.MustCompile(`^(ARTICLE|PRODUCT|PAGE)$`).MatchString(b.Kind) || seen[b.Kind] || b.Count < 1 || b.Count > 12 || b.ChannelId < 0 || (b.Kind == "PAGE" && b.ChannelId != 0) {
			return "", errs.New("首页区块类型、条数或栏目不正确")
		}
		seen[b.Kind] = true
	}
	if cfg.Theme == "business" {
		return "v3", nil
	}
	return "v1", nil
}
func CMSGetSite(ctx context.Context) (*model.SiteState, error) { return repository.CMSGetSite(ctx) }
func CMSSaveSite(ctx context.Context, in model.SiteInput, operator string, assetsAllowed bool) error {
	if _, err := ValidateSite(in.Config); err != nil {
		return err
	}
	if !assetsAllowed && (in.Config.LogoAssetId > 0 || in.Config.IconAssetId > 0 || len(in.Config.Banners) > 0) {
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
