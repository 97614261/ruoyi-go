package model

// CMSAssetStats describes live catalog rows, not disk quota or backup size.
type CMSAssetStats struct {
	Total     int64 `json:"total"`
	ByteSize  int64 `json:"byteSize"`
	MaxWidth  int   `json:"maxWidth"`
	MaxHeight int   `json:"maxHeight"`
}
