package model

type CMSChannelInput struct {
	ChannelId      int64  `json:"channelId" binding:"min=0"`
	ParentId       int64  `json:"parentId" binding:"min=0"`
	Kind           string `json:"kind" binding:"required,oneof=ARTICLE PRODUCT"`
	Name           string `json:"name" binding:"notblank,xss,max=60"`
	Slug           string `json:"slug" binding:"required,max=80"`
	OrderNum       int    `json:"orderNum" binding:"min=0,max=100000"`
	Status         string `json:"status" binding:"required,oneof=0 1"`
	CoverAssetId   int64  `json:"coverAssetId" binding:"min=0"`
	SeoTitle       string `json:"seoTitle" binding:"xss,max=150"`
	SeoDescription string `json:"seoDescription" binding:"xss,max=300"`
	Remark         string `json:"remark" binding:"xss,max=500"`
	Version        int64  `json:"version" binding:"min=0"`
}
type CMSChannelQuery struct {
	ParentId *int64 `form:"parentId" binding:"omitempty,min=0"`
	Kind     string `form:"kind" binding:"omitempty,oneof=ARTICLE PRODUCT"`
	Name     string `form:"name" binding:"max=60"`
	Status   string `form:"status" binding:"omitempty,oneof=0 1"`
}
type CMSAssetQuery struct {
	Name string `form:"name" binding:"max=150"`
}
type CMSAssetInput struct {
	Name    string `json:"name" binding:"notblank,xss,max=150"`
	AltText string `json:"altText" binding:"xss,max=200"`
	Remark  string `json:"remark" binding:"xss,max=500"`
	Version int64  `json:"version" binding:"required,min=1"`
}
type CMSVersion struct {
	Version int64 `json:"version" binding:"required,min=1"`
}
type CMSAssetReference struct {
	RefId     int64  `gorm:"column:ref_id" json:"refId"`
	OwnerType string `gorm:"column:owner_type" json:"ownerType"`
	OwnerId   int64  `gorm:"column:owner_id" json:"ownerId"`
	OwnerName string `gorm:"column:owner_name" json:"ownerName"`
}
