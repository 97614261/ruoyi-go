package apitest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ruoyi-go/internal/repository"
)

// TestDetailContractNullFields 详情接口里 Java 没 select 的列必须输出 null。
//
// 【为什么这类差异一直没被发现】
// Java 的 MyBatis `selectXxxVo` 只 select 一部分列，没选的字段 Jackson 输出 null；
// Go 是把整行读回来直接序列化，于是新增后是 ""、修改后是 "admin"。
//
// 接口照样返回 200，字段名也对，只有值不同 —— **现有测试断言的是 Go 自己的输出，
// 这类漂移一个都发现不了**。只有双端对拍或这样的定点断言能抓住。
//
// 判断依据是 Java 的 mapper xml，不是实体定义：
//
//	selectPostVo      少 update_by / update_time
//	selectDictTypeVo  少 update_by / update_time
//	selectDictDataVo  少 update_by / update_time
//	selectRoleVo      少 create_by / update_by / update_time
//	selectMenuVo      少 create_by / update_by / update_time / remark
//	selectDeptById    单独一条 SQL，见 TestDeptDetailContract
//	selectNoticeVo    全选，无差异
//	selectConfigVo    全选，无差异
func TestDetailContractNullFields(t *testing.T) {
	// 每种实体都先建后改，确保数据库里 update_by 确实被写成了 admin ——
	// 只建不改的话 update_by 是空串，测不出"真实值泄漏"这一半
	t.Run("岗位", func(t *testing.T) {
		body := newPostPayload("contract")
		id := createPost(t, body)

		updated := payload(body)
		updated["postId"] = id
		updated["postName"] = testPrefix + "岗位改名contract"
		mustOK(t, doPut(t, "/system/post", updated), "修改岗位")

		detail := dataObject(t, doGet(t, "/system/post/"+idPath(id)), "岗位详情")
		assertJavaNull(t, detail, "岗位详情", "updateBy", "updateTime")
		assertNotNull(t, detail, "岗位详情", "createBy", "createTime")
	})

	t.Run("字典类型", func(t *testing.T) {
		body := newDictTypePayload("contract")
		id := createDictType(t, body)

		updated := payload(body)
		updated["dictId"] = id
		updated["dictName"] = testPrefix + "字典改名contract"
		mustOK(t, doPut(t, "/system/dict/type", updated), "修改字典类型")

		detail := dataObject(t, doGet(t, "/system/dict/type/"+idPath(id)), "字典类型详情")
		assertJavaNull(t, detail, "字典类型详情", "updateBy", "updateTime")
	})

	t.Run("字典数据", func(t *testing.T) {
		typeBody := newDictTypePayload("dcontract")
		createDictType(t, typeBody)
		dictType := fmt.Sprint(typeBody["dictType"])

		dataBody := newDictDataPayload(dictType, "c1")
		id := createDictData(t, dataBody)

		updated := payload(dataBody)
		updated["dictCode"] = id
		updated["dictLabel"] = testPrefix + "标签改名c1"
		mustOK(t, doPut(t, "/system/dict/data", updated), "修改字典数据")

		detail := dataObject(t, doGet(t, "/system/dict/data/"+idPath(id)), "字典数据详情")
		assertJavaNull(t, detail, "字典数据详情", "updateBy", "updateTime")
	})

	t.Run("菜单", func(t *testing.T) {
		body := newDirPayload("contract")
		id := createMenu(t, body)

		updated := payload(body)
		updated["menuId"] = id
		updated["menuName"] = testPrefix + "菜单改名contract"
		mustOK(t, doPut(t, "/system/menu", updated), "修改菜单")

		detail := dataObject(t, doGet(t, "/system/menu/"+idPath(id)), "菜单详情")
		// selectMenuVo 连 create_by 和 remark 都没选
		assertJavaNull(t, detail, "菜单详情",
			"createBy", "updateBy", "updateTime", "remark", "parentName")
		assertNotNull(t, detail, "菜单详情", "createTime")

		// Java 实体里 children 是 new ArrayList<>()，永远是空数组不是 null
		children, ok := detail["children"]
		if !ok {
			t.Errorf("菜单详情应输出 children 键，实际字段=%v", topKeys(detail))
		} else if list, isList := children.([]any); !isList || len(list) != 0 {
			t.Errorf("菜单详情的 children 应为空数组，实际 %#v", children)
		}

		// Java 是 ifnull(perms,'') as perms —— 取不到时是空串，不是 null
		if perms, ok := detail["perms"]; !ok {
			t.Error("菜单详情应输出 perms 键")
		} else if perms != "" {
			t.Errorf("perms 取不到时应为空串（Java 用了 ifnull(perms,'')），实际 %#v", perms)
		}
	})

	t.Run("角色", func(t *testing.T) {
		body := newRolePayload("contract")
		id := createRole(t, body)

		updated := payload(body)
		updated["roleId"] = id
		updated["roleName"] = testPrefix + "角色改名contract"
		mustOK(t, doPut(t, "/system/role", updated), "修改角色")

		detail := dataObject(t, doGet(t, "/system/role/"+idPath(id)), "角色详情")
		// selectRoleVo 连 create_by 都没选
		assertJavaNull(t, detail, "角色详情", "createBy", "updateBy", "updateTime")
		assertNotNull(t, detail, "角色详情", "createTime")
	})
}

// TestDeptDetailContract 部门详情是全项目唯一一处「详情与列表选列不同」的接口。
//
// Java 的 selectDeptById 没有 include selectDeptVo，而是单独写了一条 SQL：
// 比列表**少了** create_by / create_time / del_flag，
// **多了** parent_name（子查询取父部门名）。两个方向都有差异。
func TestDeptDetailContract(t *testing.T) {
	parentID := createDept(t, newDeptPayload(rootDeptID, "contract父"))
	childID := createDept(t, newDeptPayload(parentID, "contract子"))

	// 改一次，让数据库里的 update_by 变成 admin
	updated := with(newDeptPayload(parentID, "contract子"), "deptId", childID)
	updated["orderNum"] = 77
	mustOK(t, doPut(t, "/system/dept", updated), "修改部门")

	detail := dataObject(t, doGet(t, "/system/dept/"+idPath(childID)), "部门详情")

	// 详情没选这些列
	assertJavaNull(t, detail, "部门详情", "createBy", "createTime", "delFlag",
		"updateBy", "updateTime", "remark")

	// 详情独有：parentName 必须有值，而且键不能因为 omitempty 消失
	assertField(t, detail, "parentName", testPrefix+"部门contract父", "部门详情的父部门名")

	// 根部门没有父级：Java 的子查询取不到，输出 null —— 键要在，值为 null
	rootDetail := dataObject(t, doGet(t, "/system/dept/"+idPath(rootDeptID)), "根部门详情")
	if value, ok := rootDetail["parentName"]; !ok {
		t.Errorf("根部门详情也要输出 parentName 键（值为 null），实际字段=%v", topKeys(rootDetail))
	} else if value != nil {
		t.Errorf("根部门的 parentName 应为 null，实际 %v", value)
	}
}

// TestNoticeAndConfigDetailKeepValues 公告和参数的 selectVo 是全选的，
// 不能"顺手"也把 updateBy 抹掉。
//
// 这条是反向保险：契约转换很容易被当成"所有详情都该抹 updateBy"而滥用。
func TestNoticeAndConfigDetailKeepValues(t *testing.T) {
	t.Run("公告", func(t *testing.T) {
		body := newNoticePayload("contract")
		id := createNotice(t, body)

		updated := payload(body)
		updated["noticeId"] = id
		updated["noticeTitle"] = testPrefix + "公告改名contract"
		mustOK(t, doPut(t, "/system/notice", updated), "修改公告")

		detail := dataObject(t, doGet(t, "/system/notice/"+idPath(id)), "公告详情")
		assertNotNull(t, detail, "公告详情", "updateBy", "updateTime", "createBy")
		assertField(t, detail, "updateBy", "admin", "公告详情的更新者")
	})

	t.Run("参数配置", func(t *testing.T) {
		body := newConfigPayload("contract")
		id := createConfig(t, body)

		updated := payload(body)
		updated["configId"] = id
		updated["configValue"] = "changed"
		mustOK(t, doPut(t, "/system/config", updated), "修改参数")

		detail := dataObject(t, doGet(t, "/system/config/"+idPath(id)), "参数详情")
		assertNotNull(t, detail, "参数详情", "updateBy", "updateTime", "createBy")
		assertField(t, detail, "updateBy", "admin", "参数详情的更新者")
	})
}

// assertJavaNull 断言这些字段**存在且为 null**。
//
// 不能只判断"没有值"：Go 的零值 ""、空数组、缺失键，在 JSON 里
// 和 null 是三种不同的东西，前端 v-if / === 的判断结果也不同。
func assertJavaNull(t *testing.T, obj map[string]any, what string, keys ...string) {
	t.Helper()
	for _, key := range keys {
		value, ok := obj[key]
		if !ok {
			t.Errorf("%s：字段 %q 应当存在且为 null（Java 会输出这个键），实际整个键都没有", what, key)
			continue
		}
		if value != nil {
			t.Errorf("%s：字段 %q 应为 null（Java 的 selectVo 没 select 这一列），实际 %#v",
				what, key, value)
		}
	}
}

// assertColumn 直接查数据库，断言某一列的值。
//
// 【什么时候才该用它】
// 只在「这个行为真实存在，但没有任何接口会暴露它」时用。
// 典型就是 update_by：Java 的 selectVo 不选这一列，所以详情和列表都是 null ——
// 可 UpdateXxx 要是漏了写操作人，操作审计就悄悄丢了，接口层怎么测都测不出来。
//
// 【不要拿它当常规断言】能从接口验的就从接口验。
// 直接查库会绕开整条业务链路，测出来的是"数据库里是什么"，
// 不是"用户看到的是什么" —— 后者才是契约。
func assertColumn(t *testing.T, table, column, idColumn string, id int64, want string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var got string
	query := fmt.Sprintf("SELECT IFNULL(%s,'') FROM %s WHERE %s = ?", column, table, idColumn)
	if err := repository.DB(ctx).Raw(query, id).Scan(&got).Error; err != nil {
		t.Fatalf("查 %s.%s 失败: %v", table, column, err)
	}
	if got != want {
		t.Errorf("%s.%s（%s=%d）期望 %q，实际 %q —— 接口层看不到这一列，只能查库验证",
			table, column, idColumn, id, want, got)
	}
}

// assertNotNull 断言这些字段有值 —— 防止契约转换被滥用成"详情一律抹空"。
func assertNotNull(t *testing.T, obj map[string]any, what string, keys ...string) {
	t.Helper()
	for _, key := range keys {
		value, ok := obj[key]
		if !ok {
			t.Errorf("%s：缺少字段 %q", what, key)
			continue
		}
		if value == nil || value == "" {
			t.Errorf("%s：字段 %q 不该为空（Java 的 selectVo 选了这一列），实际 %#v",
				what, key, value)
		}
	}
}
