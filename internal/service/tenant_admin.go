package service

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/redisx"
	"ruoyi-go/pkg/tenantctx"
)

const maxTenantMenuNodes = 2000

type TenantMenusInput struct {
	Version           int64    `json:"version" binding:"gt=0"`
	ConfirmTenantCode string   `json:"confirmTenantCode" binding:"required,max=32"`
	MenuIDs           *[]int64 `json:"menuIds"`
	Reason            string   `json:"reason" binding:"notblank,xss,max=200"`
}

type TenantOwnerPasswordInput struct {
	Version           int64  `json:"version" binding:"gt=0"`
	OwnerUserID       int64  `json:"ownerUserId" binding:"gt=0"`
	ConfirmTenantCode string `json:"confirmTenantCode" binding:"required,max=32"`
	NewPassword       string `json:"newPassword" binding:"required"`
	ConfirmPassword   string `json:"confirmPassword" binding:"required"`
	OperatorPassword  string `json:"operatorPassword" binding:"required"`
	Reason            string `json:"reason" binding:"notblank,xss,max=200"`
}

func platformActor(ctx context.Context, operator *model.SysUser, ip string) (model.PlatformActor, error) {
	s := tenantctx.Get(ctx)
	if operator == nil || !operator.IsPlatform() || !s.Platform || operator.TenantID != s.ID || operator.UserID < 1 {
		return model.PlatformActor{}, errs.New("仅平台管理员可操作")
	}
	return model.PlatformActor{TenantID: s.ID, UserID: operator.UserID, UserName: operator.UserName, IP: ip}, nil
}

func TenantMenus(ctx context.Context, operator *model.SysUser, id int64) (model.TenantMenuSettings, error) {
	if _, err := platformActor(ctx, operator, ""); err != nil {
		return model.TenantMenuSettings{}, err
	}
	tenant, catalog, checked, err := repository.CustomerTenantMenuState(ctx, id, maxTenantMenuNodes)
	if err != nil {
		return model.TenantMenuSettings{}, err
	}
	catalog = reachableCustomerMenus(catalog)
	visible := map[int64]bool{}
	for _, menu := range catalog {
		visible[menu.MenuID] = true
	}
	keys := []int64{}
	for _, key := range checked {
		if visible[key] {
			keys = append(keys, key)
		}
	}
	return model.TenantMenuSettings{TenantID: id, TenantCode: tenant.TenantCode, TenantName: tenant.TenantName, Version: tenant.Version,
		Menus: BuildMenuTreeSelect(catalog), CheckedKeys: keys}, nil
}

func SaveTenantMenus(ctx context.Context, operator *model.SysUser, id int64, body TenantMenusInput, ip string) error {
	actor, err := platformActor(ctx, operator, ip)
	if err != nil {
		return err
	}
	if id < 1 || body.Version < 1 || body.MenuIDs == nil || strings.TrimSpace(body.Reason) == "" || utf8.RuneCountInString(body.Reason) > 200 {
		return errs.New("请完整提交版本、菜单选择及操作原因；清空授权须明确提交空数组")
	}
	// Same order as user/role/menu writers. The permission lock also fences login
	// snapshots while the persistent tenant version and role grants are replaced.
	userWriteMu.Lock()
	defer userWriteMu.Unlock()
	roleWriteMu.Lock()
	defer roleWriteMu.Unlock()
	menuWriteMu.Lock()
	defer menuWriteMu.Unlock()
	permissionRefreshMu.Lock()
	defer permissionRefreshMu.Unlock()
	_, catalog, _, err := repository.CustomerTenantMenuState(ctx, id, maxTenantMenuNodes)
	if err != nil {
		return err
	}
	ids, err := normalizeTenantMenus(*body.MenuIDs, reachableCustomerMenus(catalog))
	if err != nil {
		return err
	}
	return repository.ReplaceCustomerTenantMenus(ctx, id, body.Version, body.ConfirmTenantCode, ids, strings.TrimSpace(body.Reason), actor)
}

func ResetTenantOwnerPassword(ctx context.Context, operator *model.SysUser, id int64, body TenantOwnerPasswordInput, ip string) (bool, error) {
	actor, err := platformActor(ctx, operator, ip)
	if err != nil {
		return false, err
	}
	if err = validateTenantOwnerPassword(body); err != nil {
		return false, err
	}
	userWriteMu.Lock()
	defer userWriteMu.Unlock()
	if err = verifyPlatformPassword(ctx, actor.UserID, body.OperatorPassword); err != nil {
		return false, err
	}
	tenant, err := repository.TenantByID(ctx, id)
	if err != nil {
		return false, err
	}
	if tenant == nil || tenant.TenantKind != "01" || tenant.OwnerUserID == nil || *tenant.OwnerUserID != body.OwnerUserID {
		return false, errs.New("企业不存在或管理员已变更，请刷新后重试")
	}
	ownerCtx := tenantctx.With(ctx, id, false)
	owner, err := repository.SelectUserByID(ownerCtx, body.OwnerUserID)
	if err != nil {
		return false, err
	}
	if owner == nil {
		return false, errs.New("企业管理员不存在")
	}
	if bcrypt.CompareHashAndPassword([]byte(owner.Password), []byte(body.NewPassword)) == nil {
		return false, errs.New("新密码不能与原密码相同")
	}
	hash, err := HashPassword(body.NewPassword)
	if err != nil {
		return false, err
	}
	permissionRefreshMu.Lock()
	err = repository.ResetCustomerOwnerPassword(ctx, id, body.Version, body.OwnerUserID, body.ConfirmTenantCode, hash, strings.TrimSpace(body.Reason), actor)
	permissionRefreshMu.Unlock()
	if err != nil {
		return false, err
	}
	// The DB version already rejects every old tenant session. Counter cleanup is
	// post-commit convenience, not a prerequisite for the security fence.
	if err = redisx.C().Del(ctx, redisx.TenantPasswordKey(ownerCtx, owner.UserName)).Err(); err != nil {
		slog.WarnContext(ctx, "企业管理员密码已重置，登录失败计数清理未完成", "tenantId", id, "ownerUserId", owner.UserID)
		return false, nil
	}
	return true, nil
}

func verifyPlatformPassword(ctx context.Context, id int64, password string) error {
	user, err := repository.SelectUserByID(ctx, id)
	if err != nil {
		return err
	}
	if user == nil || !user.IsPlatform() || user.Status != model.StatusNormal {
		return errs.New("平台管理员身份已失效")
	}
	key := redisx.TenantPasswordKey(ctx, user.UserName)
	count, err := currentRetryCount(ctx, key)
	if err != nil {
		return err
	}
	if count >= maxPasswordRetry {
		return passwordLockedError()
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) != nil {
		return recordPasswordFailure(ctx, key)
	}
	count, err = clearPasswordFailuresOnSuccess(ctx, key)
	if err != nil {
		return err
	}
	if count >= maxPasswordRetry {
		return passwordLockedError()
	}
	return nil
}

func validateTenantOwnerPassword(body TenantOwnerPasswordInput) error {
	length := utf8.RuneCountInString(body.NewPassword)
	if body.Version < 1 || body.OwnerUserID < 1 || body.ConfirmTenantCode == "" || strings.TrimSpace(body.Reason) == "" || utf8.RuneCountInString(body.Reason) > 200 || body.OperatorPassword == "" {
		return errs.New("请完整填写企业确认、平台密码、操作原因及版本信息")
	}
	if length < 8 || length > 20 {
		return errs.New("新密码长度必须在8到20个字符之间")
	}
	if body.NewPassword != body.ConfirmPassword {
		return errs.New("两次输入的新密码不一致")
	}
	return nil
}

func reachableCustomerMenus(catalog []model.SysMenu) []model.SysMenu {
	byID := map[int64]model.SysMenu{}
	for _, menu := range catalog {
		byID[menu.MenuID] = menu
	}
	result := []model.SysMenu{}
	for _, menu := range catalog {
		id, seen := menu.MenuID, map[int64]bool{}
		for depth := 0; depth < 64 && id != 0; depth++ {
			node, ok := byID[id]
			if !ok || seen[id] {
				break
			}
			seen[id] = true
			id = node.ParentID
		}
		if id == 0 {
			result = append(result, menu)
		}
	}
	return result
}

func normalizeTenantMenus(requested []int64, catalog []model.SysMenu) ([]int64, error) {
	if len(requested) > maxTenantMenuNodes {
		return nil, errs.New("一次最多选择2000个功能节点")
	}
	byID, selected := map[int64]model.SysMenu{}, map[int64]bool{}
	for _, menu := range catalog {
		byID[menu.MenuID] = menu
	}
	for _, id := range requested {
		if id < 1 {
			return nil, errs.New("菜单ID无效")
		}
		path := map[int64]bool{}
		for depth := 0; id != 0; depth++ {
			menu, ok := byID[id]
			if !ok || path[id] || depth >= 64 {
				return nil, errs.New("包含不可授权功能或异常菜单层级，请刷新后重试")
			}
			path[id], selected[id] = true, true
			id = menu.ParentID
		}
	}
	if len(selected) > maxTenantMenuNodes {
		return nil, errs.New("补齐上级后超过2000个节点上限")
	}
	ids := make([]int64, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}
