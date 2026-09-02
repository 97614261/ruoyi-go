package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"ruoyi-go/pkg/response"
)

// exportWaitTimeout 拿不到名额时最多等多久。
//
// 【为什么要等一下而不是立刻拒绝】一次 10 万行的导出约 0.8 秒。
// 两个人前后脚点导出是很正常的事，立刻拒绝会让人莫名其妙。
// 等几秒能把这种小波峰吸收掉，又不会让请求无限堆积。
const exportWaitTimeout = 5 * time.Second

// exportSlots 导出名额。容量由 service.MaxConcurrentExports 决定，
// 但那是 L3、这里是 L4，不能反向引用，所以在 InitExportLimit 里注入。
var exportSlots = make(chan struct{}, 1)

// InitExportLimit 设置同时导出的上限，必须在注册路由前调用。
func InitExportLimit(max int) {
	if max <= 0 {
		max = 1
	}
	exportSlots = make(chan struct{}, max)
}

// ExportLimit 限制同时进行的导出数量。
//
// 【为什么需要】实测每 10 万行导出占约 90 MB 堆峰值（见 service.MaxExportRows）。
// 只限行数不限并发的话，行数上限就是摆设 —— 10 个人各导 10 万行就是 900 MB。
//
// 【错误响应必须走 FailDownload】前端 download() 靠 blobValidate() 区分文件和错误，
// 而它是严格相等比较 data.type !== 'application/json'。
// 用 c.JSON 写出来的是 application/json; charset=utf-8，比较不相等，
// 前端会把这段错误当成文件存下来 —— 用户拿到一个打不开的 .xlsx，
// 而且看不到任何提示。导出接口上的每一条错误路径都要走这里。
func ExportLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		timer := time.NewTimer(exportWaitTimeout)
		defer timer.Stop()

		select {
		case exportSlots <- struct{}{}:
			defer func() { <-exportSlots }()
			c.Next()

		case <-timer.C:
			slog.WarnContext(c.Request.Context(), "导出被限流",
				"path", c.Request.URL.Path, "clientIP", c.ClientIP())
			response.FailDownload(c, response.CodeError,
				"当前正在导出的任务较多，服务器暂时无法处理新的导出请求。"+
					"请等待十几秒后重试，或缩小查询范围让导出更快完成。")
			c.Abort()

		case <-c.Request.Context().Done():
			// 用户等不及自己关掉了页面，不用再往下走
			c.Abort()
		}
	}
}
