package response

import (
	"github.com/gin-gonic/gin"
	"mime"
)

// FileBytes emits only a fully prepared file. Callers return FailDownload before this on any validation error.
func FileBytes(c *gin.Context, filename, contentType string, data []byte) {
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(200, contentType, data)
}
