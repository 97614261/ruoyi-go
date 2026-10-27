// Package main is an explicit, local-only ERP verification executable.
// It is never linked into cmd/server and never migrates or resets credentials.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/redisx"
	"ruoyi-go/pkg/tenantctx"
	"strings"
	"time"
	_ "time/tzdata"
)

// Synthetic test users only. This is never used for an existing account.
const testPassword = "Verify_test_872!"

type api struct {
	base   string
	cache  *redis.Client
	client *http.Client
}

func timeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 20*time.Second)
}
func fail(s string) error { return errors.New(s) }
func must(e error) {
	if e != nil {
		panic(e)
	}
} // Drill only; main recovers and never prints raw DB/config errors.
func save(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(path, b, 0600)
}
func load(path string, v any) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func ident(s string) string  { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }
func inside(root, p string) bool {
	r, e := filepath.Rel(root, p)
	return e == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) && !filepath.IsAbs(r)
}
func field(v map[string]any, k string) int64 { n, _ := v[k].(float64); return int64(n) }
func data(v map[string]any) map[string]any   { d, _ := v["data"].(map[string]any); return d }
func (a api) raw(method, path, token, content string, in io.Reader) ([]byte, http.Header, int, error) {
	ctx, cancel := timeout()
	defer cancel()
	r, e := http.NewRequestWithContext(ctx, method, a.base+path, in)
	if e != nil {
		return nil, nil, 0, fail("request construction")
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	r.Header.Set("Content-Type", content)
	out, e := a.client.Do(r)
	if e != nil {
		return nil, nil, 0, fail("HTTP unavailable")
	}
	defer out.Body.Close()
	b, e := io.ReadAll(io.LimitReader(out.Body, 16<<20))
	return b, out.Header, out.StatusCode, e
}
func (a api) call(method, path, token string, v any) (map[string]any, error) {
	var in io.Reader
	if v != nil {
		b, e := json.Marshal(v)
		if e != nil {
			return nil, e
		}
		in = bytes.NewReader(b)
	}
	b, _, _, e := a.raw(method, path, token, "application/json", in)
	if e != nil {
		return nil, e
	}
	var out map[string]any
	if json.Unmarshal(b, &out) != nil {
		return nil, fail("non JSON reply")
	}
	return out, nil
}
func (a api) ok(method, path, token string, v any) map[string]any {
	out, e := a.call(method, path, token, v)
	must(e)
	if out["code"] != float64(200) {
		panic(fmt.Errorf("%s %s denied (code=%v)", method, path, out["code"]))
	}
	return out
}
func (a api) denied(method, path, token string, v any) {
	out, e := a.call(method, path, token, v)
	must(e)
	if out["code"] == float64(200) {
		panic(fail("unauthorized request accepted: " + path))
	}
}
func (a api) login(code, name, password string) string {
	c := a.ok("GET", "/captchaImage?tenantCode="+url.QueryEscape(code), "", nil)
	body := map[string]any{"tenantCode": code, "username": name, "password": password}
	if c["captchaEnabled"] == true {
		ctx, cancel := timeout()
		defer cancel()
		t, e := repository.ResolveTenant(ctx, code)
		must(e)
		if t == nil {
			panic(fail("tenant missing"))
		}
		ctx = tenantctx.With(ctx, t.TenantID, code == "platform")
		uuid, _ := c["uuid"].(string)
		answer, e := a.cache.Get(ctx, redisx.TenantKey(ctx, redisx.KeyCaptchaCode, uuid)).Result()
		must(e)
		body["code"], body["uuid"] = answer, uuid
	}
	v := a.ok("POST", "/login", "", body)
	token, _ := v["token"].(string)
	if token == "" {
		panic(fail("empty login token"))
	}
	return token
}
func platform(a api) string {
	raw, e := os.ReadFile(accountsPath)
	must(e)
	fixed := []byte{}
	quoted, escaped := false, false
	for _, b := range raw {
		if quoted && !escaped && b < 32 {
			fixed = append(fixed, []byte(fmt.Sprintf(`\u%04x`, b))...)
			continue
		}
		fixed = append(fixed, b)
		if b == '"' && !escaped {
			quoted = !quoted
		}
		if b == '\\' && !escaped {
			escaped = true
		} else {
			escaped = false
		}
	}
	var rec struct {
		Username  string            `json:"username"`
		Passwords map[string]string `json:"passwords"`
	}
	must(json.Unmarshal(fixed, &rec))
	ctx, cancel := timeout()
	defer cancel()
	t, e := repository.ResolveTenant(ctx, "platform")
	must(e)
	if t == nil {
		panic(fail("platform missing"))
	}
	u, e := repository.SelectUserAccountByUserName(tenantctx.With(ctx, t.TenantID, true), rec.Username)
	must(e)
	if u == nil || bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(rec.Passwords["platform"])) != nil {
		panic(fail("platform private record mismatch; no reset"))
	}
	return a.login("platform", rec.Username, rec.Passwords["platform"])
}
func sourceConfig() (*config.Config, *mysql.Config) {
	cfg, e := config.Load(configPath)
	must(e)
	d, e := mysql.ParseDSN(cfg.MySQL.DSN)
	must(e)
	h, _, e := net.SplitHostPort(d.Addr)
	must(e)
	if d.Net != "tcp" || d.DBName != "ruoyi_go_mt" || (h != "127.0.0.1" && h != "localhost" && h != "::1") {
		panic(fail("original/remote database refused"))
	}
	root, e := filepath.Abs(".")
	must(e)
	p, e := filepath.Abs(cfg.Upload.Path)
	must(e)
	if !inside(root, p) || p == root || cfg.Upload.URLPrefix != "/profile" {
		panic(fail("unexpected upload root"))
	}
	validateLocalRedis(cfg.Redis.Addr, cfg.Redis.DB)
	return cfg, d
}
func initRepo(cfg *config.Config) { must(repository.Init(cfg.MySQL)) }
func sourceAPI(cfg *config.Config) api {
	return api{"http://127.0.0.1:8081", redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB}), &http.Client{Timeout: 20 * time.Second}}
}
func openDB(d *mysql.Config) *sql.DB {
	c := d.Clone()
	c.ParseTime = false
	c.MultiStatements = false
	c.Timeout = 5 * time.Second
	c.ReadTimeout = 30 * time.Second
	c.WriteTimeout = 30 * time.Second
	db, e := sql.Open("mysql", c.FormatDSN())
	must(e)
	db.SetMaxOpenConns(2)
	return db
}
