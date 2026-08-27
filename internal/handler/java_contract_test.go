package handler

import (
	"encoding/json"
	"reflect"
	"testing"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/types"
)

// TestContractBaseMapsMatchModelJSON 保证逐字段转换没有改变模型原本的 JSON 语义。
// Java 特有的 null/空数组改写由各 contractXxx 函数在这层基础上继续完成。
func TestContractBaseMapsMatchModelJSON(t *testing.T) {
	order := 3
	deptID := int64(10)
	text := "value"
	dept := model.SysDept{
		DeptID: 10, ParentID: 1, Ancestors: "0,1", DeptName: "研发部", OrderNum: &order,
		Leader: &text, Phone: &text, Email: &text, Status: model.StatusNormal,
		CreateTime: types.Now(),
	}
	role := model.SysRole{
		RoleID: model.AdminRoleID, RoleName: "管理员", RoleKey: model.AdminRoleKey,
		RoleSort: &order, Status: model.StatusNormal, Remark: &text,
	}

	cases := []struct {
		name  string
		model any
		got   map[string]any
	}{
		{"角色", role, roleMap(role)},
		{"岗位", model.SysPost{PostID: 1, PostSort: &order, Remark: &text}, postMap(model.SysPost{PostID: 1, PostSort: &order, Remark: &text})},
		{"部门", dept, deptMap(dept)},
		{"菜单", model.SysMenu{MenuID: 1, OrderNum: &order, Component: &text, Perms: &text}, menuMap(model.SysMenu{MenuID: 1, OrderNum: &order, Component: &text, Perms: &text})},
		{"字典类型", model.SysDictType{DictID: 1, Remark: &text}, dictTypeMap(model.SysDictType{DictID: 1, Remark: &text})},
		{"字典数据", model.SysDictData{DictCode: 1, DictSort: &order, CssClass: &text, Default: true}, dictDataMap(model.SysDictData{DictCode: 1, DictSort: &order, CssClass: &text, Default: true})},
		{"用户", model.SysUser{UserID: model.AdminUserID, DeptID: &deptID, Password: "must-not-leak", Dept: &dept, Roles: []model.SysRole{role}}, userMap(model.SysUser{UserID: model.AdminUserID, DeptID: &deptID, Password: "must-not-leak", Dept: &dept, Roles: []model.SysRole{role}})},
		{"公告", model.SysNotice{NoticeID: 1, NoticeTitle: "公告", Remark: &text}, noticeMap(model.SysNotice{NoticeID: 1, NoticeTitle: "公告", Remark: &text})},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := normalizedJSONMap(t, tc.model)
			got := normalizedJSONMap(t, tc.got)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("逐字段 map 与模型 JSON 不一致\nwant: %#v\n got: %#v", want, got)
			}
		})
	}
}

func normalizedJSONMap(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	return result
}
