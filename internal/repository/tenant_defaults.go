package repository

import "strings"

// Versioned public onboarding defaults, not arbitrary platform configuration.
// Adding a secret platform setting must never silently copy it to new customers.
const onboardingConfigKeys = `'sys.index.skinName','sys.index.sideTheme','sys.account.chrtype','sys.account.initPasswordModify','sys.account.passwordValidateDays','sys.account.captchaEnabled','sys.account.registerUser','sys.user.initPassword','sys.login.blackIPList'`
const onboardingDictTypes = `'sys_user_sex','sys_show_hide','sys_normal_disable','sys_job_status','sys_job_group','sys_yes_no','sys_notice_type','sys_notice_status','sys_oper_type','sys_common_status'`

// New platform-only permissions must use platform:*. Legacy platform tools are
// explicitly excluded too, even if an old grant record has not yet been cleaned.
func customerMenuCondition(alias string) string {
	p := ""
	if alias != "" {
		p = alias + "."
	}
	clauses := []string{}
	for _, prefix := range []string{"platform:%", "tool:%", "system:menu:%", "monitor:cache:%", "monitor:server:%", "monitor:online:%"} {
		clauses = append(clauses, "COALESCE("+p+"perms,'') NOT LIKE '"+prefix+"'")
	}
	clauses = append(clauses, p+"path NOT IN('platform','tenant','tool','gen','menu','online','cache','cacheList','server','druid','swagger')")
	return strings.Join(clauses, " AND ")
}
