package excelx

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
)

// ContentType xlsx 的 MIME 类型。
const ContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// WriteResponse 把数据导出成 xlsx 并写入响应。
//
// 【重要】先在内存里构建完整文件再写响应头。
// 若边生成边写，中途出错时响应头已经发出，前端拿到的是半个损坏的 xlsx；
// 而 RuoYi 前端的 download() 会先判断响应是不是 blob，不是就当 JSON 错误提示 ——
// 只有完全不写响应体，调用方才能改回 JSON 错误。
//
// 返回 error 时调用方应当返回 JSON 错误，此函数保证未写入任何响应内容。
func WriteResponse(c *gin.Context, filename, sheetName string, rows any) error {
	var buf bytes.Buffer
	if err := Export(&buf, sheetName, rows); err != nil {
		return err
	}

	// 前端 saveAs 用的是自己拼的文件名，这里的 filename 只是兜底（直接访问接口时生效）。
	// 中文文件名按 RFC 5987 编码，否则部分浏览器会乱码。
	c.Header("Content-Disposition", fmt.Sprintf(
		`attachment; filename="export.xlsx"; filename*=UTF-8''%s`, url.PathEscape(filename)))
	c.Header("Content-Type", ContentType)
	c.Data(http.StatusOK, ContentType, buf.Bytes())
	return nil
}
