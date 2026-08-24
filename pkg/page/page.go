// Package page 解析列表接口的分页与排序参数。
package page

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	DefaultPageNum  = 1
	DefaultPageSize = 10
	// MaxPageSize 硬上限。超出时截断而不是报错 —— 前端传大值通常是想"导出全部"，
	// 直接报错体验很差，截断能保证服务端不被拖垮。
	MaxPageSize = 100
)

// Query 已校验过的分页参数。
type Query struct {
	PageNum  int
	PageSize int
	// OrderBy 形如 "post_sort asc"，空串表示不排序。
	// 只可能来自白名单，可以安全拼进 SQL。
	OrderBy string
}

// Parse 从 query string 解析分页参数。
//
// allowedSort 是排序白名单：key 为前端传的字段名（驼峰），value 为数据库列名。
// 不在白名单里的排序字段一律忽略 —— orderByColumn 是直接拼进 SQL 的，
// 放任前端传值就是一个注入入口。传 nil 表示该接口不支持排序。
func Parse(c *gin.Context, allowedSort map[string]string) Query {
	q := Query{PageNum: DefaultPageNum, PageSize: DefaultPageSize}

	if n, err := strconv.Atoi(c.Query("pageNum")); err == nil && n > 0 {
		q.PageNum = n
	}
	if size, err := strconv.Atoi(c.Query("pageSize")); err == nil && size > 0 {
		if size > MaxPageSize {
			size = MaxPageSize
		}
		q.PageSize = size
	}

	column := c.Query("orderByColumn")
	if column == "" || len(allowedSort) == 0 {
		return q
	}
	dbColumn, ok := allowedSort[column]
	if !ok {
		return q
	}
	q.OrderBy = dbColumn + " " + direction(c.Query("isAsc"))
	return q
}

// Offset 计算 SQL OFFSET。
func (q Query) Offset() int {
	return (q.PageNum - 1) * q.PageSize
}

// direction 归一化排序方向。
//
// RuoYi 前端传的是 "ascending" / "descending"（Element Plus 表格的原始值），
// 不是 "asc" / "desc"，两种都要认。
func direction(isAsc string) string {
	switch strings.ToLower(strings.TrimSpace(isAsc)) {
	case "desc", "descending":
		return "desc"
	default:
		return "asc"
	}
}
