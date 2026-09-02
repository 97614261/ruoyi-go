// Command contractcheck compares normalized JSON responses from running Go and
// Java RuoYi instances. The two services must use isolated Redis databases.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/xuri/excelize/v2"
)

var corePaths = []string{
	"/captchaImage",
	"/getInfo",
	"/getRouters",
	"/system/user/",
	"/system/user/list?pageNum=1&pageSize=10",
	"/system/user/deptTree",
	"/system/role/list?pageNum=1&pageSize=10",
	"/system/role/optionselect",
	"/system/post/list?pageNum=1&pageSize=10",
	"/system/post/optionselect",
	"/system/dept/list",
	"/system/menu/list",
	"/system/menu/treeselect",
	"/system/config/list?pageNum=1&pageSize=10",
	"/system/config/configKey/sys.account.captchaEnabled",
	"/system/dict/type/list?pageNum=1&pageSize=10",
	"/system/dict/type/optionselect",
	"/system/dict/data/list?pageNum=1&pageSize=10",
	"/system/dict/data/type/sys_normal_disable",
	"/system/notice/list?pageNum=1&pageSize=10",
	"/system/notice/listTop?pageNum=1&pageSize=10",
	"/monitor/job/list?pageNum=1&pageSize=10",
	"/monitor/jobLog/list?pageNum=1&pageSize=10",
}

type dynamicPathProbe struct {
	name       string
	listPath   string
	container  string
	idField    string
	pathFormat string
	selector   func(map[string]any) bool
}

// 详情和角色树不能写死种子 ID：换库后 ID 可能变化。两端共用 MySQL，
// 所以先从各自列表响应中找一个共同 ID，再请求同一条记录。
var dynamicPathProbes = []dynamicPathProbe{
	{name: "角色部门树", listPath: "/system/role/list?pageNum=1&pageSize=100", container: "rows", idField: "roleId", pathFormat: "/system/role/deptTree/%s"},
	{name: "角色菜单树", listPath: "/system/role/list?pageNum=1&pageSize=100", container: "rows", idField: "roleId", pathFormat: "/system/menu/roleMenuTreeselect/%s"},
	{name: "用户详情", listPath: "/system/user/list?pageNum=1&pageSize=100", container: "rows", idField: "userId", pathFormat: "/system/user/%s"},
	{name: "角色详情", listPath: "/system/role/list?pageNum=1&pageSize=100", container: "rows", idField: "roleId", pathFormat: "/system/role/%s"},
	{name: "岗位详情", listPath: "/system/post/list?pageNum=1&pageSize=100", container: "rows", idField: "postId", pathFormat: "/system/post/%s"},
	{name: "部门详情（有父级）", listPath: "/system/dept/list", container: "data", idField: "deptId", pathFormat: "/system/dept/%s", selector: numericFieldNonZero("parentId")},
	{name: "部门详情（根部门）", listPath: "/system/dept/list", container: "data", idField: "deptId", pathFormat: "/system/dept/%s", selector: numericFieldZero("parentId")},
	{name: "菜单详情", listPath: "/system/menu/list", container: "data", idField: "menuId", pathFormat: "/system/menu/%s"},
	{name: "参数详情", listPath: "/system/config/list?pageNum=1&pageSize=100", container: "rows", idField: "configId", pathFormat: "/system/config/%s"},
	{name: "字典类型详情", listPath: "/system/dict/type/list?pageNum=1&pageSize=100", container: "rows", idField: "dictId", pathFormat: "/system/dict/type/%s"},
	{name: "字典数据详情", listPath: "/system/dict/data/list?pageNum=1&pageSize=100", container: "rows", idField: "dictCode", pathFormat: "/system/dict/data/%s"},
	{name: "公告详情", listPath: "/system/notice/list?pageNum=1&pageSize=100", container: "rows", idField: "noticeId", pathFormat: "/system/notice/%s"},
	{name: "任务详情", listPath: "/monitor/job/list?pageNum=1&pageSize=100", container: "rows", idField: "jobId", pathFormat: "/monitor/job/%s"},
}

var dynamicKeys = map[string]struct{}{
	"img": {}, "uuid": {}, "token": {}, "loginIp": {}, "loginDate": {},
}

type options struct {
	goBase           string
	javaBase         string
	redisAddr        string
	redisPass        string
	goRedisDB        int
	javaRedisDB      int
	username         string
	password         string
	timeout          time.Duration
	maxDiffs         int
	failOnDiff       bool
	writeProbes      bool
	fileProbes       bool
	crudProbes       bool
	permissionProbes bool
	validationProbes bool
}

type probe struct {
	name        string
	method      string
	path        string
	contentType string
	body        []byte
}

type apiClient struct {
	base  string
	token string
	http  *http.Client
	redis *redis.Client
	// sessionKey 和 redisKeys 只保存本次运行精确创建的 key，收尾时不会扫描或清空共享 DB。
	sessionKey string
	redisKeys  map[string]struct{}
}

type apiResponse struct {
	status      int
	contentType string
	body        any
}

func main() {
	os.Exit(run())
}

func run() (exitCode int) {
	var cfg options
	flag.StringVar(&cfg.goBase, "go-base", "http://127.0.0.1:8080", "Go service base URL")
	flag.StringVar(&cfg.javaBase, "java-base", "http://127.0.0.1:8081", "Java service base URL")
	flag.StringVar(&cfg.redisAddr, "redis-addr", "127.0.0.1:6379", "Redis address")
	flag.StringVar(&cfg.redisPass, "redis-password", "", "Redis password")
	flag.IntVar(&cfg.goRedisDB, "go-redis-db", 0, "Go Redis database")
	flag.IntVar(&cfg.javaRedisDB, "java-redis-db", 1, "Java Redis database")
	flag.StringVar(&cfg.username, "username", "admin", "login username")
	flag.StringVar(&cfg.password, "password", "admin123", "login password")
	flag.DurationVar(&cfg.timeout, "timeout", 20*time.Second, "request timeout")
	flag.IntVar(&cfg.maxDiffs, "max-diffs", 12, "maximum differences printed per endpoint")
	flag.BoolVar(&cfg.failOnDiff, "fail-on-diff", true, "exit non-zero when a difference is found")
	flag.BoolVar(&cfg.writeProbes, "write-probes", false, "compare non-mutating validation/error responses")
	flag.BoolVar(&cfg.fileProbes, "file-probes", false, "compare upload and empty-import contracts (use disposable upload profiles)")
	flag.BoolVar(&cfg.crudProbes, "crud-probes", false, "compare disposable create/read/update/delete scenarios")
	flag.BoolVar(&cfg.permissionProbes, "permission-probes", false, "compare disposable five-scope data-permission scenarios")
	flag.BoolVar(&cfg.validationProbes, "validation-probes", false, "compare disposable field validation and JSON type errors")
	flag.Parse()

	if cfg.goRedisDB == cfg.javaRedisDB {
		fmt.Fprintln(os.Stderr, "Go and Java must use different Redis databases")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	goRedis := redis.NewClient(&redis.Options{Addr: cfg.redisAddr, Password: cfg.redisPass, DB: cfg.goRedisDB})
	javaRedis := redis.NewClient(&redis.Options{Addr: cfg.redisAddr, Password: cfg.redisPass, DB: cfg.javaRedisDB})
	defer goRedis.Close()
	defer javaRedis.Close()
	if err := goRedis.Ping(ctx).Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Go Redis: %v\n", err)
		return 2
	}
	if err := javaRedis.Ping(ctx).Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Java Redis: %v\n", err)
		return 2
	}

	goAPI := &apiClient{base: strings.TrimRight(cfg.goBase, "/"), http: &http.Client{Timeout: cfg.timeout}, redis: goRedis, redisKeys: make(map[string]struct{})}
	javaAPI := &apiClient{base: strings.TrimRight(cfg.javaBase, "/"), http: &http.Client{Timeout: cfg.timeout}, redis: javaRedis, redisKeys: make(map[string]struct{})}
	defer func() {
		for _, client := range []*apiClient{goAPI, javaAPI} {
			if err := client.cleanup(); err != nil {
				fmt.Fprintf(os.Stderr, "contractcheck cleanup failed: %v\n", err)
				exitCode = 2
			}
		}
	}()
	var err error
	goAPI.token, err = login(goAPI, cfg.username, cfg.password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Go login: %v\n", err)
		return 2
	}
	goAPI.sessionKey, err = loginSessionKey(goAPI.token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Go token: %v\n", err)
		return 2
	}
	javaAPI.token, err = login(javaAPI, cfg.username, cfg.password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Java login: %v\n", err)
		return 2
	}
	javaAPI.sessionKey, err = loginSessionKey(javaAPI.token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Java token: %v\n", err)
		return 2
	}

	matched := 0
	for _, path := range corePaths {
		goResponse, goErr := goAPI.get(path)
		javaResponse, javaErr := javaAPI.get(path)
		if goErr != nil || javaErr != nil {
			fmt.Printf("DIFF %s\n  request error: go=%v java=%v\n", path, goErr, javaErr)
			continue
		}
		diffs := compareResponses(goResponse, javaResponse)
		if len(diffs) == 0 {
			matched++
			fmt.Printf("MATCH %s\n", path)
			continue
		}
		fmt.Printf("DIFF %s\n", path)
		limit := min(cfg.maxDiffs, len(diffs))
		for _, diff := range diffs[:limit] {
			fmt.Printf("  %s\n", diff)
		}
		if len(diffs) > limit {
			fmt.Printf("  ... %d more\n", len(diffs)-limit)
		}
	}

	dynamicMatched, dynamicTotal := compareDynamicPathProbes(goAPI, javaAPI, cfg.maxDiffs)
	matched += dynamicMatched
	total := len(corePaths) + dynamicTotal
	different := total - matched
	percent := float64(matched) * 100 / float64(total)
	fmt.Printf("SUMMARY matched=%d different=%d total=%d percent=%.2f%%\n", matched, different, total, percent)
	supplementalMatched, supplementalTotal := compareSupplementalRouteProbes(goAPI, javaAPI, cfg.maxDiffs)
	supplementalDifferent := supplementalTotal - supplementalMatched
	fmt.Printf("SUPPLEMENTAL_PROBE_SUMMARY matched=%d different=%d total=%d percent=%.2f%%\n",
		supplementalMatched, supplementalDifferent, supplementalTotal,
		float64(supplementalMatched)*100/float64(supplementalTotal))
	different += supplementalDifferent
	if cfg.writeProbes {
		probeMatched, probeTotal := compareWriteProbes(goAPI, javaAPI, cfg.maxDiffs)
		probeDifferent := probeTotal - probeMatched
		fmt.Printf("WRITE_PROBE_SUMMARY matched=%d different=%d total=%d percent=%.2f%%\n",
			probeMatched, probeDifferent, probeTotal, float64(probeMatched)*100/float64(probeTotal))
		different += probeDifferent
	}
	if cfg.fileProbes {
		probeMatched, probeTotal := compareFileProbes(goAPI, javaAPI, cfg.maxDiffs)
		probeDifferent := probeTotal - probeMatched
		fmt.Printf("FILE_PROBE_SUMMARY matched=%d different=%d total=%d percent=%.2f%%\n",
			probeMatched, probeDifferent, probeTotal, float64(probeMatched)*100/float64(probeTotal))
		different += probeDifferent
	}
	if cfg.crudProbes {
		probeMatched, probeTotal := compareCRUDProbes(goAPI, javaAPI, cfg.maxDiffs)
		probeDifferent := probeTotal - probeMatched
		fmt.Printf("CRUD_PROBE_SUMMARY matched=%d different=%d total=%d percent=%.2f%%\n",
			probeMatched, probeDifferent, probeTotal, float64(probeMatched)*100/float64(probeTotal))
		different += probeDifferent
	}
	if cfg.permissionProbes {
		probeMatched, probeTotal := comparePermissionProbes(goAPI, javaAPI, cfg.password, cfg.maxDiffs)
		probeDifferent := probeTotal - probeMatched
		fmt.Printf("PERMISSION_PROBE_SUMMARY matched=%d different=%d total=%d percent=%.2f%%\n",
			probeMatched, probeDifferent, probeTotal, float64(probeMatched)*100/float64(probeTotal))
		different += probeDifferent
	}
	if cfg.validationProbes {
		probeMatched, probeTotal := compareValidationProbes(goAPI, javaAPI, cfg.maxDiffs)
		probeDifferent := probeTotal - probeMatched
		fmt.Printf("VALIDATION_PROBE_SUMMARY matched=%d different=%d total=%d percent=%.2f%%\n",
			probeMatched, probeDifferent, probeTotal, float64(probeMatched)*100/float64(probeTotal))
		different += probeDifferent
	}
	if different > 0 && cfg.failOnDiff {
		return 1
	}
	return 0
}

func compareDynamicPathProbes(goAPI, javaAPI *apiClient, maxDiffs int) (matched, total int) {
	for _, probe := range dynamicPathProbes {
		total++
		goList, goErr := goAPI.get(probe.listPath)
		javaList, javaErr := javaAPI.get(probe.listPath)
		if goErr != nil || javaErr != nil {
			fmt.Printf("DIFF %s\n  list request error: go=%v java=%v\n", probe.name, goErr, javaErr)
			continue
		}

		id, err := commonProbeID(goList, javaList, probe)
		if err != nil {
			fmt.Printf("DIFF %s\n  resolve common ID: %v\n", probe.name, err)
			continue
		}
		path := fmt.Sprintf(probe.pathFormat, id)
		goResponse, goErr := goAPI.get(path)
		javaResponse, javaErr := javaAPI.get(path)
		if goErr != nil || javaErr != nil {
			fmt.Printf("DIFF %s %s\n  request error: go=%v java=%v\n", probe.name, path, goErr, javaErr)
			continue
		}
		diffs := compareResponses(goResponse, javaResponse)
		if len(diffs) == 0 {
			matched++
			fmt.Printf("MATCH %s %s\n", probe.name, path)
			continue
		}
		fmt.Printf("DIFF %s %s\n", probe.name, path)
		limit := min(maxDiffs, len(diffs))
		for _, diff := range diffs[:limit] {
			fmt.Printf("  %s\n", diff)
		}
		if len(diffs) > limit {
			fmt.Printf("  ... %d more\n", len(diffs)-limit)
		}
	}
	return matched, total
}

func commonProbeID(goResponse, javaResponse apiResponse, probe dynamicPathProbe) (string, error) {
	goItems, err := probeItems(goResponse, probe.container)
	if err != nil {
		return "", fmt.Errorf("Go list: %w", err)
	}
	javaItems, err := probeItems(javaResponse, probe.container)
	if err != nil {
		return "", fmt.Errorf("Java list: %w", err)
	}

	javaIDs := make(map[string]struct{}, len(javaItems))
	for _, item := range javaItems {
		if probe.selector != nil && !probe.selector(item) {
			continue
		}
		if id, ok := probeItemID(item, probe.idField); ok {
			javaIDs[id] = struct{}{}
		}
	}
	for _, item := range goItems {
		if probe.selector != nil && !probe.selector(item) {
			continue
		}
		id, ok := probeItemID(item, probe.idField)
		if !ok {
			continue
		}
		if _, exists := javaIDs[id]; exists {
			return id, nil
		}
	}
	return "", fmt.Errorf("no common %s in %q", probe.idField, probe.listPath)
}

func probeItems(response apiResponse, container string) ([]map[string]any, error) {
	if response.status != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", response.status)
	}
	body, ok := response.body.(map[string]any)
	if !ok || body["code"] != float64(200) {
		return nil, fmt.Errorf("unexpected response %s", jsonValue(response.body))
	}
	rawItems, ok := body[container].([]any)
	if !ok {
		return nil, fmt.Errorf("field %q is not an array", container)
	}
	items := make([]map[string]any, 0, len(rawItems))
	for _, raw := range rawItems {
		if item, ok := raw.(map[string]any); ok {
			items = append(items, item)
		}
	}
	return items, nil
}

func probeItemID(item map[string]any, field string) (string, bool) {
	switch value := item[field].(type) {
	case float64:
		id := int64(value)
		if id <= 0 || float64(id) != value {
			return "", false
		}
		return strconv.FormatInt(id, 10), true
	case string:
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 {
			return "", false
		}
		return strconv.FormatInt(id, 10), true
	default:
		return "", false
	}
}

func numericFieldZero(field string) func(map[string]any) bool {
	return func(item map[string]any) bool {
		value, ok := item[field].(float64)
		return ok && value == 0
	}
}

func numericFieldNonZero(field string) func(map[string]any) bool {
	return func(item map[string]any) bool {
		value, ok := item[field].(float64)
		return ok && value != 0
	}
}

type uploadPart struct {
	fieldName string
	fileName  string
	data      []byte
}

func compareFileProbes(goAPI, javaAPI *apiClient, maxDiffs int) (int, int) {
	matched, total := 0, 0
	check := func(name string, goResponse apiResponse, goErr error, javaResponse apiResponse, javaErr error) {
		total++
		if goErr != nil || javaErr != nil {
			fmt.Printf("FILE_PROBE_DIFF %s\n  request error: go=%v java=%v\n", name, goErr, javaErr)
			return
		}
		diffs := compareResponses(goResponse, javaResponse)
		if len(diffs) == 0 {
			matched++
			fmt.Printf("FILE_PROBE_MATCH %s\n", name)
			return
		}
		fmt.Printf("FILE_PROBE_DIFF %s\n", name)
		limit := min(maxDiffs, len(diffs))
		for _, diff := range diffs[:limit] {
			fmt.Printf("  %s\n", diff)
		}
	}

	fileData := []byte("ruoyi contract probe\n")
	goUpload, goErr := goAPI.multipart(http.MethodPost, "/common/upload", []uploadPart{
		{fieldName: "file", fileName: "zz_contract_probe.txt", data: fileData},
	})
	javaUpload, javaErr := javaAPI.multipart(http.MethodPost, "/common/upload", []uploadPart{
		{fieldName: "file", fileName: "zz_contract_probe.txt", data: fileData},
	})
	normalizeUploadFields(&goUpload, "url", "fileName", "newFileName")
	normalizeUploadFields(&javaUpload, "url", "fileName", "newFileName")
	check("单文件上传成功响应", goUpload, goErr, javaUpload, javaErr)

	parts := []uploadPart{
		{fieldName: "files", fileName: "zz_contract_a.txt", data: fileData},
		{fieldName: "files", fileName: "zz_contract_b.txt", data: fileData},
	}
	goUploads, goErr := goAPI.multipart(http.MethodPost, "/common/uploads", parts)
	javaUploads, javaErr := javaAPI.multipart(http.MethodPost, "/common/uploads", parts)
	normalizeUploadFields(&goUploads, "urls", "fileNames", "newFileNames")
	normalizeUploadFields(&javaUploads, "urls", "fileNames", "newFileNames")
	check("多文件上传成功响应", goUploads, goErr, javaUploads, javaErr)

	goTemplate, goErr := goAPI.request(http.MethodPost, "/system/user/importTemplate", "", nil)
	javaTemplate, javaErr := javaAPI.request(http.MethodPost, "/system/user/importTemplate", "", nil)
	goTemplateBody, goTemplateOK := goTemplate.body.(string)
	javaTemplateBody, javaTemplateOK := javaTemplate.body.(string)
	total++
	if goErr == nil && javaErr == nil && validXLSXResponse(goTemplate) && validXLSXResponse(javaTemplate) {
		matched++
		fmt.Println("FILE_PROBE_MATCH 用户导入模板均为合法 XLSX 响应")
	} else {
		fmt.Printf("FILE_PROBE_DIFF 用户导入模板：goErr=%v javaErr=%v goStatus=%d javaStatus=%d goType=%q javaType=%q\n",
			goErr, javaErr, goTemplate.status, javaTemplate.status, goTemplate.contentType, javaTemplate.contentType)
	}

	if goErr == nil && javaErr == nil && goTemplateOK && javaTemplateOK {
		goBytes := []byte(goTemplateBody)
		javaBytes := []byte(javaTemplateBody)
		goImport, goImportErr := goAPI.multipart(http.MethodPost, "/system/user/importData", []uploadPart{
			{fieldName: "file", fileName: "go_empty_template.xlsx", data: goBytes},
		})
		javaImport, javaImportErr := javaAPI.multipart(http.MethodPost, "/system/user/importData", []uploadPart{
			{fieldName: "file", fileName: "java_empty_template.xlsx", data: javaBytes},
		})
		check("空用户模板导入错误语义", goImport, goImportErr, javaImport, javaImportErr)
	}

	return matched, total
}

func (client *apiClient) multipart(method, path string, parts []uploadPart) (apiResponse, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, item := range parts {
		part, err := writer.CreateFormFile(item.fieldName, item.fileName)
		if err != nil {
			return apiResponse{}, err
		}
		if _, err := part.Write(item.data); err != nil {
			return apiResponse{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return apiResponse{}, err
	}
	return client.request(method, path, writer.FormDataContentType(), body.Bytes())
}

func normalizeUploadFields(response *apiResponse, keys ...string) {
	body, ok := response.body.(map[string]any)
	if !ok {
		return
	}
	for _, key := range keys {
		if _, exists := body[key]; exists {
			body[key] = "<dynamic-file-path>"
		}
	}
}

func validXLSXResponse(response apiResponse) bool {
	body, ok := response.body.(string)
	if response.status != http.StatusOK || !ok || !strings.HasPrefix(body, "PK") ||
		!strings.Contains(strings.ToLower(response.contentType), "spreadsheetml") {
		return false
	}
	file, err := excelize.OpenReader(strings.NewReader(body))
	if err != nil {
		return false
	}
	defer file.Close()
	sheets := file.GetSheetList()
	if len(sheets) == 0 {
		return false
	}
	rows, err := file.GetRows(sheets[0])
	return err == nil && len(rows) > 0
}

func compareWriteProbes(goAPI, javaAPI *apiClient, maxDiffs int) (int, int) {
	probes := []probe{
		{name: "注册关闭", method: http.MethodPost, path: "/register", contentType: "application/json",
			body: []byte(`{"username":"zz_contract_probe","password":"test123456"}`)},
		{name: "单文件上传缺文件", method: http.MethodPost, path: "/common/upload"},
		{name: "多文件上传缺文件", method: http.MethodPost, path: "/common/uploads"},
		{name: "用户导入缺文件", method: http.MethodPost, path: "/system/user/importData"},
		{name: "头像上传缺文件", method: http.MethodPost, path: "/system/user/profile/avatar"},
		{name: "清理不存在缓存名", method: http.MethodDelete, path: "/monitor/cache/clearCacheName/zz_contract_missing"},
		{name: "清理不存在缓存键", method: http.MethodDelete, path: "/monitor/cache/clearCacheKey/zz_contract_missing"},
	}
	matched := 0
	for _, item := range probes {
		goResponse, goErr := goAPI.request(item.method, item.path, item.contentType, item.body)
		javaResponse, javaErr := javaAPI.request(item.method, item.path, item.contentType, item.body)
		if goErr != nil || javaErr != nil {
			fmt.Printf("WRITE_PROBE_DIFF %s (%s %s)\n  request error: go=%v java=%v\n",
				item.name, item.method, item.path, goErr, javaErr)
			continue
		}
		diffs := compareResponses(goResponse, javaResponse)
		if len(diffs) == 0 {
			matched++
			fmt.Printf("WRITE_PROBE_MATCH %s (%s %s)\n", item.name, item.method, item.path)
			continue
		}
		fmt.Printf("WRITE_PROBE_DIFF %s (%s %s)\n", item.name, item.method, item.path)
		limit := min(maxDiffs, len(diffs))
		for _, diff := range diffs[:limit] {
			fmt.Printf("  %s\n", diff)
		}
		if len(diffs) > limit {
			fmt.Printf("  ... %d more\n", len(diffs)-limit)
		}
	}
	return matched, len(probes)
}

func login(client *apiClient, username, password string) (string, error) {
	response, err := client.request(http.MethodGet, "/captchaImage", "", nil)
	if err != nil {
		return "", err
	}
	body, ok := response.body.(map[string]any)
	if !ok {
		return "", fmt.Errorf("captcha response is not an object")
	}
	payload := map[string]any{"username": username, "password": password}
	if enabled, _ := body["captchaEnabled"].(bool); enabled {
		uuid, _ := body["uuid"].(string)
		answer, err := client.redis.Get(context.Background(), "captcha_codes:"+uuid).Result()
		if err != nil {
			return "", fmt.Errorf("read captcha answer: %w", err)
		}
		payload["uuid"] = uuid
		payload["code"] = strings.Trim(answer, "\"")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	response, err = client.request(http.MethodPost, "/login", "application/json", raw)
	if err != nil {
		return "", err
	}
	body, ok = response.body.(map[string]any)
	if !ok || body["code"] != float64(200) {
		return "", fmt.Errorf("code=%v msg=%v", body["code"], body["msg"])
	}
	token, _ := body["token"].(string)
	if token == "" {
		return "", fmt.Errorf("missing token")
	}
	return token, nil
}

func (client *apiClient) get(path string) (apiResponse, error) {
	token := client.token
	if path == "/captchaImage" {
		token = ""
	}
	return client.request(http.MethodGet, path, "", nil, token)
}

func (client *apiClient) request(method, path, contentType string, body []byte, tokenOverride ...string) (apiResponse, error) {
	token := client.token
	if len(tokenOverride) > 0 {
		token = tokenOverride[0]
	}
	request, err := http.NewRequest(method, client.base+path, bytes.NewReader(body))
	if err != nil {
		return apiResponse{}, err
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.http.Do(request)
	if err != nil {
		return apiResponse{}, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return apiResponse{}, err
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		decoded = string(raw)
	}
	if path == "/captchaImage" {
		if object, ok := decoded.(map[string]any); ok {
			if uuid, _ := object["uuid"].(string); uuid != "" {
				client.redisKeys["captcha_codes:"+uuid] = struct{}{}
			}
		}
	}
	return apiResponse{status: response.StatusCode, contentType: response.Header.Get("Content-Type"), body: decoded}, nil
}

func loginSessionKey(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode JWT payload: %w", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("parse JWT payload: %w", err)
	}
	key, _ := claims["login_user_key"].(string)
	if key == "" {
		return "", fmt.Errorf("JWT is missing login_user_key")
	}
	return "login_tokens:" + key, nil
}

func (client *apiClient) cleanup() error {
	var cleanupErrors []error
	if client.token != "" {
		response, err := client.request(http.MethodPost, "/logout", "", nil)
		if err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("logout %s: %w", client.base, err))
		} else if body, ok := response.body.(map[string]any); !ok || body["code"] != float64(200) {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("logout %s returned %s", client.base, jsonValue(response.body)))
		}
	}
	if client.sessionKey != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		exists, err := client.redis.Exists(ctx, client.sessionKey).Result()
		if err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("check Redis key %s: %w", client.sessionKey, err))
		} else if exists != 0 {
			if err := client.redis.Del(ctx, client.sessionKey).Err(); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("delete Redis key %s: %w", client.sessionKey, err))
			} else {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("logout left Redis key %s; removed by fallback", client.sessionKey))
			}
		}
		cancel()
	}
	keys := make([]string, 0, len(client.redisKeys))
	for key := range client.redisKeys {
		keys = append(keys, key)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, key := range keys {
		if err := client.redis.Del(ctx, key).Err(); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("delete Redis key %s: %w", key, err))
		}
	}
	for _, key := range keys {
		exists, err := client.redis.Exists(ctx, key).Result()
		if err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("verify Redis key %s: %w", key, err))
		} else if exists != 0 {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("Redis key %s still exists after cleanup", key))
		}
	}
	return errors.Join(cleanupErrors...)
}

func compareResponses(goResponse, javaResponse apiResponse) []string {
	return compareResponsesIgnoring(goResponse, javaResponse)
}

func compareResponsesIgnoring(goResponse, javaResponse apiResponse, ignoredKeys ...string) []string {
	var diffs []string
	if goResponse.status != javaResponse.status {
		diffs = append(diffs, fmt.Sprintf("$.httpStatus: Go=%d Java=%d", goResponse.status, javaResponse.status))
	}
	ignored := make(map[string]struct{}, len(ignoredKeys))
	for _, key := range ignoredKeys {
		ignored[key] = struct{}{}
	}
	goBody := normalizeIgnored(goResponse.body, ignored)
	javaBody := normalizeIgnored(javaResponse.body, ignored)
	compareValue("$", goBody, javaBody, &diffs)
	return diffs
}

func normalize(value any) any {
	return normalizeIgnored(value, nil)
}

func normalizeIgnored(value any, ignored map[string]struct{}) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			if _, dynamic := dynamicKeys[key]; dynamic {
				continue
			}
			if _, ignored := ignored[key]; ignored {
				continue
			}
			result[key] = normalizeIgnored(child, ignored)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, child := range typed {
			result[index] = normalizeIgnored(child, ignored)
		}
		return result
	default:
		return value
	}
}

func compareValue(path string, goValue, javaValue any, diffs *[]string) {
	goMap, goIsMap := goValue.(map[string]any)
	javaMap, javaIsMap := javaValue.(map[string]any)
	if goIsMap || javaIsMap {
		if !goIsMap || !javaIsMap {
			*diffs = append(*diffs, fmt.Sprintf("%s type: Go=%T Java=%T", path, goValue, javaValue))
			return
		}
		keys := make(map[string]struct{}, len(goMap)+len(javaMap))
		for key := range goMap {
			keys[key] = struct{}{}
		}
		for key := range javaMap {
			keys[key] = struct{}{}
		}
		ordered := make([]string, 0, len(keys))
		for key := range keys {
			ordered = append(ordered, key)
		}
		sort.Strings(ordered)
		for _, key := range ordered {
			goChild, goOK := goMap[key]
			javaChild, javaOK := javaMap[key]
			if goOK != javaOK {
				goText, javaText := "<missing>", "<missing>"
				if goOK {
					goText = jsonValue(goChild)
				}
				if javaOK {
					javaText = jsonValue(javaChild)
				}
				*diffs = append(*diffs, fmt.Sprintf("%s.%s presence: Go=%t (%s) Java=%t (%s)",
					path, key, goOK, goText, javaOK, javaText))
				continue
			}
			compareValue(path+"."+key, goChild, javaChild, diffs)
		}
		return
	}

	goList, goIsList := goValue.([]any)
	javaList, javaIsList := javaValue.([]any)
	if goIsList || javaIsList {
		if !goIsList || !javaIsList {
			*diffs = append(*diffs, fmt.Sprintf("%s type: Go=%T Java=%T", path, goValue, javaValue))
			return
		}
		if len(goList) != len(javaList) {
			*diffs = append(*diffs, fmt.Sprintf("%s length: Go=%d Java=%d", path, len(goList), len(javaList)))
		}
		for index := 0; index < min(len(goList), len(javaList)); index++ {
			compareValue(path+"["+strconv.Itoa(index)+"]", goList[index], javaList[index], diffs)
		}
		return
	}

	if !reflect.DeepEqual(goValue, javaValue) {
		*diffs = append(*diffs, fmt.Sprintf("%s value: Go=%s Java=%s", path, jsonValue(goValue), jsonValue(javaValue)))
	}
}

func jsonValue(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(raw)
}
