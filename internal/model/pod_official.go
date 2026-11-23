package model

import (
	"encoding/json"
	"ruoyi-go/pkg/types"
)

type PODOfficialTemplate struct {
	TemplateID         int64      `gorm:"primaryKey;autoIncrement" json:"templateId,string"`
	Code               string     `json:"code"`
	Name               string     `json:"name"`
	Description        string     `json:"description"`
	Status             string     `json:"status"`
	CurrentVersionID   *int64     `json:"currentVersionId,omitempty,string"`
	PublishedVersionID *int64     `json:"publishedVersionId,omitempty,string"`
	Revision           int64      `json:"revision,string"`
	Sort               int        `json:"sort"`
	CreateBy           string     `json:"-"`
	CreateTime         types.Time `json:"createTime"`
	UpdateBy           string     `json:"-"`
	UpdateTime         types.Time `json:"updateTime"`
	Remark             string     `json:"-"`
}

func (PODOfficialTemplate) TableName() string { return "pod_catalog_template" }

type PODOfficialVersion struct {
	VersionID   int64               `gorm:"primaryKey;autoIncrement" json:"versionId,string"`
	TemplateID  int64               `json:"templateId,string"`
	Version     int64               `json:"version,string"`
	GraphJSON   json.RawMessage     `json:"-"`
	SpecHash    string              `json:"-"`
	PublishedAt *types.Time         `json:"publishedAt,omitempty"`
	CreateBy    string              `json:"-"`
	CreateTime  types.Time          `json:"createTime"`
	UpdateBy    string              `json:"-"`
	UpdateTime  types.Time          `json:"-"`
	Remark      string              `json:"-"`
	Graph       *PODGraph           `gorm:"-" json:"graph,omitempty"`
	Slots       []PODTemplateSlot   `gorm:"-" json:"slots,omitempty"`
	Outputs     []PODTemplateOutput `gorm:"-" json:"outputs,omitempty"`
}

func (PODOfficialVersion) TableName() string { return "pod_catalog_template_version" }

type PODOfficialAsset struct {
	AssetID     int64      `gorm:"primaryKey;autoIncrement" json:"assetId,string"`
	Name        string     `json:"name"`
	MediaType   string     `json:"mediaType"`
	ByteSize    int64      `json:"byteSize,string"`
	Width       int        `json:"width"`
	Height      int        `json:"height"`
	LicenseNote string     `json:"licenseNote"`
	Status      string     `json:"status"`
	Revision    int64      `json:"revision,string"`
	CreateBy    string     `json:"-"`
	CreateTime  types.Time `json:"createTime"`
	UpdateBy    string     `json:"-"`
	UpdateTime  types.Time `json:"-"`
	Remark      string     `json:"-"`
}

func (PODOfficialAsset) TableName() string { return "pod_catalog_asset" }

type PODOfficialAssetFile PODAssetFile

func (PODOfficialAssetFile) TableName() string { return "pod_catalog_asset_file" }

type PODOfficialRef struct {
	VersionID  int64
	NodeID     string
	AssetID    int64
	CreateBy   string
	CreateTime types.Time
	UpdateBy   string
	UpdateTime types.Time
	Remark     string
}

func (PODOfficialRef) TableName() string { return "pod_catalog_template_asset_ref" }

type PODOfficialWrite struct {
	RequestID   string    `json:"requestId"`
	Reason      string    `json:"reason"`
	Revision    string    `json:"revision,omitempty"`
	Code        string    `json:"code,omitempty"`
	Name        string    `json:"name,omitempty"`
	Description string    `json:"description,omitempty"`
	Sort        int       `json:"sort,omitempty"`
	VersionID   string    `json:"versionId,omitempty"`
	Graph       *PODGraph `json:"graph,omitempty"`
}
type PODOfficialDetail struct {
	Template *PODOfficialTemplate `json:"template"`
	Snapshot *PODOfficialVersion  `json:"snapshot,omitempty"`
}
type PODOfficialCopy struct {
	SourceAssetID int64
	Asset         PODAsset
	Files         []PODAssetFile
	SourceFiles   []PODAssetFile
}
type PODOfficialCopyPlan struct {
	Copies        []PODOfficialCopy
	PersonalNodes []string
}
type PODOfficialUseAction struct {
	ActionID            int64 `gorm:"primaryKey;autoIncrement"`
	UserID              int64
	WorkspaceID         int64
	RequestID           string
	RequestHash         string
	VersionID           int64
	State               string
	CopyMapJSON         json.RawMessage
	GraphJSON           json.RawMessage
	Name                string
	ProjectID           int64
	StorageOperationKey string
	CreateBy            string
	CreateTime          types.Time
	UpdateBy            string
	UpdateTime          types.Time
	Remark              string
}

func (PODOfficialUseAction) TableName() string { return "pod_catalog_use_action" }

type PODOfficialUseView struct {
	RequestID string `json:"requestId"`
	State     string `json:"state"`
	ProjectID int64  `json:"projectId,string"`
}
