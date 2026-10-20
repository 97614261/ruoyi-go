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
	if kind != "IN" && kind != "SALE" && kind != "SR" && kind != "PR" && kind != "COUNT" {
		return nil, nil, errs.New("单据类型不合法")
	}
	if input.WarehouseID < 1 || !erpRequestKey.MatchString(input.RequestKey) || len(input.Lines) < 1 || len(input.Lines) > 100 || utf8.RuneCountInString(input.Remark) > 500 {
		return nil, nil, errs.New("仓库、请求标识或单据明细不合法")
	}
	items := append([]model.ERPLineInput(nil), input.Lines...)
	correction := kind == "SR" || kind == "PR"
	if (correction || kind == "COUNT") && strings.TrimSpace(input.Remark) == "" {
		return nil, nil, errs.New("退货、冲销和盘点必须填写原因")
	}
	if correction != (input.SourceDocumentID != nil) || (input.SourceDocumentID != nil && *input.SourceDocumentID < 1) || (!correction && input.Reversal) {
		return nil, nil, errs.New("退货或冲销必须关联原单，普通单据不能指定来源")
	}
	for i := range items {
		if kind == "COUNT" {
			if items[i].CountedQuantity == nil {
				return nil, nil, errs.New("盘点必须填写实盘数量，零也要显式提交")
			}
			items[i].Quantity = *items[i].CountedQuantity
			items[i].CountedQuantity = nil
		} else if items[i].CountedQuantity != nil {
			return nil, nil, errs.New("普通单据不能提交实盘数量")
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].SkuID < items[j].SkuID })
	for i, item := range items {
		if item.SkuID < 1 || item.Quantity < 0 || (kind != "COUNT" && item.Quantity == 0) || item.Quantity > 1000000000 {
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
		SourceID    *int64
		Reversal    bool
	}{kind, input.WarehouseID, input.Remark, items, input.SourceDocumentID, input.Reversal})
	if kind == "IN" || kind == "SALE" {
		// Keep hashes of pre-upgrade creation requests valid.
		raw, _ = json.Marshal(struct {
			Kind        string
			WarehouseID int64
			Remark      string
			Lines       []model.ERPLineInput
		}{kind, input.WarehouseID, input.Remark, items})
	}
	hash := sha256.Sum256(raw)
	doc := &model.ERPDocument{Kind: kind, WarehouseID: input.WarehouseID, Status: "DRAFT", Version: 1, SourceDocumentID: input.SourceDocumentID, Reversal: input.Reversal, RequestKey: input.RequestKey, RequestHash: hex.EncodeToString(hash[:]), DocumentNo: kind + "-" + uuid.NewString(), ERPRecord: model.ERPRecord{CreateBy: operator, CreateTime: types.Now(), Remark: input.Remark}}
	return doc, items, nil
}
func EditERPDocument(ctx context.Context, kind string, id int64, input model.ERPDocumentEdit, operator string) error {
	if id < 1 || input.Version < 1 {
		return errs.New("单据或版本不合法")
	}
	// RequestKey is immutable create-idempotency metadata, not an edit token.
	input.RequestKey = "edit-only-key"
	doc, items, err := prepareERPDocument(kind, input.ERPDocumentInput, operator)
	if err != nil {
		return err
	}
	doc.DocumentID = id
	doc.Version = input.Version
	return repository.UpdateERPDocumentDraft(ctx, doc, items, operator)
}
func GetERPWarehouseAccess(ctx context.Context, userID int64) (*model.ERPAccessView, error) {
	if userID < 1 {
		return nil, errs.New("用户不合法")
	}
	return repository.GetERPWarehouseAccess(ctx, userID)
}
func SetERPWarehouseAccess(ctx context.Context, userID int64, input model.ERPAccessInput, operator string) error {
	if userID < 1 || input.Version < 1 || len(input.WarehouseIDs) > 200 {
		return errs.New("授权参数不合法")
	}
	sort.Slice(input.WarehouseIDs, func(i, j int) bool { return input.WarehouseIDs[i] < input.WarehouseIDs[j] })
	for i, id := range input.WarehouseIDs {
		if id < 1 || (i > 0 && id == input.WarehouseIDs[i-1]) {
			return errs.New("仓库编号不合法或重复")
		}
	}
	return repository.ReplaceERPWarehouseAccess(ctx, userID, input, operator)
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
