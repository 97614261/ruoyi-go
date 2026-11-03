// Generated cms_inquiry model, specialized for privacy projections and explicit CAS.
package model

import "ruoyi-go/pkg/types"

type CMSInquiry struct {
	InquiryId  int64      `gorm:"primaryKey" json:"inquiryId"`
	Name       string     `json:"name"`
	Contact    string     `json:"contact"`
	Message    string     `json:"message"`
	Status     string     `json:"status"`
	Note       string     `json:"note"`
	Version    int64      `json:"version"`
	IsDeleted  int        `json:"-"`
	CreateBy   string     `json:"createBy"`
	CreateTime types.Time `json:"createTime"`
	UpdateBy   string     `json:"updateBy"`
	UpdateTime types.Time `json:"updateTime"`
	Remark     string     `json:"remark"`
}

func (CMSInquiry) TableName() string { return "cms_inquiry" }

type CMSInquiryInput struct {
	Name    string `form:"name" json:"name" binding:"notblank,max=60"`
	Contact string `form:"contact" json:"contact" binding:"notblank,max=120"`
	Message string `form:"message" json:"message" binding:"notblank,max=2000"`
	Website string `form:"website" json:"website" binding:"max=200"`
	Consent string `form:"consent" json:"consent" binding:"required,eq=yes"`
	Token   string `form:"token" json:"token" binding:"required,len=64"`
}
type CMSInquiryQuery struct {
	Status string `form:"status" json:"status" binding:"omitempty,oneof=NEW DONE SPAM"`
}
type CMSInquiryUpdate struct {
	Version int64  `json:"version" binding:"required,gt=0"`
	Status  string `json:"status" binding:"required,oneof=NEW DONE SPAM"`
	Note    string `json:"note" binding:"max=500"`
}
type CMSInquirySummary struct {
	InquiryId  int64      `json:"inquiryId"`
	Name       string     `json:"name"`
	Contact    string     `json:"contact"`
	Status     string     `json:"status"`
	Version    int64      `json:"version"`
	CreateTime types.Time `json:"createTime"`
}
