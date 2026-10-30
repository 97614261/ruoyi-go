package model

import "ruoyi-go/pkg/types"

type CMSContent struct {
	ContentId           int64      `gorm:"primaryKey" json:"contentId"`
	Kind                string     `json:"kind"`
	Slug                string     `json:"slug"`
	WorkingRevisionId   *int64     `json:"workingRevisionId"`
	PublishedRevisionId *int64     `json:"publishedRevisionId"`
	RevisionSeq         int        `json:"revisionSeq"`
	IsPublished         int        `json:"isPublished"`
	EverPublished       int        `json:"everPublished"`
	IsDeleted           int        `json:"-"`
	PublishedAt         types.Time `json:"publishedAt"`
	Version             int64      `json:"version"`
	CreateBy            string     `json:"createBy"`
	CreateTime          types.Time `json:"createTime"`
	UpdateBy            string     `json:"updateBy"`
	UpdateTime          types.Time `json:"updateTime"`
	Remark              string     `json:"remark"`
}

func (CMSContent) TableName() string { return "cms_content" }

type CMSRevision struct {
	ProductCode    string     `json:"productCode"`
	ProductParams  string     `json:"productParams"`
	GalleryIds     []int64    `gorm:"serializer:json" json:"galleryIds"`
	RevisionId     int64      `gorm:"primaryKey" json:"revisionId"`
	ContentId      int64      `json:"contentId"`
	RevisionNo     int        `json:"revisionNo"`
	ChannelId      *int64     `json:"channelId"`
	Title          string     `json:"title"`
	Summary        string     `json:"summary"`
	BodyHTML       string     `gorm:"column:body_html" json:"bodyHtml"`
	Author         string     `json:"author"`
	CoverAssetId   *int64     `json:"coverAssetId"`
	CoverAltText   string     `json:"coverAltText"`
	SeoTitle       string     `json:"seoTitle"`
	SeoDescription string     `json:"seoDescription"`
	CreateBy       string     `json:"createBy"`
	CreateTime     types.Time `json:"createTime"`
	UpdateBy       string     `json:"updateBy"`
	UpdateTime     types.Time `json:"updateTime"`
	Remark         string     `json:"remark"`
}

func (CMSRevision) TableName() string { return "cms_content_revision" }

type CMSContentView struct {
	CMSContent
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	ChannelId *int64 `json:"channelId"`
}
type CMSContentDetail struct {
	CMSContent
	Draft CMSRevision `json:"draft"`
}
type CMSDraftInput struct {
	ProductCode    string  `json:"productCode" binding:"xss,max=80"`
	ProductParams  string  `json:"productParams" binding:"xss,max=2000"`
	GalleryIds     []int64 `json:"galleryIds" binding:"max=16,dive,gt=0"`
	ContentId      int64   `json:"contentId" binding:"min=0"`
	Version        int64   `json:"version" binding:"min=0"`
	Slug           string  `json:"slug" binding:"required,max=80"`
	ChannelId      int64   `json:"channelId" binding:"min=0"`
	Title          string  `json:"title" binding:"notblank,xss,max=120"`
	Summary        string  `json:"summary" binding:"xss,max=500"`
	BodyHTML       string  `json:"bodyHtml" binding:"required"`
	Author         string  `json:"author" binding:"xss,max=60"`
	CoverAssetId   int64   `json:"coverAssetId" binding:"min=0"`
	CoverAltText   string  `json:"coverAltText" binding:"xss,max=200"`
	SeoTitle       string  `json:"seoTitle" binding:"xss,max=150"`
	SeoDescription string  `json:"seoDescription" binding:"xss,max=300"`
	Remark         string  `json:"remark" binding:"xss,max=500"`
}
type CMSContentQuery struct {
	Title     string `form:"title" binding:"max=120"`
	Published *int   `form:"published" binding:"omitempty,oneof=0 1"`
}
type CMSContentAction struct {
	Version    int64 `json:"version" binding:"required,min=1"`
	RevisionId int64 `json:"revisionId" binding:"min=0"`
}

// CMSPublicSummary intentionally excludes draft pointers, audit identities and body HTML.
type CMSPublicSummary struct {
	Slug         string     `json:"slug"`
	Title        string     `json:"title"`
	Summary      string     `json:"summary"`
	PublishedAt  types.Time `json:"publishedAt"`
	CoverAssetId *int64     `json:"coverAssetId"`
}
