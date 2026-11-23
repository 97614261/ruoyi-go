package model

import (
	"encoding/json"
	"ruoyi-go/pkg/types"
)

// Adapted from the provider/catalogmodel generator scaffolds; credential and
// immutable-version operations deliberately have no generic update/delete DTO.
type PODProvider struct {
	ProviderID                 int64 `gorm:"primaryKey;autoIncrement"`
	ProviderCode               string
	Name                       string
	AdapterKind                string
	Region                     string
	Endpoint                   string
	Status                     string
	Revision                   int64
	CurrentCredentialVersionID *int64
	CreateBy                   string
	CreateTime                 types.Time
	UpdateBy                   string
	UpdateTime                 types.Time
	Remark                     string
}

func (PODProvider) TableName() string { return "pod_provider" }

type PODProviderCredential struct {
	CredentialVersionID int64      `gorm:"primaryKey;autoIncrement" json:"-"`
	ProviderID          int64      `json:"-"`
	Version             int64      `json:"-"`
	SourceType          string     `json:"-"`
	KeyID               string     `json:"-"`
	Nonce               []byte     `json:"-"`
	Ciphertext          []byte     `json:"-"`
	EnvName             string     `json:"-"`
	State               string     `json:"-"`
	CreateBy            string     `json:"-"`
	CreateTime          types.Time `json:"-"`
	UpdateBy            string     `json:"-"`
	UpdateTime          types.Time `json:"-"`
	Remark              string     `json:"-"`
}

func (PODProviderCredential) TableName() string { return "pod_provider_credential_version" }

type PODCatalogModel struct {
	ModelID          int64 `gorm:"primaryKey;autoIncrement"`
	ModelCode        string
	ProviderID       int64
	Kind             string
	Name             string
	CurrentVersionID *int64
	Status           string
	Visible          bool
	Sort             int
	Revision         int64
	CreateBy         string
	CreateTime       types.Time
	UpdateBy         string
	UpdateTime       types.Time
	Remark           string
}

func (PODCatalogModel) TableName() string { return "pod_model" }

type PODCatalogModelVersion struct {
	ModelVersionID   int64 `gorm:"primaryKey;autoIncrement"`
	ModelID          int64
	Version          int64
	OfficialModelID  string
	AdapterKind      string
	AdapterVersion   string
	Endpoint         string
	Region           string
	CapabilitiesJSON json.RawMessage
	CostRuleJSON     json.RawMessage
	PointRuleJSON    json.RawMessage
	TimeoutSeconds   int
	SpecHash         string
	CreateBy         string
	CreateTime       types.Time
	UpdateBy         string
	UpdateTime       types.Time
	Remark           string
}

func (PODCatalogModelVersion) TableName() string { return "pod_model_version" }

type PODModelCapabilities struct {
	MaxInputBytes      int               `json:"maxInputBytes,omitempty"`
	MaxOutputTokens    int               `json:"maxOutputTokens,omitempty"`
	MaxPromptChars     int               `json:"maxPromptChars,omitempty"`
	MaxImages          int               `json:"maxImages,omitempty"`
	MaxReferenceImages int               `json:"maxReferenceImages,omitempty"`
	Ratios             []string          `json:"ratios,omitempty"`
	TextAPI            string            `json:"textApi,omitempty"`
	ImageSizes         map[string]string `json:"imageSizes,omitempty"`
	ImageQuality       string            `json:"imageQuality,omitempty"`
	ImageResponse      string            `json:"imageResponse,omitempty"`
}
type PODModelRate struct {
	Version          int    `json:"version"`
	Currency         string `json:"currency,omitempty"`
	Unit             string `json:"unit"`
	InputPerMillion  int64  `json:"inputPerMillion,string"`
	OutputPerMillion int64  `json:"outputPerMillion,string"`
	PerImage         int64  `json:"perImage,string"`
}

type PODCatalogWrite struct {
	RequestID       string                `json:"requestId"`
	Revision        string                `json:"revision,omitempty"`
	Reason          string                `json:"reason"`
	Code            string                `json:"code,omitempty"`
	Name            string                `json:"name,omitempty"`
	AdapterKind     string                `json:"adapterKind,omitempty"`
	Region          string                `json:"region,omitempty"`
	Endpoint        string                `json:"endpoint,omitempty"`
	ProviderID      string                `json:"providerId,omitempty"`
	Kind            string                `json:"kind,omitempty"`
	Status          string                `json:"status,omitempty"`
	Visible         *bool                 `json:"visible,omitempty"`
	Sort            *int                  `json:"sort,omitempty"`
	SourceType      string                `json:"sourceType,omitempty"`
	Secret          string                `json:"secret,omitempty"`
	EnvName         string                `json:"envName,omitempty"`
	OfficialModelID string                `json:"officialModelId,omitempty"`
	TimeoutSeconds  int                   `json:"timeoutSeconds,omitempty"`
	Capabilities    *PODModelCapabilities `json:"capabilities,omitempty"`
	CostRule        *PODModelRate         `json:"costRule,omitempty"`
	PointRule       *PODModelRate         `json:"pointRule,omitempty"`
	TextModelID     *string               `json:"textModelId,omitempty"`
	ImageModelID    *string               `json:"imageModelId,omitempty"`
}
type PODCatalogFilter struct {
	Code       string
	Name       string
	Kind       string
	Status     string
	ProviderID int64
}
type PODProviderView struct {
	ProviderID           int64  `json:"providerId,string"`
	Code                 string `json:"code"`
	Name                 string `json:"name"`
	AdapterKind          string `json:"adapterKind"`
	Region               string `json:"region"`
	Endpoint             string `json:"endpoint"`
	Status               string `json:"status"`
	Revision             int64  `json:"revision,string"`
	CredentialConfigured bool   `json:"credentialConfigured"`
	CredentialVersion    int64  `json:"credentialVersion,string"`
	CredentialSourceType string `json:"credentialSourceType"`
}
type PODCatalogModelView struct {
	ModelID                      int64       `json:"modelId,string"`
	Code                         string      `json:"code"`
	ProviderID                   int64       `json:"providerId,string"`
	ProviderName                 string      `json:"providerName"`
	ProviderStatus               string      `json:"providerStatus"`
	ProviderCredentialConfigured bool        `json:"providerCredentialConfigured"`
	ProviderAdapterKind          string      `json:"providerAdapterKind"`
	ProviderEndpoint             string      `json:"providerEndpoint"`
	Kind                         string      `json:"kind"`
	Name                         string      `json:"name"`
	CurrentVersionID             int64       `json:"currentVersionId,string"`
	CurrentVersion               int64       `json:"currentVersion,string"`
	Status                       string      `json:"status"`
	Visible                      bool        `json:"visible"`
	Sort                         int         `json:"sort"`
	Revision                     int64       `json:"revision,string"`
	VerifiedAt                   *types.Time `json:"verifiedAt,omitempty"`
}
type PODCatalogVersionView struct {
	ModelVersionID  int64                `json:"modelVersionId,string"`
	ModelID         int64                `json:"modelId,string"`
	Version         int64                `json:"version,string"`
	OfficialModelID string               `json:"officialModelId"`
	AdapterKind     string               `json:"adapterKind"`
	AdapterVersion  string               `json:"adapterVersion"`
	Endpoint        string               `json:"endpoint"`
	Region          string               `json:"region"`
	Capabilities    PODModelCapabilities `json:"capabilities"`
	CostRule        PODModelRate         `json:"costRule"`
	PointRule       PODModelRate         `json:"pointRule"`
	TimeoutSeconds  int                  `json:"timeoutSeconds"`
	SpecHash        string               `json:"specHash"`
	CreateTime      types.Time           `json:"createTime"`
}
type PODCatalogDefaults struct {
	Revision       int64  `json:"revision,string"`
	TextModelID    string `json:"textModelId"`
	ImageModelID   string `json:"imageModelId"`
	ManagedEnabled bool   `json:"managedEnabled"`
}
type PODCatalogResult struct {
	RequestID    string `json:"requestId"`
	ActionType   string `json:"actionType"`
	ResourceType string `json:"resourceType"`
	ResourceID   int64  `json:"resourceId,string"`
	Revision     int64  `json:"revision,string"`
	Version      int64  `json:"version,string"`
}
type PODCatalogAuditSnapshot struct {
	Revision     int64  `json:"revision,string"`
	Code         string `json:"code,omitempty"`
	Endpoint     string `json:"endpoint,omitempty"`
	Status       string `json:"status,omitempty"`
	Version      int64  `json:"version,string"`
	SourceType   string `json:"sourceType,omitempty"`
	TextModelID  string `json:"textModelId,omitempty"`
	ImageModelID string `json:"imageModelId,omitempty"`
}
type PODAdminAction struct {
	ActionID     int64 `gorm:"primaryKey;autoIncrement"`
	ActorUserID  int64
	RequestID    string
	RequestHash  string
	ActionType   string
	ResourceType string
	ResourceID   int64
	State        string
	ResultJSON   json.RawMessage
	CreateBy     string
	CreateTime   types.Time
	UpdateBy     string
	UpdateTime   types.Time
	Remark       string
}

func (PODAdminAction) TableName() string { return "pod_admin_action" }
