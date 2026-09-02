package main

import (
	"fmt"
	"net/http"
)

type supplementalProbe struct {
	name         string
	method       string
	path         string
	contentType  string
	body         []byte
	unauthorized bool
	semanticOnly bool
}

type supplementalDynamicProbe struct {
	name       string
	listPath   string
	container  string
	idField    string
	pathFormat string
	fallbackID string
}

func supplementalRouteProbes() []supplementalProbe {
	return []supplementalProbe{
		{name: "普通文件下载缺失文件", method: http.MethodGet, path: "/common/download?fileName=zz_contract_missing.txt"},
		{name: "资源文件下载非法资源", method: http.MethodGet, path: "/common/download/resource?resource=/profile/zz_contract_missing.txt"},
		{name: "缓存概览", method: http.MethodGet, path: "/monitor/cache", semanticOnly: true},
		{name: "缓存分类", method: http.MethodGet, path: "/monitor/cache/getNames"},
		{name: "缓存键列表", method: http.MethodGet, path: "/monitor/cache/getKeys/sys_config:", semanticOnly: true},
		{name: "缓存值", method: http.MethodGet, path: "/monitor/cache/getValue/sys_config:/zz_contract_missing"},
		{name: "登录日志列表", method: http.MethodGet, path: "/monitor/logininfor/list?pageNum=1&pageSize=10", semanticOnly: true},
		{name: "解锁不存在账号", method: http.MethodGet, path: "/monitor/logininfor/unlock/zz_contract_missing"},
		{name: "在线用户列表", method: http.MethodGet, path: "/monitor/online/list?pageNum=1&pageSize=10", semanticOnly: true},
		{name: "操作日志列表", method: http.MethodGet, path: "/monitor/operlog/list?pageNum=1&pageSize=10", semanticOnly: true},
		{name: "服务器信息", method: http.MethodGet, path: "/monitor/server", semanticOnly: true},
		{name: "个人资料", method: http.MethodGet, path: "/system/user/profile"},
		{name: "解锁屏幕", method: http.MethodPost, path: "/unlockscreen", contentType: "application/json", body: []byte(`{"password":"admin123"}`)},

		{name: "清空全部缓存未登录", method: http.MethodDelete, path: "/monitor/cache/clearCacheAll", unauthorized: true},
		{name: "清空任务日志未登录", method: http.MethodDelete, path: "/monitor/jobLog/clean", unauthorized: true},
		{name: "删除任务日志未登录", method: http.MethodDelete, path: "/monitor/jobLog/0", unauthorized: true},
		{name: "清空登录日志未登录", method: http.MethodDelete, path: "/monitor/logininfor/clean", unauthorized: true},
		{name: "删除登录日志未登录", method: http.MethodDelete, path: "/monitor/logininfor/0", unauthorized: true},
		{name: "强退在线用户未登录", method: http.MethodDelete, path: "/monitor/online/zz_contract_missing", unauthorized: true},
		{name: "清空操作日志未登录", method: http.MethodDelete, path: "/monitor/operlog/clean", unauthorized: true},
		{name: "删除操作日志未登录", method: http.MethodDelete, path: "/monitor/operlog/0", unauthorized: true},
		{name: "刷新参数缓存未登录", method: http.MethodDelete, path: "/system/config/refreshCache", unauthorized: true},
		{name: "刷新字典缓存未登录", method: http.MethodDelete, path: "/system/dict/type/refreshCache", unauthorized: true},
		{name: "标记公告已读未登录", method: http.MethodPost, path: "/system/notice/markRead", contentType: "application/json", body: []byte(`{"noticeId":0}`), unauthorized: true},
		{name: "标记全部公告已读未登录", method: http.MethodPost, path: "/system/notice/markReadAll", contentType: "application/json", body: []byte(`{}`), unauthorized: true},
		{name: "修改任务状态未登录", method: http.MethodPut, path: "/monitor/job/changeStatus", contentType: "application/json", body: []byte(`{"jobId":0,"status":"1"}`), unauthorized: true},
		{name: "执行任务未登录", method: http.MethodPut, path: "/monitor/job/run", contentType: "application/json", body: []byte(`{"jobId":0,"jobGroup":"DEFAULT"}`), unauthorized: true},
		{name: "部门排序未登录", method: http.MethodPut, path: "/system/dept/updateSort", contentType: "application/json", body: []byte(`[]`), unauthorized: true},
		{name: "菜单排序未登录", method: http.MethodPut, path: "/system/menu/updateSort", contentType: "application/json", body: []byte(`[]`), unauthorized: true},
		{name: "取消角色授权未登录", method: http.MethodPut, path: "/system/role/authUser/cancel?userId=0&roleId=0", unauthorized: true},
		{name: "批量取消角色授权未登录", method: http.MethodPut, path: "/system/role/authUser/cancelAll?userIds=0&roleId=0", unauthorized: true},
		{name: "批量角色授权未登录", method: http.MethodPut, path: "/system/role/authUser/selectAll?userIds=0&roleId=0", unauthorized: true},
		{name: "角色状态未登录", method: http.MethodPut, path: "/system/role/changeStatus", contentType: "application/json", body: []byte(`{"roleId":0,"status":"1"}`), unauthorized: true},
		{name: "角色数据范围未登录", method: http.MethodPut, path: "/system/role/dataScope", contentType: "application/json", body: []byte(`{"roleId":0,"dataScope":"1","deptIds":[]}`), unauthorized: true},
		{name: "保存用户角色未登录", method: http.MethodPut, path: "/system/user/authRole?userId=0&roleIds=", unauthorized: true},
		{name: "用户状态未登录", method: http.MethodPut, path: "/system/user/changeStatus", contentType: "application/json", body: []byte(`{"userId":0,"status":"1"}`), unauthorized: true},
		{name: "修改个人资料未登录", method: http.MethodPut, path: "/system/user/profile", contentType: "application/json", body: []byte(`{}`), unauthorized: true},
		{name: "修改个人密码未登录", method: http.MethodPut, path: "/system/user/profile/updatePwd?oldPassword=x&newPassword=y", unauthorized: true},
		{name: "重置用户密码未登录", method: http.MethodPut, path: "/system/user/resetPwd", contentType: "application/json", body: []byte(`{"userId":0,"password":"test123"}`), unauthorized: true},
	}
}

// compareSupplementalRouteProbes gives every remaining Java route at least one
// executable scenario. Destructive endpoints use an unauthenticated rejection
// scenario; this verifies routing and the shared security contract without
// deleting logs, sessions, or cache data from the comparison database.
func compareSupplementalRouteProbes(goAPI, javaAPI *apiClient, maxDiffs int) (matched, total int) {
	probes := supplementalRouteProbes()

	for _, item := range probes {
		total++
		var tokenOverride []string
		if item.unauthorized {
			tokenOverride = []string{""}
		}
		goResponse, goErr := goAPI.request(item.method, item.path, item.contentType, item.body, tokenOverride...)
		javaResponse, javaErr := javaAPI.request(item.method, item.path, item.contentType, item.body, tokenOverride...)
		if supplementalResponsesMatch(item, goResponse, goErr, javaResponse, javaErr, maxDiffs) {
			matched++
		}
	}

	// The primary setup already proves both login endpoints can issue a token.
	total++
	if goAPI.token != "" && javaAPI.token != "" {
		matched++
		fmt.Println("SUPPLEMENTAL_PROBE_MATCH 登录成功（两端均签发 token）")
	} else {
		fmt.Println("SUPPLEMENTAL_PROBE_DIFF 登录成功（至少一端没有 token）")
	}

	goLogout, goErr := goAPI.request(http.MethodPost, "/logout", "", nil, "")
	javaLogout, javaErr := javaAPI.request(http.MethodPost, "/logout", "", nil, "")
	total++
	if probeResponsesMatch("无会话登出", "SUPPLEMENTAL_PROBE", goLogout, goErr, javaLogout, javaErr, maxDiffs) {
		matched++
	}

	dynamicMatched, dynamicTotal := compareSupplementalDynamicReads(goAPI, javaAPI, maxDiffs)
	exportMatched, exportTotal := compareSupplementalExports(goAPI, javaAPI)
	return matched + dynamicMatched + exportMatched, total + dynamicTotal + exportTotal
}

func compareSupplementalDynamicReads(goAPI, javaAPI *apiClient, maxDiffs int) (matched, total int) {
	probes := supplementalDynamicRouteProbes()

	for _, item := range probes {
		total++
		goList, goListErr := goAPI.get(item.listPath)
		javaList, javaListErr := javaAPI.get(item.listPath)
		id := item.fallbackID
		if goListErr == nil && javaListErr == nil {
			probe := dynamicPathProbe{listPath: item.listPath, container: item.container, idField: item.idField}
			if commonID, err := commonProbeID(goList, javaList, probe); err == nil {
				id = commonID
			}
		}

		path := fmt.Sprintf(item.pathFormat, id)
		goResponse, goErr := goAPI.get(path)
		javaResponse, javaErr := javaAPI.get(path)
		probe := supplementalProbe{name: item.name, method: http.MethodGet, path: path}
		if supplementalResponsesMatch(probe, goResponse, goErr, javaResponse, javaErr, maxDiffs) {
			matched++
		}
	}
	return matched, total
}

func supplementalDynamicRouteProbes() []supplementalDynamicProbe {
	return []supplementalDynamicProbe{
		{name: "任务日志详情", listPath: "/monitor/jobLog/list?pageNum=1&pageSize=100", container: "rows", idField: "jobLogId", pathFormat: "/monitor/jobLog/%s", fallbackID: "0"},
		{name: "排除部门子树", listPath: "/system/dept/list", container: "data", idField: "deptId", pathFormat: "/system/dept/list/exclude/%s", fallbackID: "0"},
		{name: "公告已读用户列表", listPath: "/system/notice/list?pageNum=1&pageSize=100", container: "rows", idField: "noticeId", pathFormat: "/system/notice/readUsers/list?noticeId=%s&pageNum=1&pageSize=10", fallbackID: "0"},
		{name: "角色已分配用户", listPath: "/system/role/list?pageNum=1&pageSize=100", container: "rows", idField: "roleId", pathFormat: "/system/role/authUser/allocatedList?roleId=%s&pageNum=1&pageSize=10", fallbackID: "0"},
		{name: "角色未分配用户", listPath: "/system/role/list?pageNum=1&pageSize=100", container: "rows", idField: "roleId", pathFormat: "/system/role/authUser/unallocatedList?roleId=%s&pageNum=1&pageSize=10", fallbackID: "0"},
		{name: "用户授权角色", listPath: "/system/user/list?pageNum=1&pageSize=100", container: "rows", idField: "userId", pathFormat: "/system/user/authRole/%s", fallbackID: "0"},
	}
}

func supplementalResponsesMatch(item supplementalProbe, goResponse apiResponse, goErr error,
	javaResponse apiResponse, javaErr error, maxDiffs int) bool {
	if goErr != nil || javaErr != nil {
		fmt.Printf("SUPPLEMENTAL_PROBE_DIFF %s\n  request error: go=%v java=%v\n", item.name, goErr, javaErr)
		return false
	}
	if item.semanticOnly {
		goOK := goResponse.status == http.StatusOK && responseCode(goResponse) == 200
		javaOK := javaResponse.status == http.StatusOK && responseCode(javaResponse) == 200
		if goOK && javaOK {
			fmt.Printf("SUPPLEMENTAL_PROBE_MATCH %s（双方成功，动态运行数据不做值相等比较）\n", item.name)
			return true
		}
		fmt.Printf("SUPPLEMENTAL_PROBE_DIFF %s\n  semantic success: go=%t java=%t\n", item.name, goOK, javaOK)
		return false
	}
	if item.unauthorized {
		goCode, javaCode := responseCode(goResponse), responseCode(javaResponse)
		if goCode == http.StatusUnauthorized && javaCode == http.StatusUnauthorized {
			fmt.Printf("SUPPLEMENTAL_PROBE_MATCH %s（双方均拒绝未登录请求）\n", item.name)
			return true
		}
		fmt.Printf("SUPPLEMENTAL_PROBE_DIFF %s\n  unauthorized rejection: goCode=%d javaCode=%d\n", item.name, goCode, javaCode)
		return false
	}
	return probeResponsesMatch(item.name, "SUPPLEMENTAL_PROBE", goResponse, nil, javaResponse, nil, maxDiffs)
}

func compareSupplementalExports(goAPI, javaAPI *apiClient) (matched, total int) {
	paths := supplementalExportPaths()
	for _, path := range paths {
		total++
		goResponse, goErr := goAPI.request(http.MethodPost, path, "", nil)
		javaResponse, javaErr := javaAPI.request(http.MethodPost, path, "", nil)
		if goErr == nil && javaErr == nil && validXLSXResponse(goResponse) && validXLSXResponse(javaResponse) {
			matched++
			fmt.Printf("SUPPLEMENTAL_PROBE_MATCH 导出 %s（双方均为合法 XLSX）\n", path)
			continue
		}
		fmt.Printf("SUPPLEMENTAL_PROBE_DIFF 导出 %s\n  goErr=%v javaErr=%v goStatus=%d javaStatus=%d goType=%q javaType=%q\n",
			path, goErr, javaErr, goResponse.status, javaResponse.status, goResponse.contentType, javaResponse.contentType)
	}
	return matched, total
}

func supplementalExportPaths() []string {
	return []string{
		"/monitor/job/export", "/monitor/jobLog/export", "/monitor/logininfor/export",
		"/monitor/operlog/export", "/system/config/export", "/system/dict/data/export",
		"/system/dict/type/export", "/system/post/export", "/system/role/export", "/system/user/export",
	}
}
