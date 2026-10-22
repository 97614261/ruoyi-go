package model

import "ruoyi-go/pkg/types"

// ErpShop generated from erp_shop; extended with versioned, owner-only writes.
type ErpShop struct {
	TenantID    int64      `gorm:"column:tenant_id" json:"-"`
	ShopId      int64      `gorm:"column:shop_id;primaryKey;autoIncrement" json:"shopId"`
	ShopCode    string     `gorm:"column:shop_code" json:"shopCode"`
	ShopName    string     `gorm:"column:shop_name" json:"shopName"`
	ChannelCode string     `gorm:"column:channel_code" json:"channelCode"`
	SiteCode    string     `gorm:"column:site_code" json:"siteCode"`
	Currency    string     `gorm:"column:currency" json:"currency"`
	Status      string     `gorm:"column:status" json:"status"`
	Version     int64      `gorm:"column:version" json:"version"`
	CreateBy    string     `gorm:"column:create_by" json:"createBy"`
	CreateTime  types.Time `gorm:"column:create_time" json:"createTime"`
	UpdateBy    string     `gorm:"column:update_by" json:"updateBy"`
	UpdateTime  types.Time `gorm:"column:update_time" json:"updateTime"`
	Remark      string     `gorm:"column:remark" json:"remark"`
}

func (ErpShop) TableName() string { return "erp_shop" }

// Write DTO deliberately excludes tenant, primary key and audit fields.
type ERPShopWrite struct {
	ShopCode    string `json:"shopCode" binding:"required,max=64"`
	ShopName    string `json:"shopName" binding:"required,max=120"`
	ChannelCode string `json:"channelCode" binding:"required,max=32"`
	SiteCode    string `json:"siteCode" binding:"required,max=32"`
	Currency    string `json:"currency" binding:"required,len=3"`
	Status      string `json:"status" binding:"required,oneof=0 1"`
	Version     int64  `json:"version"`
	Remark      string `json:"remark" binding:"max=500"`
}
type ErpShopQuery struct {
	ShopCode    string `form:"shopCode" binding:"max=64"`
	ShopName    string `form:"shopName" binding:"max=120"`
	ChannelCode string `form:"channelCode" binding:"max=32"`
	Status      string `form:"status" binding:"omitempty,oneof=0 1"`
}

var ErpShopSortColumns = map[string]string{
	"shopId": "shop_id", "shopCode": "shop_code", "shopName": "shop_name",
	"channelCode": "channel_code", "siteCode": "site_code", "currency": "currency", "status": "status",
}

type ERPChannel struct {
	ChannelCode string   `json:"channelCode"`
	ChannelName string   `json:"channelName"`
	SupportsAPI bool     `json:"supportsApi"`
	Sources     []string `json:"sources"`
	Description string   `json:"description"`
}
type ERPShopAccess struct {
	ERPRecord
	AccessID int64 `gorm:"column:access_id;primaryKey;autoIncrement" json:"accessId"`
	UserID   int64 `gorm:"column:user_id" json:"userId"`
	ShopID   int64 `gorm:"column:shop_id" json:"shopId"`
}

func (ERPShopAccess) TableName() string { return "erp_shop_access" }

type ERPShopPolicy struct {
	ERPRecord
	PolicyID int64 `gorm:"column:policy_id;primaryKey;autoIncrement" json:"policyId"`
	UserID   int64 `gorm:"column:user_id" json:"userId"`
	Version  int64 `gorm:"column:version" json:"version"`
}

func (ERPShopPolicy) TableName() string { return "erp_shop_policy" }

type ERPShopAccessRequest struct {
	Version int64    `json:"version" binding:"required,min=1"`
	ShopIDs *[]int64 `json:"shopIds" binding:"required,max=200,dive,min=1"`
}
type ERPShopAccessView struct {
	UserID  int64   `json:"userId"`
	ShopIDs []int64 `json:"shopIds"`
	Version int64   `json:"version"`
}
