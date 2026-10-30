package model

type PortalCard struct {
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
