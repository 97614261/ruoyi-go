// Command perfbench 是服务压测：并发打 HTTP 接口，统计 QPS 和延迟分位。
//
// 【和 perfseed -explain 的区别】
// perfseed -explain 直接连数据库跑单条 SQL，单并发，测的是"这条查询本身多慢"。
// perfbench 打的是真实接口，经过 gin 路由、中间件、service、GORM、JSON 序列化、
// Redis 会话校验，而且是并发的 —— 测的是"这个服务能扛多少"。
//
// 两个都要跑：SQL 慢就先修 SQL，SQL 不慢但接口慢，才轮到查 Go 侧。
//
// 【为什么自己写而不用 hey】
// 拿 token 太麻烦：验证码开着的时候要先取 uuid、再从 Redis 里读答案、
// 再登录换 token，最后才能压。这些步骤写进工具里，一条命令就跑完。
//
// 用法（服务要先起来）：
//
//	go run ./cmd/server
//	go run ./cmd/perfbench -c 50 -n 2000
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ruoyi-go/internal/config"
	"ruoyi-go/pkg/redisx"
)

// target 一个压测目标。
type target struct {
	name string
	// method 为空表示 GET
	method string
	path   string
	why    string
	// anonymous 为真则不带 token
	anonymous bool
	// login 为真表示每次请求都要重新登录（压 /login 用）
	login bool
	// goOnly 表示 Java 版没有这个接口，跨版本对比时要跳过
	goOnly bool
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "失败: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		configPath  = flag.String("config", "configs/application.yml", "配置文件路径")
		baseURL     = flag.String("url", "http://127.0.0.1:8080", "服务地址")
		concurrency = flag.Int("c", 50, "并发数")
		requests    = flag.Int("n", 1000, "每个接口发多少个请求")
		username    = flag.String("u", "admin", "压测用的账号")
		password    = flag.String("p", "admin123", "密码")
		only        = flag.String("only", "", "只压某个接口，按名字模糊匹配")
		timeout     = flag.Duration("timeout", 15*time.Second, "单个请求的超时")
		warmup      = flag.Int("warmup", 200, "每个接口正式计时前先跑多少个请求（结果丢弃）")
		crossImpl   = flag.Bool("cross", false, "跨实现对比模式：跳过对方没有的接口")
	)
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	// 登录接口要读 Redis 里的验证码答案，和接口测试是同一套办法：
	// 不为了跑压测去改数据库里的验证码开关
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := redisx.Init(ctx, redisx.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
		PoolSize: cfg.Redis.PoolSize,
	}); err != nil {
		return fmt.Errorf("连接 Redis 失败（压测需要它来读验证码答案）: %w", err)
	}
	defer redisx.Close()

	client := newClient(*concurrency, *timeout)

	token, err := login(client, *baseURL, *username, *password)
	if err != nil {
		return fmt.Errorf("登录失败: %w\n提示：服务起了吗？账号密码对吗？", err)
	}
	fmt.Printf("登录成功，开始压测：并发 %d，每个接口 %d 个请求\n", *concurrency, *requests)

	targets := []target{
		{
			name: "登录", method: http.MethodPost, path: "/login", anonymous: true, login: true,
			why: "bcrypt 故意很慢（约 60ms），是唯一的 CPU 大头",
		},
		{
			name: "getInfo", path: "/getInfo",
			why: "每次刷页面都调，走 Redis，验证缓存路径",
		},
		{
			name: "用户列表", path: "/system/user/list?pageNum=1&pageSize=10",
			why: "带数据权限的最重的读",
		},
		{
			name: "用户列表-深翻页", path: "/system/user/list?pageNum=500&pageSize=10",
			why: "OFFSET 5000，看翻页衰减",
		},
		{
			name: "部门树", path: "/system/dept/list",
			why: "树形构建，纯 CPU",
		},
		{
			name: "字典（走缓存）", path: "/system/dict/data/type/sys_normal_disable",
			why: "命中 Redis 就该是亚毫秒，明显更慢说明缓存没生效",
		},
		{
			name: "操作日志列表", path: "/monitor/operlog/list?pageNum=1&pageSize=10",
			why: "50 万行的表",
		},
		{
			name: "健康检查", path: "/health", anonymous: true, goOnly: true,
			why: "会 ping MySQL 和 Redis，是这套依赖的延迟下限。Java 版没有这个接口",
		},
	}

	results := make([]benchResult, 0, len(targets))
	for _, t := range targets {
		if *only != "" && !contains(t.name, *only) {
			continue
		}
		if *crossImpl && t.goOnly {
			fmt.Printf("\n=== %s ===\n跨实现对比模式，Java 版没有这个接口，跳过\n", t.name)
			continue
		}
		// 【必须等系统缓回来再压下一个】上一个目标可能把数据库压满了，
		// 客户端超时并不会让 MySQL 那边的查询立刻停下。不等的话，
		// 下一个接口测的其实是"在别人的负载下有多慢"，数字毫无意义 ——
		// 早先 字典/部门树 集体崩掉就是这么来的。
		waitUntilCalm(client, *baseURL, token)

		// 【预热】JVM 要 JIT 编译热点代码，冷启动的前几百个请求慢一个数量级。
		// 不预热就去和 Go 比，等于拿 Java 最差的状态去比 —— 那不是对比，是构陷。
		// Go 侧也需要：连接池要建满、GC 要进入稳态。
		if *warmup > 0 {
			warm := benchmark(client, *baseURL, t, token, *username, *password,
				min(*concurrency, 10), *warmup)
			fmt.Printf("\n[预热] %s：%d 个请求，失败 %d，P50 %s\n",
				t.name, warm.total, warm.failed, percentileOf(warm, 50))
			waitUntilCalm(client, *baseURL, token)
		}

		result := benchmark(client, *baseURL, t, token, *username, *password, *concurrency, *requests)
		results = append(results, result)
		printResult(result)
	}

	printSummary(results)
	return nil
}

// waitUntilCalm 等到探针连续几次都很快，说明依赖缓过来了。
//
// 探针用 /getInfo：只读 Redis，两个实现都有，够轻也够敏感 ——
// 数据库被压满时它同样会变慢。
// 不能用 /health，Java 版没有那个接口。
func waitUntilCalm(client *http.Client, baseURL, token string) {
	const (
		fastEnough = 100 * time.Millisecond
		needStreak = 3
		maxWait    = 60 * time.Second
	)

	deadline := time.Now().Add(maxWait)
	streak := 0
	for time.Now().Before(deadline) {
		start := time.Now()
		_, err := getJSON(client, baseURL+"/getInfo", token)
		cost := time.Since(start)

		if err == nil && cost < fastEnough {
			streak++
			if streak >= needStreak {
				return
			}
		} else {
			streak = 0
		}
		time.Sleep(500 * time.Millisecond)
	}
	fmt.Printf("⚠ 等了 %s 系统仍未恢复，下面的数字会偏悲观\n", maxWait)
}

func percentileOf(r benchResult, p int) time.Duration {
	if len(r.latency) == 0 {
		return 0
	}
	return percentile(r.latency, p).Round(time.Millisecond)
}

// newClient 复用连接的 HTTP 客户端。
//
// 【必须调大 MaxIdleConnsPerHost】默认只有 2，压测时绝大多数请求都在
// 重新建 TCP 连接，测出来的是握手开销不是服务性能。
func newClient(concurrency int, timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = concurrency * 2
	transport.MaxIdleConnsPerHost = concurrency * 2
	return &http.Client{Transport: transport, Timeout: timeout}
}

// login 走完整登录流程，验证码答案直接从 Redis 取。
func login(client *http.Client, baseURL, username, password string) (string, error) {
	body := map[string]any{"username": username, "password": password}

	captcha, err := getJSON(client, baseURL+"/captchaImage", "")
	if err != nil {
		return "", err
	}
	if enabled, _ := captcha["captchaEnabled"].(bool); enabled {
		uuid, _ := captcha["uuid"].(string)
		answer, err := redisx.C().Get(context.Background(), redisx.CaptchaKey(uuid)).Result()
		if err != nil {
			return "", fmt.Errorf("读取验证码答案失败: %w", err)
		}
		// 【两个实现存进 Redis 的格式不一样】
		// Go 侧存的是裸字符串 1234；Java 侧的 RedisTemplate 配的是
		// FastJson2JsonRedisSerializer，String 也会被 JSON 序列化成 "1234"（带引号）。
		// 不剥掉引号，压 Java 版时每次登录都会验证码错误。
		body["uuid"] = uuid
		body["code"] = strings.Trim(answer, `"`)
	}

	payload, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/login", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	token, _ := result["token"].(string)
	if token == "" {
		return "", fmt.Errorf("响应里没有 token: %v", result)
	}
	return token, nil
}

func getJSON(client *http.Client, url, token string) (map[string]any, error) {
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// abortThreshold 失败率超过它就提前收工。
//
// 一个接口 100% 超时的话，剩下几百个请求只是把同样的结论重复几百遍，
// 白白多花几分钟，还会让后面的目标更难恢复。
const abortThreshold = 0.5

// minSamplesBeforeAbort 至少跑这么多个再判断，避免开头几个抖动就误停。
const minSamplesBeforeAbort = 50

type benchResult struct {
	target  target
	total   int // 实际发出的请求数，提前终止时小于计划数
	planned int
	failed  int64
	aborted bool
	elapsed time.Duration
	latency []time.Duration // 已排序
	// errors 记录若干条不同的错误，不能只留第一条：
	// 上一轮就因为只看第一条，把"客户端超时"当成了全部原因，
	// 实际后面大量是服务端返回的 500
	errors []string
}

func benchmark(client *http.Client, baseURL string, t target, token, username, password string,
	concurrency, requests int) benchResult {

	var (
		wg      sync.WaitGroup
		failed  int64
		done    int64
		abort   atomic.Bool
		mu      sync.Mutex
		latency = make([]time.Duration, 0, requests)
		errSeen = make(map[string]bool)
		errList []string
		queue   = make(chan int, requests)
	)

	for i := 0; i < requests; i++ {
		queue <- i
	}
	close(queue)

	start := time.Now()
	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range queue {
				if abort.Load() {
					return
				}

				begin := time.Now()
				err := fire(client, baseURL, t, token, username, password)
				cost := time.Since(begin)

				finished := atomic.AddInt64(&done, 1)
				if err != nil {
					bad := atomic.AddInt64(&failed, 1)
					mu.Lock()
					// 最多留 3 条不同的错误，够看出是超时还是 500
					if text := truncate(err.Error(), 160); !errSeen[text] && len(errList) < 3 {
						errSeen[text] = true
						errList = append(errList, text)
					}
					mu.Unlock()

					if finished >= minSamplesBeforeAbort &&
						float64(bad)/float64(finished) > abortThreshold {
						abort.Store(true)
					}
					continue
				}

				mu.Lock()
				latency = append(latency, cost)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	sort.Slice(latency, func(i, j int) bool { return latency[i] < latency[j] })
	return benchResult{
		target: t, total: int(done), planned: requests, failed: failed,
		aborted: abort.Load(), elapsed: elapsed, latency: latency, errors: errList,
	}
}

func fire(client *http.Client, baseURL string, t target, token, username, password string) error {
	if t.login {
		_, err := login(client, baseURL, username, password)
		return err
	}

	method := t.method
	if method == "" {
		method = http.MethodGet
	}
	req, err := http.NewRequest(method, baseURL+t.path, nil)
	if err != nil {
		return err
	}
	if !t.anonymous {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 必须读完再关，否则连接不能复用，测的就成了建连开销
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 120))
	}

	// 业务 code 也要看：HTTP 200 但 code=500 同样是失败，
	// 只看状态码会把"每次都报错"压成漂亮的 QPS
	var result struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(body, &result); err == nil && result.Code != 0 && result.Code != 200 {
		return fmt.Errorf("业务 code=%d: %s", result.Code, truncate(string(body), 120))
	}
	return nil
}

func printResult(r benchResult) {
	fmt.Printf("\n=== %s ===\n%s\n", r.target.name, r.target.why)

	// 耗时和失败数无论如何都要打出来。上一轮全失败时只打了一句
	// "全部失败"，连跑了多久都看不到，没法判断是慢死的还是快速报错
	note := ""
	if r.aborted {
		note = fmt.Sprintf("（失败率过高，发了 %d/%d 个就提前终止）", r.total, r.planned)
	}
	fmt.Printf("请求 %d  失败 %d  耗时 %s%s\n",
		r.total, r.failed, r.elapsed.Round(time.Millisecond), note)

	if len(r.latency) > 0 {
		fmt.Printf("成功 %d  QPS %.0f  延迟 P50 %s  P90 %s  P99 %s  最大 %s\n",
			len(r.latency),
			float64(len(r.latency))/r.elapsed.Seconds(),
			percentile(r.latency, 50).Round(time.Millisecond),
			percentile(r.latency, 90).Round(time.Millisecond),
			percentile(r.latency, 99).Round(time.Millisecond),
			r.latency[len(r.latency)-1].Round(time.Millisecond))
	} else {
		fmt.Println("没有一个请求成功")
	}

	for i, text := range r.errors {
		fmt.Printf("错误%d: %s\n", i+1, text)
	}
}

func printSummary(results []benchResult) {
	fmt.Printf("\n\n=== 汇总 ===\n")
	fmt.Printf("%-20s %10s %10s %10s %10s\n", "接口", "QPS", "P50", "P99", "失败/总数")
	for _, r := range results {
		counts := fmt.Sprintf("%d/%d", r.failed, r.total)
		if len(r.latency) == 0 {
			fmt.Printf("%-20s %10s %10s %10s %10s\n", r.target.name, "-", "-", "-", counts)
			continue
		}
		fmt.Printf("%-20s %10.0f %10s %10s %10s\n",
			r.target.name,
			float64(len(r.latency))/r.elapsed.Seconds(),
			percentile(r.latency, 50).Round(time.Millisecond),
			percentile(r.latency, 99).Round(time.Millisecond),
			counts)
	}
}

// percentile 取分位值，输入必须已排序。
func percentile(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	index := len(sorted) * p / 100
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func contains(haystack, needle string) bool {
	return needle == "" || strings.Contains(haystack, needle)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
