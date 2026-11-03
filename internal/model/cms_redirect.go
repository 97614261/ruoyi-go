// CRUD scaffold specialized for disabled drafts and explicit routing publication.
package model

import "ruoyi-go/pkg/types"

type CMSRedirect struct {
	RedirectId int64      `gorm:"primaryKey" json:"redirectId"`
	SourcePath string     `json:"sourcePath"`
	TargetId   int64      `json:"targetId"`
	Status     string     `json:"status"`
	Version    int64      `json:"version"`
	CreateBy   string     `json:"createBy"`
	CreateTime types.Time `json:"createTime"`
	UpdateBy   string     `json:"updateBy"`
	UpdateTime types.Time `json:"updateTime"`
	Remark     string     `json:"remark"`
}

func (CMSRedirect) TableName() string { return "cms_redirect" }

type CMSRedirectView struct {
	CMSRedirect
	TargetPath string `gorm:"-" json:"targetPath"`
}
type CMSRedirectInput struct {
	RedirectId int64  `json:"redirectId" binding:"min=0"`
	SourcePath string `json:"sourcePath" binding:"required,max=200"`
	TargetId   int64  `json:"targetId" binding:"required,gt=0"`
	Version    int64  `json:"version" binding:"min=0"`
	Remark     string `json:"remark" binding:"xss,max=500"`
}
type CMSRedirectQuery struct {
	SourcePath string `form:"sourcePath" binding:"max=200"`
	Status     string `form:"status" binding:"omitempty,oneof=0 1"`
}
type CMSRedirectStatus struct {
	Version int64  `json:"version" binding:"required,gt=0"`
	Status  string `json:"status" binding:"required,oneof=0 1"`
}
