package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

const permissionProbeTotal = 18

type permissionFixture struct {
	admin         *apiClient
	prefix        string
	password      string
	deptA         int64
	deptChild     int64
	deptPeer      int64
	actorRole     int64
	roleA         int64
	roleChild     int64
	rolePeer      int64
	actor         int64
	userA         int64
	userChild     int64
	userPeer      int64
	actorName     string
	userAName     string
	userChildName string
	userPeerName  string
	cleaned       bool
}

type permissionExpectation struct {
	name    string
	scope   string
	deptIDs []int64
	users   []int64
	roles   []int64
	depts   []int64
}

func comparePermissionProbes(goAdmin, javaAdmin *apiClient, password string, maxDiffs int) (matched, total int) {
	total = permissionProbeTotal
	seed := strconv.FormatInt(time.Now().UnixNano(), 36)
	if len(seed) > 6 {
		seed = seed[len(seed)-6:]
	}
	fixture := &permissionFixture{
		admin: goAdmin,
		// Java login rejects usernames longer than 20 before querying the user.
		prefix:   "zcs_" + seed,
		password: "test123456",
	}
	defer func() {
		if err := fixture.cleanup(); err != nil {
			total++
			fmt.Printf("PERMISSION_PROBE_DIFF 清理一次性数据\n  %v\n", err)
		}
	}()

	if err := fixture.setup(); err != nil {
		fmt.Printf("PERMISSION_PROBE_DIFF 初始化一次性数据\n  %v\n", err)
		return 0, total
	}

	expectations := []permissionExpectation{
		{name: "全部数据", scope: "1", users: []int64{fixture.actor, fixture.userA, fixture.userChild, fixture.userPeer}, roles: []int64{fixture.actorRole, fixture.roleA, fixture.roleChild, fixture.rolePeer}, depts: []int64{fixture.deptA, fixture.deptChild, fixture.deptPeer}},
		{name: "自定义（仅平级部门）", scope: "2", deptIDs: []int64{fixture.deptPeer}, users: []int64{fixture.userPeer}, roles: []int64{fixture.rolePeer}, depts: []int64{fixture.deptPeer}},
		{name: "本部门", scope: "3", users: []int64{fixture.actor, fixture.userA}, roles: []int64{fixture.actorRole, fixture.roleA}, depts: []int64{fixture.deptA}},
		{name: "本部门及以下", scope: "4", users: []int64{fixture.actor, fixture.userA, fixture.userChild}, roles: []int64{fixture.actorRole, fixture.roleA, fixture.roleChild}, depts: []int64{fixture.deptA, fixture.deptChild}},
		{name: "仅本人", scope: "5", users: []int64{fixture.actor}},
	}

	for _, expectation := range expectations {
		if err := fixture.setScope(expectation.scope, expectation.deptIDs); err != nil {
			fmt.Printf("PERMISSION_PROBE_DIFF %s：设置数据范围\n  %v\n", expectation.name, err)
			continue
		}
		goUser, err := newScopedClient(goAdmin, fixture.actorName, fixture.password)
		if err != nil {
			fmt.Printf("PERMISSION_PROBE_DIFF %s：Go 登录\n  %v\n", expectation.name, err)
			continue
		}
		javaUser, err := newScopedClient(javaAdmin, fixture.actorName, fixture.password)
		if err != nil {
			_ = goUser.cleanup()
			fmt.Printf("PERMISSION_PROBE_DIFF %s：Java 登录\n  %v\n", expectation.name, err)
			continue
		}

		paths := []struct {
			name      string
			path      string
			container string
			idField   string
			expected  []int64
		}{
			{name: "用户列表", path: "/system/user/list?pageNum=1&pageSize=100&userName=" + url.QueryEscape(fixture.prefix), container: "rows", idField: "userId", expected: expectation.users},
			{name: "角色列表", path: "/system/role/list?pageNum=1&pageSize=100&roleName=" + url.QueryEscape(fixture.prefix), container: "rows", idField: "roleId", expected: expectation.roles},
			{name: "部门列表", path: "/system/dept/list?deptName=" + url.QueryEscape(fixture.prefix), container: "data", idField: "deptId", expected: expectation.depts},
		}
		for _, item := range paths {
			goResponse, goErr := goUser.get(item.path)
			javaResponse, javaErr := javaUser.get(item.path)
			if permissionResultMatches(goResponse, goErr, javaResponse, javaErr, item.container, item.idField, item.expected, maxDiffs, expectation.name+"/"+item.name) {
				matched++
			}
		}
		if err := errors.Join(goUser.cleanup(), javaUser.cleanup()); err != nil {
			total++
			fmt.Printf("PERMISSION_PROBE_DIFF %s：清理登录会话\n  %v\n", expectation.name, err)
		}
	}

	goNoPerm, goErr := newScopedClient(goAdmin, fixture.userAName, fixture.password)
	javaNoPerm, javaErr := newScopedClient(javaAdmin, fixture.userAName, fixture.password)
	if goErr != nil || javaErr != nil {
		fmt.Printf("PERMISSION_PROBE_DIFF 无功能权限账号登录\n  go=%v java=%v\n", goErr, javaErr)
		return matched, total
	}
	for _, item := range []struct{ name, path string }{
		{name: "用户列表", path: "/system/user/list?pageNum=1&pageSize=10"},
		{name: "角色列表", path: "/system/role/list?pageNum=1&pageSize=10"},
		{name: "部门列表", path: "/system/dept/list"},
	} {
		goResponse, goRequestErr := goNoPerm.get(item.path)
		javaResponse, javaRequestErr := javaNoPerm.get(item.path)
		if probeResponsesMatch("无功能权限/"+item.name, "PERMISSION_PROBE", goResponse, goRequestErr, javaResponse, javaRequestErr, maxDiffs) {
			matched++
		}
	}
	if err := errors.Join(goNoPerm.cleanup(), javaNoPerm.cleanup()); err != nil {
		total++
		fmt.Printf("PERMISSION_PROBE_DIFF 无功能权限账号：清理登录会话\n  %v\n", err)
	}
	return matched, total
}

func newScopedClient(template *apiClient, username, password string) (*apiClient, error) {
	client := &apiClient{
		base: template.base, http: template.http, redis: template.redis,
		redisKeys: make(map[string]struct{}),
	}
	var err error
	client.token, err = login(client, username, password)
	if err != nil {
		_ = client.cleanup()
		return nil, err
	}
	client.sessionKey, err = loginSessionKey(client.token)
	if err != nil {
		_ = client.cleanup()
		return nil, err
	}
	return client, nil
}

func permissionResultMatches(goResponse apiResponse, goErr error, javaResponse apiResponse, javaErr error, container, idField string, expected []int64, maxDiffs int, name string) bool {
	if goErr != nil || javaErr != nil {
		fmt.Printf("PERMISSION_PROBE_DIFF %s\n  request error: go=%v java=%v\n", name, goErr, javaErr)
		return false
	}
	diffs := compareResponses(goResponse, javaResponse)
	goIDs, goIDsErr := responseIDSet(goResponse, container, idField)
	javaIDs, javaIDsErr := responseIDSet(javaResponse, container, idField)
	want := sortedIDs(expected)
	if len(diffs) == 0 && goIDsErr == nil && javaIDsErr == nil && equalInt64s(goIDs, want) && equalInt64s(javaIDs, want) {
		fmt.Printf("PERMISSION_PROBE_MATCH %s ids=%v\n", name, want)
		return true
	}
	fmt.Printf("PERMISSION_PROBE_DIFF %s\n", name)
	for _, diff := range diffs[:min(maxDiffs, len(diffs))] {
		fmt.Printf("  %s\n", diff)
	}
	if goIDsErr != nil || javaIDsErr != nil || !equalInt64s(goIDs, want) || !equalInt64s(javaIDs, want) {
		fmt.Printf("  semantic IDs: want=%v go=%v (err=%v) java=%v (err=%v)\n", want, goIDs, goIDsErr, javaIDs, javaIDsErr)
	}
	return false
}

func responseIDSet(response apiResponse, container, idField string) ([]int64, error) {
	items, err := probeItems(response, container)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		id, ok := probeItemID(item, idField)
		if !ok {
			return nil, fmt.Errorf("invalid %s in %s", idField, jsonValue(item))
		}
		parsed, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return nil, err
		}
		ids = append(ids, parsed)
	}
	return sortedIDs(ids), nil
}

func sortedIDs(ids []int64) []int64 {
	result := append([]int64(nil), ids...)
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func equalInt64s(left, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (fixture *permissionFixture) setup() error {
	rootID, err := rootDeptIDFromList(fixture.admin)
	if err != nil {
		return err
	}
	menuIDs, err := allMenuIDsFromAPI(fixture.admin)
	if err != nil {
		return err
	}
	fixture.deptA, err = fixture.create("/system/dept", "/system/dept/list", "data", "deptName", "deptId", map[string]any{
		"parentId": rootID, "deptName": fixture.prefix + "_dept_a", "orderNum": 91, "leader": "scope", "status": "0",
	})
	if err != nil {
		return err
	}
	fixture.deptChild, err = fixture.create("/system/dept", "/system/dept/list", "data", "deptName", "deptId", map[string]any{
		"parentId": fixture.deptA, "deptName": fixture.prefix + "_dept_child", "orderNum": 92, "leader": "scope", "status": "0",
	})
	if err != nil {
		return err
	}
	fixture.deptPeer, err = fixture.create("/system/dept", "/system/dept/list", "data", "deptName", "deptId", map[string]any{
		"parentId": rootID, "deptName": fixture.prefix + "_dept_peer", "orderNum": 93, "leader": "scope", "status": "0",
	})
	if err != nil {
		return err
	}

	createRole := func(suffix string, roleSort int, menus []int64) (int64, error) {
		name := fixture.prefix + "_" + suffix
		return fixture.create("/system/role", "/system/role/list", "rows", "roleName", "roleId", map[string]any{
			"roleName": name, "roleKey": name, "roleSort": roleSort, "status": "0", "menuIds": menus,
			"menuCheckStrictly": false, "deptCheckStrictly": false, "remark": "permission contract probe",
		})
	}
	fixture.actorRole, err = createRole("actor", 91, menuIDs)
	if err != nil {
		return err
	}
	fixture.roleA, err = createRole("role_a", 92, []int64{})
	if err != nil {
		return err
	}
	fixture.roleChild, err = createRole("role_child", 93, []int64{})
	if err != nil {
		return err
	}
	fixture.rolePeer, err = createRole("role_peer", 94, []int64{})
	if err != nil {
		return err
	}

	createUser := func(suffix string, deptID, roleID int64) (int64, string, error) {
		name := fixture.prefix + "_" + suffix
		id, createErr := fixture.create("/system/user", "/system/user/list", "rows", "userName", "userId", map[string]any{
			"userName": name, "nickName": name, "password": fixture.password, "deptId": deptID,
			"email": name + "@example.com", "phonenumber": "", "sex": "0", "status": "0",
			"postIds": []int64{}, "roleIds": []int64{roleID}, "remark": "permission contract probe",
		})
		return id, name, createErr
	}
	fixture.actor, fixture.actorName, err = createUser("actor", fixture.deptA, fixture.actorRole)
	if err != nil {
		return err
	}
	fixture.userA, fixture.userAName, err = createUser("a", fixture.deptA, fixture.roleA)
	if err != nil {
		return err
	}
	fixture.userChild, fixture.userChildName, err = createUser("child", fixture.deptChild, fixture.roleChild)
	if err != nil {
		return err
	}
	fixture.userPeer, fixture.userPeerName, err = createUser("peer", fixture.deptPeer, fixture.rolePeer)
	return err
}

func (fixture *permissionFixture) create(basePath, listPath, container, lookupField, idField string, body map[string]any) (int64, error) {
	response, err := requestJSON(fixture.admin, http.MethodPost, basePath, body)
	if err != nil {
		return 0, err
	}
	if responseCode(response) != 200 {
		return 0, fmt.Errorf("POST %s returned %s", basePath, jsonValue(response.body))
	}
	lookup := fmt.Sprint(body[lookupField])
	query := url.Values{lookupField: {lookup}}
	if container == "rows" {
		query.Set("pageNum", "1")
		query.Set("pageSize", "100")
	}
	list, err := fixture.admin.get(listPath + "?" + query.Encode())
	if err != nil {
		return 0, err
	}
	items, err := probeItems(list, container)
	if err != nil {
		return 0, err
	}
	for _, item := range items {
		if fmt.Sprint(item[lookupField]) != lookup {
			continue
		}
		id, ok := probeItemID(item, idField)
		if !ok {
			return 0, fmt.Errorf("invalid %s in %s", idField, jsonValue(item))
		}
		return strconv.ParseInt(id, 10, 64)
	}
	return 0, fmt.Errorf("created %s %q was not found", lookupField, lookup)
}

func (fixture *permissionFixture) setScope(scope string, deptIDs []int64) error {
	if deptIDs == nil {
		deptIDs = []int64{}
	}
	response, err := requestJSON(fixture.admin, http.MethodPut, "/system/role/dataScope", map[string]any{
		"roleId": fixture.actorRole, "dataScope": scope, "deptIds": deptIDs, "deptCheckStrictly": false,
	})
	if err != nil {
		return err
	}
	if responseCode(response) != 200 {
		return fmt.Errorf("dataScope=%s returned %s", scope, jsonValue(response.body))
	}
	return nil
}

func (fixture *permissionFixture) cleanup() error {
	if fixture.cleaned {
		return nil
	}
	fixture.cleaned = true
	var cleanupErrors []error
	remove := func(path string, ids ...int64) {
		for _, id := range ids {
			if id == 0 {
				continue
			}
			response, err := fixture.admin.request(http.MethodDelete, path+strconv.FormatInt(id, 10), "", nil)
			if err != nil || responseCode(response) != 200 {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("DELETE %s%d: response=%s err=%v", path, id, jsonValue(response.body), err))
			}
		}
	}
	remove("/system/user/", fixture.actor, fixture.userA, fixture.userChild, fixture.userPeer)
	remove("/system/role/", fixture.actorRole, fixture.roleA, fixture.roleChild, fixture.rolePeer)
	remove("/system/dept/", fixture.deptChild, fixture.deptA, fixture.deptPeer)
	return errors.Join(cleanupErrors...)
}

func rootDeptIDFromList(client *apiClient) (int64, error) {
	response, err := client.get("/system/dept/list")
	if err != nil {
		return 0, err
	}
	items, err := probeItems(response, "data")
	if err != nil {
		return 0, err
	}
	for _, item := range items {
		parent, ok := item["parentId"].(float64)
		if !ok || parent != 0 {
			continue
		}
		id, ok := probeItemID(item, "deptId")
		if !ok {
			continue
		}
		return strconv.ParseInt(id, 10, 64)
	}
	return 0, fmt.Errorf("no root department found")
}

func allMenuIDsFromAPI(client *apiClient) ([]int64, error) {
	response, err := client.get("/system/menu/list")
	if err != nil {
		return nil, err
	}
	items, err := probeItems(response, "data")
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		id, ok := probeItemID(item, "menuId")
		if !ok {
			continue
		}
		parsed, err := strconv.ParseInt(id, 10, 64)
		if err == nil {
			ids = append(ids, parsed)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("menu list is empty")
	}
	return ids, nil
}

func probeResponsesMatch(name, prefix string, goResponse apiResponse, goErr error, javaResponse apiResponse, javaErr error, maxDiffs int) bool {
	if goErr != nil || javaErr != nil {
		fmt.Printf("%s_DIFF %s\n  request error: go=%v java=%v\n", prefix, name, goErr, javaErr)
		return false
	}
	diffs := compareResponses(goResponse, javaResponse)
	if len(diffs) == 0 {
		fmt.Printf("%s_MATCH %s\n", prefix, name)
		return true
	}
	fmt.Printf("%s_DIFF %s\n", prefix, name)
	for _, diff := range diffs[:min(maxDiffs, len(diffs))] {
		fmt.Printf("  %s\n", diff)
	}
	return false
}
