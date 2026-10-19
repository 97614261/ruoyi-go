package service

import (
	"context"
	"golang.org/x/crypto/bcrypt"
	"regexp"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
	"strings"
	"time"
)

var tenantCodePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{2,31}$`)
var tenantOwnerNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.@-]{2,20}$`)

type TenantInput struct {
	Version     int64      `json:"version"`
	TenantID    int64      `json:"tenantId"`
	OwnerUserID int64      `json:"ownerUserId"`
	TenantCode  string     `json:"tenantCode" binding:"required"`
	TenantName  string     `json:"tenantName" binding:"notblank,xss,max=100"`
	UserName    string     `json:"userName"`
	Password    string     `json:"password"`
	Status      string     `json:"status" binding:"oneof=0 1"`
	ExpireTime  types.Time `json:"expireTime"`
}

func ValidateTenantState(t *model.SysTenant) error {
	if t == nil || t.Status != "0" || (!t.ExpireTime.IsZero() && !t.ExpireTime.Std().After(time.Now())) {
		return errs.New("企业不存在、已停用或已到期")
	}
	return nil
}
func LoginTenant(ctx context.Context, code string) (context.Context, *model.SysTenant, error) {
	code = strings.ToLower(strings.TrimSpace(code))
	if !tenantCodePattern.MatchString(code) {
		return ctx, nil, errs.New("请输入有效的企业编码")
	}
	t, err := repository.ResolveTenant(ctx, code)
	if err != nil {
		return ctx, nil, err
	}
	if err = ValidateTenantState(t); err != nil {
		return ctx, nil, err
	}
	return tenantctx.With(ctx, t.TenantID, t.TenantKind == "00"), t, nil
}
func ValidateSessionTenant(ctx context.Context, l *model.LoginUser) (context.Context, error) {
	if l == nil || l.User == nil || l.TenantID < 1 || l.User.TenantID != l.TenantID {
		return ctx, errs.New("租户会话无效，请重新登录")
	}
	t, err := repository.TenantByID(ctx, l.TenantID)
	if err != nil {
		return ctx, err
	}
	if err = ValidateTenantState(t); err != nil {
		return ctx, err
	}
	if l.TenantVersion != t.Version || l.User.IsPlatform() != (t.TenantKind == "00") {
		return ctx, errs.New("企业状态已变更，请重新登录")
	}
	return tenantctx.With(ctx, t.TenantID, l.User.IsPlatform()), nil
}
func ListTenants(ctx context.Context, name string, pg page.Query) ([]model.SysTenant, int64, error) {
	if !tenantctx.Get(ctx).Platform {
		return nil, 0, errs.New("仅平台管理员可管理租户")
	}
	return repository.TenantPage(ctx, name, pg)
}
func SaveTenant(ctx context.Context, in TenantInput, operator *model.SysUser, ip string) error {
	if !tenantctx.Get(ctx).Platform {
		return errs.New("仅平台管理员可管理租户")
	}
	actor, actorErr := platformActor(ctx, operator, ip)
	if actorErr != nil {
		return actorErr
	}
	if in.TenantID > 0 {
		if in.Version < 1 {
			return errs.New("修改企业必须提交当前版本")
		}
		userWriteMu.Lock()
		defer userWriteMu.Unlock()
		roleWriteMu.Lock()
		defer roleWriteMu.Unlock()
		return repository.UpdateTenant(ctx, in.TenantID, in.TenantName, in.Status, in.ExpireTime, in.OwnerUserID, actor, in.Version)
	}
	if !tenantCodePattern.MatchString(in.TenantCode) || in.TenantCode == "platform" {
		return errs.New("企业编码需为3到32位小写字母、数字或连字符，且不能使用保留编码")
	}
	if !tenantOwnerNamePattern.MatchString(in.UserName) || len(in.Password) < 8 || len(in.Password) > 20 {
		return errs.New("管理员账号需2到20位字母、数字或_.@-，初始密码需8到20位")
	}
	existing, err := repository.ResolveTenant(ctx, in.TenantCode)
	if err != nil {
		return err
	}
	if existing != nil {
		return errs.New("企业编码已存在")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	t := model.SysTenant{TenantCode: in.TenantCode, TenantName: in.TenantName, Status: in.Status, ExpireTime: in.ExpireTime, CreateBy: actor.UserName}
	return repository.ProvisionTenant(ctx, &t, in.UserName, string(hash))
}
