package model

// SessionIdentity is display metadata for the authenticated workspace, never an
// authorization input. Do not expose the tenant catalog or account credentials.
type SessionIdentity struct {
	TenantID      int64    `json:"tenantId"`
	TenantCode    string   `json:"tenantCode"`
	TenantName    string   `json:"tenantName"`
	UserName      string   `json:"userName"`
	RoleNames     []string `json:"roleNames"`
	PlatformAdmin bool     `json:"platformAdmin"`
	TenantAdmin   bool     `json:"tenantAdmin"`
}
