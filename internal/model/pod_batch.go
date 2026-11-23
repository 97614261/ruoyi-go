package model

import (
	"encoding/json"
	"ruoyi-go/pkg/types"
)

const PODBatchMaxItems = 10

// Batch execution is an action resource, not a second run engine.
type PODBatch struct {
	QuoteID         string `gorm:"-"`
	SourceHash      string `gorm:"-"`
	BatchID         int64  `gorm:"primaryKey;autoIncrement"`
	WorkspaceID     int64
	UserID          int64
	RequestID       string
	RequestHash     string
	TemplateID      int64
	TemplateVersion int64
	Name            string
	ExecutionMode   string
	State           string
	ImpactJSON      json.RawMessage
	CreateBy        string
	CreateTime      types.Time
	UpdateBy        string
	UpdateTime      types.Time
}

func (PODBatch) TableName() string { return "pod_batch" }

type PODBatchItem struct {
	BatchID     int64 `gorm:"primaryKey;autoIncrement:false"`
	ItemNo      int   `gorm:"primaryKey;autoIncrement:false"`
	WorkspaceID int64
	Name        string
	GraphJSON   json.RawMessage
	State       string
	ProjectID   int64
	RunID       int64
	ErrorCode   string
	CheckedAt   types.Time
	CreateBy    string
	CreateTime  types.Time
	UpdateBy    string
	UpdateTime  types.Time
}

func (PODBatchItem) TableName() string { return "pod_batch_item" }

type PODBatchAssetRef struct {
	BatchID     int64 `gorm:"primaryKey;autoIncrement:false"`
	AssetID     int64 `gorm:"primaryKey;autoIncrement:false"`
	WorkspaceID int64
}

func (PODBatchAssetRef) TableName() string { return "pod_batch_asset_ref" }

type PODBatchInput struct {
	Name     string               `json:"name"`
	Bindings []PODTemplateBinding `json:"bindings"`
}
type PODBatchCreate struct {
	QuoteID       string          `json:"quoteId,omitempty"`
	RequestID     string          `json:"requestId"`
	TemplateID    string          `json:"templateId"`
	Version       string          `json:"version"`
	Name          string          `json:"name"`
	ExecutionMode string          `json:"executionMode"`
	Items         []PODBatchInput `json:"items"`
}
type PODBatchImpact struct {
	Items             int   `json:"items"`
	TextCalls         int   `json:"textCalls"`
	ImageCalls        int   `json:"imageCalls"`
	Images            int   `json:"images"`
	ReservationMicros int64 `json:"reservationMicros,string"`
	TestCredits       int   `json:"testCredits"`
}
type PODBatchView struct {
	Billing         *PODBillingView    `json:"billing,omitempty"`
	BatchID         int64              `json:"batchId,string"`
	TemplateID      int64              `json:"templateId,string"`
	TemplateVersion int64              `json:"templateVersion,string"`
	Name            string             `json:"name"`
	ExecutionMode   string             `json:"executionMode"`
	State           string             `json:"state"`
	Impact          PODBatchImpact     `json:"impact"`
	CreateTime      types.Time         `json:"createTime"`
	Items           []PODBatchItemView `json:"items,omitempty"`
}
type PODBatchItemView struct {
	ItemNo    int    `json:"itemNo"`
	Name      string `json:"name"`
	State     string `json:"state"`
	ProjectID int64  `json:"projectId,string"`
	RunID     int64  `json:"runId,string"`
	ErrorCode string `json:"errorCode"`
}
