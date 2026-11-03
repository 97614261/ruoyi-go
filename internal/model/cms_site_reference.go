package model

type CMSSiteReferenceQuery struct {
	Type string `form:"type" binding:"required,oneof=channel content"`
	Kind string `form:"kind" binding:"omitempty,oneof=ARTICLE PRODUCT PAGE"`
	Name string `form:"name" binding:"max=120"`
	IDs  string `form:"ids" binding:"max=2100"`
}
type CMSSiteReference struct {
	RefID   int64  `json:"refId"`
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	Slug    string `json:"-"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
}
