// Package response 统一响应封装，对齐 RuoYi 前端的契约。
//
// 【关键】RuoYi 的 AjaxResult 继承自 HashMap，扩展字段是**平铺在顶层**的，
// 不是嵌在 data 里。TableDataInfo 的 total/rows 同样平铺。
// 所以这里用 map 实现而不是 struct —— 不要"优化"成 struct，会破坏契约。
//
// 三种形态：
//
//	普通  {"code":200,"msg":"操作成功","data":{...}}   data 为 nil 时整个键不出现
//	分页  {"code":200,"msg":"查询成功","total":100,"rows":[...]}
//	平铺  {"code":200,"msg":"操作成功","token":"..."}  见 docs/CONVENTIONS.md 第一节
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// 业务状态码。注意与 HTTP 状态码无关，HTTP 恒为 200。
const (
	CodeSuccess      = 200
	CodeUnauthorized = 401
	CodeForbidden    = 403
	CodeError        = 500
)

const (
	MsgSuccess = "操作成功"
	MsgQuery   = "查询成功"
)

// Result 对应 AjaxResult。
type Result map[string]any

// New 构造一个只含 code/msg 的结果。
func New(code int, msg string) Result {
	return Result{"code": code, "msg": msg}
}

// Put 追加平铺字段。
//
// 用于 login 的 token、getInfo 的 user/roles/permissions、
// captchaImage 的 uuid/img 等——这些字段前端是从顶层读的。
//
//	response.New(CodeSuccess, MsgSuccess).Put("token", tok).JSON(c)
func (r Result) Put(key string, value any) Result {
	r[key] = value
	return r
}

// JSON 写出响应。HTTP 状态码恒为 200，错误通过 body 里的 code 表达。
func (r Result) JSON(c *gin.Context) {
	c.JSON(http.StatusOK, r)
}

// Ok 返回 {"code":200,"msg":"操作成功"}。
func Ok(c *gin.Context) {
	New(CodeSuccess, MsgSuccess).JSON(c)
}

// OkMsg 成功并自定义提示语。
func OkMsg(c *gin.Context, msg string) {
	New(CodeSuccess, msg).JSON(c)
}

// OkData 成功并携带 data。
//
// data 为 nil 时不输出 data 键（与 Java 版 AjaxResult 一致，
// 前端某些地方用 `if (res.data)` 判断，输出 "data":null 会导致行为差异）。
//
// 注意：传入"值为 nil 的接口"（如 (*User)(nil)）不会被识别为 nil，
// 这与 Java 版行为略有出入，调用方应传真正的 nil。
func OkData(c *gin.Context, data any) {
	r := New(CodeSuccess, MsgSuccess)
	if data != nil {
		r["data"] = data
	}
	r.JSON(c)
}

// Fail 返回 code=500 的业务错误。
func Fail(c *gin.Context, msg string) {
	New(CodeError, msg).JSON(c)
}

// FailCode 返回指定 code 的错误，用于 401/403。
func FailCode(c *gin.Context, code int, msg string) {
	New(code, msg).JSON(c)
}

// Page 对应 TableDataInfo：total 和 rows 平铺在顶层，没有 data 包裹。
//
// rows 为 nil 时输出空数组而不是 null，避免前端表格组件报错。
func Page(c *gin.Context, rows any, total int64) {
	if rows == nil {
		rows = []any{}
	}
	Result{
		"code":  CodeSuccess,
		"msg":   MsgQuery,
		"rows":  rows,
		"total": total,
	}.JSON(c)
}
