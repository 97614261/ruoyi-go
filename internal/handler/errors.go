package handler

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/response"
)

// fail 把 error 翻译成响应。
//
// 业务错误（*errs.BizError）的 Msg 直接展示给用户；
// 其他错误一律按系统异常处理，详情只进日志 ——
// 不能把 SQL 报错、堆栈、内部路径漏到响应里。
func fail(c *gin.Context, err error) {
	if bizErr := errs.As(err); bizErr != nil {
		if bizErr.Cause != nil {
			slog.Warn("业务处理失败", "path", c.Request.URL.Path, "err", bizErr.Cause)
		}
		response.FailCode(c, bizErr.Code, bizErr.Msg)
		return
	}
	slog.Error("系统异常", "path", c.Request.URL.Path, "err", err)
	response.FailCode(c, response.CodeError, "系统异常，请联系管理员")
}

// failDownload 是 fail 的下载接口版本。
//
// 导出、下载模板这类返回文件的接口出错时必须用它，
// 原因见 response.FailDownload 的注释（Content-Type 不能带 charset）。
func failDownload(c *gin.Context, err error) {
	if bizErr := errs.As(err); bizErr != nil {
		if bizErr.Cause != nil {
			slog.Warn("导出失败", "path", c.Request.URL.Path, "err", bizErr.Cause)
		}
		response.FailDownload(c, bizErr.Code, bizErr.Msg)
		return
	}
	slog.Error("导出异常", "path", c.Request.URL.Path, "err", err)
	response.FailDownload(c, response.CodeError, "导出失败，请联系管理员")
}
