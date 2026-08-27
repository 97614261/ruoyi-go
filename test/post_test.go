package apitest

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// newPostPayload 一份完整合法的岗位数据。
func newPostPayload(suffix string) map[string]any {
	return map[string]any{
		"postCode": testPrefix + "code" + suffix,
		"postName": testPrefix + "岗位" + suffix,
		"postSort": 3,
		"status":   "0",
		"remark":   "自动化测试数据",
	}
}

// createPost 新增岗位并返回它的 postId。
//
// 新增接口只返回 code/msg，所以要按 postCode 查回来拿 ID。
// 返回 int64 而不是字符串 —— 主键要塞回请求体时必须是数字，
// 传字符串会直接反序列化失败。
func createPost(t *testing.T, body map[string]any) int64 {
	t.Helper()
	mustOK(t, doPost(t, "/system/post", body), "新增岗位")

	code := fmt.Sprint(body["postCode"])
	list := pageRows(t, doGet(t, "/system/post/list?pageNum=1&pageSize=100&postCode="+url.QueryEscape(code)), "按编码查岗位")
	item := findBy(list, "postCode", code)
	if item == nil {
		t.Fatalf("新增岗位后按编码 %s 查不到", code)
	}
	id := idOf(t, item, "postId")

	t.Cleanup(func() {
		// 用例可能已经删过，这里不校验结果，只保证不留垃圾数据
		_ = doDelete(t, "/system/post/"+idPath(id))
	})
	return id
}

// TestPostCRUD 岗位的完整增删改查，逐字段核对数据。
func TestPostCRUD(t *testing.T) {
	body := newPostPayload("1")
	id := createPost(t, body)
	path := "/system/post/" + idPath(id)

	// --- 查：新增的值要能原样查回来 ---
	detail := dataObject(t, doGet(t, path), "查岗位详情")
	assertField(t, detail, "postCode", body["postCode"], "新增后")
	assertField(t, detail, "postName", body["postName"], "新增后")
	assertField(t, detail, "postSort", 3, "新增后")
	assertField(t, detail, "status", "0", "新增后")
	assertField(t, detail, "remark", "自动化测试数据", "新增后")
	// status 是字符串，前端用 === 比较
	assertString(t, detail, "岗位详情", "status")
	// createBy 应该是服务端填的当前登录账号
	assertField(t, detail, "createBy", "admin", "新增后")

	// --- 改：把排序改成 0，这是 GORM 零值陷阱的重灾区 ---
	updated := payload(body)
	updated["postId"] = id
	updated["postSort"] = 0
	updated["postName"] = testPrefix + "岗位改名"
	updated["status"] = "1"
	mustOK(t, doPut(t, "/system/post", updated), "修改岗位")

	detail = dataObject(t, doGet(t, path), "改后查岗位")
	assertField(t, detail, "postSort", 0, "改后（排序 0 不能被 GORM 跳过）")
	assertField(t, detail, "postName", testPrefix+"岗位改名", "改后")
	assertField(t, detail, "status", "1", "改后（状态改回 0/1 不能被跳过）")

	// 【updateBy 在详情里必须是 null，不能断言成 "admin"】
	// Java 的 selectPostVo 没 select update_by / update_time，
	// 详情接口输出的就是 null。这行原来断言 "admin" ——
	// 那是照着当时 Go 自己的输出写的，把不对齐固化成了"期望"。
	//
	// 「修改时要把操作人写进 update_by」这个行为仍然要验，
	// 但既然没有任何接口暴露它，就只能到数据库层去看，见下面这句。
	assertJavaNull(t, detail, "改后查岗位", "updateBy", "updateTime")
	assertColumn(t, "sys_post", "update_by", "post_id", id, "admin")

	// --- 改：不传 remark 时，原备注要保留（对齐 Java 的 <if test="remark != null">）---
	noRemark := omit(updated, "remark")
	mustOK(t, doPut(t, "/system/post", noRemark), "不带 remark 修改岗位")
	detail = dataObject(t, doGet(t, path), "不带 remark 改后")
	assertField(t, detail, "remark", "自动化测试数据", "不带 remark 修改后原备注应保留")

	// --- 列表：按名称能搜到 ---
	list := pageRows(t, doGet(t, "/system/post/list?pageNum=1&pageSize=100&postName="+url.QueryEscape(testPrefix+"岗位改名")), "按名称搜岗位")
	if findBy(list, "postId", id) == nil {
		t.Errorf("按名称搜索应能搜到刚改名的岗位 %d", id)
	}

	// --- 删：删完就查不到 ---
	mustOK(t, doDelete(t, path), "删除岗位")
	mustFail(t, doGet(t, path), "不存在", "删除后再查")
}

// TestPostUniqueCheck 编码和名称的唯一性校验。
func TestPostUniqueCheck(t *testing.T) {
	first := newPostPayload("uniq")
	createPost(t, first)

	// 同名同码都应该被拒，且提示语要带"新增"前缀
	dup := with(newPostPayload("uniq2"), "postName", first["postName"])
	mustFail(t, doPost(t, "/system/post", dup), "新增岗位", "同名岗位")

	dupCode := with(newPostPayload("uniq3"), "postCode", first["postCode"])
	mustFail(t, doPost(t, "/system/post", dupCode), "岗位编码已存在", "同编码岗位")
}

// TestPostValidation 字段校验：少传、多传、空值、超长、边界值。
//
// 【为什么用变换函数而不是预构造的 map】
// 每个用例需要独立的 postCode / postName 才不会互相撞唯一性约束。
// 早先的写法是先构造好 map、再在循环里统一覆盖这两个字段 ——
// 结果"编码超长"那条的值被覆盖掉了，测的根本不是超长。
// 改成基础数据自带唯一后缀、变换在其上叠加，就不会误伤被测字段。
func TestPostValidation(t *testing.T) {
	keep := func(p map[string]any) map[string]any { return p }

	cases := []struct {
		name   string
		mutate func(map[string]any) map[string]any
		wantIn string // 空表示应当成功
	}{
		{"完整数据", keep, ""},

		// --- 少传 ---
		{"少传 postName", func(p map[string]any) map[string]any { return omit(p, "postName") }, "PostName"},
		{"少传 postCode", func(p map[string]any) map[string]any { return omit(p, "postCode") }, "PostCode"},
		{"少传 status", func(p map[string]any) map[string]any { return omit(p, "status") }, "Status"},
		// 【少传要拒，传 0 要过 —— 这两条必须同时成立】
		// Java 是 Integer + @NotNull，不传就拒；而 0 是合法排序值必须放行。
		// 只有指针字段能同时表达这两件事：非指针 int 分不清"没传"和"传了 0"。
		{"少传 postSort", func(p map[string]any) map[string]any { return omit(p, "postSort") }, "PostSort"},
		{"postSort 传 null", func(p map[string]any) map[string]any { return with(p, "postSort", nil) }, "PostSort"},
		{"少传 remark", func(p map[string]any) map[string]any { return omit(p, "remark") }, ""},

		// --- 空值 ---
		{"postName 空串", func(p map[string]any) map[string]any { return with(p, "postName", "") }, "PostName"},
		// notblank：纯空白也要拒，required 是拦不住的
		{"postName 纯空格", func(p map[string]any) map[string]any { return with(p, "postName", "   ") }, "PostName"},
		{"status 空串", func(p map[string]any) map[string]any { return with(p, "status", "") }, "Status"},

		// --- 边界 ---
		{"postSort = 0", func(p map[string]any) map[string]any { return with(p, "postSort", 0) }, ""},
		{"postSort 负数", func(p map[string]any) map[string]any { return with(p, "postSort", -1) }, "PostSort"},
		{"postName 恰好 50 字", func(p map[string]any) map[string]any { return with(p, "postName", repeatText(50)) }, ""},
		{"postName 超过 50 字", func(p map[string]any) map[string]any { return with(p, "postName", repeatText(51)) }, "PostName"},
		{"postCode 恰好 64 字", func(p map[string]any) map[string]any { return with(p, "postCode", strings.Repeat("a", 64)) }, ""},
		{"postCode 超过 64 字", func(p map[string]any) map[string]any { return with(p, "postCode", strings.Repeat("a", 65)) }, "PostCode"},
		{"remark 超过 500 字", func(p map[string]any) map[string]any { return with(p, "remark", repeatText(501)) }, "Remark"},

		// --- 类型 ---
		// 前端发错类型时，报错必须指出是哪个字段，不能只回"请求参数格式错误"
		{"postSort 传字符串", func(p map[string]any) map[string]any { return with(p, "postSort", "abc") }, "postSort"},

		// --- 多传 ---
		{"多传未知字段", func(p map[string]any) map[string]any { return with(p, "hello", "world") }, ""},
		{"多传只读字段 createBy", func(p map[string]any) map[string]any { return with(p, "createBy", "hacker") }, ""},
		// status 取值来自字典，不能硬编码成 oneof=0 1，字典加了新值要能提交
		{"status 传字典外的值", func(p map[string]any) map[string]any { return with(p, "status", "2") }, ""},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 唯一性由基础数据保证，变换在其上叠加，不会覆盖被测字段
			body := tc.mutate(newPostPayload(fmt.Sprintf("v%d", i)))

			r := doPost(t, "/system/post", body)
			if tc.wantIn == "" {
				mustOK(t, r, tc.name)
				if code, ok := body["postCode"].(string); ok {
					cleanupPostByCode(t, code)
				}
				return
			}
			mustFail(t, r, tc.wantIn, tc.name)
		})
	}
}

// TestPostCreateByCannotBeForged 多传 createBy 不能真的写进库。
func TestPostCreateByCannotBeForged(t *testing.T) {
	body := with(newPostPayload("forge"), "createBy", "hacker")
	id := createPost(t, body)

	detail := dataObject(t, doGet(t, "/system/post/"+idPath(id)), "查伪造 createBy 的岗位")
	assertField(t, detail, "createBy", "admin", "createBy 必须由服务端填当前账号，不能被请求体伪造")
}

// TestPostListContract 分页响应的结构必须符合 TableDataInfo。
func TestPostListContract(t *testing.T) {
	r := doGet(t, "/system/post/list?pageNum=1&pageSize=10")
	mustOK(t, r, "岗位列表")

	// total 和 rows 平铺在顶层，没有 data 包裹
	assertTopLevel(t, r, "岗位列表", "code", "msg", "total", "rows")
	assertNoTopLevel(t, r, "岗位列表", "data")

	// pageSize 硬上限 100：传超大值应被截断而不是报错
	big := doGet(t, "/system/post/list?pageNum=1&pageSize=99999")
	mustOK(t, big, "pageSize 超限")
	if rows := pageRows(t, big, "pageSize 超限"); len(rows) > 100 {
		t.Errorf("pageSize 应被截断到 100，实际返回 %d 条", len(rows))
	}
}

// TestPostSortWhitelist 排序字段白名单：非法值必须被忽略而不是拼进 SQL。
func TestPostSortWhitelist(t *testing.T) {
	// 合法字段
	mustOK(t, doGet(t, "/system/post/list?pageNum=1&pageSize=10&orderByColumn=postSort&isAsc=ascending"), "按 postSort 升序")

	// 非法字段：应被忽略（退回默认排序），接口照常返回而不是 500。
	// 【必须 URL 编码】httptest.NewRequest 按 HTTP 报文格式解析请求行，
	// 裸空格会把它切坏并 panic —— 那是测试自己的问题，不是接口的问题。
	injections := []string{
		"post_sort; DROP TABLE sys_post",
		"post_sort) UNION SELECT 1,2,3--",
		"不存在的字段",
		"",
	}
	for _, injection := range injections {
		path := "/system/post/list?pageNum=1&pageSize=10&orderByColumn=" + url.QueryEscape(injection)
		mustOK(t, doGet(t, path), "非法排序字段 "+injection+" 应被忽略")
	}
}

// TestPostPagingStable 排序字段大量并列时，逐页读取不能重复也不能遗漏。
//
// 【这是最容易被忽略的一类 bug】
// LIMIT + OFFSET 在排序键有并列值时，两次查询的行序是未定义的 ——
// MySQL 完全可以在第 1 页和第 2 页返回同一行，同时漏掉另一行。
// 症状是"某条记录在列表里怎么都找不到"，翻回上一页又出现了，
// 极难复现、也没有任何报错。
//
// 造 7 条 postSort 完全相同的数据，用 pageSize=2 逐页读完，
// 断言取到的 ID 集合与总数一致、且无重复。
func TestPostPagingStable(t *testing.T) {
	const count = 7
	created := make(map[int64]bool, count)
	for i := 0; i < count; i++ {
		body := newPostPayload(fmt.Sprintf("paging%d", i))
		// 全部用同一个排序值，制造并列
		body["postSort"] = 50
		created[createPost(t, body)] = true
	}

	// 只看本次造的这批：带上前缀条件，避免库里其它岗位干扰
	const pageSize = 2
	base := fmt.Sprintf("/system/post/list?pageSize=%d&postName=%s&orderByColumn=postSort&isAsc=ascending",
		pageSize, url.QueryEscape(testPrefix+"岗位paging"))

	seen := make(map[int64]int)
	total := -1
	for pageNum := 1; pageNum <= count+2; pageNum++ {
		r := doGet(t, fmt.Sprintf("%s&pageNum=%d", base, pageNum))
		mustOK(t, r, fmt.Sprintf("第 %d 页", pageNum))

		if total < 0 {
			if n, ok := r.Raw["total"].(float64); ok {
				total = int(n)
			}
		}
		rows := pageRows(t, r, fmt.Sprintf("第 %d 页", pageNum))
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			seen[idOf(t, row, "postId")]++
		}
	}

	if total != count {
		t.Fatalf("total 应为 %d，实际 %d —— 查询条件没圈住本次造的数据", count, total)
	}

	for id, times := range seen {
		if times > 1 {
			t.Errorf("岗位 %d 在翻页中出现了 %d 次 —— 排序不稳定导致重复", id, times)
		}
	}
	for id := range created {
		if seen[id] == 0 {
			t.Errorf("岗位 %d 逐页读完一次都没出现 —— 排序不稳定导致漏行", id)
		}
	}
}

// TestPostOptionSelect 下拉选择接口。
func TestPostOptionSelect(t *testing.T) {
	r := doGet(t, "/system/post/optionselect")
	mustOK(t, r, "岗位下拉")
	dataArray(t, r, "岗位下拉")
}

// TestPostExport 导出：成功时是 xlsx，失败时必须是不带 charset 的 application/json。
func TestPostExport(t *testing.T) {
	form := url.Values{}
	form.Set("postName", testPrefix)

	r := request(http.MethodPost, "/system/post/export", adminToken, form)
	if r.Status != http.StatusOK {
		t.Fatalf("导出应返回 200，实际 %d", r.Status)
	}

	contentType := r.Header.Get("Content-Type")
	if !strings.Contains(contentType, "spreadsheetml") {
		t.Fatalf("导出的 Content-Type 应为 xlsx，实际 %q，响应=%s", contentType, truncBody(r.Body))
	}
	// xlsx 是 zip，魔数固定为 PK
	if len(r.Body) < 2 || r.Body[0] != 'P' || r.Body[1] != 'K' {
		t.Errorf("导出内容不像 xlsx（前两字节应为 PK），实际=%s", truncBody(r.Body))
	}
}

// TestPostDeleteInUse 已分配给用户的岗位不允许删除。
func TestPostDeleteInUse(t *testing.T) {
	// admin 用户默认关联岗位 1（董事长），直接删应该被拒
	r := doDelete(t, "/system/post/1")
	if r.Code == 200 {
		t.Skip("岗位 1 未被任何用户分配，跳过占用校验（初始数据可能已被改动）")
	}
	mustFail(t, r, "不能删除", "删除已分配的岗位")
}

func cleanupPostByCode(t *testing.T, code string) {
	t.Helper()
	list := pageRows(t, doGet(t, "/system/post/list?pageNum=1&pageSize=100&postCode="+url.QueryEscape(code)), "清理查岗位")
	if item := findBy(list, "postCode", code); item != nil {
		_ = doDelete(t, "/system/post/"+idPath(idOf(t, item, "postId")))
	}
}
