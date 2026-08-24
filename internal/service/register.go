package service

import (
	"context"
	"strings"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/types"
)

// ConfigKeyRegisterUser sys_config 里控制是否开放注册的键。
const ConfigKeyRegisterUser = "sys.account.registerUser"

// 账号与密码长度限制，对齐 Java 版 UserConstants。
const (
	userNameMinLength = 2
	userNameMaxLength = 20
	passwordMinLength = 5
	passwordMaxLength = 20
)

// Register 用户自助注册。
//
// 注册出来的账号没有部门、没有角色 —— 与 Java 一致。
// 管理员需要在用户管理里补齐，否则该账号登录后看不到任何菜单。
func Register(ctx context.Context, body model.RegisterBody, ip, userAgent string) error {
	enabled, err := GetConfigValueByKey(ctx, ConfigKeyRegisterUser)
	if err != nil {
		return err
	}
	if !strings.EqualFold(enabled, "true") {
		return errs.New("当前系统没有开启注册功能！")
	}

	captchaOn, err := CaptchaEnabled(ctx)
	if err != nil {
		return err
	}
	if captchaOn {
		if err := VerifyCaptcha(ctx, body.UUID, body.Code); err != nil {
			return err
		}
	}

	username := strings.TrimSpace(body.Username)
	switch {
	case username == "":
		return errs.New("用户名不能为空")
	case body.Password == "":
		return errs.New("用户密码不能为空")
	case len([]rune(username)) < userNameMinLength || len([]rune(username)) > userNameMaxLength:
		return errs.Newf("账户长度必须在%d到%d个字符之间", userNameMinLength, userNameMaxLength)
	case len(body.Password) < passwordMinLength || len(body.Password) > passwordMaxLength:
		return errs.Newf("密码长度必须在%d到%d个字符之间", passwordMinLength, passwordMaxLength)
	}

	count, err := repository.CountUserByName(ctx, username, 0)
	if err != nil {
		return err
	}
	if count > 0 {
		return errs.Newf("保存用户'%s'失败，注册账号已存在", username)
	}

	hashed, err := HashPassword(body.Password)
	if err != nil {
		return err
	}

	user := &model.SysUser{
		UserName: username,
		NickName: username,
		Password: hashed,
		UserType: "00",
		Status:   model.StatusNormal,
		DelFlag:  model.DelFlagExist,
		CreateBy: username,
		// 注册账号不设 PwdUpdateDate，登录后会提示修改初始密码
		CreateTime: types.Now(),
	}
	if err := repository.InsertUser(ctx, user); err != nil {
		return errs.New("注册失败,请联系系统管理人员")
	}

	browser, os := parseUserAgent(userAgent)
	RecordLogininfor(ctx, username, ip, browser, os, model.LoginStatusSuccess, "注册成功")
	return nil
}
