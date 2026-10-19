package multitenant_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"

	"ruoyi-go/internal/config"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/router"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/redisx"
)

// Disconnect only this test process's pools, never the actual local servers.
func TestDependencyFailuresDenyAuthenticatedRequests(t *testing.T) {
	root := os.Getenv("MT_TEST_ROOT")
	if root == "" {
		t.Skip("explicit local multitenant acceptance only")
	}
	cfg, err := config.Load(filepath.Join(root, "configs/multitenant.local.yml"))
	if err != nil {
		t.Fatal(err)
	}
	dsn, err := mysql.ParseDSN(cfg.MySQL.DSN)
	if err != nil {
		t.Fatal("invalid local configuration")
	}
	host, _, err := net.SplitHostPort(dsn.Addr)
	if err != nil || dsn.DBName != "ruoyi_go_mt" || (host != "localhost" && host != "127.0.0.1" && host != "::1") {
		t.Fatal("refusing nonlocal/original database")
	}
	if err = repository.Init(cfg.MySQL); err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err = redisx.Init(ctx, redisx.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB}); err != nil {
		t.Fatal(err)
	}
	defer redisx.Close()
	service.InitToken(cfg.JWT)
	data, err := os.ReadFile(filepath.Join(root, "logs/multitenant-accounts.local.json"))
	if err != nil {
		t.Fatal(err)
	}
	var accounts struct {
		Passwords map[string]string `json:"passwords"`
	}
	if err = json.Unmarshal(data, &accounts); err != nil {
		t.Fatal(err)
	}
	s := &suite{t: t, base: "http://127.0.0.1:8081", root: root, client: &http.Client{Timeout: 15 * time.Second}, passwords: accounts.Passwords, redis: redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})}
	defer s.redis.Close()
	token := s.login("platform")
	defer s.req("POST", "/logout", token, nil)
	cfg.Server.Mode = "release"
	app := router.New(cfg)
	request := func(method, path string, body any) (int, map[string]any) {
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		r := httptest.NewRequest(method, path, bytes.NewReader(raw))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		var result map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal("fault response is not JSON")
		}
		return w.Code, result
	}
	_, healthy := request("GET", "/getInfo", nil)
	s.ok(healthy)
	before, err := repository.ResolveTenant(ctx, "demo")
	if err != nil {
		t.Fatal(err)
	}
	assertClosed := func(label string) {
		t.Run(label, func(t *testing.T) {
			for _, method := range []string{"GET", "PUT"} {
				path := "/getInfo"
				if method == "PUT" {
					path = "/platform/tenant/10000/menus"
				}
				_, result := request(method, path, map[string]any{"version": before.Version, "confirmTenantCode": "demo", "menuIds": []int64{}, "reason": "fault check"})
				if result["code"] != float64(401) {
					t.Fatal("dependency failure did not deny authentication")
				}
			}
			status, _ := request("GET", "/health", nil)
			if status != 503 {
				t.Fatal("dependency outage health check must be 503")
			}
		})
	}
	if err = repository.Close(); err != nil {
		t.Fatal(err)
	}
	assertClosed("database_pool_disconnected")
	if err = repository.Init(cfg.MySQL); err != nil {
		t.Fatal(err)
	}
	if err = redisx.Close(); err != nil {
		t.Fatal(err)
	}
	assertClosed("redis_pool_disconnected")
	after, err := repository.ResolveTenant(ctx, "demo")
	if err != nil || after.Version != before.Version {
		t.Fatal("denied mutation changed tenant state")
	}
}
