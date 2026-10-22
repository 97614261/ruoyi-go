package service

import (
	"context"
	"regexp"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
	"sort"
	"strings"
	"unicode/utf8"
)

// This directory labels manually maintained shops, NOT working API integrations.
func erpChannels() []model.ERPChannel {
	result := []model.ERPChannel{}
	for _, v := range [][2]string{{"MANUAL", "手工/其他"}, {"AMAZON", "Amazon"}, {"SHOPEE", "Shopee"}, {"LAZADA", "Lazada"}, {"TIKTOK", "TikTok Shop"}, {"ALIEXPRESS", "AliExpress"}} {
		result = append(result, model.ERPChannel{ChannelCode: v[0], ChannelName: v[1], Sources: []string{"MANUAL", "IMPORT"}, Description: "仅登记店铺资料；未接入平台API，无自动授权、拉单或回传"})
	}
	return result
}
func ListERPChannels(ctx context.Context, pg page.Query) ([]model.ERPChannel, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	all := erpChannels()
	start := pg.Offset()
	if start >= len(all) {
		return []model.ERPChannel{}, int64(len(all)), nil
	}
	size := pg.PageSize
	if size < 1 {
		size = page.DefaultPageSize
	}
	if size > page.MaxPageSize {
		size = page.MaxPageSize
	}
	end := start + size
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], int64(len(all)), nil
}
func ListErpShopPage(ctx context.Context, query model.ErpShopQuery, pg page.Query) ([]model.ErpShop, int64, error) {
	return repository.SelectErpShopPage(ctx, query, pg)
}
func GetErpShop(ctx context.Context, id int64) (*model.ErpShop, error) {
	if id < 1 {
		return nil, errs.New("店铺编号不合法")
	}
	target, err := repository.SelectErpShopByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, errs.New("店铺不存在或无店铺权限")
	}
	return target, nil
}

var erpShopCodePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
var erpSitePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{0,31}$`)
var erpCurrencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

func prepareERPShop(input model.ERPShopWrite) (*model.ErpShop, error) {
	input.ShopCode = strings.TrimSpace(input.ShopCode)
	input.ShopName = strings.TrimSpace(input.ShopName)
	input.ChannelCode = strings.ToUpper(strings.TrimSpace(input.ChannelCode))
	input.SiteCode = strings.ToUpper(strings.TrimSpace(input.SiteCode))
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	if !erpShopCodePattern.MatchString(input.ShopCode) {
		return nil, errs.New("店铺编码需为1—64位字母数字及._-，并以字母数字开头")
	}
	if input.ShopName == "" || utf8.RuneCountInString(input.ShopName) > 120 || utf8.RuneCountInString(input.Remark) > 500 {
		return nil, errs.New("店铺名称不能为空且最多120字，备注最多500字")
	}
	found := false
	for _, channel := range erpChannels() {
		if channel.ChannelCode == input.ChannelCode {
			found = true
			break
		}
	}
	if !found {
		return nil, errs.New("渠道不在当前只读目录中")
	}
	if !erpSitePattern.MatchString(input.SiteCode) {
		return nil, errs.New("站点编码需为1—32位字母数字及_-，并以字母数字开头")
	}
	if !erpCurrencyPattern.MatchString(input.Currency) {
		return nil, errs.New("币种需为3位英文字母；当前不提供换汇或平台币种校验")
	}
	if input.Status != "0" && input.Status != "1" {
		return nil, errs.New("店铺状态不合法")
	}
	return &model.ErpShop{ShopCode: input.ShopCode, ShopName: input.ShopName, ChannelCode: input.ChannelCode, SiteCode: input.SiteCode, Currency: input.Currency, Status: input.Status, Remark: input.Remark, Version: input.Version}, nil
}
func CreateErpShop(ctx context.Context, input model.ERPShopWrite, operator string) (*model.ErpShop, error) {
	target, err := prepareERPShop(input)
	if err != nil {
		return nil, err
	}
	target.Version = 1
	target.CreateBy = operator
	target.CreateTime = types.Now()
	if err = repository.InsertErpShop(ctx, target); err != nil {
		return nil, err
	}
	return target, nil
}
func UpdateErpShop(ctx context.Context, id int64, input model.ERPShopWrite, operator string) error {
	if id < 1 || input.Version < 1 {
		return errs.New("店铺或版本不合法，请刷新后重试")
	}
	target, err := prepareERPShop(input)
	if err != nil {
		return err
	}
	target.ShopId = id
	target.UpdateBy = operator
	target.UpdateTime = types.Now()
	return repository.UpdateErpShop(ctx, target)
}
func GetERPShopAccess(ctx context.Context, userID int64) (*model.ERPShopAccessView, error) {
	if userID < 1 {
		return nil, errs.New("用户不合法")
	}
	return repository.GetERPShopAccess(ctx, userID)
}
func SetERPShopAccess(ctx context.Context, userID, version int64, ids []int64, operator string) error {
	if userID < 1 || version < 1 || ids == nil || len(ids) > 200 {
		return errs.New("授权参数不合法，shopIds必须显式提供，空数组表示撤回")
	}
	ids = append([]int64{}, ids...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for i, id := range ids {
		if id < 1 || (i > 0 && id == ids[i-1]) {
			return errs.New("店铺编号不合法或重复")
		}
	}
	return repository.ReplaceERPShopAccess(ctx, userID, version, ids, operator)
}
