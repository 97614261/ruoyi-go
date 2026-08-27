package main

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"testing"
)

func TestLoginSessionKey(t *testing.T) {
	payload, err := json.Marshal(map[string]any{"login_user_key": "abc-123", "sub": "admin"})
	if err != nil {
		t.Fatal(err)
	}
	token := "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
	key, err := loginSessionKey(token)
	if err != nil || key != "login_tokens:abc-123" {
		t.Fatalf("key=%q err=%v", key, err)
	}
}

func TestLoginSessionKeyRejectsInvalidTokens(t *testing.T) {
	for _, token := range []string{"", "two.parts", "a.not-base64!.c", "a.e30.c"} {
		if _, err := loginSessionKey(token); err == nil {
			t.Errorf("loginSessionKey(%q) should fail", token)
		}
	}
}

func TestCompareResponsesIgnoring(t *testing.T) {
	goResponse := apiResponse{status: 200, body: map[string]any{
		"code": float64(200), "data": map[string]any{"id": float64(1), "name": "same", "token": "go"},
	}}
	javaResponse := apiResponse{status: 200, body: map[string]any{
		"code": float64(200), "data": map[string]any{"id": float64(2), "name": "same", "token": "java"},
	}}
	if diffs := compareResponsesIgnoring(goResponse, javaResponse, "id"); len(diffs) != 0 {
		t.Fatalf("diffs=%v", diffs)
	}
}

func TestReplaceCRUDValues(t *testing.T) {
	input := map[string]any{"name": "go-unique", "nested": []any{"java-unique", float64(1)}}
	got := replaceCRUDValues(input, map[string]string{"go-unique": "<unique>", "java-unique": "<unique>"})
	want := map[string]any{"name": "<unique>", "nested": []any{"<unique>", float64(1)}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%#v want=%#v", got, want)
	}
}

func TestDefaultProbeCount(t *testing.T) {
	if got := len(corePaths) + len(dynamicPathProbes); got != 36 {
		t.Fatalf("默认对拍探针应为 36 个，实际 %d；增删探针时必须同步更新约定和报告", got)
	}
}

func TestPermissionProbeCount(t *testing.T) {
	if permissionProbeTotal != 18 {
		t.Fatalf("数据权限探针应为 18 个，实际 %d", permissionProbeTotal)
	}
}

func TestResponseIDSetIsSorted(t *testing.T) {
	response := apiResponse{status: 200, body: map[string]any{
		"code": float64(200),
		"rows": []any{
			map[string]any{"userId": float64(9)},
			map[string]any{"userId": float64(2)},
		},
	}}
	ids, err := responseIDSet(response, "rows", "userId")
	if err != nil || !reflect.DeepEqual(ids, []int64{2, 9}) {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
}

func TestCommonProbeID(t *testing.T) {
	response := func(rows ...map[string]any) apiResponse {
		items := make([]any, 0, len(rows))
		for _, row := range rows {
			items = append(items, row)
		}
		return apiResponse{status: 200, body: map[string]any{"code": float64(200), "rows": items}}
	}
	probe := dynamicPathProbe{
		name: "有父级部门", container: "rows", idField: "deptId",
		selector: numericFieldNonZero("parentId"),
	}
	goResponse := response(
		map[string]any{"deptId": float64(100), "parentId": float64(0)},
		map[string]any{"deptId": float64(103), "parentId": float64(101)},
	)
	javaResponse := response(
		map[string]any{"deptId": float64(200), "parentId": float64(100)},
		map[string]any{"deptId": float64(103), "parentId": float64(101)},
	)

	id, err := commonProbeID(goResponse, javaResponse, probe)
	if err != nil || id != "103" {
		t.Fatalf("id=%q err=%v", id, err)
	}
}

func TestCommonProbeIDRejectsMissingCommonRecord(t *testing.T) {
	response := func(id float64) apiResponse {
		return apiResponse{status: 200, body: map[string]any{
			"code": float64(200),
			"data": []any{map[string]any{"menuId": id}},
		}}
	}
	probe := dynamicPathProbe{name: "菜单", container: "data", idField: "menuId", listPath: "/system/menu/list"}
	if _, err := commonProbeID(response(1), response(2), probe); err == nil {
		t.Fatal("两端没有共同 ID 时必须失败，不能拿不同记录做详情对比")
	}
}
