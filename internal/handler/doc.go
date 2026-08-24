// Package handler HTTP 处理器。[L4]
//
// 职责限于：绑定参数、调用 service、写响应。
//
// 【硬约束】
//   - 不准直接引用 repository，必须经 service
//   - 不准手写 c.JSON，一律走 pkg/response
//   - 注意平铺字段：login 的 token、getInfo 的 user/roles/permissions
//     不在 data 里，用 response.New(...).Put(...) 构造
package handler
