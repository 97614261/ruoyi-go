package page

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestStable 排序表达式拼接。
//
// 【这个文件的由来】Stable 原先只收一个参数，兜底列用
// strings.Fields(fallback)[0] 从 SQL 片段里猜。遇到多列 fallback
// "r.role_sort, r.role_id" 时切出来是 "r.role_sort," —— 带着逗号，
// 拼出 "r.role_sort asc, r.role_sort," 直接是语法错误、接口 500。
//
// 而 pkg/page 当时**一个测试都没有**，全量接口测试也发现不了 ——
// 因为没有任何用例给角色/字典列表传 orderByColumn。
func TestStable(t *testing.T) {
	cases := []struct {
		name       string
		orderBy    string
		fallback   string
		tiebreaker string
		want       string
	}{
		{
			name:     "没传排序：用 fallback",
			fallback: "post_id", tiebreaker: "post_id",
			want: "post_id",
		},
		{
			name:     "没传排序：fallback 带方向要原样保留",
			fallback: "info_id DESC", tiebreaker: "info_id",
			want: "info_id DESC",
		},
		{
			name:     "没传排序：fallback 是多列",
			fallback: "dict_sort, dict_code", tiebreaker: "dict_code",
			want: "dict_sort, dict_code",
		},
		{
			name:    "传了排序：追加兜底列",
			orderBy: "create_time asc",
			// 就是这一组曾经拼出 "create_time asc, r.role_sort,"
			fallback: "r.role_sort, r.role_id", tiebreaker: "r.role_id",
			want: "create_time asc, r.role_id",
		},
		{
			name:     "传了排序：单列 fallback",
			orderBy:  "post_sort desc",
			fallback: "post_id", tiebreaker: "post_id",
			want: "post_sort desc, post_id",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := Query{OrderBy: tc.orderBy}
			got := q.Stable(tc.fallback, tc.tiebreaker)
			if got != tc.want {
				t.Errorf("Stable(%q, %q) 在 OrderBy=%q 时 = %q，期望 %q",
					tc.fallback, tc.tiebreaker, tc.orderBy, got, tc.want)
			}
			assertNoDanglingComma(t, got)
		})
	}
}

// assertNoDanglingComma 结果不能以逗号结尾，也不能有连续逗号。
//
// 这两种都会让 MySQL 直接报语法错误，而 Go 侧只会看到一个 500。
func assertNoDanglingComma(t *testing.T, expr string) {
	t.Helper()
	if len(expr) > 0 && expr[len(expr)-1] == ',' {
		t.Errorf("排序表达式不能以逗号结尾：%q", expr)
	}
	for i := 0; i+1 < len(expr); i++ {
		if expr[i] == ',' && expr[i+1] == ',' {
			t.Errorf("排序表达式有连续逗号：%q", expr)
			return
		}
	}
}

// TestParsePageSize 分页参数的边界。
func TestParsePageSize(t *testing.T) {
	cases := []struct {
		name  string
		query string
		num   int
		size  int
	}{
		{"缺省", "", DefaultPageNum, DefaultPageSize},
		{"正常值", "pageNum=3&pageSize=20", 3, 20},
		{"超过上限要截断而不是报错", "pageSize=9999", DefaultPageNum, MaxPageSize},
		{"恰好等于上限", "pageSize=100", DefaultPageNum, 100},
		{"零和负数按缺省处理", "pageNum=0&pageSize=-5", DefaultPageNum, DefaultPageSize},
		{"非数字按缺省处理", "pageNum=abc&pageSize=x", DefaultPageNum, DefaultPageSize},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := parseWith(tc.query, nil)
			if q.PageNum != tc.num || q.PageSize != tc.size {
				t.Errorf("pageNum=%d pageSize=%d，期望 %d / %d",
					q.PageNum, q.PageSize, tc.num, tc.size)
			}
		})
	}
}

// TestParseOrderByWhitelist orderByColumn 直连 SQL，必须走白名单。
func TestParseOrderByWhitelist(t *testing.T) {
	allowed := map[string]string{
		"postSort":   "post_sort",
		"createTime": "create_time",
	}

	cases := []struct {
		name  string
		query string
		want  string
	}{
		{"白名单内", "orderByColumn=postSort&isAsc=asc", "post_sort asc"},
		{"Element Plus 的原始值也要认", "orderByColumn=postSort&isAsc=descending", "post_sort desc"},
		{"没传方向默认升序", "orderByColumn=createTime", "create_time asc"},

		// 下面这些必须被忽略，返回空串 —— 空串会让 Stable 走 fallback 分支
		{"白名单外的列", "orderByColumn=password", ""},
		{"注入尝试：分号", "orderByColumn=post_sort;DROP TABLE sys_post", ""},
		{"注入尝试：子查询", "orderByColumn=(SELECT 1)", ""},
		{"注入尝试：注释", "orderByColumn=post_sort--", ""},
		{"没传排序列", "isAsc=desc", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := parseWith(tc.query, allowed)
			if q.OrderBy != tc.want {
				t.Errorf("OrderBy = %q，期望 %q", q.OrderBy, tc.want)
			}
		})
	}
}

// TestParseNilWhitelist 白名单为 nil 表示该接口不支持排序。
func TestParseNilWhitelist(t *testing.T) {
	q := parseWith("orderByColumn=postSort", nil)
	if q.OrderBy != "" {
		t.Errorf("白名单为 nil 时不该产生排序，实际 %q", q.OrderBy)
	}
}

func TestOffset(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	cases := []struct {
		num, size, want int
	}{
		{1, 10, 0},
		{2, 10, 10},
		{5, 20, 80},
		{0, 10, 0},
		{maxInt, MaxPageSize, maxInt},
	}
	for _, tc := range cases {
		q := Query{PageNum: tc.num, PageSize: tc.size}
		if got := q.Offset(); got != tc.want {
			t.Errorf("PageNum=%d PageSize=%d 的 Offset = %d，期望 %d",
				tc.num, tc.size, got, tc.want)
		}
	}
}

func parseWith(rawQuery string, allowed map[string]string) Query {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	// 【不能写成 httptest.NewRequest("GET", "/?"+rawQuery, nil)】
	// 它会解析整个 target，遇到裸空格直接 panic ——
	// 而注入用例里恰恰有 "post_sort;DROP TABLE sys_post" 这种带空格的值。
	// 转义掉又会让用例读起来不像攻击载荷，所以直接塞 RawQuery：
	// url.ParseQuery 对空格是宽容的，正好保留原样。
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Request.URL.RawQuery = rawQuery
	return Parse(c, allowed)
}
