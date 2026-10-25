package middleware

import "github.com/gin-gonic/gin"

// DownloadErrorJSON runs before authentication/body limits/recovery for an
// explicit binary route. Successful exporters overwrite the content type;
// early JSON refusals remain exact application/json for blobValidate().
func DownloadErrorJSON(path string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == path {
			c.Header("Content-Type", "application/json")
			c.Header("Cache-Control", "no-store")
		}
		c.Next()
	}
}
