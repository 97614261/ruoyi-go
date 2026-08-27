package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type crudSpec struct {
	name        string
	basePath    string
	listPath    string
	idField     string
	lookupField string
	lookupQuery string
	container   string
	ignoredKeys []string
	createBody  func(unique string) map[string]any
	updateBody  func(id float64, unique string) map[string]any
}

func compareCRUDProbes(goAPI, javaAPI *apiClient, maxDiffs int) (matched, total int) {
	seed := strconv.FormatInt(time.Now().UnixNano(), 36)
	if len(seed) > 8 {
		seed = seed[len(seed)-8:]
	}
	parentDeptID, err := rootDeptIDFromList(goAPI)
	if err != nil {
		fmt.Printf("CRUD_PROBE_DIFF 初始化动态部门样本\n  %v\n", err)
		return 0, 60
	}
	dictType, err := firstDictType(goAPI)
	if err != nil {
		fmt.Printf("CRUD_PROBE_DIFF 初始化动态字典样本\n  %v\n", err)
		return 0, 60
	}
	specs := []crudSpec{
		{
			name: "用户", basePath: "/system/user", listPath: "/system/user/list",
			idField: "userId", lookupField: "userName", lookupQuery: "userName",
			ignoredKeys: []string{"userId", "createTime", "updateTime", "loginDate", "pwdUpdateDate"},
			createBody: func(unique string) map[string]any {
				return map[string]any{"userName": unique, "nickName": unique, "password": "test123456", "deptId": parentDeptID, "email": "", "phonenumber": "", "sex": "0", "status": "0", "postIds": []int64{}, "roleIds": []int64{}, "remark": "CRUD contract probe"}
			},
			updateBody: func(id float64, unique string) map[string]any {
				return map[string]any{"userId": id, "userName": unique, "nickName": unique + "_u", "deptId": parentDeptID, "email": "", "phonenumber": "", "sex": "1", "status": "1", "postIds": []int64{}, "roleIds": []int64{}, "remark": "CRUD contract updated"}
			},
		},
		{
			name: "角色", basePath: "/system/role", listPath: "/system/role/list",
			idField: "roleId", lookupField: "roleName", lookupQuery: "roleName",
			ignoredKeys: []string{"roleId", "createTime", "updateTime"},
			createBody: func(unique string) map[string]any {
				return map[string]any{"roleName": unique, "roleKey": unique, "roleSort": 77, "status": "0", "menuIds": []int64{}, "menuCheckStrictly": false, "deptCheckStrictly": false, "remark": "CRUD contract probe"}
			},
			updateBody: func(id float64, unique string) map[string]any {
				return map[string]any{"roleId": id, "roleName": unique + "_u", "roleKey": unique, "roleSort": 78, "status": "1", "menuIds": []int64{}, "menuCheckStrictly": false, "deptCheckStrictly": false, "remark": "CRUD contract updated"}
			},
		},
		{
			name: "部门", basePath: "/system/dept", listPath: "/system/dept/list", container: "data",
			idField: "deptId", lookupField: "deptName", lookupQuery: "deptName",
			ignoredKeys: []string{"deptId", "createTime", "updateTime"},
			createBody: func(unique string) map[string]any {
				return map[string]any{"parentId": parentDeptID, "deptName": unique, "orderNum": 77, "leader": "contract", "phone": "", "email": "", "status": "0"}
			},
			updateBody: func(id float64, unique string) map[string]any {
				return map[string]any{"deptId": id, "parentId": parentDeptID, "deptName": unique + "_u", "orderNum": 78, "leader": "updated", "phone": "", "email": "", "status": "1"}
			},
		},
		{
			name: "菜单", basePath: "/system/menu", listPath: "/system/menu/list", container: "data",
			idField: "menuId", lookupField: "menuName", lookupQuery: "menuName",
			ignoredKeys: []string{"menuId", "createTime", "updateTime"},
			createBody: func(unique string) map[string]any {
				return map[string]any{"menuName": unique, "parentId": 0, "orderNum": 77, "path": unique, "menuType": "M", "isFrame": "1", "isCache": "0", "visible": "0", "status": "0", "icon": "#"}
			},
			updateBody: func(id float64, unique string) map[string]any {
				return map[string]any{"menuId": id, "menuName": unique + "_u", "parentId": 0, "orderNum": 78, "path": unique, "menuType": "M", "isFrame": "1", "isCache": "0", "visible": "1", "status": "1", "icon": "#"}
			},
		},
		{
			name: "岗位", basePath: "/system/post", listPath: "/system/post/list",
			idField: "postId", lookupField: "postCode", lookupQuery: "postCode",
			ignoredKeys: []string{"postId", "createTime", "updateTime"},
			createBody: func(unique string) map[string]any {
				return map[string]any{"postCode": unique, "postName": unique, "postSort": 77, "status": "0", "remark": "CRUD contract probe"}
			},
			updateBody: func(id float64, unique string) map[string]any {
				return map[string]any{"postId": id, "postCode": unique, "postName": unique + "_u", "postSort": 78, "status": "1", "remark": "CRUD contract updated"}
			},
		},
		{
			name: "参数", basePath: "/system/config", listPath: "/system/config/list",
			idField: "configId", lookupField: "configKey", lookupQuery: "configKey",
			ignoredKeys: []string{"configId", "createTime", "updateTime"},
			createBody: func(unique string) map[string]any {
				return map[string]any{"configName": "契约参数", "configKey": unique, "configValue": "before", "configType": "N", "remark": "CRUD contract probe"}
			},
			updateBody: func(id float64, unique string) map[string]any {
				return map[string]any{"configId": id, "configName": "契约参数已更新", "configKey": unique, "configValue": "after", "configType": "N", "remark": "CRUD contract updated"}
			},
		},
		{
			name: "字典类型", basePath: "/system/dict/type", listPath: "/system/dict/type/list",
			idField: "dictId", lookupField: "dictType", lookupQuery: "dictType",
			ignoredKeys: []string{"dictId", "createTime", "updateTime"},
			createBody: func(unique string) map[string]any {
				return map[string]any{"dictName": "契约字典", "dictType": unique, "status": "0", "remark": "CRUD contract probe"}
			},
			updateBody: func(id float64, unique string) map[string]any {
				return map[string]any{"dictId": id, "dictName": "契约字典已更新", "dictType": unique, "status": "1", "remark": "CRUD contract updated"}
			},
		},
		{
			name: "字典数据", basePath: "/system/dict/data", listPath: "/system/dict/data/list",
			idField: "dictCode", lookupField: "dictLabel", lookupQuery: "dictLabel",
			ignoredKeys: []string{"dictCode", "createTime", "updateTime"},
			createBody: func(unique string) map[string]any {
				return map[string]any{"dictSort": 77, "dictLabel": unique, "dictValue": unique, "dictType": dictType, "cssClass": "", "listClass": "default", "isDefault": "N", "status": "0", "remark": "CRUD contract probe"}
			},
			updateBody: func(id float64, unique string) map[string]any {
				return map[string]any{"dictCode": id, "dictSort": 78, "dictLabel": unique + "_u", "dictValue": unique, "dictType": dictType, "cssClass": "", "listClass": "primary", "isDefault": "Y", "status": "1", "remark": "CRUD contract updated"}
			},
		},
		{
			name: "公告", basePath: "/system/notice", listPath: "/system/notice/list",
			idField: "noticeId", lookupField: "noticeTitle", lookupQuery: "noticeTitle",
			ignoredKeys: []string{"noticeId", "createTime", "updateTime"},
			createBody: func(unique string) map[string]any {
				return map[string]any{"noticeTitle": unique, "noticeType": "2", "noticeContent": "contract before", "status": "0", "remark": "CRUD contract probe"}
			},
			updateBody: func(id float64, unique string) map[string]any {
				return map[string]any{"noticeId": id, "noticeTitle": unique, "noticeType": "1", "noticeContent": "contract after", "status": "1", "remark": "CRUD contract updated"}
			},
		},
		{
			name: "定时任务", basePath: "/monitor/job", listPath: "/monitor/job/list",
			idField: "jobId", lookupField: "jobName", lookupQuery: "jobName",
			ignoredKeys: []string{"jobId", "createTime", "updateTime", "nextValidTime"},
			createBody: func(unique string) map[string]any {
				return map[string]any{"jobName": unique, "jobGroup": "DEFAULT", "invokeTarget": "ryTask.ryNoParams", "cronExpression": "0 0 3 * * ?", "misfirePolicy": "3", "concurrent": "1", "status": "1", "remark": "CRUD contract probe"}
			},
			updateBody: func(id float64, unique string) map[string]any {
				return map[string]any{"jobId": id, "jobName": unique + "_u", "jobGroup": "DEFAULT", "invokeTarget": "ryTask.ryNoParams", "cronExpression": "0 30 4 * * ?", "misfirePolicy": "2", "concurrent": "0", "status": "1", "remark": "CRUD contract updated"}
			},
		},
	}

	check := func(name string, goResponse apiResponse, goErr error, javaResponse apiResponse, javaErr error, goUnique, javaUnique string, ignored ...string) {
		total++
		if goErr != nil || javaErr != nil {
			fmt.Printf("CRUD_PROBE_DIFF %s\n  request error: go=%v java=%v\n", name, goErr, javaErr)
			return
		}
		replacements := map[string]string{
			goUnique:          "<unique>",
			javaUnique:        "<unique>",
			goUnique + "_u":   "<unique-updated>",
			javaUnique + "_u": "<unique-updated>",
		}
		goResponse.body = replaceCRUDValues(goResponse.body, replacements)
		javaResponse.body = replaceCRUDValues(javaResponse.body, replacements)
		diffs := compareResponsesIgnoring(goResponse, javaResponse, ignored...)
		if len(diffs) == 0 {
			matched++
			fmt.Printf("CRUD_PROBE_MATCH %s\n", name)
			return
		}
		fmt.Printf("CRUD_PROBE_DIFF %s\n", name)
		for _, diff := range diffs[:min(maxDiffs, len(diffs))] {
			fmt.Printf("  %s\n", diff)
		}
	}

	for index, spec := range specs {
		goUnique := fmt.Sprintf("zz_contract_%s_g_%s", strconv.Itoa(index), seed)
		javaUnique := fmt.Sprintf("zz_contract_%s_j_%s", strconv.Itoa(index), seed)
		specStartTotal := total
		markRemainingDifferent := func(reason string) {
			for total-specStartTotal < 6 {
				total++
				fmt.Printf("CRUD_PROBE_DIFF %s后续场景未执行：%s\n", spec.name, reason)
			}
		}
		goID, javaID := float64(0), float64(0)
		defer func(spec crudSpec, goUnique, javaUnique string) {
			if goID == 0 {
				goID, _ = locateCreatedID(goAPI, spec, goUnique)
			}
			if goID != 0 {
				_, _ = goAPI.request(http.MethodDelete, spec.basePath+"/"+formatID(goID), "", nil)
			}
			if javaID == 0 {
				javaID, _ = locateCreatedID(javaAPI, spec, javaUnique)
			}
			if javaID != 0 {
				_, _ = javaAPI.request(http.MethodDelete, spec.basePath+"/"+formatID(javaID), "", nil)
			}
		}(spec, goUnique, javaUnique)

		goCreate, goErr := requestJSON(goAPI, http.MethodPost, spec.basePath, spec.createBody(goUnique))
		javaCreate, javaErr := requestJSON(javaAPI, http.MethodPost, spec.basePath, spec.createBody(javaUnique))
		check(spec.name+"新增", goCreate, goErr, javaCreate, javaErr, goUnique, javaUnique)
		if goErr != nil || javaErr != nil || responseCode(goCreate) != 200 || responseCode(javaCreate) != 200 {
			markRemainingDifferent("新增未同时成功")
			continue
		}

		goID, goErr = locateCreatedID(goAPI, spec, goUnique)
		javaID, javaErr = locateCreatedID(javaAPI, spec, javaUnique)
		if goErr != nil || javaErr != nil {
			fmt.Printf("CRUD_PROBE_DIFF %s定位记录\n  go=%v java=%v\n", spec.name, goErr, javaErr)
			markRemainingDifferent("无法定位新增记录")
			continue
		}

		goDetail, goErr := goAPI.get(spec.basePath + "/" + formatID(goID))
		javaDetail, javaErr := javaAPI.get(spec.basePath + "/" + formatID(javaID))
		check(spec.name+"新增后详情", goDetail, goErr, javaDetail, javaErr, goUnique, javaUnique, spec.ignoredKeys...)

		goUpdate, goErr := requestJSON(goAPI, http.MethodPut, spec.basePath, spec.updateBody(goID, goUnique))
		javaUpdate, javaErr := requestJSON(javaAPI, http.MethodPut, spec.basePath, spec.updateBody(javaID, javaUnique))
		check(spec.name+"修改", goUpdate, goErr, javaUpdate, javaErr, goUnique, javaUnique)

		goDetail, goErr = goAPI.get(spec.basePath + "/" + formatID(goID))
		javaDetail, javaErr = javaAPI.get(spec.basePath + "/" + formatID(javaID))
		check(spec.name+"修改后详情", goDetail, goErr, javaDetail, javaErr, goUnique, javaUnique, spec.ignoredKeys...)

		goDelete, goErr := goAPI.request(http.MethodDelete, spec.basePath+"/"+formatID(goID), "", nil)
		javaDelete, javaErr := javaAPI.request(http.MethodDelete, spec.basePath+"/"+formatID(javaID), "", nil)
		check(spec.name+"删除", goDelete, goErr, javaDelete, javaErr, goUnique, javaUnique)

		deletedGoID, deletedJavaID := goID, javaID
		if goErr == nil && responseCode(goDelete) == 200 {
			goID = 0
		}
		if javaErr == nil && responseCode(javaDelete) == 200 {
			javaID = 0
		}
		goMissing, goErr := goAPI.get(spec.basePath + "/" + formatID(deletedGoID))
		javaMissing, javaErr := javaAPI.get(spec.basePath + "/" + formatID(deletedJavaID))
		check(spec.name+"删除后详情", goMissing, goErr, javaMissing, javaErr, goUnique, javaUnique, spec.ignoredKeys...)
	}
	return matched, total
}

func replaceCRUDValues(value any, replacements map[string]string) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			result[key] = replaceCRUDValues(child, replacements)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, child := range typed {
			result[index] = replaceCRUDValues(child, replacements)
		}
		return result
	case string:
		if replacement, ok := replacements[typed]; ok {
			return replacement
		}
		return typed
	default:
		return value
	}
}

func requestJSON(client *apiClient, method, path string, body map[string]any) (apiResponse, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return apiResponse{}, err
	}
	return client.request(method, path, "application/json", raw)
}

func locateCreatedID(client *apiClient, spec crudSpec, unique string) (float64, error) {
	query := url.Values{"pageNum": {"1"}, "pageSize": {"20"}, spec.lookupQuery: {unique}}
	response, err := client.get(spec.listPath + "?" + query.Encode())
	if err != nil {
		return 0, err
	}
	body, ok := response.body.(map[string]any)
	if !ok || body["code"] != float64(200) {
		return 0, fmt.Errorf("list returned %s", jsonValue(response.body))
	}
	container := spec.container
	if container == "" {
		container = "rows"
	}
	rows, _ := body[container].([]any)
	for _, raw := range rows {
		row, _ := raw.(map[string]any)
		if value, _ := row[spec.lookupField].(string); value == unique {
			if id, ok := row[spec.idField].(float64); ok && id > 0 {
				return id, nil
			}
		}
	}
	return 0, fmt.Errorf("record %q not found", unique)
}

func firstDictType(client *apiClient) (string, error) {
	response, err := client.get("/system/dict/type/optionselect")
	if err != nil {
		return "", err
	}
	items, err := probeItems(response, "data")
	if err != nil {
		return "", err
	}
	for _, item := range items {
		if dictType, _ := item["dictType"].(string); dictType != "" {
			return dictType, nil
		}
	}
	return "", fmt.Errorf("no dictionary type available")
}

func responseCode(response apiResponse) int {
	body, _ := response.body.(map[string]any)
	code, _ := body["code"].(float64)
	return int(code)
}

func formatID(id float64) string {
	return strconv.FormatInt(int64(id), 10)
}
