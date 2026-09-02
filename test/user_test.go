package apitest

import (
	"bytes"
	"context"
	"fmt"
	"hash/crc32"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/redisx"
)

const (
	adminUserID = 1
	// testDeptID 研发部门，RuoYi 初始数据里就有
	testDeptID = 103
	// commonRoleID 普通角色
	commonRoleID = 2
)

func newUserPayload(suffix string) map[string]any {
	userName := testPrefix + suffix
	if len([]rune(userName)) > 20 {
		userName = fmt.Sprintf("%su%08x", testPrefix, crc32.ChecksumIEEE([]byte(suffix)))
	}
	return map[string]any{
		"userName":    userName,
		"nickName":    testPrefix + "昵称" + suffix,
		"password":    "test123456",
		"deptId":      testDeptID,
		"email":       "zz_" + suffix + "@example.com",
		"phonenumber": "",
		"sex":         "0",
		"status":      "0",
		"postIds":     []int64{},
		"roleIds":     []int64{commonRoleID},
		"remark":      "测试用户",
	}
}

func createUser(t *testing.T, body map[string]any) int64 {
	t.Helper()
	mustOK(t, doPost(t, "/system/user", body), "新增用户")

	name := fmt.Sprint(body["userName"])
	list := pageRows(t, doGet(t, "/system/user/list?pageSize=100&userName="+url.QueryEscape(name)), "查用户")
	item := findBy(list, "userName", name)
	if item == nil {
		t.Fatalf("新增用户后按账号 %s 查不到", name)
	}
	id := idOf(t, item, "userId")
	trackRedisKey(redisx.LoginUserSessionsKey(id))
	trackRedisKey(redisx.LoginUserGenerationKey(id))

	t.Cleanup(func() { _ = doDelete(t, "/system/user/"+idPath(id)) })
	return id
}

// TestUserCRUD 用户增删改查。
func TestUserCRUD(t *testing.T) {
	body := newUserPayload("crud")
	id := createUser(t, body)
	path := "/system/user/" + idPath(id)

	detail := dataObject(t, doGet(t, path), "查用户")
	assertField(t, detail, "userName", body["userName"], "新增后")
	assertField(t, detail, "nickName", body["nickName"], "新增后")
	assertField(t, detail, "email", body["email"], "新增后")
	assertField(t, detail, "deptId", testDeptID, "新增后")
	assertString(t, detail, "用户详情", "status", "sex")

	// 新建用户的 pwdUpdateDate 必须是空的 —— 前端靠它弹"请修改初始密码"。
	// 键可以不出现，也可以是 null，但不能是一个真实时间。
	if v, ok := detail["pwdUpdateDate"]; ok && v != nil && v != "" {
		t.Errorf("新建用户的 pwdUpdateDate 应为空（Java 版建号时不写这个字段），实际 %v", v)
	}

	updated := payload(body)
	updated["userId"] = id
	updated["nickName"] = testPrefix + "改名"
	updated["remark"] = "改后的备注"
	delete(updated, "password") // 编辑时前端不传密码
	mustOK(t, doPut(t, "/system/user", updated), "修改用户")

	detail = dataObject(t, doGet(t, path), "改后查用户")
	assertField(t, detail, "nickName", testPrefix+"改名", "改后")
	assertField(t, detail, "remark", "改后的备注", "改后")

	mustOK(t, doDelete(t, path), "删除用户")
	mustFail(t, doGet(t, path), "用户不存在", "删除后再查")
}

func TestUserListDeptProjectionAndQueryCount(t *testing.T) {
	body := newUserPayload("list_dept")
	createUser(t, body)
	userName := fmt.Sprint(body["userName"])

	expectedDept := dataObject(t, doGet(t, "/system/dept/"+idPath(testDeptID)), "查询预期部门")
	rows := pageRows(t, doGet(t, "/system/user/list?pageSize=10&userName="+url.QueryEscape(userName)), "用户列表部门投影")
	item := findBy(rows, "userName", userName)
	if item == nil {
		t.Fatalf("用户列表找不到测试用户 %s", userName)
	}
	dept, ok := item["dept"].(map[string]any)
	if !ok {
		t.Fatalf("用户列表必须返回部门对象，实际 %T(%v)", item["dept"], item["dept"])
	}
	for _, key := range []string{"deptId", "deptName", "leader"} {
		assertField(t, dept, key, expectedDept[key], "用户列表部门投影")
	}

	// 只统计带有本测试 context 标记的查询，避免后台任务影响计数。
	type queryCountKey struct{}
	ctx := context.WithValue(context.Background(), queryCountKey{}, true)
	var queryCount atomic.Int32
	callbackName := fmt.Sprintf("test:user-page-query-count:%p", &queryCount)
	db := repository.DB(ctx)
	if err := db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if marked, _ := tx.Statement.Context.Value(queryCountKey{}).(bool); marked {
			queryCount.Add(1)
		}
	}); err != nil {
		t.Fatalf("注册查询计数器失败：%v", err)
	}
	defer func() {
		if err := db.Callback().Query().Remove(callbackName); err != nil {
			t.Errorf("移除查询计数器失败：%v", err)
		}
	}()

	list, total, err := repository.SelectUserPage(ctx, model.UserQuery{UserName: userName},
		page.Query{PageNum: 1, PageSize: 10}, nil)
	if err != nil {
		t.Fatalf("直接查询用户分页失败：%v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("用户分页结果错误：total=%d len=%d", total, len(list))
	}
	if got := queryCount.Load(); got != 2 {
		t.Fatalf("用户分页应固定为 COUNT + 分页 JOIN 两条 SQL，实际 %d 条", got)
	}
}

// TestUserDeleteRevokesSessions 删除账号后，已经签发的会话必须立即失效。
func TestUserDeleteRevokesSessions(t *testing.T) {
	body := newUserPayload("delete_session")
	id := createUser(t, body)
	token, err := loginAs(fmt.Sprint(body["userName"]), "test123456")
	if err != nil {
		t.Fatalf("测试账号登录失败：%v", err)
	}

	mustOK(t, doDelete(t, "/system/user/"+idPath(id)), "删除在线用户")
	assertTokenUnauthorized(t, token, "删除用户")
}

func TestFullUserEditDisablesExistingSession(t *testing.T) {
	body := newUserPayload("edit_disable_session")
	id := createUser(t, body)
	token, err := loginAs(fmt.Sprint(body["userName"]), "test123456")
	if err != nil {
		t.Fatalf("测试账号登录失败：%v", err)
	}

	updated := payload(body)
	updated["userId"] = id
	updated["status"] = "1"
	delete(updated, "password")
	mustOK(t, doPut(t, "/system/user", updated), "完整编辑停用用户")
	assertTokenUnauthorized(t, token, "完整编辑停用用户")
}

func TestMissingUserUpdatesDoNotReportSuccess(t *testing.T) {
	const missingID = int64(9_000_000_000_000)
	mustFail(t, doPut(t, "/system/user/changeStatus", map[string]any{
		"userId": missingID, "status": "1",
	}), "", "修改不存在用户状态")
	mustFail(t, doPut(t, "/system/user/resetPwd", map[string]any{
		"userId": missingID, "password": "test123456",
	}), "", "重置不存在用户密码")
}

// TestUserBatchDeleteRevokesSessions 批量删除只走一次兼容扫描，但每个用户的会话都要撤销。
func TestUserBatchDeleteRevokesSessions(t *testing.T) {
	body1 := newUserPayload("batchdel1")
	body2 := newUserPayload("batchdel2")
	id1 := createUser(t, body1)
	id2 := createUser(t, body2)
	token1, err := loginAs(fmt.Sprint(body1["userName"]), "test123456")
	if err != nil {
		t.Fatalf("第一个测试账号登录失败：%v", err)
	}
	token2, err := loginAs(fmt.Sprint(body2["userName"]), "test123456")
	if err != nil {
		t.Fatalf("第二个测试账号登录失败：%v", err)
	}

	path := fmt.Sprintf("/system/user/%d,%d", id1, id2)
	mustOK(t, doDelete(t, path), "批量删除在线用户")
	assertTokenUnauthorized(t, token1, "批量删除第一个用户")
	assertTokenUnauthorized(t, token2, "批量删除第二个用户")
}

func TestPasswordResetRevokesBeforeFailedDatabaseWrite(t *testing.T) {
	body := newUserPayload("revoke_before_write")
	userID := createUser(t, body)
	token := mustLogin(t, fmt.Sprint(body["userName"]))
	if err := repository.DeleteUserByIDs(context.Background(), []int64{userID}); err != nil {
		t.Fatalf("清理测试用户关联失败：%v", err)
	}
	if err := repository.DB(context.Background()).Where("user_id = ?", userID).Delete(&model.SysUser{}).Error; err != nil {
		t.Fatalf("物理删除测试用户失败：%v", err)
	}
	admin, err := repository.SelectUserByID(context.Background(), model.AdminUserID)
	if err != nil || admin == nil {
		t.Fatalf("读取管理员失败：admin=%v err=%v", admin, err)
	}

	err = service.ResetUserPwd(context.Background(), admin, userID, "newPassword123", "admin")
	if err == nil {
		t.Fatal("不存在的用户应使数据库更新失败")
	}
	assertTokenUnauthorized(t, token, "数据库写入失败前也必须先撤销旧会话")
}

// TestUserDetailContract 用户详情是混合形态：data 与四个平铺字段并存。
func TestUserDetailContract(t *testing.T) {
	id := createUser(t, newUserPayload("detail"))

	r := doGet(t, "/system/user/"+idPath(id))
	mustOK(t, r, "查用户详情")
	assertTopLevel(t, r, "用户详情", "code", "msg", "data", "postIds", "roleIds", "roles", "posts")

	roleIDs, ok := r.Raw["roleIds"].([]any)
	if !ok || len(roleIDs) == 0 {
		t.Errorf("新增时指定了角色，roleIds 不该为空，实际 %v", r.Raw["roleIds"])
	}

	// 【提权防线】候选角色列表里不能出现 admin 角色，
	// 否则任何有用户编辑权的人都能把别人提成超级管理员
	roles := toObjects(t, r.Raw["roles"], "用户详情的 roles")
	if findBy(roles, "roleId", adminRoleID) != nil {
		t.Errorf("普通用户的候选角色里不该出现超级管理员角色（roleId=%d）", adminRoleID)
	}

	// admin 自己编辑自己时才看得到 admin 角色
	r = doGet(t, "/system/user/"+idPath(adminUserID))
	mustOK(t, r, "查 admin 详情")
	roles = toObjects(t, r.Raw["roles"], "admin 详情的 roles")
	if findBy(roles, "roleId", adminRoleID) == nil {
		t.Errorf("admin 自己的候选角色里应包含超级管理员角色，否则编辑一次就把自己的角色弄丢了")
	}
}

// TestUserNewFormContract 新增用户弹窗：不带 userId，只有 roles 和 posts。
func TestUserNewFormContract(t *testing.T) {
	r := doGet(t, "/system/user")
	mustOK(t, r, "新增用户弹窗")

	assertTopLevel(t, r, "新增用户弹窗", "code", "msg", "roles", "posts")
	assertNoTopLevel(t, r, "新增用户弹窗", "data", "postIds", "roleIds")

	roles := toObjects(t, r.Raw["roles"], "新增弹窗的 roles")
	if findBy(roles, "roleId", adminRoleID) != nil {
		t.Errorf("新增用户时的候选角色里不该出现超级管理员角色")
	}
}

// TestUserPasswordNeverReturned 任何接口都不能把密码带出去。
//
// helper 里每个请求都做了递归检查，这里再显式确认几个最容易漏的位置：
// 密码字段应该整个不出现，而不是返回空串或掩码。
func TestUserPasswordNeverReturned(t *testing.T) {
	id := createUser(t, newUserPayload("pwd"))

	detail := dataObject(t, doGet(t, "/system/user/"+idPath(id)), "查用户")
	assertNoKey(t, detail, "password", "用户详情")

	rows := pageRows(t, doGet(t, "/system/user/list?pageSize=10"), "用户列表")
	for _, row := range rows {
		assertNoKey(t, row, "password", "用户列表行")
	}

	profile := dataObject(t, doGet(t, "/system/user/profile"), "个人信息")
	assertNoKey(t, profile, "password", "个人信息")
}

// TestUserResetPwd 重置密码后，新密码必须真的能登录。
//
// 只断言接口返回 200 是不够的：密码存成明文、存错列、bcrypt 没生效，
// 接口一样返回成功，只有真登录一次才能发现。
func TestUserResetPwd(t *testing.T) {
	body := newUserPayload("reset")
	id := createUser(t, body)
	userName := fmt.Sprint(body["userName"])

	token1, err := loginAs(userName, "test123456")
	if err != nil {
		t.Fatalf("新建用户应能用初始密码登录：%v", err)
	}
	token2, err := loginAs(userName, "test123456")
	if err != nil {
		t.Fatalf("第二个设备登录失败：%v", err)
	}

	mustOK(t, doPut(t, "/system/user/resetPwd", map[string]any{
		"userId": id, "password": "reset654321",
	}), "重置密码")
	assertTokenUnauthorized(t, token1, "管理员重置密码后的第一个会话")
	assertTokenUnauthorized(t, token2, "管理员重置密码后的第二个会话")

	if _, err := loginAs(userName, "reset654321"); err != nil {
		t.Errorf("重置后应能用新密码登录：%v", err)
	}
	if _, err := loginAs(userName, "test123456"); err == nil {
		t.Errorf("重置后旧密码不该还能登录")
	}
}

// TestUserChangeStatus 停用的账号不能登录。
func TestUserChangeStatus(t *testing.T) {
	body := newUserPayload("status")
	id := createUser(t, body)
	userName := fmt.Sprint(body["userName"])
	token1, err := loginAs(userName, "test123456")
	if err != nil {
		t.Fatalf("第一个设备登录失败：%v", err)
	}
	token2, err := loginAs(userName, "test123456")
	if err != nil {
		t.Fatalf("第二个设备登录失败：%v", err)
	}
	mustFail(t, doPut(t, "/system/user/changeStatus", map[string]any{
		"userId": id, "status": "2",
	}), "Status", "拒绝未知账号状态")

	mustOK(t, doPut(t, "/system/user/changeStatus", map[string]any{
		"userId": id, "status": "1",
	}), "停用账号（只传两个字段）")
	assertTokenUnauthorized(t, token1, "停用后的第一个会话")
	assertTokenUnauthorized(t, token2, "停用后的第二个会话")

	detail := dataObject(t, doGet(t, "/system/user/"+idPath(id)), "查账号状态")
	assertField(t, detail, "status", "1", "停用后")

	if _, err := loginAs(userName, "test123456"); err == nil {
		t.Errorf("停用的账号不该还能登录")
	}

	mustOK(t, doPut(t, "/system/user/changeStatus", map[string]any{
		"userId": id, "status": "0",
	}), "重新启用")
	assertTokenUnauthorized(t, token1, "重新启用后的旧会话")
	assertTokenUnauthorized(t, token2, "重新启用后的另一个旧会话")
	if _, err := loginAs(userName, "test123456"); err != nil {
		t.Errorf("重新启用后应能登录：%v", err)
	}
}

// TestUserAuthRole 分配角色：查询是平铺形态，保存的参数在 query string 里。
func TestUserAuthRole(t *testing.T) {
	id := createUser(t, newUserPayload("authrole"))

	r := doGet(t, "/system/user/authRole/"+idPath(id))
	mustOK(t, r, "查用户的可分配角色")
	assertTopLevel(t, r, "用户授权角色", "code", "msg", "user", "roles")
	assertNoTopLevel(t, r, "用户授权角色", "data")

	mustFail(t, doPut(t, fmt.Sprintf("/system/user/authRole?userId=%d&roleIds=%d", id, adminRoleID), nil),
		"不允许给普通用户分配超级管理员角色", "直接提交保留角色")

	// 参数走 query string，roleIds 是逗号拼接的
	mustOK(t, doPut(t, fmt.Sprintf("/system/user/authRole?userId=%d&roleIds=%d", id, commonRoleID), nil), "保存角色分配")

	detail := doGet(t, "/system/user/"+idPath(id))
	roleIDs, _ := detail.Raw["roleIds"].([]any)
	if len(roleIDs) != 1 || fmt.Sprint(roleIDs[0]) != fmt.Sprint(commonRoleID) {
		t.Errorf("保存后 roleIds 应为 [%d]，实际 %v", commonRoleID, detail.Raw["roleIds"])
	}

	// 一个角色都不选也要能保存（清空授权）
	mustOK(t, doPut(t, fmt.Sprintf("/system/user/authRole?userId=%d&roleIds=", id), nil), "清空角色分配")
	detail = doGet(t, "/system/user/"+idPath(id))
	if roleIDs, _ = detail.Raw["roleIds"].([]any); len(roleIDs) != 0 {
		t.Errorf("清空后 roleIds 应为空，实际 %v", detail.Raw["roleIds"])
	}
}

// TestUserAdminProtected 超级管理员账号不允许被改、停用、重置密码、删除。
func TestUserAdminProtected(t *testing.T) {
	body := newUserPayload("adminedit")
	body["userId"] = adminUserID
	mustFail(t, doPut(t, "/system/user", body), "不允许操作超级管理员用户", "修改 admin 账号")

	mustFail(t, doPut(t, "/system/user/changeStatus", map[string]any{
		"userId": adminUserID, "status": "1",
	}), "不允许操作超级管理员用户", "停用 admin 账号")

	mustFail(t, doPut(t, "/system/user/resetPwd", map[string]any{
		"userId": adminUserID, "password": "whatever123",
	}), "不允许操作超级管理员用户", "重置 admin 密码")

	// 删除自己会先撞"当前用户不能删除"，而当前登录的就是 admin
	mustFail(t, doDelete(t, "/system/user/"+idPath(adminUserID)), "当前用户不能删除", "删除 admin 账号")

	// 确认 admin 还能登录（前面几次操作没有半途写进去）
	if _, err := loginAs("admin", "admin123"); err != nil {
		t.Fatalf("admin 账号被上面的操作改坏了：%v", err)
	}
}

// TestUserProfile 个人中心：查、改资料、改密码。
//
// 全程用新建的测试账号，不碰 admin —— 改坏 admin 会让后面所有用例失败。
func TestUserProfile(t *testing.T) {
	body := newUserPayload("profile")
	createUser(t, body)
	userName := fmt.Sprint(body["userName"])

	token, err := loginAs(userName, "test123456")
	if err != nil {
		t.Fatalf("测试账号登录失败：%v", err)
	}

	// 查：混合形态，data 与 roleGroup / postGroup 并存
	r := request(http.MethodGet, "/system/user/profile", token, nil)
	mustOK(t, r, "查个人信息")
	assertTopLevel(t, r, "个人信息", "code", "msg", "data", "roleGroup", "postGroup")
	profile := dataObject(t, r, "个人信息")
	assertField(t, profile, "avatar", nil, "个人信息空头像应对齐 Java 会话对象")
	if _, ok := profile["params"].(map[string]any); !ok {
		t.Fatalf("个人信息 params 应为 Java 会话兼容对象，实际 %#v", profile["params"])
	}

	// 改资料
	r = request(http.MethodPut, "/system/user/profile", token, map[string]any{
		"nickName":    testPrefix + "自己改的昵称",
		"email":       "zz_profile_new@example.com",
		"phonenumber": "13800000001",
		"sex":         "1",
	})
	mustOK(t, r, "改个人资料")

	r = request(http.MethodGet, "/system/user/profile", token, nil)
	profile = dataObject(t, r, "改后的个人信息")
	assertField(t, profile, "nickName", testPrefix+"自己改的昵称", "改后")
	assertField(t, profile, "sex", "1", "改后")

	// 【越权防线】个人中心不能改部门、状态、角色 —— 改了就能绕过数据权限
	assertField(t, profile, "deptId", testDeptID, "个人中心不该能改自己的部门")
	assertField(t, profile, "status", "0", "个人中心不该能改自己的状态")

	r = request(http.MethodPut, "/system/user/profile", token, map[string]any{
		"nickName": testPrefix + "试图改部门",
		"deptId":   100,
		"status":   "1",
		"userId":   1,
	})
	mustOK(t, r, "带上部门和状态字段再改一次")
	profile = dataObject(t, request(http.MethodGet, "/system/user/profile", token, nil), "再改后的个人信息")
	assertField(t, profile, "deptId", testDeptID, "多传 deptId 也不该生效")
	assertField(t, profile, "status", "0", "多传 status 也不该生效")

	// 【改密码的参数在 JSON body 里，不是查询串】
	// 前端 updateUserPwd 是 `data: data`，Java 是 @RequestBody。
	// 这里原来写的是查询串 —— 和当时的实现一致，所以测试一直是绿的，
	// 而真实前端改密码从来没成功过。**测实现不测契约就是这个下场。**
	const pwdPath = "/system/user/profile/updatePwd"

	r = request(http.MethodPut, pwdPath, token, map[string]any{
		"oldPassword": "wrongpass", "newPassword": "newpass123",
	})
	mustFail(t, r, "旧密码错误", "旧密码填错")

	r = request(http.MethodPut, pwdPath, token, map[string]any{
		"oldPassword": "test123456", "newPassword": "test123456",
	})
	mustFail(t, r, "不能与旧密码相同", "新旧密码相同")

	r = request(http.MethodPut, pwdPath, token, map[string]any{
		"oldPassword": "test123456", "newPassword": "abc",
	})
	mustFail(t, r, "5 到 20", "新密码太短")

	// 【回归断言】查询串必须**不生效**。
	// 只要它还能改成功，就说明 handler 又在读 query，前端照样是坏的。
	r = request(http.MethodPut, pwdPath+"?oldPassword=test123456&newPassword=querypwd123", token, nil)
	if r.Code == 200 {
		t.Error("参数放在查询串里不该生效 —— handler 必须只读 JSON body")
	}
	if _, err := loginAs(userName, "querypwd123"); err == nil {
		t.Fatal("查询串里的新密码竟然生效了，说明 handler 还在读 c.Query")
	}

	r = request(http.MethodPut, pwdPath, token, map[string]any{
		"oldPassword": "test123456", "newPassword": "newpass123",
	})
	mustOK(t, r, "改密码")
	assertTokenUnauthorized(t, token, "个人改密后的当前会话")

	if _, err := loginAs(userName, "newpass123"); err != nil {
		t.Errorf("改密码后应能用新密码登录：%v", err)
	}
}

func assertTokenUnauthorized(t *testing.T, token, what string) {
	t.Helper()
	r := request(http.MethodGet, "/getInfo", token, nil)
	if r.Code != 401 {
		t.Fatalf("%s后旧 token 应失效（code=401），实际 code=%d，msg=%q", what, r.Code, r.Msg)
	}
}

// TestUserImportTemplate 导入模板必须包含"部门编号"列。
//
// 【这是回归测试】模板一度复用了导出的列集合，导致标了 type:import 的
// 部门编号列没进模板。用户照模板填完导入，所有人都没有部门，
// 而接口全程返回成功。
func TestUserImportTemplate(t *testing.T) {
	r := request(http.MethodPost, "/system/user/importTemplate", adminToken, nil)
	if r.Status != http.StatusOK {
		t.Fatalf("下载导入模板：HTTP 状态码应为 200，实际 %d", r.Status)
	}
	if len(r.Body) == 0 {
		t.Fatal("导入模板不该是空文件")
	}

	file, err := excelize.OpenReader(bytes.NewReader(r.Body))
	if err != nil {
		t.Fatalf("导入模板不是合法的 xlsx（前 200 字节：%s）：%v", truncBody(r.Body), err)
	}
	defer file.Close()

	rows, err := file.GetRows(file.GetSheetName(0))
	if err != nil || len(rows) == 0 {
		t.Fatalf("读不到模板的表头行：%v", err)
	}
	header := strings.Join(rows[0], "|")

	for _, want := range []string{"部门编号", "登录名称", "用户名称"} {
		if !strings.Contains(header, want) {
			t.Errorf("导入模板的表头应包含 %q，实际表头=%s", want, header)
		}
	}
	// 导出专用列不该出现在导入模板里
	for _, unwanted := range []string{"用户序号", "最后登录IP", "部门名称"} {
		if strings.Contains(header, unwanted) {
			t.Errorf("导入模板不该出现导出专用列 %q，实际表头=%s", unwanted, header)
		}
	}
}

// TestUserImportEmptyTemplate 对齐 Java：上传只有表头的合法模板时，
// 应返回明确的“数据不能为空”，而不是把它误判成损坏的 Excel。
func TestUserImportEmptyTemplate(t *testing.T) {
	template := request(http.MethodPost, "/system/user/importTemplate", adminToken, nil)
	if template.Status != http.StatusOK || len(template.Body) == 0 {
		t.Fatalf("准备空导入模板失败：HTTP=%d，响应=%s", template.Status, truncBody(template.Body))
	}

	r := requestMultipart(http.MethodPost, "/system/user/importData", adminToken,
		"file", "user_template.xlsx", template.Body)
	mustFail(t, r, "导入用户数据不能为空！", "导入只有表头的用户模板")
}

func TestUserImportAcceptsLegacyXLS(t *testing.T) {
	data, err := os.ReadFile("../pkg/excelx/testdata/table.xls")
	if err != nil {
		t.Fatalf("读取旧版 Excel 样本失败：%v", err)
	}
	r := requestMultipart(http.MethodPost, "/system/user/importData", adminToken,
		"file", "legacy_users.xls", data)
	if strings.Contains(r.Msg, "解析 Excel 失败") {
		t.Fatalf(".xls 应进入业务字段校验而不是文件解析失败，实际 msg=%q", r.Msg)
	}
	mustFail(t, r, "登录名称不能为空", "旧版 .xls 已成功解析")
}

// TestUserImportMessageEscapesHTML 导入结果由前端 v-html 展示，账号等动态值必须转义。
func TestUserImportMessageEscapesHTML(t *testing.T) {
	template := request(http.MethodPost, "/system/user/importTemplate", adminToken, nil)
	if template.Status != http.StatusOK || len(template.Body) == 0 {
		t.Fatalf("准备用户导入模板失败：HTTP=%d，响应=%s", template.Status, truncBody(template.Body))
	}

	file, err := excelize.OpenReader(bytes.NewReader(template.Body))
	if err != nil {
		t.Fatalf("打开用户导入模板失败：%v", err)
	}
	defer file.Close()
	sheet := file.GetSheetName(0)
	rows, err := file.GetRows(sheet)
	if err != nil || len(rows) == 0 {
		t.Fatalf("读取用户导入模板表头失败：%v", err)
	}

	maliciousUserName := `<img src=x onerror=x>`
	for column, header := range rows[0] {
		var value string
		switch header {
		case "登录名称":
			value = maliciousUserName
		case "用户名称":
			value = "测试昵称"
		default:
			continue
		}
		cell, err := excelize.CoordinatesToCellName(column+1, 2)
		if err != nil {
			t.Fatalf("生成模板单元格坐标失败：%v", err)
		}
		if err := file.SetCellValue(sheet, cell, value); err != nil {
			t.Fatalf("填写用户导入模板失败：%v", err)
		}
	}
	data, err := file.WriteToBuffer()
	if err != nil {
		t.Fatalf("生成恶意账号导入文件失败：%v", err)
	}

	r := requestMultipart(http.MethodPost, "/system/user/importData", adminToken,
		"file", "unsafe_user.xlsx", data.Bytes())
	mustFail(t, r, "导入失败", "导入含 HTML 的账号")
	if strings.Contains(r.Msg, maliciousUserName) {
		t.Fatalf("导入提示不能原样回显 HTML，实际 msg=%q", r.Msg)
	}
	if !strings.Contains(r.Msg, "&lt;img src=x onerror=x&gt;") {
		t.Errorf("导入提示应转义账号，实际 msg=%q", r.Msg)
	}
	if !strings.Contains(r.Msg, "<br/>") {
		t.Errorf("导入提示应保留前端约定的 <br/> 换行，实际 msg=%q", r.Msg)
	}
}

// TestUserImportTemplateNeedsLoginOnly 对齐 Java SysUserController：
// importTemplate 没有 @PreAuthorize，只要求登录；真正导入 importData 才要求
// system:user:import。普通登录用户拿到的也必须是 xlsx，而不是 403 JSON。
func TestUserImportTemplateNeedsLoginOnly(t *testing.T) {
	userBody := newUserPayload("templateperm")
	userBody["roleIds"] = []int64{} // 明确不给 system:user:import
	createUser(t, userBody)
	token := mustLogin(t, fmt.Sprint(userBody["userName"]))

	r := request(http.MethodPost, "/system/user/importTemplate", token, nil)
	if r.Code == 403 {
		t.Fatalf("Java 的导入模板接口只要求登录，Go 不应额外要求 system:user:import：msg=%q", r.Msg)
	}
	if _, err := excelize.OpenReader(bytes.NewReader(r.Body)); err != nil {
		t.Fatalf("普通登录用户应拿到合法 xlsx，实际响应=%s", truncBody(r.Body))
	}
}

// TestUserNewFormTrailingSlash 对齐 Vue3 的真实请求和 Java 的双路径映射。
// getUser(undefined) 会请求 /system/user/；这里要求路由直接返回业务响应，
// 不能依赖客户端跟随 Gin 的尾斜杠重定向。
func TestUserNewFormTrailingSlash(t *testing.T) {
	r := request(http.MethodGet, "/system/user/", adminToken, nil)
	if r.Status != http.StatusOK {
		t.Fatalf("GET /system/user/ 应直接返回 200，实际 HTTP %d", r.Status)
	}
	mustOK(t, r, "带尾斜杠获取新增用户表单数据")
}

// TestUserExportIsFile 导出返回的是真正的 xlsx，不是 JSON。
func TestUserExportIsFile(t *testing.T) {
	r := request(http.MethodPost, "/system/user/export", adminToken, url.Values{"pageSize": {"10"}})
	if r.Status != http.StatusOK {
		t.Fatalf("导出用户：HTTP 状态码应为 200，实际 %d", r.Status)
	}
	if _, err := excelize.OpenReader(bytes.NewReader(r.Body)); err != nil {
		t.Fatalf("导出的应是合法 xlsx，实际响应=%s", truncBody(r.Body))
	}
}

// TestUserDeptTree 用户页左侧的部门树。
func TestUserDeptTree(t *testing.T) {
	r := doGet(t, "/system/user/deptTree")
	mustOK(t, r, "用户部门树")
	tree := dataArray(t, r, "用户部门树")
	if len(tree) == 0 {
		t.Fatal("部门树不该为空")
	}
	assertTreeShape(t, tree, "用户部门树")
}

// TestUserValidation 用户字段校验。
func TestUserValidation(t *testing.T) {
	keep := func(p map[string]any) map[string]any { return p }

	cases := []struct {
		name   string
		mutate func(map[string]any) map[string]any
		wantIn string
	}{
		{"完整数据", keep, ""},

		{"少传 userName", func(p map[string]any) map[string]any { return omit(p, "userName") }, "UserName"},
		{"userName 纯空格", func(p map[string]any) map[string]any { return with(p, "userName", "   ") }, "UserName"},
		{"userName 含 HTML", func(p map[string]any) map[string]any {
			return with(p, "userName", "<script>x</script>")
		}, "UserName"},
		{"userName 超过 30 字", func(p map[string]any) map[string]any { return with(p, "userName", repeatText(31)) }, "UserName"},

		// nickName 只有 @Xss 和 @Size，没有 @NotBlank —— 昵称允许为空
		{"少传 nickName", func(p map[string]any) map[string]any { return omit(p, "nickName") }, ""},
		{"nickName 空串", func(p map[string]any) map[string]any { return with(p, "nickName", "") }, ""},
		{"nickName 含 HTML", func(p map[string]any) map[string]any { return with(p, "nickName", "<b>x</b>") }, "NickName"},
		{"nickName 超过 30 字", func(p map[string]any) map[string]any { return with(p, "nickName", repeatText(31)) }, "NickName"},

		// 邮箱的两个失败点：不传（nil/零值）和传空串，都必须放行
		{"少传 email", func(p map[string]any) map[string]any { return omit(p, "email") }, ""},
		{"email 空串", func(p map[string]any) map[string]any { return with(p, "email", "") }, ""},
		{"email 格式非法", func(p map[string]any) map[string]any { return with(p, "email", "not-an-email") }, "Email"},

		{"少传 phonenumber", func(p map[string]any) map[string]any { return omit(p, "phonenumber") }, ""},
		{"phonenumber 空串", func(p map[string]any) map[string]any { return with(p, "phonenumber", "") }, ""},
		{"phonenumber 超过 11 位", func(p map[string]any) map[string]any {
			return with(p, "phonenumber", "138000000012")
		}, "Phonenumber"},

		// 新增时密码必填，但拦在 service 而不是 binding（编辑时同一结构体不传密码是合法的）
		{"少传 password", func(p map[string]any) map[string]any { return omit(p, "password") }, "密码不能为空"},
		{"password 只有 4 位", func(p map[string]any) map[string]any { return with(p, "password", "1234") }, "Password"},
		{"password 超过 20 位", func(p map[string]any) map[string]any {
			return with(p, "password", "123456789012345678901")
		}, "Password"},

		// 性别仍跟字典扩展；用户状态参与登录授权，只允许正常/停用。
		{"sex 传字典外的值", func(p map[string]any) map[string]any { return with(p, "sex", "9") }, ""},
		{"status 传字典外的值", func(p map[string]any) map[string]any { return with(p, "status", "2") }, "Status"},

		{"少传 deptId", func(p map[string]any) map[string]any { return omit(p, "deptId") }, ""},
		{"少传 remark", func(p map[string]any) map[string]any { return omit(p, "remark") }, ""},
		{"remark 超过 500 字", func(p map[string]any) map[string]any { return with(p, "remark", repeatText(501)) }, "Remark"},

		{"多传未知字段", func(p map[string]any) map[string]any { return with(p, "hello", "world") }, ""},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.mutate(newUserPayload(fmt.Sprintf("v%d", i)))
			r := doPost(t, "/system/user", body)
			if tc.wantIn == "" {
				mustOK(t, r, tc.name)
				return
			}
			mustFail(t, r, tc.wantIn, tc.name)
		})
	}
}

// TestUserUnique 账号、手机号、邮箱都不能重复。
func TestUserUnique(t *testing.T) {
	body := newUserPayload("uniq")
	body["phonenumber"] = "13900000001"
	createUser(t, body)

	dupName := newUserPayload("uniq2")
	dupName["userName"] = body["userName"]
	mustFail(t, doPost(t, "/system/user", dupName), "登录账号已存在", "重复的登录账号")

	dupPhone := newUserPayload("uniq3")
	dupPhone["phonenumber"] = body["phonenumber"]
	mustFail(t, doPost(t, "/system/user", dupPhone), "手机号码已存在", "重复的手机号")

	dupEmail := newUserPayload("uniq4")
	dupEmail["email"] = body["email"]
	mustFail(t, doPost(t, "/system/user", dupEmail), "邮箱账号已存在", "重复的邮箱")
}
