package service

import (
	"strconv"
	"strings"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
)

var contractConfigKeys = stringSet(
	ConfigKeyAccountChrtype,
	ConfigKeyInitPasswordModify,
	ConfigKeyPasswordValidateDays,
	ConfigKeyCaptchaEnabled,
	ConfigKeyRegisterUser,
	ConfigKeyInitPassword,
	ConfigKeyLoginBlackIPList,
)

var contractDictTypes = stringSet(
	"sys_job_group",
	"sys_job_status",
	"sys_common_status",
	"sys_oper_type",
	"sys_yes_no",
	"sys_normal_disable",
	"sys_show_hide",
	"sys_notice_status",
	"sys_notice_type",
	"sys_user_sex",
)

// These identifiers are referenced by the bundled Vue client, Go routes, or
// the retained Java-compatible menu tree. Add new statically referenced
// permissions here before exposing them to consumers.
var contractMenuPermissions = stringSet(
	"system:user:list", "system:user:query", "system:user:add", "system:user:edit",
	"system:user:remove", "system:user:export", "system:user:import", "system:user:resetPwd",
	"system:role:list", "system:role:query", "system:role:add", "system:role:edit",
	"system:role:remove", "system:role:export",
	"system:menu:list", "system:menu:query", "system:menu:add", "system:menu:edit", "system:menu:remove",
	"system:dept:list", "system:dept:query", "system:dept:add", "system:dept:edit", "system:dept:remove",
	"system:post:list", "system:post:query", "system:post:add", "system:post:edit",
	"system:post:remove", "system:post:export",
	"system:dict:list", "system:dict:query", "system:dict:add", "system:dict:edit",
	"system:dict:remove", "system:dict:export",
	"system:config:list", "system:config:query", "system:config:add", "system:config:edit",
	"system:config:remove", "system:config:export",
	"system:notice:list", "system:notice:query", "system:notice:add", "system:notice:edit", "system:notice:remove",
	"monitor:online:list", "monitor:online:query", "monitor:online:batchLogout", "monitor:online:forceLogout",
	"monitor:job:list", "monitor:job:query", "monitor:job:add", "monitor:job:edit",
	"monitor:job:remove", "monitor:job:changeStatus", "monitor:job:export",
	"monitor:operlog:list", "monitor:operlog:query", "monitor:operlog:remove", "monitor:operlog:export",
	"monitor:logininfor:list", "monitor:logininfor:query", "monitor:logininfor:remove",
	"monitor:logininfor:export", "monitor:logininfor:unlock",
	"monitor:druid:list", "monitor:server:list", "monitor:cache:list",
	"tool:build:list", "tool:gen:list", "tool:gen:query", "tool:gen:edit",
	"tool:gen:remove", "tool:gen:import", "tool:gen:preview", "tool:gen:code", "tool:swagger:list",
)

func stringSet(values ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func isContractConfigKey(key string) bool {
	_, ok := contractConfigKeys[key]
	return ok
}

func isContractDictType(dictType string) bool {
	_, ok := contractDictTypes[dictType]
	return ok
}

func containsContractMenuPermission(raw string) bool {
	for _, permission := range strings.Split(raw, ",") {
		if _, ok := contractMenuPermissions[strings.TrimSpace(permission)]; ok {
			return true
		}
	}
	return false
}

func validateConfigType(configType string) error {
	if configType != model.ConfigTypeBuiltin && configType != model.ConfigTypeCustom {
		return errs.New("系统内置只能是Y或N")
	}
	return nil
}

func validateContractConfigValue(key, value string) error {
	trimmed := strings.TrimSpace(value)
	switch key {
	case ConfigKeyCaptchaEnabled, ConfigKeyRegisterUser:
		if !strings.EqualFold(trimmed, "true") && !strings.EqualFold(trimmed, "false") {
			return errs.Newf("参数键名%s的值只能是true或false", key)
		}
	case ConfigKeyInitPasswordModify:
		if trimmed != "0" && trimmed != "1" {
			return errs.Newf("参数键名%s的值只能是0或1", key)
		}
	case ConfigKeyPasswordValidateDays:
		days, err := strconv.Atoi(trimmed)
		if err != nil || days < 0 || days >= 365 {
			return errs.Newf("参数键名%s的值必须是0到364之间的整数", key)
		}
	case ConfigKeyAccountChrtype:
		if len(trimmed) != 1 || trimmed[0] < '0' || trimmed[0] > '4' {
			return errs.Newf("参数键名%s的值只能是0到4", key)
		}
	case ConfigKeyInitPassword:
		if !validPasswordLength(value) {
			return errs.Newf("初始密码长度必须在%d到%d个字符之间", passwordMinLength, passwordMaxLength)
		}
	}
	return nil
}

func validateConfigCreate(config *model.SysConfig) error {
	if err := validateConfigType(config.ConfigType); err != nil {
		return err
	}
	if isContractConfigKey(config.ConfigKey) && config.ConfigType != model.ConfigTypeBuiltin {
		return errs.Newf("契约参数【%s】必须标记为系统内置", config.ConfigKey)
	}
	return validateContractConfigValue(config.ConfigKey, config.ConfigValue)
}

func validateConfigUpdate(existing, candidate *model.SysConfig) error {
	if err := validateConfigType(candidate.ConfigType); err != nil {
		return err
	}
	if isContractConfigKey(existing.ConfigKey) && candidate.ConfigType != model.ConfigTypeBuiltin {
		return errs.Newf("契约参数【%s】必须标记为系统内置", existing.ConfigKey)
	}
	if existing.ConfigType == model.ConfigTypeBuiltin && candidate.ConfigType != model.ConfigTypeBuiltin {
		return errs.Newf("内置参数【%s】不能改为非内置", existing.ConfigKey)
	}
	if existing.ConfigKey != candidate.ConfigKey &&
		(existing.ConfigType == model.ConfigTypeBuiltin || isContractConfigKey(existing.ConfigKey) ||
			isContractConfigKey(candidate.ConfigKey)) {
		return errs.Newf("契约参数键名【%s】不能改名", existing.ConfigKey)
	}
	return validateContractConfigValue(candidate.ConfigKey, candidate.ConfigValue)
}
