package model

import "ruoyi-go/pkg/types"

// CmsChannel 门户栏目。
type CmsChannel struct {
	ParentName     string     `gorm:"column:parent_name;->" json:"parentName"`
	CoverName      string     `gorm:"column:cover_name;->" json:"coverName"`
	ChannelId      int64      `gorm:"column:channel_id;primaryKey;autoIncrement" json:"channelId"`
	ParentId       int64      `gorm:"column:parent_id" json:"parentId"`
	Kind           string     `gorm:"column:kind" json:"kind"`
	Name           string     `gorm:"column:name" json:"name"`
	Slug           string     `gorm:"column:slug" json:"slug"`
	OrderNum       int        `gorm:"column:order_num" json:"orderNum"`
	Status         string     `gorm:"column:status" json:"status"`
	CoverAssetId   *int64     `gorm:"column:cover_asset_id" json:"coverAssetId"`
	SeoTitle       string     `gorm:"column:seo_title" json:"seoTitle"`
	SeoDescription string     `gorm:"column:seo_description" json:"seoDescription"`
	Version        int64      `gorm:"column:version" json:"version"`
	CreateBy       string     `gorm:"column:create_by" json:"createBy"`
	CreateTime     types.Time `gorm:"column:create_time" json:"createTime"`
	UpdateBy       string     `gorm:"column:update_by" json:"updateBy"`
	UpdateTime     types.Time `gorm:"column:update_time" json:"updateTime"`
	Remark         string     `gorm:"column:remark" json:"remark"`
}

func (CmsChannel) TableName() string { return "cms_channel" }
