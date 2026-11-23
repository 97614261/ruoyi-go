package model

import (
	"ruoyi-go/pkg/types"
	"time"
)

type PODTaskFilter struct {
	Begin, End      time.Time
	UserID, ModelID int64
	State           string
}
type PODTaskView struct {
	RunID         int64      `json:"runId,string"`
	UserID        int64      `json:"userId,string"`
	ProjectID     int64      `json:"projectId,string"`
	ExecutionMode string     `json:"executionMode"`
	Mode          string     `json:"mode"`
	State         string     `json:"state"`
	Attempt       int        `json:"attempt"`
	CreateTime    types.Time `json:"createTime"`
	UpdateTime    types.Time `json:"updateTime"`
}
type PODTaskNodeView struct {
	NodeID         string `json:"nodeId"`
	NodeType       string `json:"nodeType"`
	State          string `json:"state"`
	Attempt        int    `json:"attempt"`
	ModelID        int64  `json:"modelId,string"`
	ModelVersionID int64  `json:"modelVersionId,string"`
	ErrorCode      string `json:"errorCode"`
}
type PODAdminBatchView struct {
	BatchID       int64      `json:"batchId,string"`
	UserID        int64      `json:"userId,string"`
	State         string     `json:"state"`
	ExecutionMode string     `json:"executionMode"`
	CreateTime    types.Time `json:"createTime"`
}
type PODAdminBatchItem struct {
	ItemNo    int    `json:"itemNo"`
	State     string `json:"state"`
	RunID     int64  `json:"runId,string"`
	ErrorCode string `json:"errorCode"`
}
type PODUsageGroup struct {
	UserID         int64  `json:"userId,string"`
	ModelID        int64  `json:"modelId,string"`
	Model          string `json:"model"`
	Calls          int64  `json:"calls"`
	ChargedMicros  int64  `json:"chargedMicros,string"`
	ReservedMicros int64  `json:"reservedMicros,string"`
	PointsSpent    int64  `json:"pointsSpent,string"`
	PointsHeld     int64  `json:"pointsHeld,string"`
}
type PODOperationWrite struct {
	RequestID      string `json:"requestId"`
	Reason         string `json:"reason"`
	Revision       string `json:"revision,omitempty"`
	OperatorUserID string `json:"operatorUserId,omitempty"`
	OwnerUserID    string `json:"ownerUserId,omitempty"`
	ResourceType   string `json:"resourceType,omitempty"`
	ResourceID     string `json:"resourceId,omitempty"`
	ExpiresAt      string `json:"expiresAt,omitempty"`
}
type PODPrivateGrant struct {
	GrantID        int64      `gorm:"primaryKey;autoIncrement" json:"grantId,string"`
	OperatorUserID int64      `json:"operatorUserId,string"`
	OwnerUserID    int64      `json:"ownerUserId,string"`
	ResourceType   string     `json:"resourceType"`
	ResourceID     int64      `json:"resourceId,string"`
	Purpose        string     `json:"purpose"`
	ExpiresAt      time.Time  `json:"expiresAt"`
	Revoked        bool       `json:"revoked"`
	Revision       int64      `json:"revision,string"`
	CreateBy       string     `json:"-"`
	CreateTime     types.Time `json:"createTime"`
	UpdateBy       string     `json:"-"`
	UpdateTime     types.Time `json:"-"`
	Remark         string     `json:"-"`
}

func (PODPrivateGrant) TableName() string { return "pod_private_access_grant" }

type PODPrivateRun struct {
	RunID  int64            `json:"runId,string"`
	UserID int64            `json:"userId,string"`
	Graph  *PODGraph        `json:"graph"`
	Nodes  []PODPrivateNode `json:"nodes"`
}
type PODPrivateNode struct {
	NodeID string       `json:"nodeId"`
	Output PODRunOutput `json:"output"`
}
type PODOperationAudit struct {
	State          string     `json:"state,omitempty"`
	Revision       int64      `json:"revision,string,omitempty"`
	OperatorUserID int64      `json:"operatorUserId,string,omitempty"`
	ResourceType   string     `json:"resourceType,omitempty"`
	ResourceID     int64      `json:"resourceId,string,omitempty"`
	ExpiresAt      *time.Time `json:"expiresAt,omitempty"`
	Version        int64      `json:"version,string,omitempty"`
}

// Kept server-side; never return raw stored request or provider receipt JSON.
type PODOperationMutation struct {
	ActorID, ID, Revision       int64
	Op, Permission, Hash, Trace string
	Request                     PODOperationWrite
	Grant                       PODPrivateGrant
}
