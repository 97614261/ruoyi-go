package model

import "ruoyi-go/pkg/types"

const PODAssetProductPage = "product-page"

// Adapted from the generated asset scaffold; ownership and disk keys are never bound from a request.
type PODAsset struct {
	AssetID        int64      `gorm:"column:asset_id;primaryKey;autoIncrement"`
	WorkspaceID    int64      `gorm:"column:workspace_id"`
	ParentAssetID  int64      `gorm:"column:parent_asset_id"`
	Source         string     `gorm:"column:source"`
	OriginalName   string     `gorm:"column:original_name"`
	MediaType      string     `gorm:"column:media_type"`
	ByteSize       int64      `gorm:"column:byte_size"`
	Width          int        `gorm:"column:width"`
	Height         int        `gorm:"column:height"`
	Revision       int64      `gorm:"column:revision"`
	DelFlag        string     `gorm:"column:del_flag"`
	CreateBy       string     `gorm:"column:create_by"`
	CreateTime     types.Time `gorm:"column:create_time"`
	UpdateBy       string     `gorm:"column:update_by"`
	UpdateTime     types.Time `gorm:"column:update_time"`
	Remark         string     `gorm:"column:remark"`
	ReferenceCount int64      `gorm:"column:reference_count;->;-:migration"`
}
type PODAssetFile struct {
	AssetID    int64      `gorm:"column:asset_id;primaryKey"`
	Variant    string     `gorm:"column:variant;primaryKey"`
	StorageKey string     `gorm:"column:storage_key"`
	MediaType  string     `gorm:"column:media_type"`
	ByteSize   int64      `gorm:"column:byte_size"`
	SHA256     string     `gorm:"column:sha256"`
	Width      int        `gorm:"column:width"`
	Height     int        `gorm:"column:height"`
	CreateBy   string     `gorm:"column:create_by"`
	CreateTime types.Time `gorm:"column:create_time"`
	UpdateBy   string     `gorm:"column:update_by"`
	UpdateTime types.Time `gorm:"column:update_time"`
	Remark     string     `gorm:"column:remark"`
}
type PODProjectAssetRef struct {
	ProjectID   int64      `gorm:"column:project_id;primaryKey"`
	NodeID      string     `gorm:"column:node_id;primaryKey"`
	AssetID     int64      `gorm:"column:asset_id;primaryKey"`
	WorkspaceID int64      `gorm:"column:workspace_id"`
	CreateBy    string     `gorm:"column:create_by"`
	CreateTime  types.Time `gorm:"column:create_time"`
	UpdateBy    string     `gorm:"column:update_by"`
	UpdateTime  types.Time `gorm:"column:update_time"`
	Remark      string     `gorm:"column:remark"`
}
type PODAssetView struct {
	AssetID        int64      `json:"assetId,string"`
	WorkspaceID    int64      `json:"workspaceId,string"`
	ParentAssetID  int64      `json:"parentAssetId,string"`
	Source         string     `json:"source"`
	OriginalName   string     `json:"originalName"`
	MediaType      string     `json:"mediaType"`
	ByteSize       int64      `json:"byteSize"`
	Width          int        `json:"width"`
	Height         int        `json:"height"`
	Revision       int64      `json:"revision,string"`
	Deleted        bool       `json:"deleted"`
	ReferenceCount int64      `json:"referenceCount"`
	CreateTime     types.Time `json:"createTime"`
}
type PODAssetLimits struct {
	MaxUploadBytes   int64 `json:"maxUploadBytes"`
	MaxPixels        int64 `json:"maxPixels"`
	MaxDimension     int   `json:"maxDimension"`
	MaxImagesPerNode int   `json:"maxImagesPerNode"`
}
