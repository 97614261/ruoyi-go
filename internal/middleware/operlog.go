package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/types"
)

// 各字段的截断上限，对应 sys_oper_log 的列长（varchar(2000)）。
const (
	maxParamLength  = 2000
	maxResultLength = 2000
	maxErrorLength  = 2000
)

// 脱敏规则。**两种格式都要覆盖**：
//
//	JSON 体：{"password":"123456"}
//	查询串：?oldPassword=123456&newPassword=abc
//
// 只做 JSON 是不够的 —— /system/user/profile/updatePwd 的参数就在 query 里，
// 漏掉的话明文密码会原样落进一张可以被导出的表。
var (
	sensitiveJSON  = regexp.MustCompile(`"(password|oldPassword|newPassword|confirmPassword)"\s*:\s*"[^"]*"`)
	sensitiveQuery = regexp.MustCompile(`(?i)\b(password|oldPassword|newPassword|confirmPassword)=[^&\s]*`)
)

// OperLog 记录操作日志。
//
// 对应 Java 版的 @Log 注解 + LogAspect：
//
//	post.POST("", middleware.OperLog("岗位管理", model.BusinessTypeInsert), handler.PostAdd)
//
// 只挂在增删改授权这类写操作上，查询不挂 —— Java 也是这么用的，
// 给查询也记日志会让这张表迅速膨胀到没法用。
func OperLog(title string, businessType int) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		requestBody := captureRequestBody(c)

		writer := &responseCapture{ResponseWriter: c.Writer, body: &bytes.Buffer{}}
		c.Writer = writer

		c.Next()

		// 必须在启动 goroutine 前把数据取完：gin.Context 是池化复用的，
		// 请求结束后再碰 c 就是读到别人的请求。
		record := buildOperLog(c, title, businessType, requestBody, writer, start)

		// 异步写库：日志不该拖慢请求，也不该因为写失败影响业务结果。
		// 必须用新的 context —— 请求的 ctx 在 handler 返回后就被取消了。
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := service.RecordOperLog(ctx, record); err != nil {
				slog.Warn("记录操作日志失败", "title", title, "err", err)
			}
		}()
	}
}

// captureRequestBody 读出请求体供记录，并把它还给 handler。
//
// 【关键】必须完整读取再原样塞回去。早先的实现用 LimitReader 只读前 4KB，
// 结果超过 4KB 的请求体（比如公告的富文本正文）被截断，
// handler 拿到残缺 JSON 直接解析失败 —— 表现为"保存没反应"，极难查。
//
// multipart 直接跳过：那是文件上传，整个读进内存毫无意义，
// 而且 handler 还要再读一遍，等于双倍内存。
func captureRequestBody(c *gin.Context) string {
	if c.Request.Body == nil || c.Request.Method == http.MethodGet {
		return ""
	}
	contentType := c.GetHeader("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return "[multipart 文件上传，参数未记录]"
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return ""
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	return string(body)
}

func buildOperLog(c *gin.Context, title string, businessType int,
	requestBody string, writer *responseCapture, start time.Time) *model.SysOperLog {

	record := &model.SysOperLog{
		Title:         title,
		BusinessType:  businessType,
		RequestMethod: c.Request.Method,
		OperatorType:  1, // 后台用户
		OperURL:       truncate(c.Request.URL.Path, 255),
		OperIP:        c.ClientIP(),
		OperTime:      types.Now(),
		CostTime:      time.Since(start).Milliseconds(),
		Status:        model.OperStatusSuccess,
	}

	if loginUser := CurrentUser(c); loginUser != nil && loginUser.User != nil {
		record.OperName = loginUser.User.UserName
		if loginUser.User.Dept != nil {
			record.DeptName = loginUser.User.Dept.DeptName
		}
	}

	// 请求体为空时退回查询串（PUT ?a=b 这类接口）
	params := requestBody
	if params == "" {
		params = c.Request.URL.RawQuery
	}
	record.OperParam = truncate(desensitize(params), maxParamLength)

	responseBody := writer.body.String()
	record.JSONResult = truncate(responseBody, maxResultLength)

	// 业务失败也要标记成异常，否则日志里全是"正常"，排查时毫无价值
	if code, msg := extractCode(responseBody); code != 0 && code != 200 {
		record.Status = model.OperStatusFail
		record.ErrorMsg = truncate(msg, maxErrorLength)
	}
	if writer.Status() >= http.StatusBadRequest {
		record.Status = model.OperStatusFail
	}
	return record
}

// responseCapture 旁路记录响应体。
//
// 只截 JSON 响应：导出接口返回的是 xlsx 二进制，记进 varchar 列
// 既没有可读性，非法 UTF-8 字节还可能让 INSERT 直接报错。
type responseCapture struct {
	gin.ResponseWriter
	body    *bytes.Buffer
	checked bool
	skip    bool
}

func (w *responseCapture) shouldCapture() bool {
	if !w.checked {
		w.checked = true
		contentType := w.Header().Get("Content-Type")
		w.skip = contentType != "" && !strings.Contains(contentType, "json")
	}
	return !w.skip && w.body.Len() < maxResultLength
}

func (w *responseCapture) Write(b []byte) (int, error) {
	if w.shouldCapture() {
		w.body.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

func (w *responseCapture) WriteString(s string) (int, error) {
	if w.shouldCapture() {
		w.body.WriteString(s)
	}
	return w.ResponseWriter.WriteString(s)
}

// extractCode 从响应体里取业务 code 和 msg。
func extractCode(body string) (int, string) {
	if body == "" || body[0] != '{' {
		return 0, ""
	}
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return 0, ""
	}
	return result.Code, result.Msg
}

// desensitize 把参数里的密码替换掉，JSON 和查询串两种格式都处理。
func desensitize(params string) string {
	params = sensitiveJSON.ReplaceAllString(params, `"$1":"******"`)
	return sensitiveQuery.ReplaceAllString(params, `$1=******`)
}

// truncate 按字符截断到指定字节数。
//
// 直接按字节切会切断多字节字符，产生非法 UTF-8 导致 MySQL 插入失败；
// 这里从字节位置回退到最近的字符边界。
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8StartsAt(s, cut) {
		cut--
	}
	return s[:cut]
}

// utf8StartsAt 判断字节位置 i 是否是一个字符的起始。
//
// UTF-8 的续字节高两位固定是 10，据此回退即可。
func utf8StartsAt(s string, i int) bool {
	return i >= len(s) || s[i]&0xC0 != 0x80
}
