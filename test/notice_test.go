package apitest

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func newNoticePayload(suffix string) map[string]any {
	return map[string]any{
		"noticeTitle":   testPrefix + "公告" + suffix,
		"noticeType":    "1",
		"noticeContent": "<p>测试内容</p>",
		"status":        "0",
		"remark":        "测试公告",
	}
}

func createNotice(t *testing.T, body map[string]any) int64 {
	t.Helper()
	mustOK(t, doPost(t, "/system/notice", body), "新增公告")

	title := fmt.Sprint(body["noticeTitle"])
	list := pageRows(t, doGet(t, "/system/notice/list?pageSize=100&noticeTitle="+url.QueryEscape(title)), "查公告")
	item := findBy(list, "noticeTitle", title)
	if item == nil {
		t.Fatalf("新增公告后按标题 %s 查不到", title)
	}
	id := idOf(t, item, "noticeId")

	t.Cleanup(func() { _ = doDelete(t, "/system/notice/"+idPath(id)) })
	return id
}

// TestNoticeCRUD 公告增删改查。
func TestNoticeCRUD(t *testing.T) {
	body := newNoticePayload("crud")
	id := createNotice(t, body)
	path := "/system/notice/" + idPath(id)

	detail := dataObject(t, doGet(t, path), "查公告")
	assertField(t, detail, "noticeTitle", body["noticeTitle"], "新增后")
	assertField(t, detail, "noticeType", "1", "新增后")
	assertField(t, detail, "noticeContent", body["noticeContent"], "新增后")
	assertString(t, detail, "公告详情", "noticeType", "status")

	updated := payload(body)
	updated["noticeId"] = id
	updated["noticeTitle"] = testPrefix + "公告改名"
	updated["noticeContent"] = "<p>改后的内容</p>"
	mustOK(t, doPut(t, "/system/notice", updated), "修改公告")

	detail = dataObject(t, doGet(t, path), "改后查公告")
	assertField(t, detail, "noticeTitle", testPrefix+"公告改名", "改后")
	assertField(t, detail, "noticeContent", "<p>改后的内容</p>", "改后")

	mustOK(t, doDelete(t, path), "删除公告")
	mustFail(t, doGet(t, path), "不存在", "删除后再查")
}

// TestNoticeLongContent 富文本正文可以很长。
//
// 【这是回归测试】操作日志中间件早先只读请求体的前 4KB 就塞回去，
// 导致超过 4KB 的请求 handler 拿到残缺 JSON、解析失败。
// 公告正文是富文本，几千字很常见，表现是"保存没反应"，极难定位。
func TestNoticeLongContent(t *testing.T) {
	// 造一段远超 4KB 的正文
	longContent := "<p>" + strings.Repeat("这是一段很长的公告正文内容。", 800) + "</p>"
	if len(longContent) < 8192 {
		t.Fatalf("测试数据本身不够长（%d 字节），起不到回归作用", len(longContent))
	}

	body := newNoticePayload("long")
	body["noticeContent"] = longContent

	id := createNotice(t, body)
	detail := dataObject(t, doGet(t, "/system/notice/"+idPath(id)), "查长正文公告")

	got := fmt.Sprint(detail["noticeContent"])
	if got != longContent {
		t.Errorf("长正文被截断了：写入 %d 字节，读回 %d 字节", len(longContent), len(got))
	}
}

// TestNoticeContentIsNotBase64 正文是 longblob，必须以字符串返回。
//
// 建模成 []byte 的话 encoding/json 会自动 base64 编码，前端拿到一串乱码。
func TestNoticeContentIsNotBase64(t *testing.T) {
	body := newNoticePayload("b64")
	body["noticeContent"] = "<p>可读的中文内容</p>"
	id := createNotice(t, body)

	detail := dataObject(t, doGet(t, "/system/notice/"+idPath(id)), "查公告正文")
	got, ok := detail["noticeContent"].(string)
	if !ok {
		t.Fatalf("noticeContent 应为字符串，实际是 %T", detail["noticeContent"])
	}
	if !strings.Contains(got, "可读的中文内容") {
		t.Errorf("正文应原样返回，疑似被 base64 编码了：%q", got)
	}
}

// TestNoticeListTopContract listTop 是混合形态：列表在 data 里，unreadCount 平铺。
func TestNoticeListTopContract(t *testing.T) {
	r := doGet(t, "/system/notice/listTop")
	mustOK(t, r, "顶栏公告")

	// data 和 unreadCount 并存，这是 CONVENTIONS 里列出的三个混合形态之一
	assertTopLevel(t, r, "顶栏公告", "code", "msg", "data", "unreadCount")
	assertNoTopLevel(t, r, "顶栏公告", "total", "rows")

	list := dataArray(t, r, "顶栏公告")
	for _, item := range list {
		if _, ok := item["isRead"]; !ok {
			t.Errorf("顶栏公告项应带 isRead 字段，实际字段=%v", topKeys(item))
			break
		}
		// Java 的 SysNotice 会序列化该字段，但 listTop 查询不读取正文，值应为 null。
		if content, ok := item["noticeContent"]; !ok || content != nil {
			t.Errorf("顶栏公告的 noticeContent 应存在且为 null，实际=%v（字段存在=%v）", content, ok)
			break
		}
	}
}

// TestNoticeMarkRead 标记已读会让未读数减少。
func TestNoticeMarkRead(t *testing.T) {
	id := createNotice(t, newNoticePayload("read"))

	before := doGet(t, "/system/notice/listTop")
	mustOK(t, before, "标记前的顶栏公告")
	beforeUnread, _ := before.Raw["unreadCount"].(float64)

	mustOK(t, doPost(t, "/system/notice/markRead?noticeId="+idPath(id), nil), "标记单条已读")

	after := doGet(t, "/system/notice/listTop")
	afterUnread, _ := after.Raw["unreadCount"].(float64)

	if afterUnread >= beforeUnread {
		t.Errorf("标记已读后未读数应减少，标记前 %v，标记后 %v", beforeUnread, afterUnread)
	}

	// 重复标记不能报错（唯一索引冲突要被忽略）
	mustOK(t, doPost(t, "/system/notice/markRead?noticeId="+idPath(id), nil), "重复标记已读")
}

// TestNoticeMarkReadAllWithoutIDsDoesNothing 对齐 Java：ids 缺失或为空时，
// Convert.toLongArray 得到空数组，SysNoticeReadServiceImpl.markReadBatch 直接返回。
// 前端正常调用始终带 ids；不带参数只用于锁定后端边界行为。
func TestNoticeMarkReadAllWithoutIDsDoesNothing(t *testing.T) {
	id := createNotice(t, newNoticePayload("readall-empty"))

	mustOK(t, doPost(t, "/system/notice/markReadAll", nil), "不带 ids 批量标记")

	r := doGet(t, "/system/notice/listTop")
	mustOK(t, r, "不带 ids 标记后的顶栏公告")
	item := findBy(dataArray(t, r, "顶栏公告"), "noticeId", id)
	if item == nil {
		t.Fatalf("新建公告 %d 应出现在顶栏列表", id)
	}
	if read, _ := item["isRead"].(bool); read {
		t.Error("Java 在 ids 为空时不做任何操作，公告不应被标记为已读")
	}
}

// TestNoticeReadUsers 已读用户列表。
func TestNoticeReadUsers(t *testing.T) {
	id := createNotice(t, newNoticePayload("users"))
	mustOK(t, doPost(t, "/system/notice/markRead?noticeId="+idPath(id), nil), "标记已读")

	r := doGet(t, "/system/notice/readUsers/list?pageNum=1&pageSize=10&noticeId="+idPath(id))
	mustOK(t, r, "已读用户列表")
	list := pageRows(t, r, "已读用户列表")

	if findBy(list, "userName", "admin") == nil {
		t.Errorf("admin 刚标记了已读，应出现在已读用户列表里，实际 %d 条", len(list))
	}
}

// TestNoticeMarkReadAllByIDs 顶栏"全部已读"传的是指定的几条 ID。
//
// 【这条才是前端真正走的路径】原来只测了不带 ids 的调用 ——
// 而前端 markAllRead() 是 `ids = noticeList.map(n => n.noticeId).join(',')`，
// 永远带 ids。只测另一条分支等于没测。
func TestNoticeMarkReadAllByIDs(t *testing.T) {
	first := createNotice(t, newNoticePayload("batch1"))
	second := createNotice(t, newNoticePayload("batch2"))

	mustOK(t, doPost(t, "/system/notice/markReadAll?ids="+idPath(first)+","+idPath(second), nil),
		"按 ID 批量标记已读")

	// 这两条必须都变成已读
	r := doGet(t, "/system/notice/listTop")
	mustOK(t, r, "顶栏公告")
	for _, id := range []int64{first, second} {
		item := findBy(dataArray(t, r, "顶栏公告"), "noticeId", id)
		if item == nil {
			continue // 不在顶栏列表里就不校验
		}
		if read, _ := item["isRead"].(bool); !read {
			t.Errorf("公告 %d 应已被标记为已读", id)
		}
	}

	// 重复标记不能报错（唯一索引冲突要被忽略）
	mustOK(t, doPost(t, "/system/notice/markReadAll?ids="+idPath(first), nil), "重复批量标记")
}

// TestNoticeReadUsersSearch 已读用户按关键字搜索。
//
// 【参数名必须是 searchValue】前端 ReadUsers.vue 发的是 queryParams.searchValue，
// Java 的方法签名也是 readUsersList(Long noticeId, String searchValue)。
// 曾经在 Go 侧写成 userName —— 结果是搜索框输什么都返回全部：
// 参数名对不上时 Go 只拿到空值，不报错，页面看起来"正常"只是没过滤。
func TestNoticeReadUsersSearch(t *testing.T) {
	id := createNotice(t, newNoticePayload("search"))
	mustOK(t, doPost(t, "/system/notice/markRead?noticeId="+idPath(id), nil), "admin 标记已读")

	base := "/system/notice/readUsers/list?pageNum=1&pageSize=20&noticeId=" + idPath(id)

	// 用登录名搜得到
	rows := pageRows(t, doGet(t, base+"&searchValue=admin"), "按登录名搜索")
	if findBy(rows, "userName", "admin") == nil {
		t.Errorf("searchValue=admin 应能搜到 admin，实际 %d 条", len(rows))
	}

	// Java 是 user_name OR nick_name 两列都搜，前端 placeholder 也写着"登录名称 / 用户名称"。
	// admin 的 nick_name 是「若依」（「管理员」是 remark，不参与搜索）
	rows = pageRows(t, doGet(t, base+"&searchValue="+url.QueryEscape("若依")), "按用户名称搜索")
	if len(rows) == 0 {
		t.Error("searchValue 应同时匹配 nick_name（admin 的昵称是「若依」），实际搜不到")
	}

	// 搜一个一定不存在的，必须真的过滤掉 —— 这条才是防"参数没接上"的关键：
	// 参数名写错时这里会返回全部，而不是 0 条
	rows = pageRows(t, doGet(t, base+"&searchValue="+url.QueryEscape("zz_一定不存在的用户")), "搜不存在的用户")
	if len(rows) != 0 {
		t.Errorf("搜索条件应当生效，期望 0 条，实际返回 %d 条 —— 说明 searchValue 根本没被接收", len(rows))
	}
}

// TestNoticeDetailNeedsNoPermission 公告详情不能挂权限标识。
//
// 顶栏那个公告铃铛（HeaderNotice/DetailView.vue）就是调 getNotice(id) 展开正文的，
// 所有登录用户都点得到。挂上 system:notice:query 之后，
// 没有公告管理权限的普通员工点开就是 403 —— Java 版这个方法上没有 @PreAuthorize。
func TestNoticeDetailNeedsNoPermission(t *testing.T) {
	id := createNotice(t, newNoticePayload("perm"))

	// 建一个只有普通角色、没有任何公告权限的账号
	userBody := newUserPayload("noticeperm")
	createUser(t, userBody)
	token := mustLogin(t, fmt.Sprint(userBody["userName"]))

	r := request(http.MethodGet, "/system/notice/"+idPath(id), token, nil)
	if r.Code == 403 {
		t.Fatal("普通用户读公告详情不该被拒 —— 顶栏公告铃铛就是调这个接口的")
	}
	mustOK(t, r, "普通用户查公告详情")

	// 顶栏列表同样是所有人可见
	mustOK(t, request(http.MethodGet, "/system/notice/listTop", token, nil), "普通用户查顶栏公告")
}

// TestNoticeReadUsersNeedsListPermission 对齐 Java SysNoticeController：
// 已读用户列表明确要求 system:notice:list，不能因为顶栏公告相关接口免权限，
// 就把这个管理端接口也一起放开。
func TestNoticeReadUsersNeedsListPermission(t *testing.T) {
	id := createNotice(t, newNoticePayload("readusersperm"))
	mustOK(t, doPost(t, "/system/notice/markRead?noticeId="+idPath(id), nil), "管理员标记公告已读")

	userBody := newUserPayload("readusersperm")
	userBody["roleIds"] = []int64{} // 明确不给任何角色和公告权限
	createUser(t, userBody)
	token := mustLogin(t, fmt.Sprint(userBody["userName"]))

	r := request(http.MethodGet,
		"/system/notice/readUsers/list?pageNum=1&pageSize=10&noticeId="+idPath(id), token, nil)
	if r.Code != 403 {
		t.Fatalf("无 system:notice:list 权限时 Java 会返回 403，Go 实际 code=%d，msg=%q", r.Code, r.Msg)
	}
}

// TestNoticeValidation 公告的字段校验。
func TestNoticeValidation(t *testing.T) {
	keep := func(p map[string]any) map[string]any { return p }

	cases := []struct {
		name   string
		mutate func(map[string]any) map[string]any
		wantIn string
	}{
		{"完整数据", keep, ""},
		{"少传 noticeTitle", func(p map[string]any) map[string]any { return omit(p, "noticeTitle") }, "NoticeTitle"},
		{"noticeTitle 纯空格", func(p map[string]any) map[string]any { return with(p, "noticeTitle", "  ") }, "NoticeTitle"},
		{"noticeTitle 恰好 50 字", func(p map[string]any) map[string]any { return with(p, "noticeTitle", repeatText(50)) }, ""},
		{"noticeTitle 超过 50 字", func(p map[string]any) map[string]any { return with(p, "noticeTitle", repeatText(51)) }, "NoticeTitle"},
		// @Xss：标题不能含 HTML 标签
		{"noticeTitle 含 HTML", func(p map[string]any) map[string]any {
			return with(p, "noticeTitle", "<script>alert(1)</script>")
		}, "NoticeTitle"},
		{"少传 noticeType", func(p map[string]any) map[string]any { return omit(p, "noticeType") }, "NoticeType"},
		// 正文不限长度（longblob），且允许 HTML
		{"正文含 HTML", func(p map[string]any) map[string]any {
			return with(p, "noticeContent", "<p><img src=x></p>")
		}, ""},
		{"少传正文", func(p map[string]any) map[string]any { return omit(p, "noticeContent") }, ""},

		// 注意 sys_notice.remark 是 varchar(255)，不是其它表的 500
		{"remark 恰好 255 字", func(p map[string]any) map[string]any { return with(p, "remark", repeatText(255)) }, ""},
		{"remark 超过 255 字", func(p map[string]any) map[string]any { return with(p, "remark", repeatText(256)) }, "Remark"},

		{"多传未知字段", func(p map[string]any) map[string]any { return with(p, "hello", "world") }, ""},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.mutate(newNoticePayload(fmt.Sprintf("v%d", i)))
			r := doPost(t, "/system/notice", body)
			if tc.wantIn == "" {
				mustOK(t, r, tc.name)
				return
			}
			mustFail(t, r, tc.wantIn, tc.name)
		})
	}
}
