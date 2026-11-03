package model

type PortalCard struct {
	TitleParts   []PortalTextPart `gorm:"-" json:"-"`
	SummaryParts []PortalTextPart `gorm:"-" json:"-"`
	ContentId    int64
	CMSPublicSummary
	Kind      string
	ChannelId *int64
	URL       string
}
type PortalResults struct {
	Rows     []PortalCard
	Total    int64
	Channels []CmsChannel
}
