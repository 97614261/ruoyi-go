package response

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

// FailDownload 文件下载类接口专用的错误响应。
//
// 【必须用它，不能用 Fail / FailCode】
//
// RuoYi 前端的 download() 靠 blobValidate() 区分"文件"和"错误"：
//
//	export function blobValidate(data) {
//	  return data.type !== 'application/json'   // 严格相等
//	}
//
// gin 的 c.JSON 写的是 "application/json; charset=utf-8"，
// 严格比较不相等 → 前端当成文件 → saveAs 存下一个内容是 JSON 的 .xlsx。
// 用户拿到打不开的文件，且**看不到任何错误提示**。
//
// Spring 返回的是不带 charset 的 "application/json"，所以 Java 版没这个问题。
// 这里用 c.Data 直接指定 Content-Type，绕开 gin 的默认值。
func FailDownload(c *gin.Context, code int, msg string) {
	body, err := json.Marshal(Result{"code": code, "msg": msg})
	if err != nil {
		// 理论上不可能，兜底成最简单的合法 JSON
		body = []byte(`{"code":500,"msg":"导出失败"}`)
	}
	c.Data(http.StatusOK, "application/json", body)
}
