package model

import (
	"encoding/json"
	"time"
)

// BoundModel is an internal immutable execution/pricing snapshot, never a creator DTO.
type PODBoundModel struct {
	ItemNo          int                    `json:"itemNo"`
	NodeID          string                 `json:"nodeId"`
	ModelID         int64                  `json:"modelId,string"`
	ModelVersionID  int64                  `json:"modelVersionId,string"`
	ProviderID      int64                  `json:"providerId,string"`
	Name            string                 `json:"name"`
	Kind            string                 `json:"kind"`
	Version         PODCatalogModelVersion `json:"version"`
	Capabilities    PODModelCapabilities   `json:"capabilities"`
	CostRule        PODModelRate           `json:"costRule"`
	PointRule       PODModelRate           `json:"pointRule"`
	Points          int64                  `json:"points,string"`
	CostMicros      int64                  `json:"costMicros,string"`
	InputTokenBound int64                  `json:"inputTokenBound,string"`
	ImageCount      int                    `json:"imageCount"`
}
type PODCreatorModel struct {
	ModelID         int64                `json:"modelId,string"`
	ModelVersionID  int64                `json:"modelVersionId,string"`
	Name            string               `json:"name"`
	Kind            string               `json:"kind"`
	OfficialModelID string               `json:"officialModelId"`
	Capabilities    PODModelCapabilities `json:"capabilities"`
	PointRule       PODModelRate         `json:"pointRule"`
	Default         bool                 `json:"default"`
}
type PODExecutionQuote struct {
	QuoteID              string `gorm:"primaryKey"`
	UserID               int64
	WorkspaceID          int64
	Kind                 string
	SourceHash           string
	SelectionJSON        json.RawMessage
	PolicyRevision       int64
	CatalogHash          string
	PointsUpperBound     int64
	CostUpperMicros      int64
	ExpiresAt            time.Time
	State                string
	ConsumedResourceType string
	ConsumedResourceID   int64
	ConsumedRequestID    *string
	CreateTime           time.Time
}

func (PODExecutionQuote) TableName() string { return "pod_execution_quote" }

type PODExecutionSnapshot struct {
	ScopeKind         string `gorm:"primaryKey"`
	ScopeID           int64  `gorm:"primaryKey;autoIncrement:false"`
	WorkspaceID       int64
	UserID            int64
	QuoteID           string
	BillingScheme     string
	PolicyRevision    int64
	SelectionJSON     json.RawMessage
	HoldPoints        int64
	UnallocatedPoints int64
	CreateTime        time.Time
}

func (PODExecutionSnapshot) TableName() string { return "pod_execution_snapshot" }

type PODRunModelBinding struct {
	RunID          int64  `gorm:"primaryKey;autoIncrement:false"`
	NodeID         string `gorm:"primaryKey"`
	WorkspaceID    int64
	UserID         int64
	ModelID        int64
	ModelVersionID int64
	ProviderID     int64
	SelectionJSON  json.RawMessage
}

func (PODRunModelBinding) TableName() string { return "pod_run_model_binding" }

type PODCallBilling struct {
	SubmissionKey       string `gorm:"primaryKey"`
	RunID               int64
	NodeID              string
	UserID              int64
	WorkspaceID         int64
	ModelVersionID      int64
	CredentialVersionID int64
	HeldPoints          int64
	ChargedPoints       int64
	ReleasedPoints      int64
	State               string
	UsageJSON           json.RawMessage
	CreateTime          time.Time
}

func (PODCallBilling) TableName() string { return "pod_call_billing" }

type PODQuoteRequest struct {
	Kind  string          `json:"kind"`
	Run   *PODRunCreate   `json:"run,omitempty"`
	Batch *PODBatchCreate `json:"batch,omitempty"`
	RunID string          `json:"runId,omitempty"`
}
type PODQuoteLine struct {
	Kind            string `json:"kind"`
	OfficialModelID string `json:"officialModelId"`
	Images          int    `json:"images"`
	ItemNo          int    `json:"itemNo"`
	NodeID          string `json:"nodeId"`
	ModelID         int64  `json:"modelId,string"`
	ModelVersionID  int64  `json:"modelVersionId,string"`
	Name            string `json:"name"`
	Points          int64  `json:"points,string"`
}
type PODQuoteView struct {
	QuoteID      string         `json:"quoteId"`
	Kind         string         `json:"kind"`
	State        string         `json:"state"`
	ExpiresAt    time.Time      `json:"expiresAt"`
	Points       int64          `json:"points,string"`
	Lines        []PODQuoteLine `json:"lines"`
	ResourceType string         `json:"resourceType"`
	ResourceID   int64          `json:"resourceId,string"`
}
type PODBillingView struct {
	Scheme   string `json:"scheme"`
	Held     int64  `json:"held,string"`
	Spent    int64  `json:"spent,string"`
	Released int64  `json:"released,string"`
}

// QuotePlan stays on the server and binds the exact normalized request and graphs.
type PODQuotePlan struct {
	Source     PODQuoteRequest `json:"source"`
	Models     []PODBoundModel `json:"models"`
	RunAttempt int             `json:"runAttempt,omitempty"`
}
