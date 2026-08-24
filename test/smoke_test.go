package apitest

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// TestSmokeGetEndpoints 把所有只读接口挨个打一遍。
//
// 这是最便宜的一层防护：路由注册顺序写错（静态路由排在 /:id 之后）、
// handler 忘了注册、SQL 写错列名 —— 这类问题一律在这里暴露，
// 不用等到点前端。
func TestSmokeGetEndpoints(t *testing.T) {
	cases := []struct {
		path  string
		paged bool // 分页接口，顺带断言 total/rows 平铺
	}{
		// 认证与首页
		{"/getInfo", false},
		{"/getRouters", false},

		// 系统监控
		{"/monitor/online/list?pageNum=1&pageSize=10", true},
		{"/monitor/server", false},
		{"/monitor/cache", false},
		{"/monitor/cache/getNames", false},
		{"/monitor/logininfor/list?pageNum=1&pageSize=10", true},
		{"/monitor/operlog/list?pageNum=1&pageSize=10", true},

		// 岗位
		{"/system/post/optionselect", false},
		{"/system/post/list?pageNum=1&pageSize=10", true},

		// 部门
		{"/system/dept/list", false},
		{"/system/dept/list/exclude/100", false},
		{"/system/dept/100", false},

		// 角色
		{"/system/role/list?pageNum=1&pageSize=10", true},
		{"/system/role/optionselect", false},
		{"/system/role/deptTree/2", false},
		{"/system/role/authUser/allocatedList?pageNum=1&pageSize=10&roleId=2", true},
		{"/system/role/authUser/unallocatedList?pageNum=1&pageSize=10&roleId=2", true},
		{"/system/role/2", false},

		// 用户
		{"/system/user/list?pageNum=1&pageSize=10", true},
		{"/system/user/deptTree", false},
		{"/system/user/profile", false},
		{"/system/user/authRole/1", false},
		{"/system/user", false},
		{"/system/user/1", false},

		// 菜单
		{"/system/menu/list", false},
		{"/system/menu/treeselect", false},
		{"/system/menu/roleMenuTreeselect/2", false},
		{"/system/menu/1", false},

		// 参数配置
		{"/system/config/list?pageNum=1&pageSize=10", true},
		{"/system/config/configKey/sys.user.initPassword", false},
		{"/system/config/1", false},

		// 字典
		{"/system/dict/type/list?pageNum=1&pageSize=10", true},
		{"/system/dict/type/optionselect", false},
		{"/system/dict/type/1", false},
		{"/system/dict/data/list?pageNum=1&pageSize=10", true},
		{"/system/dict/data/type/sys_normal_disable", false},
		{"/system/dict/data/1", false},

		// 定时任务
		{"/monitor/job/list?pageNum=1&pageSize=10", true},
		{"/monitor/jobLog/list?pageNum=1&pageSize=10", true},

		// 通知公告
		{"/system/notice/list?pageNum=1&pageSize=10", true},
		{"/system/notice/listTop", false},
		{"/system/notice/readUsers/list?pageNum=1&pageSize=10&noticeId=1", true},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			r := doGet(t, tc.path)
			mustOK(t, r, tc.path)
			if tc.paged {
				pageRows(t, r, tc.path)
			}
		})
	}
}

// TestSmokeCacheDetail 缓存监控的两个带参接口，参数从 getNames 里取，避免写死。
func TestSmokeCacheDetail(t *testing.T) {
	names := dataArray(t, doGet(t, "/monitor/cache/getNames"), "缓存名称列表")
	if len(names) == 0 {
		t.Skip("没有可用的缓存名称，跳过")
	}

	cacheName, _ := names[0]["cacheName"].(string)
	if cacheName == "" {
		t.Fatalf("缓存名称项应有 cacheName 字段，实际字段=%v", topKeys(names[0]))
	}

	r := doGet(t, "/monitor/cache/getKeys/"+url.PathEscape(cacheName))
	mustOK(t, r, "查缓存键列表")

	// 【是字符串数组，不是对象数组】Java 版返回的是 TreeSet<String>，
	// 前端直接把每个元素当键名渲染。包成 {cacheKey: "..."} 前端就显示空白。
	keys := toObjects2Strings(t, r.Raw["data"], "缓存键列表")
	if len(keys) == 0 {
		return // 该缓存暂时没有键，正常
	}

	// 键名是完整的 key（含 sys_config: 这样的前缀），getValue 拿的就是它
	mustOK(t, doGet(t, "/monitor/cache/getValue/"+url.PathEscape(cacheName)+"/"+url.PathEscape(keys[0])), "查缓存值")
}

// TestSmokeExports 所有导出接口都要吐出合法的 xlsx。
//
// 【注意】导出用 POST + 表单，不是 GET + query string。
// 用 GET 或 JSON 提交都会拿到空查询条件、静默导出全表。
func TestSmokeExports(t *testing.T) {
	paths := []string{
		"/system/post/export",
		"/system/role/export",
		"/system/user/export",
		"/system/config/export",
		"/system/dict/type/export",
		"/system/dict/data/export",
		"/monitor/logininfor/export",
		"/monitor/operlog/export",
		"/monitor/job/export",
		"/monitor/jobLog/export",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			r := request(http.MethodPost, path, adminToken, url.Values{"pageNum": {"1"}, "pageSize": {"10"}})
			if r.Status != http.StatusOK {
				t.Fatalf("%s：HTTP 状态码应为 200，实际 %d", path, r.Status)
			}
			if _, err := excelize.OpenReader(bytes.NewReader(r.Body)); err != nil {
				t.Fatalf("%s：导出的应是合法 xlsx，实际响应=%s", path, truncBody(r.Body))
			}
		})
	}
}

// TestSmokeExportContentType 导出失败时的 Content-Type 必须是不带 charset 的 application/json。
//
// 前端 blobValidate() 是严格相等比较：data.type !== 'application/json'。
// gin 的 c.JSON 写的是 "application/json; charset=utf-8"，比较不相等 →
// 前端把错误当成文件存下来，用户拿到一个打不开的 .xlsx，且看不到任何提示。
func TestSmokeExportContentType(t *testing.T) {
	// 用 deptId 传非数字触发绑定失败 —— 这是最稳的错误路径。
	// （非法的 orderByColumn 会被白名单直接忽略，导出照样成功，触发不了。）
	r := request(http.MethodPost, "/system/user/export", adminToken,
		url.Values{"deptId": {"这不是数字"}})

	if r.Status != http.StatusOK {
		t.Fatalf("导出失败时 HTTP 状态码也应为 200，实际 %d", r.Status)
	}
	if r.Code != 500 {
		t.Fatalf("这个请求本应触发导出失败，实际 code=%d，响应=%s", r.Code, truncBody(r.Body))
	}

	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("导出接口的错误响应 Content-Type 必须恰好是 %q（前端做严格相等比较），实际 %q",
			"application/json", contentType)
	}
}

// TestAuthRequired 没有 token 或 token 非法时，一律返回业务 code 401。
//
// HTTP 状态码仍是 200 —— RuoYi 前端只看 body 里的 code。
func TestAuthRequired(t *testing.T) {
	protected := []string{
		"/getInfo",
		"/system/user/list",
		"/system/user/profile",
		"/system/dept/list",
		"/monitor/server",
	}

	for _, path := range protected {
		t.Run("无 token "+path, func(t *testing.T) {
			r := request(http.MethodGet, path, "", nil)
			if r.Status != http.StatusOK {
				t.Fatalf("%s：未认证时 HTTP 状态码也应为 200，实际 %d", path, r.Status)
			}
			if r.Code != 401 {
				t.Errorf("%s：未认证时业务 code 应为 401，实际 %d（msg=%q）", path, r.Code, r.Msg)
			}
		})
	}

	r := request(http.MethodGet, "/system/user/list", "这不是一个合法的token", nil)
	if r.Code != 401 {
		t.Errorf("token 非法时业务 code 应为 401，实际 %d（msg=%q）", r.Code, r.Msg)
	}
}

// TestAnonymousPaths 放行路径不带 token 也能访问。
func TestAnonymousPaths(t *testing.T) {
	for _, path := range []string{"/captchaImage", "/health"} {
		r := request(http.MethodGet, path, "", nil)
		if r.Status != http.StatusOK {
			t.Errorf("%s 应匿名可访问，实际 HTTP %d", path, r.Status)
		}
	}
}

// TestCaptchaContract 验证码接口的平铺字段。
func TestCaptchaContract(t *testing.T) {
	r := request(http.MethodGet, "/captchaImage", "", nil)
	mustOK(t, r, "获取验证码")
	assertTopLevel(t, r, "验证码", "code", "msg", "captchaEnabled")
	assertNoTopLevel(t, r, "验证码", "data")

	enabled, _ := r.Raw["captchaEnabled"].(bool)
	if enabled {
		assertTopLevel(t, r, "验证码（已开启）", "uuid", "img")
		if img, _ := r.Raw["img"].(string); img == "" {
			t.Error("开启验证码时 img 不该为空")
		}
	} else {
		// 关闭时这两个键整个不出现，前端据此跳过验证码输入框
		assertNoTopLevel(t, r, "验证码（已关闭）", "uuid", "img")
	}
}

// TestGetInfoContract 首页初始化接口的平铺字段。
func TestGetInfoContract(t *testing.T) {
	r := doGet(t, "/getInfo")
	mustOK(t, r, "获取用户信息")

	assertTopLevel(t, r, "getInfo", "code", "msg", "user", "roles", "permissions",
		"pwdChrtype", "isDefaultModifyPwd", "isPasswordExpired")
	assertNoTopLevel(t, r, "getInfo", "data")

	// admin 的权限集合是通配符
	perms := toObjects2Strings(t, r.Raw["permissions"], "permissions")
	if len(perms) == 0 {
		t.Fatal("permissions 不该为空")
	}
	if !contains(perms, "*:*:*") {
		t.Errorf("admin 的 permissions 应为 [\"*:*:*\"]，实际 %v", perms)
	}

	roles := toObjects2Strings(t, r.Raw["roles"], "roles")
	if !contains(roles, "admin") {
		t.Errorf("admin 的 roles 应包含 admin，实际 %v", roles)
	}

	// user 是对象且不含密码
	user, ok := r.Raw["user"].(map[string]any)
	if !ok {
		t.Fatalf("user 应为对象，实际 %T", r.Raw["user"])
	}
	assertNoKey(t, user, "password", "getInfo 的 user")
}

// TestGetRoutersContract 路由树放在 data 里（跟 deptTree 的平铺形态不同，别搞混）。
func TestGetRoutersContract(t *testing.T) {
	r := doGet(t, "/getRouters")
	mustOK(t, r, "获取路由")
	assertNoTopLevel(t, r, "getRouters", "total", "rows")

	routers := dataArray(t, r, "路由树")
	if len(routers) == 0 {
		t.Fatal("路由树不该为空")
	}
	for _, route := range routers {
		for _, key := range []string{"name", "path", "component", "meta"} {
			if _, ok := route[key]; !ok {
				t.Errorf("路由节点缺少 %q 字段，实际字段=%v", key, topKeys(route))
			}
		}
		// hidden 是 bool，前端用 v-if 判断
		if hidden, ok := route["hidden"]; ok {
			if _, isBool := hidden.(bool); !isBool {
				t.Errorf("路由的 hidden 应为布尔，实际 %T", hidden)
			}
		}
	}
}

// TestLogoutInvalidatesToken 登出后 token 立刻失效。
//
// 这是 JWT + Redis 双层设计的意义所在：会话在服务端，删掉就是删掉了。
// 如果用户信息直接塞在 JWT 里，登出只能靠前端删本地存储，服务端拦不住。
func TestLogoutInvalidatesToken(t *testing.T) {
	body := newUserPayload("logout")
	createUser(t, body)

	token, err := loginAs(fmt.Sprint(body["userName"]), "test123456")
	if err != nil {
		t.Fatalf("测试账号登录失败：%v", err)
	}

	r := request(http.MethodGet, "/getInfo", token, nil)
	mustOK(t, r, "登出前访问 getInfo")

	r = request(http.MethodPost, "/logout", token, nil)
	mustOK(t, r, "登出")

	r = request(http.MethodGet, "/getInfo", token, nil)
	if r.Code != 401 {
		t.Errorf("登出后旧 token 应立刻失效（code=401），实际 code=%d，msg=%q", r.Code, r.Msg)
	}
}

// TestPageSizeCap pageSize 超过上限时截断，而不是报错。
func TestPageSizeCap(t *testing.T) {
	r := doGet(t, "/monitor/operlog/list?pageNum=1&pageSize=9999")
	mustOK(t, r, "超大 pageSize 的列表")

	rows := pageRows(t, r, "超大 pageSize 的列表")
	if len(rows) > 100 {
		t.Errorf("pageSize 上限是 100，实际返回了 %d 行", len(rows))
	}

	total, _ := r.Raw["total"].(float64)
	if total <= 100 {
		t.Logf("提示：当前 total=%v，不足 100 行，这个用例只验证了不报错", total)
	}
}

// TestSortColumnWhitelist orderByColumn 直连 SQL 是注入面，必须走白名单。
func TestSortColumnWhitelist(t *testing.T) {
	bad := []string{
		"user_id; DROP TABLE sys_user",
		"(SELECT 1)",
		"user_id) --",
		"没有这一列",
	}

	for _, column := range bad {
		t.Run(column, func(t *testing.T) {
			r := doGet(t, "/system/user/list?pageNum=1&pageSize=10&orderByColumn="+url.QueryEscape(column)+"&isAsc=asc")

			if r.Status != http.StatusOK {
				t.Fatalf("HTTP 状态码应为 200，实际 %d", r.Status)
			}
			// 白名单外的列要么被忽略、要么明确报错，两种都行；
			// 不能接受的是把驱动的 SQL 错误原样回显给用户
			for _, leak := range []string{"SQL", "sql:", "Error 1", "syntax", "sys_user"} {
				if strings.Contains(r.Msg, leak) {
					t.Errorf("错误提示里泄漏了数据库细节（含 %q）：%q", leak, r.Msg)
				}
			}
		})
	}

	// 白名单内的列必须真的生效
	mustOK(t, doGet(t, "/system/user/list?pageNum=1&pageSize=10&orderByColumn=createTime&isAsc=desc"), "按白名单列排序")
}

// ---------- 本文件用到的小工具 ----------

func toObjects2Strings(t *testing.T, raw any, what string) []string {
	t.Helper()
	list, ok := raw.([]any)
	if !ok {
		t.Fatalf("%s 应为数组，实际是 %T", what, raw)
	}
	result := make([]string, 0, len(list))
	for _, item := range list {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("%s 的元素应为字符串，实际是 %T", what, item)
		}
		result = append(result, text)
	}
	return result
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
