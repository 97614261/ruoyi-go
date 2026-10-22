package service

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
	"strings"
	"unicode"
	"unicode/utf8"
)

// External identifiers stay text, case sensitive, with leading zeroes intact.
// Reject edge whitespace instead of silently merging IDs (MySQL 5.7 ignores
// trailing spaces for varchar equality even under *_bin collations).
func shopItemIdentifier(value string, optional bool) (*string, error) {
	if value == "" && optional {
		return nil, nil
	}
	if value == "" || strings.TrimSpace(value) != value || utf8.RuneCountInString(value) > 128 || !utf8.ValidString(value) {
		return nil, errs.New("商品标识需为1—128字且不能有首尾空白")
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return nil, errs.New("商品标识不能含控制字符")
		}
	}
	return &value, nil
}
func prepareERPShopItem(input model.ERPShopItemWrite) (*model.ErpShopItem, error) {
	if input.ShopID < 1 {
		return nil, errs.New("请选择店铺")
	}
	code, err := shopItemIdentifier(input.ShopItemCode, false)
	if err != nil {
		return nil, err
	}
	optional := func(v *string) (*string, error) {
		if v == nil {
			return nil, nil
		}
		return shopItemIdentifier(*v, true)
	}
	product, err := optional(input.ExternalProductID)
	if err != nil {
		return nil, err
	}
	variant, err := optional(input.ExternalVariantID)
	if err != nil {
		return nil, err
	}
	title := strings.TrimSpace(input.Title)
	if title == "" || utf8.RuneCountInString(title) > 200 || utf8.RuneCountInString(input.Remark) > 500 {
		return nil, errs.New("标题不能为空且最多200字，备注最多500字")
	}
	if input.Status != "0" && input.Status != "1" {
		return nil, errs.New("商品状态不合法")
	}
	return &model.ErpShopItem{ERPRecord: model.ERPRecord{Remark: input.Remark}, ShopID: input.ShopID, ShopItemCode: *code, ExternalProductID: product, ExternalVariantID: variant, Title: title, Status: input.Status, Version: input.Version}, nil
}
func ListERPShopItems(ctx context.Context, q model.ERPShopItemQuery, pg page.Query) ([]model.ERPShopItemView, int64, error) {
	return repository.SelectERPShopItemPage(ctx, q, pg)
}
func GetERPShopItem(ctx context.Context, id int64) (*model.ERPShopItemView, error) {
	if id < 1 {
		return nil, errs.New("商品编号不合法")
	}
	return repository.SelectERPShopItem(ctx, id)
}
func CreateERPShopItem(ctx context.Context, input model.ERPShopItemWrite, operator string) (*model.ErpShopItem, error) {
	target, err := prepareERPShopItem(input)
	if err != nil {
		return nil, err
	}
	target.Version = 1
	target.CreateBy = operator
	target.CreateTime = types.Now()
	if err = repository.InsertERPShopItem(ctx, target); err != nil {
		return nil, err
	}
	return target, nil
}
func EditERPShopItem(ctx context.Context, id int64, input model.ERPShopItemWrite, operator string) error {
	if id < 1 || input.Version < 1 {
		return errs.New("商品或版本不合法")
	}
	target, err := prepareERPShopItem(input)
	if err != nil {
		return err
	}
	target.ItemID = id
	target.UpdateBy = operator
	target.UpdateTime = types.Now()
	return repository.UpdateERPShopItem(ctx, target)
}
func SetERPShopItemMapping(ctx context.Context, id int64, input model.ERPMappingWrite, operator string) (*model.ERPMappingResult, error) {
	if id < 1 || input.Version < 1 || input.SkuID < 1 || utf8.RuneCountInString(input.Remark) > 500 {
		return nil, errs.New("配对参数不合法")
	}
	return repository.ChangeERPShopItemMapping(ctx, id, input.Version, input.SkuID, input.Remark, operator, false)
}
func CancelERPShopItemMapping(ctx context.Context, id, version int64, operator string) (*model.ERPMappingResult, error) {
	if id < 1 || version < 1 {
		return nil, errs.New("商品或版本不合法")
	}
	return repository.ChangeERPShopItemMapping(ctx, id, version, 0, "", operator, true)
}
