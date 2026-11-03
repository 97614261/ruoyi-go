package service

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"strconv"
	"strings"
)

func CMSSiteReferences(ctx context.Context, q model.CMSSiteReferenceQuery, allowed []string, pg page.Query) ([]model.CMSSiteReference, int64, error) {
	if q.Type != "channel" && q.Type != "content" {
		return nil, 0, errs.New("引用类型不正确")
	}
	kinds := []string{}
	for _, kind := range allowed {
		if (kind == "ARTICLE" || kind == "PRODUCT" || (q.Type == "content" && kind == "PAGE")) && (q.Kind == "" || q.Kind == kind) {
			kinds = append(kinds, kind)
		}
	}
	if len(kinds) == 0 {
		return nil, 0, errs.ErrForbidden
	}
	ids := []int64{}
	if q.IDs != "" {
		parts := strings.Split(q.IDs, ",")
		if len(parts) > 100 {
			return nil, 0, errs.New("一次最多查询100个引用")
		}
		seen := map[int64]bool{}
		for _, part := range parts {
			id, e := strconv.ParseInt(part, 10, 64)
			if e != nil || id < 1 || seen[id] {
				return nil, 0, errs.New("引用编号无效或重复")
			}
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return repository.CMSSiteReferences(ctx, q, kinds, ids, pg)
}
