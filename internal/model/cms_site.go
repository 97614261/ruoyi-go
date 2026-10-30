package model

import "ruoyi-go/pkg/types"

type SiteLink struct {
	Placement string `json:"placement"`
	Label     string `json:"label"`
	ContentId int64  `json:"contentId"`
	URL       string `json:"url"`
}
type SiteBanner struct {
	Title     string `json:"title"`
	AssetId   int64  `json:"assetId"`
	ContentId int64  `json:"contentId"`
	Enabled   bool   `json:"enabled"`
	URL       string `json:"-"`
}
type SiteBlock struct {
	Kind      string `json:"kind"`
	ChannelId int64  `json:"channelId"`
	Count     int    `json:"count"`
	Enabled   bool   `json:"enabled"`
}
type SiteConfig struct {
	ThemeVersion string       `json:"-"`
	IconAssetId  int64        `json:"iconAssetId"`
	FontScale    string       `json:"fontScale"`
	FrameWidth   string       `json:"frameWidth"`
	Radius       string       `json:"radius"`
	Theme        string       `json:"theme"`
	SiteName     string       `json:"siteName"`
	Slogan       string       `json:"slogan"`
	Palette      string       `json:"palette"`
	Density      string       `json:"density"`
	LogoAssetId  int64        `json:"logoAssetId"`
	Footer       string       `json:"footer"`
	Phone        string       `json:"phone"`
	Email        string       `json:"email"`
	Navigation   []SiteLink   `json:"navigation"`
	Banners      []SiteBanner `json:"banners"`
	Blocks       []SiteBlock  `json:"blocks"`
}
type CMSSite struct {
	SiteId             int64  `gorm:"primaryKey"`
	DraftJSON          string `gorm:"column:draft_json" json:"-"`
	PublishedReleaseId *int64 `json:"publishedReleaseId"`
	Version            int64  `json:"version"`
	CreateBy           string
	CreateTime         types.Time
	UpdateBy           string
	UpdateTime         types.Time
	Remark             string
}

func (CMSSite) TableName() string { return "cms_site" }

type SiteRelease struct {
	ReleaseId    int64      `gorm:"primaryKey" json:"releaseId"`
	SiteId       int64      `json:"-"`
	ConfigJSON   string     `gorm:"column:config_json" json:"-"`
	ThemeVersion string     `json:"themeVersion"`
	CreateBy     string     `json:"createBy"`
	CreateTime   types.Time `json:"createTime"`
	UpdateBy     string     `json:"-"`
	UpdateTime   types.Time `json:"-"`
	Remark       string     `json:"remark"`
}

func (SiteRelease) TableName() string { return "cms_site_release" }

type SiteInput struct {
	Version int64      `json:"version" binding:"required,min=1"`
	Config  SiteConfig `json:"config"`
	Remark  string     `json:"remark" binding:"xss,max=500"`
}
type SiteAction struct {
	Version   int64 `json:"version" binding:"required,min=1"`
	ReleaseId int64 `json:"releaseId" binding:"min=0"`
}
type SiteState struct {
	Version            int64      `json:"version"`
	PublishedReleaseId *int64     `json:"publishedReleaseId"`
	Config             SiteConfig `json:"config"`
}
