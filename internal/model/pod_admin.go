package model

import (
	"encoding/json"
	"time"

	"ruoyi-go/pkg/types"
)

const PODAdminAccess = "podAdmin:access"
const PODAdminOverviewQuery = "podAdmin:overview:query"
const PODAdminAuditList = "podAdmin:audit:list"
const PODAdminAuditQuery = "podAdmin:audit:query"

// Only these capabilities may be advertised; later stages add their own checks.
var PODAdminPermissions = []string{
	PODAdminAccess, PODAdminOverviewQuery, PODAdminAuditList, PODAdminAuditQuery,
	"podAdmin:creator:list", "podAdmin:creator:query", "podAdmin:creator:add", "podAdmin:creator:edit", "podAdmin:creator:grant",
	"podAdmin:points:query", "podAdmin:points:adjust",
	"podAdmin:provider:list", "podAdmin:provider:query", "podAdmin:provider:edit", "podAdmin:provider:credential",
	"podAdmin:model:list", "podAdmin:model:query", "podAdmin:model:edit", "podAdmin:model:check",
	"podAdmin:task:list", "podAdmin:task:query", "podAdmin:task:cancel", "podAdmin:usage:query",
	"podAdmin:template:list", "podAdmin:template:query", "podAdmin:template:edit", "podAdmin:template:publish",
	"podAdmin:privacy:grant", "podAdmin:private:read",
}

type PODPlatformState struct {
	StateID             int64
	Revision            int64
	ManagedEnabled      bool
	DefaultTextModelID  *int64
	DefaultImageModelID *int64
	LegacyRunMaxID      int64
	LegacyBatchMaxID    int64
	CutoverTime         *time.Time
}

type PODAdminBootstrap struct {
	UserID         int64                `json:"userId,string"`
	UserName       string               `json:"userName"`
	NickName       string               `json:"nickName"`
	Permissions    []string             `json:"permissions"`
	SchemaVersion  string               `json:"schemaVersion"`
	Stage          int                  `json:"stage"`
	ManagedEnabled bool                 `json:"managedEnabled"`
	Capabilities   PODAdminCapabilities `json:"capabilities"`
	CatalogWrites  bool                 `json:"catalogWrites"`
}

type PODAdminCapabilities struct {
	Overview       bool `json:"overview"`
	Audits         bool `json:"audits"`
	Creators       bool `json:"creators"`
	Models         bool `json:"models"`
	Tasks          bool `json:"tasks"`
	Templates      bool `json:"templates"`
	Usage          bool `json:"usage"`
	PrivateContent bool `json:"privateContent"`
}

type PODAdminOverview struct {
	BeginDate      string             `json:"beginDate"`
	EndDate        string             `json:"endDate"`
	ManagedEnabled bool               `json:"managedEnabled"`
	Creators       int64              `json:"creators"`
	Projects       int64              `json:"projects"`
	Assets         int64              `json:"assets"`
	StorageBytes   int64              `json:"storageBytes,string"`
	Runs           int64              `json:"runs"`
	Batches        int64              `json:"batches"`
	ChargedMicros  int64              `json:"chargedMicros,string"`
	ReservedMicros int64              `json:"reservedMicros,string"`
	States         []PODAdminRunCount `gorm:"-" json:"states"`
}
type PODAdminRunCount struct {
	State string `json:"state"`
	Count int64  `json:"count"`
}

type PODAdminAudit struct {
	AuditID      int64 `gorm:"primaryKey;autoIncrement"`
	ActorUserID  int64
	OwnerUserID  int64
	ActionType   string
	ResourceType string
	ResourceID   int64
	Reason       string
	BeforeJSON   json.RawMessage
	AfterJSON    json.RawMessage
	TraceID      string
	Outcome      string
	CreateBy     string
	CreateTime   types.Time
	UpdateBy     string
	UpdateTime   types.Time
	Remark       string
}

func (PODAdminAudit) TableName() string { return "pod_admin_audit" }

// A typed allowlist prevents future database JSON fields leaking through DTOs.
type PODAdminAuditSnapshot struct {
	SchemaVersion  string                   `json:"schemaVersion,omitempty"`
	ManagedEnabled *bool                    `json:"managedEnabled,omitempty"`
	Catalog        *PODCatalogAuditSnapshot `json:"catalog,omitempty"`
	Creator        *PODCreatorAuditSnapshot `json:"creator,omitempty"`
	Operation      *PODOperationAudit       `json:"operation,omitempty"`
}
type PODAdminAuditView struct {
	AuditID      int64                  `json:"auditId,string"`
	ActorUserID  int64                  `json:"actorUserId,string"`
	OwnerUserID  int64                  `json:"ownerUserId,string"`
	ActionType   string                 `json:"actionType"`
	ResourceType string                 `json:"resourceType"`
	ResourceID   int64                  `json:"resourceId,string"`
	Reason       string                 `json:"reason"`
	TraceID      string                 `json:"traceId"`
	Outcome      string                 `json:"outcome"`
	CreateTime   types.Time             `json:"createTime"`
	Before       *PODAdminAuditSnapshot `json:"before,omitempty"`
	After        *PODAdminAuditSnapshot `json:"after,omitempty"`
}
type PODAdminAuditFilter struct {
	ActorUserID int64
	OwnerUserID int64
	ActionType  string
	Outcome     string
	Begin       time.Time
	End         time.Time
}
