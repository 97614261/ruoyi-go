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
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

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
	purgeRateLimit()
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
func purgeRateLimit() {
	ctx := context.Background()
	_ = redisx.ScanKeys(ctx, redisx.KeyRateLimit+"*", 200, func(key string) error {
		redisx.C().Del(ctx, key)
		return nil
	})
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
	for _, item := range collectTestItems(listPath, "rows", nameKey) {
		if id, ok := numericID(item[idKey]); ok {
			request(http.MethodDelete, deletePrefix+id, adminToken, nil)
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
	return token, nil
}
