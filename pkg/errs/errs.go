// Package errs 业务错误类型。
//
// 约定：service 层返回 *BizError 表示"可以直接展示给用户"的错误；
// 其他 error 一律视为系统异常，在 handler 边界统一转成 500 + 通用提示，
// 详细信息只进日志，不进响应。
package errs

import (
	"errors"
	"fmt"
)

// BizError 业务错误，Code 对应响应体里的 code。
type BizError struct {
	Code int
	Msg  string
	// Cause 保留底层错误用于日志，不会出现在响应里
	Cause error
}

func (e *BizError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Msg, e.Cause)
	}
	return e.Msg
}

func (e *BizError) Unwrap() error { return e.Cause }

// New 构造一个 code=500 的业务错误。
func New(msg string) *BizError {
	return &BizError{Code: 500, Msg: msg}
}

// Newf 同 New，支持格式化。
func Newf(format string, a ...any) *BizError {
	return &BizError{Code: 500, Msg: fmt.Sprintf(format, a...)}
}

// WithCode 构造指定 code 的业务错误。
func WithCode(code int, msg string) *BizError {
	return &BizError{Code: code, Msg: msg}
}

// Wrap 在保留底层错误的同时给出面向用户的提示。
func Wrap(cause error, msg string) *BizError {
	return &BizError{Code: 500, Msg: msg, Cause: cause}
}

// 常用哨兵错误
var (
	ErrUnauthorized = &BizError{Code: 401, Msg: "认证失败，无法访问系统资源"}
	ErrForbidden    = &BizError{Code: 403, Msg: "没有权限，请联系管理员授权"}
	ErrNotFound     = &BizError{Code: 500, Msg: "数据不存在"}
)

// As 提取 *BizError，不是业务错误时返回 nil。
func As(err error) *BizError {
	var be *BizError
	if errors.As(err, &be) {
		return be
	}
	return nil
}
