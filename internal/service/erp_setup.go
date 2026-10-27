package service

import (
	"context"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/tenantctx"
	"sort"
	"strings"
)

func setupOwner(ctx context.Context, u *model.LoginUser) error {
	if u == nil || u.User == nil || u.User.UserType != "01" || u.User.IsPlatform() || tenantctx.Get(ctx).Platform || tenantctx.ID(ctx) != u.User.TenantID || erpctx.UserID(ctx) != u.User.UserID {
		return errs.New("仅企业所有者可操作启用配置")
	}
	return nil
}
func ERPSetup(ctx context.Context, u *model.LoginUser) (*model.ERPSetup, error) {
	if e := setupOwner(ctx, u); e != nil {
		return nil, e
	}
	if !u.HasPermission("erp:setup:list") {
		return nil, errs.New("无启用检查权限")
	}
	allowed := map[string]bool{}
	for _, p := range u.Permissions {
		allowed[p] = true
	}
	return repository.SelectERPSetup(ctx, allowed)
}

type roleTemplateSpec struct {
	key, name, description string
	permissions            []string
}

func erpRoleTemplateSpecs() []roleTemplateSpec {
	manager := []string{}
	for p := range erpMenuPermissions {
		if !strings.HasSuffix(p, ":grant") && p != "erp:setup:list" {
			manager = append(manager, p)
		}
	}
	sort.Strings(manager)
	return []roleTemplateSpec{
		{"erp_manager", "业务管理员", "ERP业务读写；不包含平台、企业开通、员工管理或范围授权。", manager},
		{"erp_purchaser", "采购员", "供应商、采购价格、下单收货及原批退货；不包含付款记账。", strings.Fields("erp:overview:list erp:supplier:list erp:supplier:query erp:supplier:add erp:supplier:edit erp:purchase:list erp:purchase:query erp:purchase:add erp:purchase:edit erp:purchase:confirm erp:purchase:cancel erp:purchase:close erp:purchase:receive erp:purchaseReceipt:list erp:purchaseReceipt:query erp:purchaseReturnOrder:list erp:purchaseReturnOrder:query erp:purchaseReturnOrder:return erp:sku:list erp:warehouse:list")},
		{"erp_warehouse", "仓库员", "无价收货、拣货扫码复核后发货、库存查询及调拨；不包含采购价格、收件人、财务或手工直接发货。", strings.Fields("erp:overview:list erp:purchase:receive erp:purchaseReceipt:list erp:purchaseReceipt:query erp:stock:list erp:ledger:list erp:sku:list erp:warehouse:list erp:package:list erp:package:query erp:package:edit erp:picking:list erp:picking:query erp:picking:check erp:picking:ship erp:shipment:list erp:transfer:list erp:transfer:query erp:transfer:dispatch erp:transfer:receive")},
		{"erp_order", "订单员", "订单录入审核、配对及包裹发货；订单详情可读取收件资料。", strings.Fields("erp:overview:list erp:order:list erp:order:query erp:order:add erp:order:edit erp:order:review erp:order:cancel erp:order:import erp:importJob:list erp:importJob:query erp:shop:list erp:shopItem:list erp:shopItem:query erp:shopItem:add erp:shopItem:edit erp:shopItem:map erp:sku:list erp:warehouse:list erp:stock:list erp:package:list erp:package:query erp:package:edit erp:package:ship erp:shipment:list")},
		{"erp_finance", "财务员", "应收应付、抵减、收付款核销和冲销、对账；仅本系统记账。", strings.Fields("erp:overview:list erp:financeAccount:list erp:financeAccount:query erp:payable:list erp:payable:query erp:payable:add erp:payable:adjust erp:payment:list erp:payment:query erp:payment:add erp:payment:reverse erp:financeStatement:list erp:financeStatement:export erp:receivable:list erp:receivable:query erp:receivable:add erp:receivable:adjust erp:collection:list erp:collection:query erp:collection:add erp:collection:reverse erp:shopStatement:list erp:shopStatement:export erp:supplier:list erp:warehouse:list erp:shop:list")},
	}
}

func ERPRoleTemplates(ctx context.Context, u *model.LoginUser) ([]model.ERPRoleTemplate, error) {
	if e := setupOwner(ctx, u); e != nil {
		return nil, e
	}
	if !u.HasPermission("system:role:add") {
		return nil, errs.New("无新增角色权限")
	}
	menus, e := ListMenus(ctx, u.User, model.MenuQuery{Status: "0"})
	if e != nil {
		return nil, e
	}
	return buildERPRoleTemplates(menus), nil
}
func buildERPRoleTemplates(menus []model.SysMenu) []model.ERPRoleTemplate {
	byPermission := map[string]model.SysMenu{}
	byID := map[int64]model.SysMenu{}
	for _, m := range menus {
		if m.Status != "0" {
			continue
		}
		byID[m.MenuID] = m
		if m.Perms != nil {
			byPermission[*m.Perms] = m
		}
	}
	out := []model.ERPRoleTemplate{}
	for _, s := range erpRoleTemplateSpecs() {
		r := model.ERPRoleTemplate{TemplateKey: s.key, Name: s.name, Description: s.description, MenuIDs: []int64{}, Permissions: []model.ERPRoleTemplatePermission{}, UnavailablePermissions: []string{}}
		ids := map[int64]bool{}
		for _, p := range s.permissions {
			m, ok := byPermission[p]
			if !ok {
				r.UnavailablePermissions = append(r.UnavailablePermissions, p)
				continue
			}
			r.Permissions = append(r.Permissions, model.ERPRoleTemplatePermission{Permission: p, Label: m.MenuName, MenuID: m.MenuID})
			for id := m.MenuID; id > 0 && !ids[id]; {
				node, found := byID[id]
				if !found {
					break
				}
				ids[id] = true
				id = node.ParentID
			}
		}
		for id := range ids {
			r.MenuIDs = append(r.MenuIDs, id)
		}
		sort.Slice(r.MenuIDs, func(i, j int) bool { return r.MenuIDs[i] < r.MenuIDs[j] })
		out = append(out, r)
	}
	return out
}
