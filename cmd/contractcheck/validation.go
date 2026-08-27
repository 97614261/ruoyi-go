package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type validationProbe struct {
	name         string
	method       string
	path         string
	body         map[string]any
	cleanupSpec  *crudSpec
	cleanupValue string
}

func compareValidationProbes(goAPI, javaAPI *apiClient, maxDiffs int) (matched, total int) {
	seed := fmt.Sprintf("zzv_%x", time.Now().UnixNano())
	jobInvoke := "ryTask.ryParams('" + seed + "')"
	jobCleanup := &crudSpec{
		basePath: "/monitor/job", listPath: "/monitor/job/list",
		idField: "jobId", lookupField: "invokeTarget", lookupQuery: "invokeTarget",
	}
	probes := []validationProbe{
		{name: "用户账号超过30字符", method: http.MethodPost, path: "/system/user", body: map[string]any{
			"userName": strings.Repeat("u", 31), "nickName": seed, "password": "test123456", "email": "", "status": "0",
		}},
		{name: "用户邮箱格式非法", method: http.MethodPost, path: "/system/user", body: map[string]any{
			"userName": seed + "_user", "nickName": seed, "password": "test123456", "email": "not-an-email", "status": "0",
		}},
		{name: "角色缺少名称", method: http.MethodPost, path: "/system/role", body: map[string]any{
			"roleKey": seed, "roleSort": 1, "status": "0",
		}},
		{name: "角色排序类型非法", method: http.MethodPost, path: "/system/role", body: map[string]any{
			"roleName": seed, "roleKey": seed, "roleSort": "not-a-number", "status": "0",
		}},
		{name: "部门缺少名称", method: http.MethodPost, path: "/system/dept", body: map[string]any{
			"parentId": 0, "orderNum": 1, "status": "0",
		}},
		{name: "部门邮箱格式非法", method: http.MethodPost, path: "/system/dept", body: map[string]any{
			"parentId": 0, "deptName": seed, "orderNum": 1, "email": "not-an-email", "status": "0",
		}},
		{name: "菜单缺少名称", method: http.MethodPost, path: "/system/menu", body: map[string]any{
			"parentId": 0, "orderNum": 1, "path": seed, "menuType": "M", "isFrame": "1", "isCache": "0", "visible": "0", "status": "0",
		}},
		{name: "菜单排序类型非法", method: http.MethodPost, path: "/system/menu", body: map[string]any{
			"menuName": seed, "parentId": 0, "orderNum": "not-a-number", "path": seed, "menuType": "M", "isFrame": "1", "isCache": "0", "visible": "0", "status": "0",
		}},
		{name: "岗位缺少编码", method: http.MethodPost, path: "/system/post", body: map[string]any{
			"postName": seed, "postSort": 1, "status": "0",
		}},
		{name: "岗位排序类型非法", method: http.MethodPost, path: "/system/post", body: map[string]any{
			"postCode": seed, "postName": seed, "postSort": "not-a-number", "status": "0",
		}},
		{name: "参数缺少键名", method: http.MethodPost, path: "/system/config", body: map[string]any{
			"configName": seed, "configValue": "value", "configType": "N",
		}},
		{name: "字典类型格式非法", method: http.MethodPost, path: "/system/dict/type", body: map[string]any{
			"dictName": seed, "dictType": "Invalid-Type", "status": "0",
		}},
		{name: "字典数据缺少标签", method: http.MethodPost, path: "/system/dict/data", body: map[string]any{
			"dictSort": 1, "dictValue": seed, "dictType": "sys_normal_disable", "status": "0",
		}},
		{name: "公告缺少标题", method: http.MethodPost, path: "/system/notice", body: map[string]any{
			"noticeType": "1", "noticeContent": "validation", "status": "0",
		}},
		{name: "任务缺少名称", method: http.MethodPost, path: "/monitor/job", body: map[string]any{
			"jobGroup": "DEFAULT", "invokeTarget": jobInvoke, "cronExpression": "0 0 3 * * ?", "misfirePolicy": "3", "concurrent": "1", "status": "1",
		}, cleanupSpec: jobCleanup, cleanupValue: jobInvoke},
		{name: "任务 Cron 非法", method: http.MethodPost, path: "/monitor/job", body: map[string]any{
			"jobName": seed, "jobGroup": "DEFAULT", "invokeTarget": "ryTask.ryNoParams", "cronExpression": "not-a-cron", "misfirePolicy": "3", "concurrent": "1", "status": "1",
		}},
	}

	semanticMatched := 0
	for _, item := range probes {
		total++
		goResponse, goErr := requestJSON(goAPI, item.method, item.path, item.body)
		javaResponse, javaErr := requestJSON(javaAPI, item.method, item.path, item.body)
		goCode, javaCode := responseCode(goResponse), responseCode(javaResponse)
		if err := errors.Join(
			cleanupValidationMutation(goAPI, item, goCode),
			cleanupValidationMutation(javaAPI, item, javaCode),
		); err != nil {
			fmt.Printf("VALIDATION_PROBE_DIFF %s\n  cleanup failed: %v\n", item.name, err)
			continue
		}
		if goErr == nil && javaErr == nil && goCode != 200 && javaCode != 200 {
			semanticMatched++
		}
		if goErr == nil && javaErr == nil && (goCode == 200 || javaCode == 200) {
			fmt.Printf("VALIDATION_PROBE_DIFF %s\n  invalid payload unexpectedly succeeded: goCode=%d javaCode=%d\n", item.name, responseCode(goResponse), responseCode(javaResponse))
			continue
		}
		if probeResponsesMatch(item.name, "VALIDATION_PROBE", goResponse, goErr, javaResponse, javaErr, maxDiffs) {
			matched++
		}
	}
	fmt.Printf("VALIDATION_SEMANTIC_SUMMARY rejectedByBoth=%d different=%d total=%d percent=%.2f%%\n",
		semanticMatched, total-semanticMatched, total, float64(semanticMatched)*100/float64(total))
	return matched, total
}

func cleanupValidationMutation(client *apiClient, item validationProbe, code int) error {
	if code != 200 || item.cleanupSpec == nil {
		return nil
	}
	id, err := locateCreatedID(client, *item.cleanupSpec, item.cleanupValue)
	if err != nil {
		return err
	}
	response, err := client.request(http.MethodDelete, item.cleanupSpec.basePath+"/"+formatID(id), "", nil)
	if err != nil {
		return err
	}
	if responseCode(response) != 200 {
		return fmt.Errorf("DELETE returned %s", jsonValue(response.body))
	}
	return nil
}
