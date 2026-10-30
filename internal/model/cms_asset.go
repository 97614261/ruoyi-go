package model

import "ruoyi-go/pkg/types"

// CmsAsset 门户私有素材。
type CmsAsset struct {
	ReferenceCount int64      `gorm:"-" json:"referenceCount"`
	AssetId        int64      `gorm:"column:asset_id;primaryKey;autoIncrement" json:"assetId"`
	StorageKey     string     `gorm:"column:storage_key" json:"-"`
	Name           string     `gorm:"column:name" json:"name"`
	AltText        string     `gorm:"column:alt_text" json:"altText"`
	MimeType       string     `gorm:"column:mime_type" json:"mimeType"`
	ByteSize       int64      `gorm:"column:byte_size" json:"byteSize"`
	Width          int        `gorm:"column:width" json:"width"`
	Height         int        `gorm:"column:height" json:"height"`
	IsDeleted      int        `gorm:"column:is_deleted" json:"isDeleted"`
	Version        int64      `gorm:"column:version" json:"version"`
	CreateBy       string     `gorm:"column:create_by" json:"createBy"`
	CreateTime     types.Time `gorm:"column:create_time" json:"createTime"`
	UpdateBy       string     `gorm:"column:update_by" json:"updateBy"`
	UpdateTime     types.Time `gorm:"column:update_time" json:"updateTime"`
	Remark         string     `gorm:"column:remark" json:"remark"`
}

func (CmsAsset) TableName() string { return "cms_asset" }
