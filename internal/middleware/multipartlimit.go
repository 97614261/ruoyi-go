package middleware

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"ruoyi-go/pkg/response"
)

// MultipartBodyLimit enforces a total multipart request cap before handlers access files.
func MultipartBodyLimit(maxBytes, maxMemory int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isMultipartRequest(c.GetHeader("Content-Type")) {
			c.Next()
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		if err := c.Request.ParseMultipartForm(maxMemory); err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				response.New(response.CodePayloadTooLarge,
					fmt.Sprintf("上传请求大小超过 %d MB", maxBytes>>20)).
					JSONStatus(c, http.StatusRequestEntityTooLarge)
			} else {
				response.Fail(c, "解析上传请求失败")
			}
			c.Abort()
			return
		}
		if c.Request.MultipartForm != nil {
			defer c.Request.MultipartForm.RemoveAll()
		}
		c.Next()
	}
}
