package model

import (
	"encoding/json"
	"time"
)

// PODProviderUsage contains no credentials, signed URLs or internal disk keys.
type PODProviderUsage struct {
	Model               string `json:"model"`
	RequestID           string `json:"requestId"`
	InputTokens         int64  `json:"inputTokens"`
	OutputTokens        int64  `json:"outputTokens"`
	ImageCount          int64  `json:"imageCount"`
	EstimatedCostMicros int64  `json:"estimatedCostMicros,string"`
}

type PODProviderCall struct {
	SubmissionKey  string `gorm:"primaryKey"`
	RunID          int64
	NodeID         string
	WorkspaceID    int64
	Model          string
	State          string
	ReservedMicros int64
	ChargedMicros  int64
	ReceiptJSON    json.RawMessage
	CreateBy       string
	UpdateBy       string
	CreateTime     time.Time
	UpdateTime     time.Time
	Remark         string
}

func (PODProviderCall) TableName() string { return "pod_provider_call" }

type PODProviderAsset struct {
	SubmissionKey string `gorm:"primaryKey"`
	ImageIndex    int    `gorm:"primaryKey"`
	AssetID       int64
	WorkspaceID   int64
	CreateBy      string
	UpdateBy      string
	CreateTime    time.Time
	UpdateTime    time.Time
	Remark        string
}

func (PODProviderAsset) TableName() string { return "pod_provider_asset" }
