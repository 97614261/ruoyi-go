package model

import (
	"encoding/json"
	"ruoyi-go/pkg/types"
	"time"
)

type PODExecutionLimits struct {
	Mode                string `mapstructure:"mode" json:"mode"`
	GlobalConcurrency   int    `mapstructure:"globalConcurrency" json:"globalConcurrency"`
	UserConcurrency     int    `mapstructure:"userConcurrency" json:"userConcurrency"`
	ProviderConcurrency int    `mapstructure:"providerConcurrency" json:"providerConcurrency"`
	MaxPending          int    `mapstructure:"maxPending" json:"maxPending"`
	MaxUserPending      int    `mapstructure:"maxUserPending" json:"maxUserPending"`
	InitialCredits      int64  `mapstructure:"initialCredits" json:"initialCredits,string"`
	TextModel           string `json:"textModel,omitempty"`
	ImageModel          string `json:"imageModel,omitempty"`
	ImageAIURL          string `json:"-"`
	ImageAIEnabled      bool   `json:"imageAIEnabled"`
}
type PODRunCreate struct {
	QuoteID       string `json:"quoteId,omitempty"`
	BatchID       int64  `json:"-"`
	BatchItemNo   int    `json:"-"`
	ExecutionMode string `json:"executionMode,omitempty"`
	ProjectID     string `json:"projectId"`
	Revision      string `json:"revision"`
	RequestID     string `json:"requestId"`
	Mode          string `json:"mode"`
	NodeID        string `json:"nodeId"`
	BaseRunID     string `json:"baseRunId"`
	TestScenario  string `json:"testScenario"`
}
type PODRunRetry struct {
	QuoteID   string `json:"quoteId,omitempty"`
	RequestID string `json:"requestId"`
}
type PODRun struct {
	RunID           int64 `gorm:"primaryKey"`
	WorkspaceID     int64
	UserID          int64
	ProjectID       int64
	ProjectRevision int64
	ProjectName     string
	RequestID       string
	RequestHash     string
	Mode            string
	ExecutionMode   string
	TargetNodeID    string
	BaseRunID       int64
	GraphJSON       json.RawMessage
	BaseOutputsJSON json.RawMessage
	State           PODRunState
	TestScenario    string
	Attempt         int
	Reserved        int64
	Spent           int64
	Settled         bool
	CreateBy        string
	UpdateBy        string
	CreateTime      time.Time
	UpdateTime      time.Time
	Remark          string
}

func (PODRun) TableName() string { return "pod_run" }

type PODNodeRun struct {
	RunID         int64  `gorm:"primaryKey"`
	NodeID        string `gorm:"primaryKey"`
	WorkspaceID   int64
	UserID        int64
	NodeType      string
	Provider      string
	ProviderModel string
	State         string
	Attempt       int
	SubmissionKey string
	LeaseToken    string
	LeaseUntil    *time.Time
	InputJSON     json.RawMessage
	OutputJSON    json.RawMessage
	ErrorCode     string
	ErrorMessage  string
	CreateBy      string
	UpdateBy      string
	CreateTime    time.Time
	UpdateTime    time.Time
	Remark        string
}

func (PODNodeRun) TableName() string { return "pod_node_run" }

type PODRunOutput struct {
	TestOnly bool              `json:"testOnly"`
	Text     string            `json:"text,omitempty"`
	AssetIDs []string          `json:"assetIds,omitempty"`
	Usage    *PODProviderUsage `json:"usage,omitempty"`
	Images   []PODImageInfo    `json:"images,omitempty"`
}

type PODImageInfo struct {
	AssetID string  `json:"assetId"`
	Width   int     `json:"width"`
	Height  int     `json:"height"`
	Kind    string  `json:"kind"`
	Format  string  `json:"format"`
	DPI     float64 `json:"effectiveDpi,omitempty"`
	Warning string  `json:"warning,omitempty"`
}
type PODRunView struct {
	ExecutionMode   string           `json:"executionMode"`
	Billing         *PODBillingView  `json:"billing,omitempty"`
	RunID           int64            `json:"runId,string"`
	ProjectID       int64            `json:"projectId,string"`
	ProjectRevision int64            `json:"projectRevision,string"`
	ProjectName     string           `json:"projectName"`
	Mode            string           `json:"mode"`
	TargetNodeID    string           `json:"nodeId"`
	State           PODRunState      `json:"state"`
	Attempt         int              `json:"attempt"`
	TestOnly        bool             `json:"testOnly"`
	TestScenario    string           `json:"testScenario"`
	Reserved        int64            `json:"reserved,string"`
	Spent           int64            `json:"spent,string"`
	CreateTime      types.Time       `json:"createTime"`
	UpdateTime      types.Time       `json:"updateTime"`
	Graph           *PODGraph        `json:"graph,omitempty"`
	Nodes           []PODNodeRunView `json:"nodes,omitempty"`
}
type PODNodeRunView struct {
	NodeID       string       `json:"nodeId"`
	NodeType     string       `json:"nodeType"`
	State        string       `json:"state"`
	Attempt      int          `json:"attempt"`
	Output       PODRunOutput `json:"output"`
	ErrorCode    string       `json:"errorCode"`
	ErrorMessage string       `json:"errorMessage"`
}
type PODCreditAccount struct {
	WorkspaceID int64     `gorm:"primaryKey" json:"-"`
	Available   int64     `json:"available,string"`
	Held        int64     `json:"held,string"`
	Spent       int64     `json:"spent,string"`
	CreateBy    string    `json:"-"`
	UpdateBy    string    `json:"-"`
	CreateTime  time.Time `json:"-"`
	UpdateTime  time.Time `json:"-"`
	Remark      string    `json:"-"`
}

func (PODCreditAccount) TableName() string { return "pod_credit_account" }

type PODCreditEvent struct {
	EventID     int64      `gorm:"primaryKey" json:"eventId,string"`
	WorkspaceID int64      `json:"-"`
	RunID       int64      `json:"runId,string"`
	EventKey    string     `json:"-"`
	Kind        string     `json:"kind"`
	Amount      int64      `json:"amount,string"`
	Available   int64      `json:"available,string"`
	Held        int64      `json:"held,string"`
	Spent       int64      `json:"spent,string"`
	CreateBy    string     `json:"-"`
	UpdateBy    string     `json:"-"`
	CreateTime  types.Time `json:"createTime"`
	UpdateTime  types.Time `json:"-"`
	Remark      string     `json:"-"`
}

func (PODCreditEvent) TableName() string { return "pod_credit_event" }

type PODOutbox struct {
	RunID          int64 `gorm:"primaryKey"`
	Active         bool
	DeliverySeq    int64
	NextDeliveryAt time.Time
	CreateBy       string
	UpdateBy       string
	CreateTime     time.Time
	UpdateTime     time.Time
	Remark         string
}

func (PODOutbox) TableName() string { return "pod_outbox" }

type PODRunAssetRef struct {
	RunID       int64 `gorm:"primaryKey"`
	AssetID     int64 `gorm:"primaryKey"`
	WorkspaceID int64
	CreateBy    string
	UpdateBy    string
	CreateTime  time.Time
	UpdateTime  time.Time
	Remark      string
}

func (PODRunAssetRef) TableName() string { return "pod_run_asset_ref" }

type PODRetryRequest struct {
	WorkspaceID int64  `gorm:"primaryKey"`
	RequestID   string `gorm:"primaryKey"`
	RunID       int64
	Attempt     int
	CreateBy    string
	UpdateBy    string
	CreateTime  time.Time
	UpdateTime  time.Time
	Remark      string
}

func (PODRetryRequest) TableName() string { return "pod_retry_request" }
