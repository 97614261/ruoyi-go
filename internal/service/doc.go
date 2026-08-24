// Package service 业务逻辑与事务边界。[L3]
//
// 可以引用：internal/repository、internal/model、internal/config，以及 pkg/ 下的任意包。
// 不能引用：handler、router、middleware，也不要引入 gin —— 业务层不应该知道 HTTP 的存在。
//
// 错误约定：需要展示给用户的错误用 errs.BizError，其余错误直接向上传递，
// 由 handler 边界统一转成 500 + 通用提示。
package service
