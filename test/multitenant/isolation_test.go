package multitenant_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/redisx"
	"ruoyi-go/pkg/tenantctx"
	"ruoyi-go/pkg/types"
	"testing"
	"time"
)

type suite struct {
	t         *testing.T
	base      string
	client    *http.Client
	redis     *redis.Client
	passwords map[string]string
	root      string
	customers [2]operationsFixture
}

func (s *suite) req(method, path, token string, body any) map[string]any {
	s.t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			s.t.Fatal(err)
		}
		r = bytes.NewReader(b)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, s.base+path, r)
	if err != nil {
		s.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := s.client.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	if err = json.NewDecoder(res.Body).Decode(&out); err != nil {
		s.t.Fatalf("%s %s: invalid JSON (%d)", method, path, res.StatusCode)
	}
	return out
}
func (s *suite) ok(v map[string]any) {
	s.t.Helper()
	if v["code"] != float64(200) {
		s.t.Fatalf("code=%v msg=%v", v["code"], v["msg"])
	}
}

// Tests that intentionally update current state explicitly fetch a fresh
// version. Stale-version negative tests continue to use req directly.
func (s *suite) updateTenant(token string, body map[string]any) map[string]any {
	s.t.Helper()
	current := s.req("GET", "/platform/tenant/"+fmt.Sprint(body["tenantId"]), token, nil)
	s.ok(current)
	copy := map[string]any{}
	for key, value := range body {
		copy[key] = value
	}
	copy["version"] = current["data"].(map[string]any)["version"]
	return s.req("PUT", "/platform/tenant", token, copy)
}
func (s *suite) login(code string) string {
	return s.loginAs(code, "admin", s.passwords[code])
}
func (s *suite) loginAs(code, username, password string) string {
	s.t.Helper()
	l := s.loginResponse(code, username, password)
	s.ok(l)
	token, _ := l["token"].(string)
	if token == "" {
		s.t.Fatal("empty token")
	}
	return token
}
func (s *suite) loginResponse(code, username, password string) map[string]any {
	s.t.Helper()
	s.waitForLoginBudget()
	c := s.req("GET", "/captchaImage?tenantCode="+code, "", nil)
	s.ok(c)
	body := map[string]any{"tenantCode": code, "username": username, "password": password}
	if c["captchaEnabled"] == true {
		id, _ := c["uuid"].(string)
		body["uuid"] = id
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		tenant, err := repository.ResolveTenant(ctx, code)
		if err != nil {
			s.t.Fatal(err)
		}
		ctx = tenantctx.With(ctx, tenant.TenantID, false)
		answer, err := s.redis.Get(ctx, redisx.TenantKey(ctx, redisx.KeyCaptchaCode, id)).Result()
		if err != nil {
			s.t.Fatal("read local test captcha:", err)
		}
		body["code"] = answer
	}
	return s.req("POST", "/login", "", body)
}

// Functional acceptance is not a login flood. Observe the real loopback-IP
// limiter and wait for its TTL; never flush/disable a security counter or retry
// arbitrary business failures to make a test pass. Leave headroom for negative
// login cases that intentionally send raw requests.
func (s *suite) waitForLoginBudget() {
	s.t.Helper()
	key := redisx.RateLimitKey("login") + ":127.0.0.1"
	deadline := time.Now().Add(70 * time.Second)
	logged := false
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		count, err := s.redis.Get(ctx, key).Int64()
		if err == redis.Nil {
			cancel()
			return
		}
		if err != nil {
			cancel()
			s.t.Fatal("cannot inspect local login test budget")
		}
		if count < 45 {
			cancel()
			return
		}
		ttl, err := s.redis.PTTL(ctx, key).Result()
		cancel()
		if err != nil || ttl <= 0 || time.Now().After(deadline) {
			s.t.Fatal("local login budget did not expire normally")
		}
		if !logged {
			s.t.Log("waiting for the existing login rate-limit window; no counters changed")
			logged = true
		}
		pause := ttl + 50*time.Millisecond
		if pause > time.Second {
			pause = time.Second
		}
		time.Sleep(pause)
	}
}
func TestTenantIsolation(t *testing.T) {
	root := os.Getenv("MT_TEST_ROOT")
	if root == "" {
		t.Skip("set MT_TEST_ROOT to explicitly enable local new-database acceptance")
	}
	cfg, err := config.Load(filepath.Join(root, "configs/multitenant.local.yml"))
	if err != nil {
		t.Fatal(err)
	}
	dsn, err := mysql.ParseDSN(cfg.MySQL.DSN)
	if err != nil || dsn.DBName != "ruoyi_go_mt" {
		t.Fatal("refusing to test a non-multitenant database")
	}
	if err = repository.Init(cfg.MySQL); err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	s := &suite{t: t, base: "http://127.0.0.1:8081", client: &http.Client{Timeout: 15 * time.Second}, passwords: loadTestAccounts(t, root), root: root,
		redis: redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})}
	defer s.redis.Close()
	p := s.login("platform")
	t.Cleanup(func() { s.req("POST", "/logout", p, nil) })
	// Mutating isolation/lifecycle checks must never operate on demo customers.
	for i := range s.customers {
		s.customers[i] = newNamedOperationsFixture(t, s, p, "admin")
		s.passwords[s.customers[i].code] = operationsPassword
	}
	a, bToken := s.customers[0].owner, s.customers[1].owner
	ai, bi := s.req("GET", "/getInfo", a, nil), s.req("GET", "/getInfo", bToken, nil)
	s.ok(ai)
	s.ok(bi)
	aid := int64(ai["user"].(map[string]any)["userId"].(float64))
	bid := int64(bi["user"].(map[string]any)["userId"].(float64))
	if aid == bid {
		t.Fatal("users must have different global IDs")
	}
	t.Run("platform_list", func(t *testing.T) { s.ok(s.req("GET", "/platform/tenant/list", p, nil)) })
	t.Run("cross_tenant_user_detail_denied", func(t *testing.T) {
		v := s.req("GET", fmt.Sprintf("/system/user/%d", bid), a, nil)
		if v["code"] == float64(200) {
			t.Fatal("foreign user detail succeeded")
		}
	})
	t.Run("platform_endpoints_denied", func(t *testing.T) {
		for _, path := range []string{"/platform/tenant/list", "/tool/gen/list", "/monitor/cache", "/monitor/server", "/monitor/online/list"} {
			v := s.req("GET", path, a, nil)
			if v["code"] != float64(403) {
				t.Fatalf("%s code=%v", path, v["code"])
			}
		}
	})
	t.Run("read_routes_and_lists", func(t *testing.T) {
		for _, token := range []string{a, bToken, p} {
			for _, path := range []string{"/getRouters", "/system/user/list", "/system/role/list", "/system/dept/list", "/system/post/list", "/system/config/list", "/system/dict/type/list", "/system/notice/list", "/monitor/job/list"} {
				s.ok(s.req("GET", path, token, nil))
			}
		}
	})
	t.Run("missing_tenant_context_denied", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := repository.SelectUserByID(ctx, aid); err == nil {
			t.Fatal("missing tenant context was accepted")
		}
	})
	t.Run("job_log_clean_is_scoped", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// Resolve the actual generated tenant ID rather than relying on seed sequence.
		tb, e := repository.ResolveTenant(ctx, s.customers[1].code)
		if e != nil {
			t.Fatal(e)
		}
		ctx = tenantctx.With(ctx, tb.TenantID, false)
		log := model.SysJobLog{JobName: "tenant-isolation-check", JobGroup: "acceptance", InvokeTarget: "test", Status: "0", CreateTime: types.Now()}
		if e = repository.InsertJobLog(ctx, &log); e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() {
			clean, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = repository.DeleteJobLogByIDs(clean, []int64{log.JobLogID})
		})
		s.ok(s.req("DELETE", "/monitor/jobLog/clean", a, nil))
		found, e := repository.SelectJobLogByID(ctx, log.JobLogID)
		if e != nil || found == nil {
			t.Fatal("cleaning A removed B log", e)
		}
	})
	t.Run("private_upload", func(t *testing.T) {
		var body bytes.Buffer
		w := multipart.NewWriter(&body)
		f, _ := w.CreateFormFile("file", "tenant-check.txt")
		_, _ = f.Write([]byte("tenant-file-check"))
		_ = w.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		r, _ := http.NewRequestWithContext(ctx, "POST", s.base+"/common/upload", &body)
		r.Header.Set("Content-Type", w.FormDataContentType())
		r.Header.Set("Authorization", "Bearer "+a)
		res, e := s.client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		var v map[string]any
		if e = json.NewDecoder(res.Body).Decode(&v); e != nil {
			t.Fatal(e)
		}
		s.ok(v)
		resource, _ := v["fileName"].(string)
		if resource == "" {
			t.Fatal("empty file resource")
		}
		for _, token := range []string{bToken, ""} {
			v := s.req("GET", resource, token, nil)
			if v["code"] == float64(200) {
				t.Fatal("private media exposed")
			}
		}
		r, _ = http.NewRequestWithContext(ctx, "GET", s.base+resource, nil)
		r.Header.Set("Authorization", "Bearer "+a)
		rr, e := s.client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer rr.Body.Close()
		content, _ := io.ReadAll(rr.Body)
		if string(content) != "tenant-file-check" {
			t.Fatal("owner cannot read upload")
		}
		cookieReq, _ := http.NewRequestWithContext(ctx, "GET", s.base+resource, nil)
		cookieReq.AddCookie(&http.Cookie{Name: "ruoyi-mt-media", Value: a})
		cookieResponse, e := s.client.Do(cookieReq)
		if e != nil {
			t.Fatal(e)
		}
		defer cookieResponse.Body.Close()
		cookieBody, _ := io.ReadAll(cookieResponse.Body)
		if string(cookieBody) != "tenant-file-check" {
			t.Fatal("media-only cookie did not authenticate image/download request")
		}
	})
	runIdentityChecks(t, s, a, bToken, p)
	runTenantAdminTools(t, s, bToken, p)
	runTenantOperations(t, s, bToken, p)
	runAdvanced(t, s, a, bToken, p, aid, bid)
}
