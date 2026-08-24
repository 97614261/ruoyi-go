package apitest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHealthChecksDependencies 健康检查必须真的探活，不能返回硬编码的 up。
//
// 【为什么这条重要】
// 进程活着不等于服务可用。数据库连不上时，一个恒返 up 的探活接口
// 会让负载均衡器继续把流量打进来，每个请求都在超时后报 500，
// 比直接摘掉这个实例糟糕得多。
func TestHealthChecksDependencies(t *testing.T) {
	r := request(http.MethodGet, "/health", "", nil)

	if r.Status != http.StatusOK {
		t.Fatalf("依赖都正常时 HTTP 状态码应为 200，实际 %d，响应=%s", r.Status, truncBody(r.Body))
	}

	var result struct {
		Status string         `json:"status"`
		MySQL  string         `json:"mysql"`
		Redis  string         `json:"redis"`
		DB     map[string]any `json:"db"`
	}
	if err := json.Unmarshal(r.Body, &result); err != nil {
		t.Fatalf("健康检查响应解析失败：%v，原文=%s", err, truncBody(r.Body))
	}

	// 【关键】必须逐项报告，只有一个笼统的 status 说明根本没去 ping
	for name, got := range map[string]string{"mysql": result.MySQL, "redis": result.Redis} {
		if got != "up" {
			t.Errorf("测试跑得起来说明 %s 是通的，健康检查却报 %q", name, got)
		}
	}
	if result.Status != "up" {
		t.Errorf("整体状态应为 up，实际 %q", result.Status)
	}

	// 连接池状态：排查连接泄漏时唯一能看的东西
	if len(result.DB) == 0 {
		t.Error("健康检查应带上连接池状态（inUse / waitCount），否则连接泄漏时无从查起")
	}
	for _, key := range []string{"maxOpen", "inUse", "idle", "waitCount"} {
		if _, ok := result.DB[key]; !ok {
			t.Errorf("连接池状态缺少 %q 字段", key)
		}
	}

	// 健康检查不能要登录 —— 探针不会带 token
	if r.Code == 401 {
		t.Error("健康检查必须匿名可访问")
	}
}

// TestHealthIsNotWrappedInAjaxResult 健康检查故意不套 AjaxResult。
//
// "HTTP 状态码一律 200" 那条规矩是给 RuoYi 前端定的（它只看 body 里的 code）。
// 但探针只认 HTTP 状态码，根本不解析 body。这里必须是裸结构 + 真实状态码。
func TestHealthIsNotWrappedInAjaxResult(t *testing.T) {
	r := request(http.MethodGet, "/health", "", nil)

	if _, ok := r.Raw["code"]; ok {
		t.Error("健康检查不该套 AjaxResult 的 code 字段，探针读的是 HTTP 状态码")
	}
	if _, ok := r.Raw["status"]; !ok {
		t.Errorf("健康检查应直接返回 status 字段，实际顶层是 %v", topKeys(r.Raw))
	}
}

// TestTraceIDInResponseHeader 每个响应都要带 traceId。
//
// 用户报"我刚才保存失败了"，没有 traceId 就只能靠时间戳在几十条并发日志里猜。
// 带上之后前端能把它显示出来，用户截个图就带着 ID。
func TestTraceIDInResponseHeader(t *testing.T) {
	first := doGet(t, "/system/post/list?pageNum=1&pageSize=1")
	id := first.Header.Get("X-Trace-Id")
	if id == "" {
		t.Fatal("响应必须带 X-Trace-Id 头")
	}

	// 每次请求必须是新的，复用等于串号
	second := doGet(t, "/system/post/list?pageNum=1&pageSize=1")
	if other := second.Header.Get("X-Trace-Id"); other == id {
		t.Errorf("两次请求的 traceId 不该相同，都是 %q", id)
	}

	// 未认证的请求也要有 —— 401 恰恰是需要追查的场景
	anonymous := request(http.MethodGet, "/system/user/list", "", nil)
	if anonymous.Header.Get("X-Trace-Id") == "" {
		t.Error("未认证的请求同样要带 traceId，否则查不了鉴权失败的原因")
	}
}

// TestTraceIDHonorsUpstream 反代传进来的 X-Request-Id 要沿用，但必须过滤。
//
// 沿用是为了把网关日志和应用日志串起来；
// 过滤是因为它来自外部输入，超长或带控制字符的值会污染日志。
func TestTraceIDHonorsUpstream(t *testing.T) {
	cases := []struct {
		name     string
		incoming string
		reuse    bool
	}{
		{"正常的网关 ID", "gw-abc123_XYZ", true},
		{"带空格", "abc 123", false},
		{"带制表符（日志注入）", "abc\tdef", false},
		{"超长", strings.Repeat("a", 200), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			req.Header.Set("X-Request-Id", tc.incoming)

			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)

			got := rec.Header().Get("X-Trace-Id")
			if got == "" {
				t.Fatal("响应必须带 X-Trace-Id 头")
			}
			if tc.reuse && got != tc.incoming {
				t.Errorf("合法的上游 ID 应被沿用，期望 %q，实际 %q", tc.incoming, got)
			}
			if !tc.reuse && got == tc.incoming {
				t.Errorf("非法的上游 ID 不该被沿用：%q", tc.incoming)
			}
		})
	}
}
