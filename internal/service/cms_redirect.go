package service

import (
	"context"
	"regexp"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"strings"
)

var redirectSource = regexp.MustCompile(`^/[a-z0-9][a-z0-9._-]*(/[a-z0-9][a-z0-9._-]*)*$`)
var redirectReserved = map[string]bool{"admin": true, "cms": true, "system": true, "monitor": true, "tool": true, "common": true, "profile": true, "assets": true, "media": true, "portal": true, "login": true, "logout": true, "register": true, "captchaimage": true, "getinfo": true, "getrouters": true, "unlockscreen": true, "health": true, "inquiry": true, "downloads": true, "sitemap": true, "sitemap.xml": true, "robots.txt": true}

func CMSRedirectSource(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) > 200 || !redirectSource.MatchString(value) {
		return "", errs.New("旧路径限200位小写字母、数字及/._-；不能含域名、查询参数、编码符或尾斜线")
	}
	first := strings.Split(strings.TrimPrefix(value, "/"), "/")[0]
	if redirectReserved[first] || value == "/news" || value == "/products" || value == "/search" || value == "/p" || value == "/news/c" || strings.HasPrefix(value, "/news/c/") || value == "/products/c" || strings.HasPrefix(value, "/products/c/") {
		return "", errs.New("不能占用后台、接口、静态资源或门户固定入口")
	}
	return value, nil
}
func CMSRedirectPage(ctx context.Context, q model.CMSRedirectQuery, pg page.Query) ([]model.CMSRedirectView, int64, error) {
	q.SourcePath = strings.TrimSpace(q.SourcePath)
	return repository.CMSRedirectPage(ctx, q, pg)
}
func CMSRedirectDetail(ctx context.Context, id int64) (*model.CMSRedirectView, error) {
	return repository.CMSRedirectDetail(ctx, id)
}
func CMSSaveRedirect(ctx context.Context, in model.CMSRedirectInput, operator string) (int64, error) {
	source, e := CMSRedirectSource(in.SourcePath)
	if e != nil {
		return 0, e
	}
	in.SourcePath = source
	if in.TargetId <= 0 || in.RedirectId < 0 || in.RedirectId > 0 && in.Version <= 0 {
		return 0, errs.New("目标内容或版本不正确")
	}
	return repository.CMSSaveRedirect(ctx, in, operator)
}
func CMSSetRedirectStatus(ctx context.Context, id int64, in model.CMSRedirectStatus, operator string) error {
	if id <= 0 || in.Version < 1 || (in.Status != "0" && in.Status != "1") {
		return errs.New("映射编号、版本或状态错误")
	}
	return repository.CMSSetRedirectStatus(ctx, id, in, operator)
}
func CMSResolveRedirect(ctx context.Context, source string) (string, error) {
	cleaned, e := CMSRedirectSource(source)
	if e != nil || cleaned != source {
		return "", errs.ErrNotFound
	}
	return repository.CMSResolveRedirect(ctx, source)
}
