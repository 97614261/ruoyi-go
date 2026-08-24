package service

import (
	"context"
	"strconv"
	"time"

	"ruoyi-go/pkg/types"
)

// sys_config 中与账号策略相关的参数键。
const (
	ConfigKeyAccountChrtype       = "sys.account.chrtype"
	ConfigKeyInitPasswordModify   = "sys.account.initPasswordModify"
	ConfigKeyPasswordValidateDays = "sys.account.passwordValidateDays"
)

// PasswordChrtype 密码字符规则，参数缺失时返回 "0"。
func PasswordChrtype(ctx context.Context) (string, error) {
	value, err := GetConfigValueByKey(ctx, ConfigKeyAccountChrtype)
	if err != nil {
		return "0", err
	}
	if value == "" {
		return "0", nil
	}
	return value, nil
}

// IsDefaultModifyPwd 是否需要提醒修改初始密码。
//
// 对应 Java 版 SysLoginController.initPasswordIsModify：
// 只有开关为 1 且用户从未改过密码（pwd_update_date 为空）时才提醒。
func IsDefaultModifyPwd(ctx context.Context, pwdUpdateDate types.Time) (bool, error) {
	value, err := GetConfigValueByKey(ctx, ConfigKeyInitPasswordModify)
	if err != nil {
		return false, err
	}
	modify, convErr := strconv.Atoi(value)
	if convErr != nil {
		return false, nil
	}
	return modify == 1 && pwdUpdateDate.IsZero(), nil
}

// IsPasswordExpired 密码是否过期。
//
// 对应 Java 版 passwordIsExpiration：有效天数 <= 0 表示不限制；
// 从未修改过初始密码的直接算过期。
func IsPasswordExpired(ctx context.Context, pwdUpdateDate types.Time) (bool, error) {
	value, err := GetConfigValueByKey(ctx, ConfigKeyPasswordValidateDays)
	if err != nil {
		return false, err
	}
	days, convErr := strconv.Atoi(value)
	if convErr != nil || days <= 0 {
		return false, nil
	}
	if pwdUpdateDate.IsZero() {
		return true, nil
	}
	elapsed := int(time.Since(pwdUpdateDate.Std()).Hours() / 24)
	return elapsed > days, nil
}
