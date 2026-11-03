package model

import "ruoyi-go/pkg/types"

type CMSDownloadCategory struct {
	CategoryId int64  `gorm:"primaryKey" json:"categoryId"`
	Name       string `json:"name"`
	OrderNum   int    `json:"orderNum"`
	Enabled    bool   `json:"enabled"`
	IsDeleted  int    `json:"-"`
	Version    int64  `json:"version"`
	CMSWorkflowAudit
}

func (CMSDownloadCategory) TableName() string { return "cms_download_category" }

type CMSDownloadCategoryInput struct {
	CategoryId int64  `json:"categoryId" binding:"min=0"`
	Version    int64  `json:"version" binding:"min=0"`
	Name       string `json:"name" binding:"notblank,xss,max=60"`
	OrderNum   int    `json:"orderNum" binding:"min=0,max=9999"`
	Enabled    bool   `json:"enabled"`
}
type CMSDownloadCategoryQuery struct {
	Name string `form:"name" binding:"max=60"`
}
type CMSDownloadFile struct {
	FileId     int64  `gorm:"primaryKey" json:"fileId"`
	StorageKey string `json:"-"`
	Name       string `json:"name"`
	Extension  string `json:"extension"`
	MimeType   string `json:"mimeType"`
	ByteSize   int64  `json:"byteSize"`
	SHA256     string `gorm:"column:sha256" json:"sha256"`
	IsDeleted  int    `json:"-"`
	Version    int64  `json:"version"`
	CMSWorkflowAudit
}

func (CMSDownloadFile) TableName() string { return "cms_download_file" }

type CMSDownloadFileQuery struct {
	Name string `form:"name" binding:"max=160"`
}
type CMSDownload struct {
	DownloadId          int64      `gorm:"primaryKey" json:"downloadId"`
	WorkingRevisionId   *int64     `json:"workingRevisionId"`
	PublishedRevisionId *int64     `json:"publishedRevisionId"`
	RevisionSeq         int        `json:"revisionSeq"`
	IsPublished         int        `json:"isPublished"`
	IsDeleted           int        `json:"-"`
	PublishedAt         types.Time `json:"publishedAt"`
	Version             int64      `json:"version"`
	CMSWorkflowAudit
}

func (CMSDownload) TableName() string { return "cms_download" }

type CMSDownloadRevision struct {
	RevisionId int64  `gorm:"primaryKey" json:"revisionId"`
	DownloadId int64  `json:"downloadId"`
	RevisionNo int    `json:"revisionNo"`
	CategoryId int64  `json:"categoryId"`
	FileId     int64  `json:"fileId"`
	Title      string `json:"title"`
	Summary    string `json:"summary"`
	CMSWorkflowAudit
}

func (CMSDownloadRevision) TableName() string { return "cms_download_revision" }

type CMSDownloadView struct {
	CMSDownload
	Title           string `json:"title"`
	Summary         string `json:"summary"`
	CategoryId      int64  `json:"categoryId"`
	CategoryName    string `json:"categoryName"`
	FileId          int64  `json:"fileId"`
	FileName        string `json:"fileName"`
	Extension       string `json:"extension"`
	ByteSize        int64  `json:"byteSize"`
	CategoryEnabled bool   `json:"categoryEnabled"`
}
type CMSDownloadDetail struct {
	CMSDownload
	Draft CMSDownloadRevision `json:"draft"`
}
type CMSDownloadInput struct {
	DownloadId int64  `json:"downloadId" binding:"min=0"`
	Version    int64  `json:"version" binding:"min=0"`
	CategoryId int64  `json:"categoryId" binding:"required,gt=0"`
	FileId     int64  `json:"fileId" binding:"required,gt=0"`
	Title      string `json:"title" binding:"notblank,xss,max=120"`
	Summary    string `json:"summary" binding:"xss,max=500"`
}
type CMSDownloadQuery struct {
	Title      string `form:"title" binding:"max=120"`
	CategoryId int64  `form:"categoryId" binding:"min=0"`
	Published  *int   `form:"published" binding:"omitempty,oneof=0 1"`
}
type PortalDownload struct {
	DownloadId   int64      `json:"downloadId"`
	Title        string     `json:"title"`
	Summary      string     `json:"summary"`
	CategoryId   int64      `json:"categoryId"`
	CategoryName string     `json:"categoryName"`
	FileName     string     `json:"fileName"`
	Extension    string     `json:"extension"`
	ByteSize     int64      `json:"byteSize"`
	SHA256       string     `gorm:"column:sha256" json:"sha256"`
	PublishedAt  types.Time `json:"publishedAt"`
}
