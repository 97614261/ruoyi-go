package portaltest

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/redisx"
)

// This suite is deliberately separate from the legacy single-tenant test entry point.
func TestPortalBootstrap(t *testing.T) {
	if os.Getenv("PORTAL_INTEGRATION") != "1" {
		t.Skip("explicit PORTAL_INTEGRATION=1 required; only local ruoyi_go_portal")
	}
	cfg, e := config.Load("../../configs/portal.local.yml")
	if e != nil {
		t.Fatal("portal private config unavailable")
	}
	d, e := mysql.ParseDSN(cfg.MySQL.DSN)
	if e != nil || d.DBName != "ruoyi_go_portal" || cfg.Redis.DB != 14 || cfg.Server.Port != 8085 || !cfg.Portal.Enabled {
		t.Fatal("isolated portal target required")
	}
	if d.Addr != "127.0.0.1:3306" && d.Addr != "localhost:3306" {
		t.Fatal("local database required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, e := sql.Open("mysql", d.FormatDSN())
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = repository.ValidatePortalConnection(ctx, db); e != nil {
		t.Fatal(e)
	}
	original, e := repository.PortalStructure(ctx, db)
	if e != nil {
		t.Fatal(e)
	}
	t.Run("guard_rejects_manifest_drift_and_restores", func(t *testing.T) {
		zero := strings.Repeat("0", 64)
		result, e := db.ExecContext(ctx, "UPDATE portal_schema_version SET structure_sha256=? WHERE version=6 AND structure_sha256=?", zero, original)
		if e != nil {
			t.Fatal(e)
		}
		n, e := result.RowsAffected()
		if e != nil || n != 1 {
			t.Fatal("manifest ownership check failed")
		}
		restored := false
		restore := func() {
			restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer restoreCancel()
			result, e := db.ExecContext(restoreCtx, "UPDATE portal_schema_version SET structure_sha256=? WHERE version=6 AND structure_sha256=?", original, zero)
			if e != nil {
				t.Error("manifest restore failed")
				return
			}
			n, e := result.RowsAffected()
			if e != nil || n != 1 {
				t.Error("manifest restore CAS failed")
				return
			}
			restored = true
		}
		defer func() {
			if !restored {
				restore()
			}
		}()
		if e = repository.ValidatePortalConnection(ctx, db); e == nil || !strings.Contains(e.Error(), "漂移") {
			t.Error("drift was not rejected")
		}
		restore()
		if e = repository.ValidatePortalConnection(ctx, db); e != nil {
			t.Fatal(e)
		}
	})
	if t.Failed() {
		return
	}
	account := struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{}
	raw, e := os.ReadFile("../../logs/portal-accounts.local.json")
	if e != nil {
		t.Fatal("local account record unavailable")
	}
	if e = json.Unmarshal(raw, &account); e != nil {
		t.Fatal("local account record invalid")
	}
	r := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: 14})
	defer r.Close()
	client := &http.Client{Timeout: 10 * time.Second}
	call := func(method, path, token string, payload any) map[string]any {
		t.Helper()
		var data io.Reader
		if payload != nil {
			b, e := json.Marshal(payload)
			if e != nil {
				t.Fatal(e)
			}
			data = bytes.NewReader(b)
		}
		req, e := http.NewRequestWithContext(ctx, method, "http://127.0.0.1:8085"+path, data)
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response, e := client.Do(req)
		if e != nil {
			t.Fatalf("%s request failed", path)
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("%s HTTP status %d", path, response.StatusCode)
		}
		var out map[string]any
		if e = json.NewDecoder(response.Body).Decode(&out); e != nil {
			t.Fatalf("%s invalid JSON", path)
		}
		return out
	}
	cap := call("GET", "/captchaImage", "", nil)
	if cap["captchaEnabled"] != true {
		t.Fatal("captcha must remain enabled")
	}
	uuid, ok := cap["uuid"].(string)
	if !ok {
		t.Fatal("captcha missing")
	}
	answer, e := r.Get(ctx, redisx.CaptchaKey(uuid)).Result()
	if e != nil {
		t.Fatal("test captcha missing from isolated Redis")
	}
	login := call("POST", "/login", "", map[string]any{"username": account.Username, "password": account.Password, "uuid": uuid, "code": answer})
	token, ok := login["token"].(string)
	if login["code"] != float64(200) || !ok {
		t.Fatal("new portal admin login denied")
	}
	defer call("POST", "/logout", token, nil)
	info := call("GET", "/getInfo", token, nil)
	user, ok := info["user"].(map[string]any)
	if info["code"] != float64(200) || !ok || user["userName"] != "admin" || user["userId"] != float64(1) {
		t.Fatal("wrong portal identity")
	}
	routes := call("GET", "/getRouters", token, nil)
	menus, ok := routes["data"].([]any)
	if !ok || len(menus) == 0 {
		t.Fatal("portal menus unavailable")
	}
	if call("GET", "/getInfo", "", nil)["code"] != float64(401) {
		t.Fatal("anonymous access accepted")
	}
	call("POST", "/logout", token, nil)
	if call("GET", "/getInfo", token, nil)["code"] != float64(401) {
		t.Fatal("logged-out session remained active")
	}
	if e = repository.ValidatePortalConnection(ctx, db); e != nil {
		t.Fatal("schema guard changed after normal login data writes")
	}
	current, e := repository.PortalStructure(ctx, db)
	if e != nil || current != original {
		t.Fatal("normal authentication changed schema fingerprint")
	}
}
