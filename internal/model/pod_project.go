package model

import (
	"encoding/json"
	"ruoyi-go/pkg/types"
)

// Adapted from the project CRUD scaffold; transport DTOs never bind ownership.
type PODProject struct {
	ProjectID     int64           `gorm:"column:project_id;primaryKey;autoIncrement" json:"-"`
	WorkspaceID   int64           `gorm:"column:workspace_id" json:"-"`
	Name          string          `gorm:"column:name" json:"-"`
	GraphJSON     json.RawMessage `gorm:"column:graph_json" json:"-"`
	SchemaVersion int             `gorm:"column:schema_version" json:"-"`
	Revision      int64           `gorm:"column:revision" json:"-"`
	DelFlag       string          `gorm:"column:del_flag" json:"-"`
	CreateBy      string          `gorm:"column:create_by" json:"-"`
	CreateTime    types.Time      `gorm:"column:create_time" json:"-"`
	UpdateBy      string          `gorm:"column:update_by" json:"-"`
	UpdateTime    types.Time      `gorm:"column:update_time" json:"-"`
	Remark        string          `gorm:"column:remark" json:"-"`
}

type PODProjectView struct {
	ProjectID   int64      `json:"projectId,string"`
	WorkspaceID int64      `json:"workspaceId,string"`
	Name        string     `json:"name"`
	Revision    int64      `json:"revision,string"`
	Deleted     bool       `json:"deleted"`
	CreateTime  types.Time `json:"createTime"`
	UpdateTime  types.Time `json:"updateTime"`
	Graph       *PODGraph  `json:"graph,omitempty"`
}

type PODProjectCreate struct {
	Name string `json:"name"`
}
type PODProjectUpdate struct {
	Revision string    `json:"revision"`
	Name     *string   `json:"name,omitempty"`
	Graph    *PODGraph `json:"graph,omitempty"`
}
type PODProjectVersion struct {
	Revision string `json:"revision"`
}
