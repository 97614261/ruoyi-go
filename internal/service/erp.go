package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	"regexp"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
	"sort"
	"strings"
	"unicode/utf8"
)

var erpRequestKey = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// Only navigation is filtered: platform menu management/grant trees still
// contain ERP so the platform can explicitly enable customer features.
func hideERPForPlatform(ctx context.Context, menus []model.SysMenu) []model.SysMenu {
	if !tenantctx.Get(ctx).Platform {
		return menus
	}
	filtered := make([]model.SysMenu, 0, len(menus))
	for _, m := range menus {
		if m.Path == "erp" || (m.Perms != nil && strings.HasPrefix(*m.Perms, "erp:")) {
			continue
		}
		filtered = append(filtered, m)
	}
	return filtered
}
func prepareERPDocument(kind string, input model.ERPDocumentInput, operator string) (*model.ERPDocument, []model.ERPLineInput, error) {
	if kind != "IN" && kind != "SALE" {
		return nil, nil, errs.New("单据类型不合法")
	}
	if input.WarehouseID < 1 || !erpRequestKey.MatchString(input.RequestKey) || len(input.Lines) < 1 || len(input.Lines) > 100 || utf8.RuneCountInString(input.Remark) > 500 {
		return nil, nil, errs.New("仓库、请求标识或单据明细不合法")
	}
	items := append([]model.ERPLineInput(nil), input.Lines...)
	sort.Slice(items, func(i, j int) bool { return items[i].SkuID < items[j].SkuID })
	for i, item := range items {
		if item.SkuID < 1 || item.Quantity < 1 || item.Quantity > 1000000000 {
			return nil, nil, errs.New("商品和数量必须为正整数，单行数量不超过十亿")
		}
		if i > 0 && items[i-1].SkuID == item.SkuID {
			return nil, nil, errs.New("同一单据不能重复选择SKU")
		}
	}
	input.Lines = items
	input.Remark = strings.TrimSpace(input.Remark)
	raw, _ := json.Marshal(struct {
		Kind        string
		WarehouseID int64
		Remark      string
		Lines       []model.ERPLineInput
	}{kind, input.WarehouseID, input.Remark, items})
	hash := sha256.Sum256(raw)
	doc := &model.ERPDocument{Kind: kind, WarehouseID: input.WarehouseID, Status: "DRAFT", RequestKey: input.RequestKey, RequestHash: hex.EncodeToString(hash[:]), DocumentNo: kind + "-" + uuid.NewString(), ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now(), Remark: input.Remark}}
	return doc, items, nil
}
func CreateERPDocument(ctx context.Context, kind string, input model.ERPDocumentInput, operator string) (*model.ERPDocument, error) {
	s := tenantctx.Get(ctx)
	if s.ID < 1 || s.Platform {
		return nil, errs.New("仅企业工作空间可操作ERP")
	}
	doc, items, err := prepareERPDocument(kind, input, operator)
	if err != nil {
		return nil, err
	}
	if err = repository.InsertERPDocument(ctx, doc, items); err != nil {
		return nil, err
	}
	return repository.SelectERPDocument(ctx, kind, doc.DocumentID)
}
func ChangeERPDocument(ctx context.Context, kind string, id int64, action, operator string) error {
	if id < 1 {
		return errs.New("单据编号不合法")
	}
	return repository.TransitionERPDocument(ctx, kind, id, action, operator)
}
func GetERPDocument(ctx context.Context, kind string, id int64) (*model.ERPDocument, error) {
	if id < 1 {
		return nil, errs.New("单据编号不合法")
	}
	return repository.SelectERPDocument(ctx, kind, id)
}
