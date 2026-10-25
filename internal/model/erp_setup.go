package model

import "ruoyi-go/pkg/types"

// Fixed diagnostic checklist, not an unbounded business list or authorization.
type ERPSetupCheck struct {
	Key   string `json:"key"`
	State string `json:"state"`
}
type ERPSetup struct {
	CheckedAt types.Time      `json:"checkedAt"`
	Checks    []ERPSetupCheck `json:"checks"`
}
type ERPRoleTemplatePermission struct {
	Permission string `json:"permission"`
	Label      string `json:"label"`
	MenuID     int64  `json:"menuId"`
}
type ERPRoleTemplate struct {
	TemplateKey            string                      `json:"templateKey"`
	Name                   string                      `json:"name"`
	Description            string                      `json:"description"`
	MenuIDs                []int64                     `json:"menuIds"`
	Permissions            []ERPRoleTemplatePermission `json:"permissions"`
	UnavailablePermissions []string                    `json:"unavailablePermissions"`
}
