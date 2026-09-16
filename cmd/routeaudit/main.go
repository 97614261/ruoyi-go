// Command routeaudit compares Java controller mappings with the Go router and
// writes a reproducible route, permission, and probe-coverage matrix.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type route struct {
	Method     string
	Path       string
	Permission string
	Handler    string
	Source     string
}

type routePair struct {
	Key  string
	Go   *route
	Java *route
}

var (
	goGroupPattern     = regexp.MustCompile(`(?m)(\w+)\s*:=\s*(\w+)\.Group\(\s*"([^"]*)"`)
	goRoutePattern     = regexp.MustCompile(`(?m)(\w+)\.(GET|POST|PUT|DELETE)\(\s*"([^"]*)"`)
	goPermPattern      = regexp.MustCompile(`HasPermission\("([^"]+)"\)`)
	goRolePattern      = regexp.MustCompile(`HasRole\("([^"]+)"\)`)
	goAdminRolePattern = regexp.MustCompile(`HasRole\(model\.AdminRoleKey\)`)
	goHandlerPattern   = regexp.MustCompile(`handler\.(\w+)`)

	javaClassPattern   = regexp.MustCompile(`(?s)\bclass\s+\w+`)
	javaMappingPattern = regexp.MustCompile(`(?s)@(GetMapping|PostMapping|PutMapping|DeleteMapping|RequestMapping)\s*(?:\((.*?)\))?`)
	javaStringPattern  = regexp.MustCompile(`"([^"]*)"`)
	javaPermPattern    = regexp.MustCompile(`hasPermi\('([^']+)'\)`)
	javaRolePattern    = regexp.MustCompile(`hasRole\('([^']+)'\)`)
	javaMethodPattern  = regexp.MustCompile(`(?s)\bpublic\s+[\w<>, ?\[\].]+\s+(\w+)\s*\(`)
	pathParamPattern   = regexp.MustCompile(`\{[^/}]+\}`)
)

func main() {
	var goRouter, javaRoot, output string
	flag.StringVar(&goRouter, "go-router", "internal/router/router.go", "Go router source")
	flag.StringVar(&javaRoot, "java-root", "../RuoYi-Vue-master", "Java RuoYi source root")
	flag.StringVar(&output, "out", "docs/API_ROUTE_COVERAGE_CURRENT.md", "Markdown output path")
	flag.Parse()

	goRoutes, err := parseGoRouter(goRouter)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	javaRoutes, excluded, err := parseJavaControllers(javaRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(output, []byte(buildReport(goRoutes, javaRoutes, excluded)), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("route audit written: %s (go=%d java=%d)\n", output, len(goRoutes), len(javaRoutes))
}

func parseGoRouter(path string) ([]route, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(raw)
	groups := map[string]string{"r": "", "g": "", "authed": ""}
	for _, match := range goGroupPattern.FindAllStringSubmatch(text, -1) {
		groups[match[1]] = joinPath(groups[match[2]], match[3])
	}
	matches := goRoutePattern.FindAllStringSubmatchIndex(text, -1)
	routes := make([]route, 0, len(matches))
	for index, match := range matches {
		end := len(text)
		if index+1 < len(matches) {
			end = matches[index+1][0]
		}
		segment := text[match[0]:end]
		variable := text[match[2]:match[3]]
		permission, handler := "", ""
		if found := goPermPattern.FindStringSubmatch(segment); len(found) > 0 {
			permission = found[1]
		} else if found := goRolePattern.FindStringSubmatch(segment); len(found) > 0 {
			permission = "role:" + found[1]
		} else if goAdminRolePattern.MatchString(segment) {
			permission = "role:admin"
		}
		if found := goHandlerPattern.FindStringSubmatch(segment); len(found) > 0 {
			handler = found[1]
		}
		routes = append(routes, route{
			Method:     text[match[4]:match[5]],
			Path:       normalizePath(joinPath(groups[variable], text[match[6]:match[7]])),
			Permission: permission, Handler: handler, Source: filepath.ToSlash(path),
		})
	}
	return deduplicate(routes), nil
}

func parseJavaControllers(root string) ([]route, []string, error) {
	var routes []route
	var excluded []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "Controller.java") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(raw)
		if strings.Contains(text, `@Profile("dev")`) || strings.Contains(text, `@Profile("test")`) {
			excluded = append(excluded, filepath.ToSlash(path)+"（仅开发/测试 Profile）")
			return nil
		}
		for _, item := range parseJavaController(text, filepath.ToSlash(path)) {
			if item.Path == "/test" || strings.HasPrefix(item.Path, "/test/") {
				excluded = append(excluded, item.Method+" "+item.Path+"（Swagger 内存演示接口）")
				continue
			}
			routes = append(routes, item)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	// Spring Security registers this endpoint outside controller annotations.
	routes = append(routes, route{Method: "POST", Path: "/logout", Handler: "LogoutSuccessHandlerImpl", Source: "ruoyi-framework/.../SecurityConfig.java"})
	sort.Strings(excluded)
	return deduplicate(routes), excluded, nil
}

func parseJavaController(text, source string) []route {
	classLocation := javaClassPattern.FindStringIndex(text)
	if classLocation == nil {
		return nil
	}
	base := ""
	for _, match := range javaMappingPattern.FindAllStringSubmatch(text[:classLocation[0]], -1) {
		if match[1] == "RequestMapping" {
			if paths := javaPaths(match[2]); len(paths) > 0 {
				base = paths[0]
			}
		}
	}
	body := text[classLocation[1]:]
	matches := javaMappingPattern.FindAllStringSubmatchIndex(body, -1)
	result := make([]route, 0, len(matches))
	previousEnd := 0
	for index, match := range matches {
		annotation := body[match[2]:match[3]]
		args := ""
		if match[4] >= 0 {
			args = body[match[4]:match[5]]
		}
		method := javaHTTPMethod(annotation, args)
		if method == "" {
			previousEnd = match[1]
			continue
		}
		permission := ""
		if found := javaPermPattern.FindStringSubmatch(body[previousEnd:match[0]]); len(found) > 0 {
			permission = found[1]
		} else if found := javaRolePattern.FindStringSubmatch(body[previousEnd:match[0]]); len(found) > 0 {
			permission = "role:" + found[1]
		}
		next := len(body)
		if index+1 < len(matches) {
			next = matches[index+1][0]
		}
		handler := ""
		if found := javaMethodPattern.FindStringSubmatch(body[match[1]:next]); len(found) > 0 {
			handler = found[1]
		}
		paths := javaPaths(args)
		if len(paths) == 0 {
			paths = []string{""}
		}
		for _, pathPart := range paths {
			result = append(result, route{Method: method, Path: normalizePath(joinPath(base, pathPart)), Permission: permission, Handler: handler, Source: source})
		}
		previousEnd = match[1]
	}
	return result
}

func javaHTTPMethod(annotation, args string) string {
	switch annotation {
	case "GetMapping":
		return "GET"
	case "PostMapping":
		return "POST"
	case "PutMapping":
		return "PUT"
	case "DeleteMapping":
		return "DELETE"
	case "RequestMapping":
		for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
			if strings.Contains(args, "RequestMethod."+method) {
				return method
			}
		}
	}
	return ""
}

func javaPaths(args string) []string {
	matches := javaStringPattern.FindAllStringSubmatch(args, -1)
	paths := make([]string, 0, len(matches))
	for _, match := range matches {
		paths = append(paths, match[1])
	}
	return paths
}

func joinPath(base, child string) string {
	if child == "" {
		if base == "" {
			return "/"
		}
		return base
	}
	trailing := strings.HasSuffix(child, "/")
	joined := strings.TrimRight(base, "/") + "/" + strings.TrimLeft(child, "/")
	if trailing && !strings.HasSuffix(joined, "/") {
		joined += "/"
	}
	return joined
}

func normalizePath(path string) string {
	path = strings.ReplaceAll(path, "//", "/")
	segments := strings.Split(path, "/")
	for index, segment := range segments {
		if strings.HasPrefix(segment, ":") {
			segments[index] = "{" + strings.TrimPrefix(segment, ":") + "}"
		}
	}
	if path == "" {
		return "/"
	}
	result := strings.Join(segments, "/")
	if len(result) > 1 {
		result = strings.TrimSuffix(result, "/")
	}
	return result
}

func routeKey(item route) string {
	return item.Method + " " + pathParamPattern.ReplaceAllString(item.Path, "{}")
}

func deduplicate(routes []route) []route {
	byKey := make(map[string]route, len(routes))
	for _, item := range routes {
		byKey[routeKey(item)] = item
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]route, 0, len(keys))
	for _, key := range keys {
		result = append(result, byKey[key])
	}
	return result
}

func pairRoutes(goRoutes, javaRoutes []route) []routePair {
	pairs := make(map[string]*routePair, len(goRoutes)+len(javaRoutes))
	for index := range goRoutes {
		item := &goRoutes[index]
		key := routeKey(*item)
		pairs[key] = &routePair{Key: key, Go: item}
	}
	for index := range javaRoutes {
		item := &javaRoutes[index]
		key := routeKey(*item)
		if pairs[key] == nil {
			pairs[key] = &routePair{Key: key}
		}
		pairs[key].Java = item
	}
	keys := make([]string, 0, len(pairs))
	for key := range pairs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]routePair, 0, len(keys))
	for _, key := range keys {
		result = append(result, *pairs[key])
	}
	return result
}

func buildReport(goRoutes, javaRoutes []route, excluded []string) string {
	pairs := pairRoutes(goRoutes, javaRoutes)
	matched, permissionMatched, permissionDifferent, covered := 0, 0, 0, 0
	for _, pair := range pairs {
		if pair.Go != nil && pair.Java != nil {
			matched++
			if pair.Go.Permission == pair.Java.Permission {
				permissionMatched++
			} else {
				permissionDifferent++
			}
			if probeEvidence(pair.Key) != "" {
				covered++
			}
		}
	}
	coveragePercent := float64(0)
	if matched > 0 {
		coveragePercent = float64(covered) * 100 / float64(matched)
	}
	var out strings.Builder
	fmt.Fprintf(&out, "# 接口路由与对拍覆盖清单\n\n")
	fmt.Fprintf(&out, "生成命令：`go run ./cmd/routeaudit -java-root ../RuoYi-Vue-master`\n\n")
	fmt.Fprintf(&out, "## 汇总\n\n| 指标 | 数量 |\n|---|---:|\n")
	fmt.Fprintf(&out, "| Go 路由 | %d |\n| Java 路由（排除 Swagger 内存演示接口和 dev/test Profile） | %d |\n", len(goRoutes), len(javaRoutes))
	fmt.Fprintf(&out, "| 方法 + 结构化路径匹配 | %d |\n| 权限标识匹配 | %d |\n| 权限标识差异 | %d |\n", matched, permissionMatched, permissionDifferent)
	fmt.Fprintf(&out, "| 已纳入自动双端探针的匹配路由 | %d/%d（%.2f%%） |\n\n", covered, matched, coveragePercent)
	fmt.Fprintf(&out, "> “已纳入探针”只表示有自动执行入口；是否通过以对应实跑日志为准。\n")
	fmt.Fprintf(&out, "> 部分破坏性写路由只验证未登录拒绝；路由匹配数量不能解读为全部成功副作用已对拍。\n")
	fmt.Fprintf(&out, "> 路径参数名会统一为 `{}` 比较，避免 `{id}` / `{userId}` 这种非契约差异。\n\n")
	fmt.Fprintf(&out, "## 完整清单\n\n| 方法与路径 | Go | Java | 权限 | 自动双端证据 |\n|---|---|---|---|---|\n")
	for _, pair := range pairs {
		goState, javaState, permission := "缺失", "缺失", "—"
		if pair.Go != nil {
			goState = "有（`" + pair.Go.Handler + "`）"
		}
		if pair.Java != nil {
			javaState = "有（`" + pair.Java.Handler + "`）"
		}
		if pair.Go != nil && pair.Java != nil {
			if pair.Go.Permission == pair.Java.Permission {
				permission = valueOrNone(pair.Go.Permission) + "（一致）"
			} else {
				permission = "Go=" + valueOrNone(pair.Go.Permission) + "；Java=" + valueOrNone(pair.Java.Permission) + "（差异）"
			}
		}
		evidence := probeEvidence(pair.Key)
		if evidence == "" {
			evidence = "未纳入"
		}
		fmt.Fprintf(&out, "| `%s` | %s | %s | %s | %s |\n", pair.Key, goState, javaState, permission, evidence)
	}
	fmt.Fprintf(&out, "\n## 排除项\n\n")
	if len(excluded) == 0 {
		fmt.Fprintf(&out, "无。\n")
	} else {
		for _, item := range excluded {
			fmt.Fprintf(&out, "- `%s`\n", strings.ReplaceAll(item, "|", "\\|"))
		}
	}
	return out.String()
}

func valueOrNone(value string) string {
	if value == "" {
		return "无"
	}
	return "`" + value + "`"
}

func probeEvidence(key string) string {
	labels := make([]string, 0, 6)
	for _, candidate := range []struct {
		label string
		set   map[string]bool
	}{{"核心 GET", coreGETEvidence}, {"CRUD", crudEvidence}, {"字段校验", validationEvidence}, {"数据/功能权限", permissionEvidence}, {"文件", fileEvidence}, {"错误场景", writeEvidence}, {"补充路由场景", supplementalEvidence}} {
		if candidate.set[key] {
			labels = append(labels, candidate.label)
		}
	}
	return strings.Join(labels, "、")
}

var coreGETEvidence = routeSet("GET", []string{
	"/captchaImage", "/getInfo", "/getRouters", "/system/user", "/system/user/list", "/system/user/deptTree",
	"/system/role/list", "/system/role/optionselect", "/system/role/deptTree/{}", "/system/menu/roleMenuTreeselect/{}",
	"/system/post/list", "/system/post/optionselect", "/system/dept/list", "/system/menu/list", "/system/menu/treeselect",
	"/system/config/list", "/system/config/configKey/{}", "/system/dict/type/list", "/system/dict/type/optionselect",
	"/system/dict/data/list", "/system/dict/data/type/{}", "/system/notice/list", "/system/notice/listTop",
	"/monitor/job/list", "/monitor/jobLog/list", "/system/user/{}", "/system/role/{}", "/system/post/{}",
	"/system/dept/{}", "/system/menu/{}", "/system/config/{}", "/system/dict/type/{}", "/system/dict/data/{}",
	"/system/notice/{}", "/monitor/job/{}",
})

var crudEvidence = func() map[string]bool {
	result := map[string]bool{}
	for _, base := range []string{"/system/user", "/system/role", "/system/dept", "/system/menu", "/system/post", "/system/config", "/system/dict/type", "/system/dict/data", "/system/notice", "/monitor/job"} {
		result["POST "+base] = true
		result["PUT "+base] = true
		result["GET "+base+"/{}"] = true
		result["DELETE "+base+"/{}"] = true
	}
	return result
}()

var validationEvidence = routeSet("POST", []string{"/system/user", "/system/role", "/system/dept", "/system/menu", "/system/post", "/system/config", "/system/dict/type", "/system/dict/data", "/system/notice", "/monitor/job"})
var permissionEvidence = routeSet("GET", []string{"/system/user/list", "/system/role/list", "/system/dept/list"})
var fileEvidence = map[string]bool{"POST /common/upload": true, "POST /common/uploads": true, "POST /system/user/importTemplate": true, "POST /system/user/importData": true}
var writeEvidence = map[string]bool{"POST /register": true, "POST /common/upload": true, "POST /common/uploads": true, "POST /system/user/importData": true, "POST /system/user/profile/avatar": true, "DELETE /monitor/cache/clearCacheName/{}": true, "DELETE /monitor/cache/clearCacheKey/{}": true}

var supplementalEvidence = routeSetPairs([]string{
	"DELETE /monitor/cache/clearCacheAll", "DELETE /monitor/jobLog/clean", "DELETE /monitor/jobLog/{}",
	"DELETE /monitor/logininfor/clean", "DELETE /monitor/logininfor/{}", "DELETE /monitor/online/{}",
	"DELETE /monitor/operlog/clean", "DELETE /monitor/operlog/{}", "DELETE /system/config/refreshCache",
	"DELETE /system/dict/type/refreshCache", "GET /common/download", "GET /common/download/resource",
	"GET /monitor/cache", "GET /monitor/cache/getKeys/{}", "GET /monitor/cache/getNames",
	"GET /monitor/cache/getValue/{}/{}", "GET /monitor/jobLog/{}", "GET /monitor/logininfor/list",
	"GET /monitor/logininfor/unlock/{}", "GET /monitor/online/list", "GET /monitor/operlog/list",
	"GET /monitor/server", "GET /system/dept/list/exclude/{}", "GET /system/notice/readUsers/list",
	"GET /system/role/authUser/allocatedList", "GET /system/role/authUser/unallocatedList",
	"GET /system/user/authRole/{}", "GET /system/user/profile", "POST /login", "POST /logout",
	"POST /monitor/job/export", "POST /monitor/jobLog/export", "POST /monitor/logininfor/export",
	"POST /monitor/operlog/export", "POST /system/config/export", "POST /system/dict/data/export",
	"POST /system/dict/type/export", "POST /system/notice/markRead", "POST /system/notice/markReadAll",
	"POST /system/post/export", "POST /system/role/export", "POST /system/user/export", "POST /unlockscreen",
	"PUT /monitor/job/changeStatus", "PUT /monitor/job/run", "PUT /system/dept/updateSort",
	"PUT /system/menu/updateSort", "PUT /system/role/authUser/cancel", "PUT /system/role/authUser/cancelAll",
	"PUT /system/role/authUser/selectAll", "PUT /system/role/changeStatus", "PUT /system/role/dataScope",
	"PUT /system/user/authRole", "PUT /system/user/changeStatus", "PUT /system/user/profile",
	"PUT /system/user/profile/updatePwd", "PUT /system/user/resetPwd",
})

func routeSet(method string, paths []string) map[string]bool {
	result := make(map[string]bool, len(paths))
	for _, path := range paths {
		result[method+" "+path] = true
	}
	return result
}

func routeSetPairs(keys []string) map[string]bool {
	result := make(map[string]bool, len(keys))
	for _, key := range keys {
		result[key] = true
	}
	return result
}
