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

// Stable 拼出**行序确定**的排序表达式，所有分页查询都必须用它。
//
//	fallback   前端没指定排序时用的完整排序，可以是多列、可以带方向，
//	           如 "post_id"、"info_id DESC"、"dict_sort, dict_code"
//	tiebreaker 唯一列（通常是主键），追加在用户指定的排序后面保证唯一，
//	           **只能是单列、不带方向**，如 "post_id"、"r.role_id"
//
// 行为：
//
//	前端没传 orderByColumn  ->  fallback
//	前端传了 createTime     ->  "create_time asc, <tiebreaker>"
//
// 【为什么两个参数都要显式传，不从字符串里猜】
// 这里踩过坑：原先只收一个参数，兜底列用 strings.Fields(fallback)[0] 推断。
// 遇到多列 fallback "r.role_sort, r.role_id" 时切出来是 "r.role_sort," ——
// 带着逗号，拼出 "r.role_sort asc, r.role_sort," 直接 SQL 语法错误、接口 500。
// **不要试图解析 SQL 片段**，调用方本来就知道主键叫什么。
//
// 【为什么必须有兜底列】
// LIMIT + OFFSET 不带 ORDER BY 时，SQL 层面行顺序是未定义的；
// 排序字段存在并列值时同样未定义。两种情况下翻页都可能
// **重复某行或漏掉某行** —— 而且是静默的，用户只会觉得"数据有时对不上"。
//
// 【和 Java 的差异是有意的】Java 的 mapper 多数只写单列排序甚至不排序，
// 并列值时行序由执行计划决定。加兜底列会让双端对拍在有并列值时报差异，
// 那是预期的：**不要靠去掉兜底来消差异**。行序不属于接口契约，
// 前端依赖的是字段和结构。
// 【没有"tiebreaker 为空就不追加"的分支，是刻意的】
// 留那个口子等于给"绕过稳定排序"开了一条不会报错的路：
// 传空串就退化成单列排序，翻页重复/漏行的问题静默回来，而且没人会发现。
// 真忘了传，拼出来的 SQL 会立刻报错 —— 那比静默降级好得多。
func (q Query) Stable(fallback, tiebreaker string) string {
	if q.OrderBy == "" {
		return fallback
	}
	return q.OrderBy + ", " + tiebreaker
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
