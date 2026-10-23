package service

import (
	"context"
	"net/mail"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
	"strings"
	"unicode"
	"unicode/utf8"
)

func prepareSupplier(in model.ERPSupplierWrite) (*model.ErpSupplier, error) {
	code, name := strings.TrimSpace(in.SupplierCode), strings.TrimSpace(in.SupplierName)
	if !erpShopCodePattern.MatchString(code) {
		return nil, errs.New("供应商编码需为1—64位字母数字及._-，并以字母数字开头")
	}
	if name == "" {
		return nil, errs.New("供应商名称不能为空")
	}
	values := []struct {
		value     string
		max       int
		multiline bool
	}{{name, 120, false}, {in.ContactName, 64, false}, {in.Phone, 64, false}, {in.Email, 128, false}, {in.Address, 500, true}, {in.Remark, 500, true}}
	for _, v := range values {
		if utf8.RuneCountInString(v.value) > v.max || !utf8.ValidString(v.value) {
			return nil, errs.New("供应商资料长度或编码不合法")
		}
		for _, r := range v.value {
			if unicode.IsControl(r) && !(v.multiline && (r == '\r' || r == '\n' || r == '\t')) {
				return nil, errs.New("供应商资料包含不允许的控制字符")
			}
		}
	}
	email := strings.TrimSpace(in.Email)
	if email != "" {
		address, err := mail.ParseAddress(email)
		if err != nil || address.Address != email {
			return nil, errs.New("供应商邮箱格式不正确")
		}
	}
	// Supplier enable/disable is a domain switch, not an editable dict enum.
	if in.Status != "0" && in.Status != "1" {
		return nil, errs.New("供应商状态仅支持启用或停用")
	}
	return &model.ErpSupplier{SupplierCode: code, SupplierName: name, ContactName: strings.TrimSpace(in.ContactName), Phone: strings.TrimSpace(in.Phone), Email: email, Address: strings.TrimSpace(in.Address), Status: in.Status, Remark: in.Remark}, nil
}
func ListErpSupplierPage(ctx context.Context, in model.ErpSupplierQuery, pg page.Query) ([]model.ErpSupplierListRow, int64, error) {
	if in.Status != "" && in.Status != "0" && in.Status != "1" {
		return nil, 0, errs.New("供应商查询状态不合法")
	}
	return repository.SelectErpSupplierPage(ctx, in, pg)
}
func GetErpSupplier(ctx context.Context, id int64) (*model.ErpSupplier, error) {
	if id < 1 {
		return nil, errs.New("供应商编号不合法")
	}
	out, err := repository.SelectErpSupplierByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, errs.New("供应商不存在或不属于当前企业")
	}
	return out, nil
}
func CreateErpSupplier(ctx context.Context, in model.ERPSupplierWrite, operator string) (*model.ERPSupplierResult, error) {
	target, err := prepareSupplier(in)
	if err != nil {
		return nil, err
	}
	target.Version = 1
	target.CreateBy = operator
	target.CreateTime = types.Now()
	if err = repository.InsertErpSupplier(ctx, target, operator); err != nil {
		return nil, err
	}
	return &model.ERPSupplierResult{SupplierId: target.SupplierId, Version: target.Version}, nil
}
func UpdateErpSupplier(ctx context.Context, id int64, in model.ERPSupplierEdit, operator string) (*model.ERPSupplierResult, error) {
	if id < 1 || in.Version < 1 {
		return nil, errs.New("供应商编号或版本不合法")
	}
	target, err := prepareSupplier(in.ERPSupplierWrite)
	if err != nil {
		return nil, err
	}
	target.SupplierId = id
	target.Version = in.Version
	target.UpdateBy = operator
	target.UpdateTime = types.Now()
	if err = repository.UpdateErpSupplier(ctx, target, operator); err != nil {
		return nil, err
	}
	return &model.ERPSupplierResult{SupplierId: id, Version: target.Version}, nil
}
