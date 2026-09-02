package apitest

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// TestMultiRolePermissionsGrouped 验证批量查询后，总权限取并集，角色权限仍各自分组。
// 如果只验证总权限，错误地把并集回填给每个角色也会假通过，进而放宽数据权限。
func TestMultiRolePermissionsGrouped(t *testing.T) {
	type menuPermission struct {
		id   int64
		perm string
	}
	var selected []menuPermission
	for _, menu := range dataArray(t, doGet(t, "/system/menu/list"), "菜单列表") {
		perm, _ := menu["perms"].(string)
		perm = strings.TrimSpace(perm)
		if perm == "" || strings.Contains(perm, ",") {
			continue
		}
		if len(selected) > 0 && selected[0].perm == perm {
			continue
		}
		selected = append(selected, menuPermission{id: idOf(t, menu, "menuId"), perm: perm})
		if len(selected) == 2 {
			break
		}
	}
	if len(selected) < 2 {
		t.Fatal("至少需要两个权限标识不同的菜单")
	}

	firstRole := newRolePayload("perm_batch_a")
	firstRole["menuIds"] = []int64{selected[0].id}
	firstRoleID := createRole(t, firstRole)
	secondRole := newRolePayload("perm_batch_b")
	secondRole["menuIds"] = []int64{selected[1].id}
	secondRoleID := createRole(t, secondRole)

	userBody := newUserPayload("perm_batch")
	userBody["roleIds"] = []int64{firstRoleID, secondRoleID}
	createUser(t, userBody)
	token := mustLogin(t, fmt.Sprint(userBody["userName"]))
	info := requestOK(t, token, "/getInfo")

	all := toObjects2Strings(t, info.Raw["permissions"], "总权限")
	if !contains(all, selected[0].perm) || !contains(all, selected[1].perm) {
		t.Fatalf("总权限应包含两个角色的权限，实际 %v", all)
	}

	user := info.Raw["user"].(map[string]any)
	roles := user["roles"].([]any)
	wants := map[int64]string{firstRoleID: selected[0].perm, secondRoleID: selected[1].perm}
	for _, raw := range roles {
		role := raw.(map[string]any)
		roleID := idOf(t, role, "roleId")
		want, ok := wants[roleID]
		if !ok {
			continue
		}
		permissions := toObjects2Strings(t, role["permissions"], "角色权限")
		if !contains(permissions, want) {
			t.Errorf("角色 %d 应包含权限 %q，实际 %v", roleID, want, permissions)
		}
		delete(wants, roleID)
	}
	if len(wants) != 0 {
		t.Fatalf("getInfo 缺少角色或角色权限未分组：%v", wants)
	}
}

// TestPermissionMatrix 逐个接口验证「未登录 / 已登录无权限 / 有权限」三种身份。
//
// 【为什么必须有这个文件】
// 其余测试全部用 admin 的 token，而 admin 拥有 *:*:*、跳过一切权限校验 ——
// 于是**漏挂权限和多挂权限一个都测不出来**：
//
//   - 漏挂：`readUsers/list` 曾经没挂 system:notice:list，
//     任何登录用户都能拉到谁读了公告（含登录名、昵称、部门、手机号）。
//     用 admin 测永远是 200，看不出问题。
//   - 多挂：`importTemplate` 曾经多挂了 system:user:import，
//     Java 只要求登录。用 admin 测同样是 200。
//
// 判断依据只有一条：**打开 Java 版对应方法，看它有没有 @PreAuthorize**。
// 路由里的注释不算证据 —— 上面两处错误，注释都写着"与 Java 一致"。
func TestPermissionMatrix(t *testing.T) {
	// 建一个明确没有任何角色的账号。不能依赖 common 角色的当前菜单配置：
	// 角色权限属于运行期数据，当前库和后续换库都可能不同。
	userBody := newUserPayload("permmatrix")
	userBody["roleIds"] = []int64{}
	createUser(t, userBody)
	plainToken := mustLogin(t, fmt.Sprint(userBody["userName"]))

	cases := []struct {
		name string
		// method 为空表示 GET
		method string
		path   string
		// needPermission 为真表示 Java 上有 @PreAuthorize，普通用户应当 403
		needPermission bool
		why            string
	}{
		{
			name: "顶栏公告列表", path: "/system/notice/listTop",
			needPermission: false,
			why:            "Java 无 @PreAuthorize，顶栏铃铛所有人都要能看",
		},
		{
			name: "公告详情", path: "/system/notice/1",
			needPermission: false,
			why:            "Java 无 @PreAuthorize，HeaderNotice/DetailView.vue 调的就是它",
		},
		{
			name: "公告已读用户列表", path: "/system/notice/readUsers/list?pageNum=1&pageSize=10&noticeId=1",
			needPermission: true,
			why:            "Java 有 @PreAuthorize('system:notice:list')，返回的是他人的姓名部门手机号",
		},
		{
			name: "用户导入模板", method: http.MethodPost, path: "/system/user/importTemplate",
			needPermission: false,
			why:            "Java 无 @PreAuthorize，模板只有表头不含数据",
		},
		{
			name: "用户列表", path: "/system/user/list?pageNum=1&pageSize=10",
			needPermission: true,
			why:            "Java 有 @PreAuthorize('system:user:list')",
		},
		{
			name: "个人信息", path: "/system/user/profile",
			needPermission: false,
			why:            "Java 无 @PreAuthorize，人人都要能看自己的资料",
		},
		{
			name: "字典按类型查", path: "/system/dict/data/type/sys_normal_disable",
			needPermission: false,
			why:            "Java 无 @PreAuthorize，前端所有下拉框都要用",
		},
		{
			name: "菜单树选择", path: "/system/menu/treeselect",
			needPermission: false,
			why:            "Java 无 @PreAuthorize，新增角色时要用，那时未必有菜单管理权限",
		},
		{
			name: "参数按键取值", path: "/system/config/configKey/sys.user.initPassword",
			needPermission: true,
			why:            "Go 安全差异：动态参数值可能包含初始密码，仅允许参数或用户管理权限读取",
		},
		{
			name: "定时任务列表", path: "/monitor/job/list?pageNum=1&pageSize=10",
			needPermission: true,
			why:            "Java 有 @PreAuthorize('monitor:job:list')",
		},
		{
			name: "在线用户", path: "/monitor/online/list?pageNum=1&pageSize=10",
			needPermission: true,
			why:            "Java 有 @PreAuthorize('monitor:online:list')",
		},
	}

	for _, tc := range cases {
		method := tc.method
		if method == "" {
			method = http.MethodGet
		}

		t.Run(tc.name+"-未登录", func(t *testing.T) {
			r := request(method, tc.path, "", nil)
			if r.Status != http.StatusOK {
				t.Fatalf("未认证时 HTTP 状态码也应为 200，实际 %d", r.Status)
			}
			if r.Code != 401 {
				t.Errorf("未登录应返回 401，实际 %d（msg=%q）", r.Code, r.Msg)
			}
		})

		t.Run(tc.name+"-已登录无权限", func(t *testing.T) {
			r := request(method, tc.path, plainToken, nil)

			if tc.needPermission {
				if r.Code != 403 {
					t.Errorf("%s —— 应当拒绝无权限用户（403），实际 code=%d。%s",
						tc.name, r.Code, tc.why)
				}
				return
			}
			if r.Code == 403 {
				t.Errorf("%s —— 不该要求权限，普通用户被拒了。%s", tc.name, tc.why)
			}
		})

		t.Run(tc.name+"-管理员", func(t *testing.T) {
			r := request(method, tc.path, adminToken, nil)
			if r.Code == 401 || r.Code == 403 {
				t.Errorf("管理员不该被拒，实际 code=%d（msg=%q）", r.Code, r.Msg)
			}
		})
	}
}

// TestAllProtectedRoutesRejectNoPermission 按 Java Controller 的
// @PreAuthorize 清单覆盖所有业务权限路由。这里故意只验证无角色用户必须 403：
// 权限中间件应先于 handler 返回，因此请求体可以留空，也不会触发业务写入。
// 清空日志、清缓存等全局操作即使路由误配也不能在当前库试，留到独立库。
func TestAllProtectedRoutesRejectNoPermission(t *testing.T) {
	userBody := newUserPayload("allprotected")
	userBody["roleIds"] = []int64{}
	createUser(t, userBody)
	token := mustLogin(t, fmt.Sprint(userBody["userName"]))

	type routeCase struct {
		name   string
		method string
		path   string
	}
	cases := []routeCase{
		// 定时任务与调度日志
		{"任务列表", http.MethodGet, "/monitor/job/list"},
		{"任务导出", http.MethodPost, "/monitor/job/export"},
		{"任务状态", http.MethodPut, "/monitor/job/changeStatus"},
		{"任务执行", http.MethodPut, "/monitor/job/run"},
		{"任务详情", http.MethodGet, "/monitor/job/999999999"},
		{"任务新增", http.MethodPost, "/monitor/job"},
		{"任务修改", http.MethodPut, "/monitor/job"},
		{"任务删除", http.MethodDelete, "/monitor/job/999999999"},
		{"调度日志列表", http.MethodGet, "/monitor/jobLog/list"},
		{"调度日志导出", http.MethodPost, "/monitor/jobLog/export"},
		{"调度日志详情", http.MethodGet, "/monitor/jobLog/999999999"},
		{"调度日志删除", http.MethodDelete, "/monitor/jobLog/999999999"},

		// 系统监控
		{"在线用户列表", http.MethodGet, "/monitor/online/list"},
		{"在线用户强退", http.MethodDelete, "/monitor/online/not-exist-token"},
		{"服务监控", http.MethodGet, "/monitor/server"},
		{"缓存监控", http.MethodGet, "/monitor/cache"},
		{"缓存名称", http.MethodGet, "/monitor/cache/getNames"},
		{"缓存键", http.MethodGet, "/monitor/cache/getKeys/sys_config"},
		{"缓存值", http.MethodGet, "/monitor/cache/getValue/sys_config/not-exist-key"},

		// 岗位
		{"岗位列表", http.MethodGet, "/system/post/list"},
		{"岗位导出", http.MethodPost, "/system/post/export"},
		{"岗位详情", http.MethodGet, "/system/post/999999999"},
		{"岗位新增", http.MethodPost, "/system/post"},
		{"岗位修改", http.MethodPut, "/system/post"},
		{"岗位删除", http.MethodDelete, "/system/post/999999999"},

		// 部门
		{"部门列表", http.MethodGet, "/system/dept/list"},
		{"部门排除子树", http.MethodGet, "/system/dept/list/exclude/999999999"},
		{"部门排序", http.MethodPut, "/system/dept/updateSort"},
		{"部门详情", http.MethodGet, "/system/dept/999999999"},
		{"部门新增", http.MethodPost, "/system/dept"},
		{"部门修改", http.MethodPut, "/system/dept"},
		{"部门删除", http.MethodDelete, "/system/dept/999999999"},

		// 角色与角色授权
		{"角色列表", http.MethodGet, "/system/role/list"},
		{"角色导出", http.MethodPost, "/system/role/export"},
		{"角色选项", http.MethodGet, "/system/role/optionselect"},
		{"角色部门树", http.MethodGet, "/system/role/deptTree/999999999"},
		{"已分配用户", http.MethodGet, "/system/role/authUser/allocatedList?roleId=999999999"},
		{"未分配用户", http.MethodGet, "/system/role/authUser/unallocatedList?roleId=999999999"},
		{"取消单个授权", http.MethodPut, "/system/role/authUser/cancel"},
		{"批量取消授权", http.MethodPut, "/system/role/authUser/cancelAll?roleId=999999999&userIds=999999999"},
		{"批量选择授权", http.MethodPut, "/system/role/authUser/selectAll?roleId=999999999&userIds=999999999"},
		{"角色数据权限", http.MethodPut, "/system/role/dataScope"},
		{"角色状态", http.MethodPut, "/system/role/changeStatus"},
		{"角色详情", http.MethodGet, "/system/role/999999999"},
		{"角色新增", http.MethodPost, "/system/role"},
		{"角色修改", http.MethodPut, "/system/role"},
		{"角色删除", http.MethodDelete, "/system/role/999999999"},

		// 用户
		{"用户列表", http.MethodGet, "/system/user/list"},
		{"用户导出", http.MethodPost, "/system/user/export"},
		{"用户导入", http.MethodPost, "/system/user/importData"},
		{"用户部门树", http.MethodGet, "/system/user/deptTree"},
		{"用户重置密码", http.MethodPut, "/system/user/resetPwd"},
		{"用户状态", http.MethodPut, "/system/user/changeStatus"},
		{"用户授权角色详情", http.MethodGet, "/system/user/authRole/999999999"},
		{"用户保存授权角色", http.MethodPut, "/system/user/authRole?userId=999999999"},
		{"用户新增表单无斜杠", http.MethodGet, "/system/user"},
		{"用户新增表单有斜杠", http.MethodGet, "/system/user/"},
		{"用户详情", http.MethodGet, "/system/user/999999999"},
		{"用户新增", http.MethodPost, "/system/user"},
		{"用户修改", http.MethodPut, "/system/user"},
		{"用户删除", http.MethodDelete, "/system/user/999999999"},

		// 菜单
		{"菜单列表", http.MethodGet, "/system/menu/list"},
		{"菜单排序", http.MethodPut, "/system/menu/updateSort"},
		{"菜单详情", http.MethodGet, "/system/menu/999999999"},
		{"菜单新增", http.MethodPost, "/system/menu"},
		{"菜单修改", http.MethodPut, "/system/menu"},
		{"菜单删除", http.MethodDelete, "/system/menu/999999999"},

		// 参数配置
		{"参数列表", http.MethodGet, "/system/config/list"},
		{"参数导出", http.MethodPost, "/system/config/export"},
		{"参数详情", http.MethodGet, "/system/config/999999999"},
		{"参数新增", http.MethodPost, "/system/config"},
		{"参数修改", http.MethodPut, "/system/config"},
		{"参数删除", http.MethodDelete, "/system/config/999999999"},

		// 字典数据与字典类型
		{"字典数据列表", http.MethodGet, "/system/dict/data/list"},
		{"字典数据导出", http.MethodPost, "/system/dict/data/export"},
		{"字典数据详情", http.MethodGet, "/system/dict/data/999999999"},
		{"字典数据新增", http.MethodPost, "/system/dict/data"},
		{"字典数据修改", http.MethodPut, "/system/dict/data"},
		{"字典数据删除", http.MethodDelete, "/system/dict/data/999999999"},
		{"字典类型列表", http.MethodGet, "/system/dict/type/list"},
		{"字典类型导出", http.MethodPost, "/system/dict/type/export"},
		{"字典类型详情", http.MethodGet, "/system/dict/type/999999999"},
		{"字典类型新增", http.MethodPost, "/system/dict/type"},
		{"字典类型修改", http.MethodPut, "/system/dict/type"},
		{"字典类型删除", http.MethodDelete, "/system/dict/type/999999999"},

		// 通知公告
		{"公告列表", http.MethodGet, "/system/notice/list"},
		{"公告已读用户", http.MethodGet, "/system/notice/readUsers/list?noticeId=999999999"},
		{"公告新增", http.MethodPost, "/system/notice"},
		{"公告修改", http.MethodPut, "/system/notice"},
		{"公告删除", http.MethodDelete, "/system/notice/999999999"},

		// 登录日志与操作日志（清空接口留到独立库）
		{"登录日志列表", http.MethodGet, "/monitor/logininfor/list"},
		{"登录日志导出", http.MethodPost, "/monitor/logininfor/export"},
		{"登录日志解锁", http.MethodGet, "/monitor/logininfor/unlock/not-exist-user"},
		{"登录日志删除", http.MethodDelete, "/monitor/logininfor/999999999"},
		{"操作日志列表", http.MethodGet, "/monitor/operlog/list"},
		{"操作日志导出", http.MethodPost, "/monitor/operlog/export"},
		{"操作日志删除", http.MethodDelete, "/monitor/operlog/999999999"},
	}

	for i, tc := range cases {
		t.Run(fmt.Sprintf("%03d-%s", i+1, tc.name), func(t *testing.T) {
			r := request(tc.method, tc.path, token, nil)
			if r.Code != 403 {
				t.Errorf("%s %s 缺少业务权限时应返回 403，实际 code=%d，msg=%q",
					tc.method, tc.path, r.Code, r.Msg)
			}
		})
	}
}
