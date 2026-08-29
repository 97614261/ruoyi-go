// Package apitest 是接口层的自动化测试。
//
// 它把整个应用装配起来（真实 MySQL + Redis），用 httptest 直接打路由，
// 不经过网络。等价于"自动点一遍"，但比手工点快几个数量级，且可重复。
//
// 运行：
//
//	go test ./test/... -v
//
// 默认读 ../configs/application.yml。想指向独立的测试库就设环境变量：
//
//	$env:RUOYI_TEST_CONFIG="../configs/application.test.yml"
//
// 【注意】测试会往库里写数据。所有用例都用 t.Cleanup 删掉自己造的数据，
// 且命名统一带 testPrefix，万一残留也能一眼认出来手工清理。
package apitest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ruoyi-go/internal/config"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/router"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/redisx"
	"ruoyi-go/pkg/validate"
)

// testPrefix 所有测试数据的名称前缀，便于识别和清理。
const testPrefix = "zz_test_"

var (
	engine     *gin.Engine
	appConfig  *config.Config
	adminToken string
)

func TestMain(m *testing.M) {
	if err := setup(); err != nil {
		fmt.Fprintf(os.Stderr, "测试初始化失败: %v\n", err)
		fmt.Fprintln(os.Stderr, "请确认 MySQL 和 Redis 已启动，且 configs/application.yml 配置正确")
		os.Exit(1)
	}
	code := m.Run()
	purgeTestData()
	if err := purgeTestRowsPhysical(); err != nil {
		fmt.Fprintf(os.Stderr, "测试收尾清理失败: %v\n", err)
		code = 1
	}
	if err := purgeRedisArtifacts(); err != nil {
		fmt.Fprintf(os.Stderr, "Redis 测试数据收尾清理失败: %v\n", err)
		code = 1
	}
	teardown()
	os.Exit(code)
}

func setup() error {
	gin.SetMode(gin.TestMode)

	cfgPath := os.Getenv("RUOYI_TEST_CONFIG")
	if cfgPath == "" {
		cfgPath = "../configs/application.yml"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	appConfig = cfg

	if err := repository.Init(cfg.MySQL); err != nil {
		return err
	}
	if err := purgeTestRowsPhysical(); err != nil {
		return fmt.Errorf("清理历史测试数据失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := redisx.Init(ctx, redisx.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
		PoolSize: cfg.Redis.PoolSize,
	}); err != nil {
		return err
	}

	service.InitToken(cfg.JWT)
	service.InitCaptcha(cfg.Captcha.Type)
	if err := validate.Register(); err != nil {
		return err
	}

	engine = router.New(cfg)

	// 后续所有用例共用管理员会话
	adminToken, err = loginAs("admin", "admin123")
	if err != nil {
		return fmt.Errorf("管理员登录失败: %w", err)
	}

	// 上一轮如果中途失败，t.Cleanup 可能没跑完，残留数据会让这一轮
	// 撞唯一性约束。开跑前先清干净，保证每次运行都是幂等的。
	purgeTestData()
	if err := purgeRateLimit(); err != nil {
		return fmt.Errorf("清理测试限流键失败: %w", err)
	}
	return nil
}

// purgeRateLimit 清掉限流计数器。
//
// 【为什么必须清】httptest 的请求全部来自同一个 clientIP，
// 而登录限流是按 IP 计数的。一轮测试里 loginAs 会被调几十次
// （每个数据权限用例都要换账号登录），跑到后面就会撞上限流，
// 报的是"访问过于频繁"而不是被测的那个错误 —— 排查时极具误导性。
//
// 不改限流阈值来迁就测试：那是生产配置，不该被测试牵着走。
func purgeRateLimit() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	keys := []string{
		redisx.RateLimitKey("captcha:192.0.2.1"),
		redisx.RateLimitKey("login:192.0.2.1"),
		redisx.RateLimitKey("register:192.0.2.1"),
	}
	return deleteRedisKeys(ctx, keys)
}

// purgeTestData 删除所有以 testPrefix 开头的测试数据。
//
// 【为什么不能只靠 t.Cleanup】
// Cleanup 是 LIFO 的，但树形数据（部门、菜单）的父子关系会在用例中被修改。
// 比如"把 A 挪到 C 下"之后，Cleanup 仍按注册顺序先删 C —— 而 C 此时有子节点 A，
// 删除被拒，C 就永久残留，下一轮新增同名部门直接失败。
//
// 所以用多轮删除：每轮删掉当前能删的，直到一轮下来没有任何进展为止。
// 这样叶子先被删掉，父节点在下一轮就能删了。
func purgeTestData() {
	// 平铺型数据，一轮就能删完
	purgeFlat("/system/post/list?pageSize=100&postName="+testPrefix, "postId", "postName", "/system/post/")
	purgeFlat("/system/role/list?pageSize=100&roleName="+testPrefix, "roleId", "roleName", "/system/role/")
	purgeFlat("/system/user/list?pageSize=100&userName="+testPrefix, "userId", "userName", "/system/user/")
	purgeFlat("/system/config/list?pageSize=100&configName="+testPrefix, "configId", "configName", "/system/config/")
	purgeFlat("/system/notice/list?pageSize=100&noticeTitle="+testPrefix, "noticeId", "noticeTitle", "/system/notice/")
	purgeFlat("/system/dict/data/list?pageSize=100&dictLabel="+testPrefix, "dictCode", "dictLabel", "/system/dict/data/")
	purgeFlat("/system/dict/type/list?pageSize=100&dictName="+testPrefix, "dictId", "dictName", "/system/dict/type/")

	// 调度日志要先于任务清理：日志是任务跑出来的，留着也只是垃圾数据
	purgeFlat("/monitor/jobLog/list?pageSize=100&jobName="+testPrefix, "jobLogId", "jobName", "/monitor/jobLog/")
	purgeFlat("/monitor/job/list?pageSize=100&jobName="+testPrefix, "jobId", "jobName", "/monitor/job/")

	// 树形数据，多轮删除
	purgeTree("/system/dept/list", "deptId", "deptName", "/system/dept/")
	purgeTree("/system/menu/list", "menuId", "menuName", "/system/menu/")
}

// purgeFlat 清理分页列表里的测试数据。
func purgeFlat(listPath, idKey, nameKey, deletePrefix string) {
	for round := 0; round < 100; round++ {
		items := collectTestItems(listPath, "rows", nameKey)
		if len(items) == 0 {
			return
		}
		for _, item := range items {
			if id, ok := numericID(item[idKey]); ok {
				request(http.MethodDelete, deletePrefix+id, adminToken, nil)
			}
		}
	}
}

// purgeTree 反复删除，直到一轮下来数量不再减少。
func purgeTree(listPath, idKey, nameKey, deletePrefix string) {
	previous := -1
	for round := 0; round < 10; round++ {
		items := collectTestItems(listPath, "data", nameKey)
		if len(items) == 0 || len(items) == previous {
			return // 删完了，或者卡住了（有非测试数据挂在下面）
		}
		previous = len(items)
		for _, item := range items {
			if id, ok := numericID(item[idKey]); ok {
				request(http.MethodDelete, deletePrefix+id, adminToken, nil)
			}
		}
	}
}

// collectTestItems 取出列表里名字以 testPrefix 开头的记录。
func collectTestItems(listPath, container, nameKey string) []map[string]any {
	r := request(http.MethodGet, listPath, adminToken, nil)
	list, ok := r.Raw[container].([]any)
	if !ok {
		return nil
	}

	var result []map[string]any
	for _, raw := range list {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := item[nameKey].(string)
		if strings.HasPrefix(name, testPrefix) {
			result = append(result, item)
		}
	}
	return result
}

func numericID(raw any) (string, bool) {
	number, ok := raw.(float64)
	if !ok {
		return "", false
	}
	return strconv.FormatInt(int64(number), 10), true
}

// purgeTestRowsPhysical 只物理删除精确 zz_test_ 前缀的数据。
//
// 业务删除接口对用户、角色和部门使用逻辑删除。接口测试如果只调用删除接口，
// 物理表会持续膨胀，并最终让唯一性、排序和双端对拍结果漂移。这里位于 test
// 包，先删关联表、再删主表，不会进入生产代码路径。
func purgeTestRowsPhysical() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	steps := []string{
		"DELETE nr FROM sys_notice_read nr LEFT JOIN sys_notice n ON n.notice_id = nr.notice_id LEFT JOIN sys_user u ON u.user_id = nr.user_id WHERE LEFT(n.notice_title, 8) = 'zz_test_' OR LEFT(u.user_name, 8) = 'zz_test_' OR LEFT(u.nick_name, 8) = 'zz_test_'",
		"DELETE ur FROM sys_user_role ur LEFT JOIN sys_user u ON u.user_id = ur.user_id LEFT JOIN sys_role r ON r.role_id = ur.role_id WHERE LEFT(u.user_name, 8) = 'zz_test_' OR LEFT(u.nick_name, 8) = 'zz_test_' OR LEFT(r.role_name, 8) = 'zz_test_' OR LEFT(r.role_key, 8) = 'zz_test_'",
		"DELETE up FROM sys_user_post up LEFT JOIN sys_user u ON u.user_id = up.user_id LEFT JOIN sys_post p ON p.post_id = up.post_id WHERE LEFT(u.user_name, 8) = 'zz_test_' OR LEFT(u.nick_name, 8) = 'zz_test_' OR LEFT(p.post_name, 8) = 'zz_test_' OR LEFT(p.post_code, 8) = 'zz_test_'",
		"DELETE rd FROM sys_role_dept rd LEFT JOIN sys_role r ON r.role_id = rd.role_id LEFT JOIN sys_dept d ON d.dept_id = rd.dept_id WHERE LEFT(r.role_name, 8) = 'zz_test_' OR LEFT(r.role_key, 8) = 'zz_test_' OR LEFT(d.dept_name, 8) = 'zz_test_'",
		"DELETE rm FROM sys_role_menu rm LEFT JOIN sys_role r ON r.role_id = rm.role_id LEFT JOIN sys_menu m ON m.menu_id = rm.menu_id WHERE LEFT(r.role_name, 8) = 'zz_test_' OR LEFT(r.role_key, 8) = 'zz_test_' OR LEFT(m.menu_name, 8) = 'zz_test_'",
		"DELETE FROM sys_job_log WHERE LEFT(job_name, 8) = 'zz_test_' OR LEFT(job_group, 8) = 'zz_test_'",
		"DELETE FROM sys_logininfor WHERE LEFT(user_name, 8) = 'zz_test_'",
		"DELETE FROM sys_oper_log WHERE LEFT(oper_name, 8) = 'zz_test_'",
		"DELETE FROM sys_notice WHERE LEFT(notice_title, 8) = 'zz_test_'",
		"DELETE FROM sys_dict_data WHERE LEFT(dict_label, 8) = 'zz_test_' OR LEFT(dict_type, 8) = 'zz_test_'",
		"DELETE FROM sys_dict_type WHERE LEFT(dict_name, 8) = 'zz_test_' OR LEFT(dict_type, 8) = 'zz_test_'",
		"DELETE FROM sys_config WHERE LEFT(config_name, 8) = 'zz_test_' OR LEFT(config_key, 8) = 'zz_test_'",
		"DELETE FROM sys_job WHERE LEFT(job_name, 8) = 'zz_test_' OR LEFT(job_group, 8) = 'zz_test_'",
		"DELETE FROM sys_user WHERE LEFT(user_name, 8) = 'zz_test_' OR LEFT(nick_name, 8) = 'zz_test_'",
		"DELETE FROM sys_role WHERE LEFT(role_name, 8) = 'zz_test_' OR LEFT(role_key, 8) = 'zz_test_'",
		"DELETE FROM sys_post WHERE LEFT(post_name, 8) = 'zz_test_' OR LEFT(post_code, 8) = 'zz_test_'",
		"DELETE FROM sys_dept WHERE LEFT(dept_name, 8) = 'zz_test_'",
		"DELETE FROM sys_menu WHERE LEFT(menu_name, 8) = 'zz_test_'",
	}

	checks := []struct {
		name  string
		query string
	}{
		{"用户", "SELECT COUNT(*) FROM sys_user WHERE LEFT(user_name, 8) = 'zz_test_' OR LEFT(nick_name, 8) = 'zz_test_'"},
		{"角色", "SELECT COUNT(*) FROM sys_role WHERE LEFT(role_name, 8) = 'zz_test_' OR LEFT(role_key, 8) = 'zz_test_'"},
		{"岗位", "SELECT COUNT(*) FROM sys_post WHERE LEFT(post_name, 8) = 'zz_test_' OR LEFT(post_code, 8) = 'zz_test_'"},
		{"部门", "SELECT COUNT(*) FROM sys_dept WHERE LEFT(dept_name, 8) = 'zz_test_'"},
		{"菜单", "SELECT COUNT(*) FROM sys_menu WHERE LEFT(menu_name, 8) = 'zz_test_'"},
		{"参数", "SELECT COUNT(*) FROM sys_config WHERE LEFT(config_name, 8) = 'zz_test_' OR LEFT(config_key, 8) = 'zz_test_'"},
		{"字典类型", "SELECT COUNT(*) FROM sys_dict_type WHERE LEFT(dict_name, 8) = 'zz_test_' OR LEFT(dict_type, 8) = 'zz_test_'"},
		{"字典数据", "SELECT COUNT(*) FROM sys_dict_data WHERE LEFT(dict_label, 8) = 'zz_test_' OR LEFT(dict_type, 8) = 'zz_test_'"},
		{"公告", "SELECT COUNT(*) FROM sys_notice WHERE LEFT(notice_title, 8) = 'zz_test_'"},
		{"任务", "SELECT COUNT(*) FROM sys_job WHERE LEFT(job_name, 8) = 'zz_test_' OR LEFT(job_group, 8) = 'zz_test_'"},
		{"任务日志", "SELECT COUNT(*) FROM sys_job_log WHERE LEFT(job_name, 8) = 'zz_test_' OR LEFT(job_group, 8) = 'zz_test_'"},
		{"登录日志", "SELECT COUNT(*) FROM sys_logininfor WHERE LEFT(user_name, 8) = 'zz_test_'"},
		{"操作日志", "SELECT COUNT(*) FROM sys_oper_log WHERE LEFT(oper_name, 8) = 'zz_test_'"},
	}

	return repository.Transaction(ctx, func(tx *gorm.DB) error {
		for _, statement := range steps {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		for _, check := range checks {
			var remaining int64
			if err := tx.Raw(check.query).Scan(&remaining).Error; err != nil {
				return err
			}
			if remaining != 0 {
				return fmt.Errorf("%s仍残留 %d 条 %s 数据", testPrefix, remaining, check.name)
			}
		}
		return nil
	})
}

func teardown() {
	_ = repository.Close()
	_ = redisx.Close()
}

// loginAs 登录并返回 token。
//
// 验证码开启时，直接从 Redis 里读出答案 —— 测试有 Redis 权限，
// 这样不用为了跑测试去改数据库里的验证码开关（改了会影响真实环境）。
func loginAs(username, password string) (string, error) {
	ctx := context.Background()

	captchaResp := request(http.MethodGet, "/captchaImage", "", nil)
	if captchaResp.Code != 200 {
		return "", fmt.Errorf("获取验证码失败: %s", captchaResp.Msg)
	}

	body := map[string]any{"username": username, "password": password}

	if enabled, _ := captchaResp.Raw["captchaEnabled"].(bool); enabled {
		uuid, _ := captchaResp.Raw["uuid"].(string)
		answer, err := redisx.C().Get(ctx, redisx.CaptchaKey(uuid)).Result()
		if err != nil {
			return "", fmt.Errorf("读取验证码答案失败: %w", err)
		}
		body["uuid"] = uuid
		body["code"] = answer
	}

	loginResp := request(http.MethodPost, "/login", "", body)
	if loginResp.Code != 200 {
		return "", fmt.Errorf("登录失败: %s", loginResp.Msg)
	}
	token, _ := loginResp.Raw["token"].(string)
	if token == "" {
		return "", fmt.Errorf("登录响应里没有 token")
	}

	claims, err := service.ParseToken(token)
	if err != nil {
		return "", fmt.Errorf("解析测试会话失败: %w", err)
	}
	trackRedisKey(redisx.LoginTokenKey(claims.LoginUserKey))
	loginUser, err := service.GetLoginUser(ctx, token)
	if err != nil {
		return "", fmt.Errorf("读取测试会话失败: %w", err)
	}
	if loginUser != nil {
		trackRedisKey(redisx.LoginUserSessionsKey(loginUser.UserID))
		trackRedisKey(redisx.LoginUserGenerationKey(loginUser.UserID))
	}
	return token, nil
}

// testRedisKeys 只登记本轮测试能精确归属的 key，避免清理共享 Redis 时
// 误删开发人员的验证码、会话或防重复提交状态。
var (
	testRedisKeysMu sync.Mutex
	testRedisKeys   = make(map[string]struct{})
)

func trackRedisKey(key string) {
	if key == "" {
		return
	}
	testRedisKeysMu.Lock()
	testRedisKeys[key] = struct{}{}
	testRedisKeysMu.Unlock()
}

// trackRequestRedisKeys 记录验证码和防重复提交中间件可能写入的精确 key。
func trackRequestRedisKeys(method, path, token string, result response) {
	cleanPath, _, _ := strings.Cut(path, "?")
	if method == http.MethodGet && cleanPath == "/captchaImage" {
		if uuid, _ := result.Raw["uuid"].(string); uuid != "" {
			trackRedisKey(redisx.CaptchaKey(uuid))
		}
	}
	if method != http.MethodPost && method != http.MethodPut {
		return
	}
	identity := "192.0.2.1"
	if token != "" {
		identity = "Bearer " + token
	}
	sum := sha256.Sum256([]byte(identity + "|" + method + "|" + cleanPath))
	trackRedisKey(redisx.RepeatSubmitKey(hex.EncodeToString(sum[:16])))
}

// purgeRedisArtifacts 删除本轮登记的 key 和明确属于 zz_test_ 账号的密码计数，
// 并在删除后逐项验证；Redis 故障不能再被静默当成测试成功。
func purgeRedisArtifacts() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	testRedisKeysMu.Lock()
	keys := make([]string, 0, len(testRedisKeys))
	for key := range testRedisKeys {
		keys = append(keys, key)
	}
	testRedisKeys = make(map[string]struct{})
	testRedisKeysMu.Unlock()

	if err := redisx.ScanKeys(ctx, redisx.KeyPwdErrCnt+testPrefix+"*", 100, func(key string) error {
		keys = append(keys, key)
		return nil
	}); err != nil {
		return fmt.Errorf("扫描测试密码计数失败: %w", err)
	}
	keys = append(keys,
		redisx.RateLimitKey("captcha:192.0.2.1"),
		redisx.RateLimitKey("login:192.0.2.1"),
		redisx.RateLimitKey("register:192.0.2.1"),
	)
	return deleteRedisKeys(ctx, keys)
}

func deleteRedisKeys(ctx context.Context, keys []string) error {
	unique := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key != "" {
			unique[key] = struct{}{}
		}
	}
	for key := range unique {
		if err := redisx.C().Del(ctx, key).Err(); err != nil {
			return fmt.Errorf("删除 %s 失败: %w", key, err)
		}
	}
	for key := range unique {
		exists, err := redisx.C().Exists(ctx, key).Result()
		if err != nil {
			return fmt.Errorf("验证 %s 失败: %w", key, err)
		}
		if exists != 0 {
			return fmt.Errorf("%s 删除后仍存在", key)
		}
	}
	return nil
}
