package model

// PlatformActor is constructed from the authenticated session, never JSON.
type PlatformActor struct {
	TenantID int64
	UserID   int64
	UserName string
	IP       string
}

type TenantMenuSettings struct {
	TenantID    int64        `json:"tenantId"`
	TenantCode  string       `json:"tenantCode"`
	TenantName  string       `json:"tenantName"`
	Version     int64        `json:"version"`
	Menus       []TreeSelect `json:"menus"`
	CheckedKeys []int64      `json:"checkedKeys"`
}
