package model

import "ruoyi-go/pkg/types"

type CMSTag struct {
	TagId      int64      `gorm:"primaryKey" json:"tagId"`
	Name       string     `json:"name" binding:"notblank,xss,max=40"`
	Status     string     `json:"status" binding:"required,oneof=0 1"`
	Version    int64      `json:"version" binding:"min=0"`
	CreateBy   string     `json:"createBy"`
	CreateTime types.Time `json:"createTime"`
	UpdateBy   string     `json:"updateBy"`
	UpdateTime types.Time `json:"updateTime"`
	Remark     string     `json:"remark" binding:"xss,max=500"`
}

func (CMSTag) TableName() string { return "cms_tag" }

type CMSTagQuery struct {
	Name   string `form:"name" binding:"max=40"`
	Status string `form:"status" binding:"omitempty,oneof=0 1"`
}
type CMSTagSnapshot struct {
	TagId int64  `json:"tagId"`
	Name  string `json:"name"`
}
type CMSContentOps struct {
	TagIds           []int64          `json:"tagIds" binding:"max=12,dive,gt=0"`
	RelatedIds       []int64          `json:"relatedIds" binding:"max=12,dive,gt=0"`
	Featured         bool             `json:"featured"`
	PinOrder         int              `json:"pinOrder" binding:"min=0,max=100000"`
	ShareTitle       string           `json:"shareTitle" binding:"xss,max=150"`
	ShareDescription string           `json:"shareDescription" binding:"xss,max=300"`
	ShareAssetId     int64            `json:"shareAssetId" binding:"min=0"`
	Tags             []CMSTagSnapshot `json:"tags,omitempty"`
}
type CMSOpsInput struct {
	Version  int64         `json:"version" binding:"required,min=1"`
	Settings CMSContentOps `json:"settings"`
}
type CMSOpsView struct {
	ContentId      int64         `json:"contentId"`
	Version        int64         `json:"version"`
	Title          string        `json:"title"`
	Summary        string        `json:"summary"`
	SeoTitle       string        `json:"seoTitle"`
	SeoDescription string        `json:"seoDescription"`
	Slug           string        `json:"slug"`
	Settings       CMSContentOps `json:"settings"`
}
type CMSRevisionOps struct {
	RevisionId int64 `gorm:"primaryKey"`
	Featured   bool
	PinOrder   int
	Payload    string
	CreateBy   string
	CreateTime types.Time
	UpdateBy   string
	UpdateTime types.Time
	Remark     string
}

func (CMSRevisionOps) TableName() string { return "cms_revision_ops" }

type CMSVisit struct {
	VisitDay   string     `gorm:"primaryKey" json:"day"`
	Dimension  string     `gorm:"primaryKey" json:"dimension"`
	Value      string     `gorm:"column:dimension_value;primaryKey" json:"value"`
	Hits       int64      `json:"hits"`
	CreateBy   string     `json:"-"`
	CreateTime types.Time `json:"-"`
	UpdateBy   string     `json:"-"`
	UpdateTime types.Time `json:"-"`
	Remark     string     `json:"-"`
}

func (CMSVisit) TableName() string { return "cms_visit_daily" }

type CMSVisitQuery struct {
	From      string `form:"from"`
	To        string `form:"to"`
	Dimension string `form:"dimension" binding:"omitempty,oneof=day page source keyword"`
}
type CMSVisitRow struct {
	Value string `json:"value"`
	Hits  int64  `json:"hits"`
}
type SiteWidgetItem struct {
	Title     string `json:"title"`
	Text      string `json:"text"`
	AssetId   int64  `json:"assetId"`
	ContentId int64  `json:"contentId"`
	URL       string `json:"-"`
}
