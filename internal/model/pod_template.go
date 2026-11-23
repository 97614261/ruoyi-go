package model

import (
	"encoding/json"
	"ruoyi-go/pkg/types"
)

// Adapted from the generated template CRUD scaffold; ownership is server-only.
type PODTemplate struct {
	TemplateID     int64 `gorm:"primaryKey;autoIncrement"`
	WorkspaceID    int64
	Name           string
	Description    string
	CurrentVersion int64
	Revision       int64
	DelFlag        string
	CreateBy       string
	CreateTime     types.Time
	UpdateBy       string
	UpdateTime     types.Time
	Remark         string
}

type PODTemplateVersion struct {
	TemplateID  int64
	Version     int64
	WorkspaceID int64
	GraphJSON   json.RawMessage
	CreateBy    string
	CreateTime  types.Time
	UpdateBy    string
	UpdateTime  types.Time
	Remark      string
}

type PODTemplateView struct {
	TemplateID     int64                   `json:"templateId,string"`
	Name           string                  `json:"name"`
	Description    string                  `json:"description"`
	CurrentVersion int64                   `json:"currentVersion,string"`
	Revision       int64                   `json:"revision,string"`
	Deleted        bool                    `json:"deleted"`
	CreateTime     types.Time              `json:"createTime"`
	UpdateTime     types.Time              `json:"updateTime"`
	Snapshot       *PODTemplateVersionView `json:"snapshot,omitempty"`
}

type PODTemplateVersionView struct {
	TemplateID int64               `json:"templateId,string"`
	Version    int64               `json:"version,string"`
	CreateTime types.Time          `json:"createTime"`
	Graph      *PODGraph           `json:"graph,omitempty"`
	Slots      []PODTemplateSlot   `json:"slots,omitempty"`
	Outputs    []PODTemplateOutput `json:"outputs,omitempty"`
}

type PODTemplateSlot struct {
	NodeID   string   `json:"nodeId"`
	Type     string   `json:"type"`
	Label    string   `json:"label"`
	Text     *string  `json:"text,omitempty"`
	AssetIDs []string `json:"assetIds,omitempty"`
}

type PODTemplateOutput struct {
	NodeID string `json:"nodeId"`
	Type   string `json:"type"`
	Label  string `json:"label"`
}

type PODTemplateSave struct {
	RequestID       string `json:"requestId"`
	ProjectID       string `json:"projectId"`
	ProjectRevision string `json:"projectRevision"`
	Revision        string `json:"revision,omitempty"`
	Name            string `json:"name"`
	Description     string `json:"description"`
}

type PODTemplateUpdate struct {
	Revision    string `json:"revision"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type PODTemplateCopy struct {
	RequestID string `json:"requestId"`
	Version   string `json:"version"`
	Name      string `json:"name"`
}

type PODTemplateBinding struct {
	NodeID   string    `json:"nodeId"`
	Text     *string   `json:"text,omitempty"`
	AssetIDs *[]string `json:"assetIds,omitempty"`
}

type PODTemplateUse struct {
	RequestID string               `json:"requestId"`
	Version   string               `json:"version"`
	Name      string               `json:"name"`
	Bindings  []PODTemplateBinding `json:"bindings"`
}

type PODTemplateAssetRef struct {
	TemplateID  int64
	Version     int64
	NodeID      string
	AssetID     int64
	WorkspaceID int64
	CreateBy    string
	CreateTime  types.Time
	UpdateBy    string
	UpdateTime  types.Time
	Remark      string
}

type PODTemplateAction struct {
	WorkspaceID int64
	RequestID   string
	Action      string
	RequestHash string
	TemplateID  int64
	Version     int64
	ProjectID   int64
	CreateBy    string
	CreateTime  types.Time
	UpdateBy    string
	UpdateTime  types.Time
	Remark      string
}
