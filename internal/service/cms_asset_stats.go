package service

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"strings"
	"unicode"
	"unicode/utf8"
)

func cmsAssetGroup(s string) (string, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > 40 || strings.ContainsAny(s, "<>\\/") || strings.IndexFunc(s, unicode.IsControl) >= 0 {
		return "", errs.New("分组最长40字，不能包含尖括号、斜线或控制字符")
	}
	return s, nil
}
func cmsAssetQuery(q model.CMSAssetQuery) (model.CMSAssetQuery, error) {
	q.Name = strings.TrimSpace(q.Name)
	if q.Ungrouped {
		if q.GroupName != nil && strings.TrimSpace(*q.GroupName) != "" {
			return q, errs.New("未分组不能与指定分组同时筛选")
		}
		v := ""
		q.GroupName = &v
	}
	if q.GroupName != nil {
		v, e := cmsAssetGroup(*q.GroupName)
		if e != nil {
			return q, e
		}
		q.GroupName = &v
	}
	if q.MaxWidth > 0 && q.MinWidth > q.MaxWidth || q.MaxBytes > 0 && q.MinBytes > q.MaxBytes {
		return q, errs.New("筛选下限不能大于上限")
	}
	return q, nil
}
func CMSAssetStatistics(ctx context.Context, q model.CMSAssetQuery) (model.CMSAssetStats, error) {
	q, e := cmsAssetQuery(q)
	if e != nil {
		return model.CMSAssetStats{}, e
	}
	return repository.CMSAssetStatistics(ctx, q)
}
