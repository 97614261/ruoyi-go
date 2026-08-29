package middleware

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"ruoyi-go/pkg/response"
)

const requestBodyCacheKey = "ruoyi.requestBody"

// RequestBodyLimit 限制普通 JSON/form 请求体，并缓存完整内容供后续中间件复用。
// multipart 文件上传使用独立的文件大小限制，不在这里读取。
func RequestBodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body == nil || c.Request.Body == http.NoBody ||
			isMultipartRequest(c.GetHeader("Content-Type")) {
			c.Next()
			return
		}

		limited := http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		body, err := io.ReadAll(limited)
		if err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				response.New(response.CodePayloadTooLarge,
					fmt.Sprintf("请求体大小超过 %d MB", maxBytes>>20)).
					JSONStatus(c, http.StatusRequestEntityTooLarge)
			} else {
				response.Fail(c, "读取请求体失败")
			}
			c.Abort()
			return
		}

		c.Set(requestBodyCacheKey, body)
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		c.Next()
	}
}

func cachedRequestBody(c *gin.Context) ([]byte, bool) {
	value, ok := c.Get(requestBodyCacheKey)
	if !ok {
		return nil, false
	}
	body, ok := value.([]byte)
	return body, ok
}

func isMultipartRequest(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && strings.EqualFold(mediaType, "multipart/form-data")
}
