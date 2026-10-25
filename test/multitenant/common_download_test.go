package multitenant_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/tenantctx"
)

func TestCommonDownloadLifecycle(t *testing.T) {
	root := os.Getenv("MT_TEST_ROOT")
	if root == "" {
		t.Skip("explicit dedicated local acceptance required")
	}
	cfg, err := config.Load(filepath.Join(root, "configs/multitenant.local.yml"))
	if err != nil {
		t.Fatal("local config unavailable")
	}
	d, err := mysql.ParseDSN(cfg.MySQL.DSN)
	if err != nil {
		t.Fatal("local DSN unavailable")
	}
	h, _, err := net.SplitHostPort(d.Addr)
	if err != nil || d.Net != "tcp" || d.DBName != repository.TenantDatabaseName || (h != "localhost" && h != "127.0.0.1" && h != "::1") {
		t.Fatal("dedicated local DB only")
	}
	// server以项目根目录启动；go test的工作目录则是本测试包目录。
	if !filepath.IsAbs(cfg.Upload.Path) {
		cfg.Upload.Path = filepath.Join(root, cfg.Upload.Path)
	}
	if filepath.Clean(cfg.Upload.Path) != filepath.Join(root, "uploadPath-mt") || cfg.Upload.URLPrefix != "/profile" {
		t.Fatal("dedicated local upload root only")
	}
	if err = repository.Init(cfg.MySQL); err != nil {
		t.Fatal("repository unavailable")
	}
	t.Cleanup(func() { _ = repository.Close() })
	cache := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	t.Cleanup(func() { _ = cache.Close() })
	s := &suite{t: t, base: "http://127.0.0.1:8081", root: root, client: &http.Client{Timeout: 20 * time.Second}, redis: cache, passwords: loadTestAccounts(t, root)}
	p := s.login("platform")
	t.Cleanup(func() { s.req("POST", "/logout", p, nil) })
	a, b := newOperationsFixture(t, s, p), newOperationsFixture(t, s, p)
	s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": "reader", "nickName": "附件读权限验收", "password": operationsPassword, "status": "0"}))
	reader := s.loginAs(a.code, "reader", operationsPassword)
	t.Cleanup(func() {
		s.req("POST", "/logout", reader, nil)
		s.req("POST", "/logout", a.owner, nil)
		s.req("POST", "/logout", b.owner, nil)
	})
	ownerID := *a.tenant.OwnerUserID
	ctx := func() (context.Context, context.CancelFunc) {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		return tenantctx.With(c, a.tenant.TenantID, false), cancel
	}
	fetch := func(token, target string, headers map[string]string) (int, http.Header, []byte) {
		t := s.t
		t.Helper()
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		r, err := http.NewRequestWithContext(c, http.MethodGet, s.base+target, nil)
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer "+token)
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		res, err := s.client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, res.Header, raw
	}
	resourceURL := func(key string) string { return "/common/download/resource?resource=" + url.QueryEscape(key) }
	namedURL := func(key string, remove bool) string {
		return "/common/download?fileName=" + url.QueryEscape(strings.TrimPrefix(key, "/profile/")) + "&delete=" + fmt.Sprint(remove)
	}
	assertDenied := func(token, target string) {
		t := s.t
		t.Helper()
		status, headers, raw := fetch(token, target, nil)
		var out map[string]any
		if status != 200 || headers.Get("Content-Type") != "application/json" || json.Unmarshal(raw, &out) != nil || out["code"] != float64(403) {
			t.Fatalf("expected exact binary-error envelope, status=%d", status)
		}
	}
	assertPresent := func(key string, want []byte) {
		t := s.t
		t.Helper()
		c, cancel := ctx()
		defer cancel()
		f, err := repository.FindFile(c, key)
		if err != nil || f == nil {
			t.Fatal("attachment registration lost", err)
		}
		_, headers, raw := fetch(a.owner, resourceURL(key), nil)
		if headers.Get("Content-Type") != "application/octet-stream" || !bytes.Equal(raw, want) {
			t.Fatal("attachment bytes lost or changed")
		}
	}
	run := func(name string, fn func(*testing.T)) {
		t.Run(name, func(t *testing.T) { old := s.t; s.t = t; defer func() { s.t = old }(); fn(t) })
	}
	var img bytes.Buffer
	if err := png.Encode(&img, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	imageReply := multipartRequest(s, "/erp/product/image", a.owner, "file", []string{"catalog.png"}, [][]byte{img.Bytes()})
	s.ok(imageReply)
	imageKey := imageReply["data"].(map[string]any)["storageKey"].(string)
	product := s.req("POST", "/erp/product", a.owner, model.CatalogWrite{RequestKey: uuid.NewString(), ProductCode: "DOWNLOAD-GUARD", ProductName: "下载保护验收商品", Images: []string{imageKey}, Variants: []model.CatalogVariant{{SkuCode: "DOWNLOAD-GUARD-SKU", Unit: "件", Status: "0"}}})
	s.ok(product)
	pid := int64(product["data"].(map[string]any)["productIds"].([]any)[0].(float64))
	run("referenced_catalog_image_cannot_be_deleted_by_download", func(t *testing.T) {
		assertDenied(a.owner, namedURL(imageKey, true))
		assertDenied(reader, namedURL(imageKey, true))
		assertPresent(imageKey, img.Bytes())
		s.ok(s.req("GET", fmt.Sprintf("/erp/product/%d", pid), a.owner, nil))
		status, _, raw := fetch(a.owner, namedURL(imageKey, false), nil)
		if status != 200 || !bytes.Equal(raw, img.Bytes()) {
			t.Fatal("ordinary named read regressed")
		}
		assertDenied(b.owner, resourceURL(imageKey))
	})
	run("regular_upload_and_avatar_remain_registered", func(t *testing.T) {
		for _, tc := range []struct {
			endpoint, field, name string
			content               []byte
		}{
			{"/common/upload", "file", "ordinary.txt", []byte("ordinary attachment")},
			{"/system/user/profile/avatar", "avatarfile", "avatar.png", img.Bytes()},
		} {
			reply := multipartRequest(s, tc.endpoint, a.owner, tc.field, []string{tc.name}, [][]byte{tc.content})
			s.ok(reply)
			key, _ := reply["fileName"].(string)
			if key == "" {
				key, _ = reply["imgUrl"].(string)
			}
			if key == "" {
				t.Fatal("missing upload resource key")
			}
			assertDenied(a.owner, namedURL(key, true))
			assertPresent(key, tc.content)
		}
	})
	writeTemporary := func() string {
		t := s.t
		t.Helper()
		key := fmt.Sprintf("/profile/tenants/%d/download/%s_report.txt", a.tenant.TenantID, uuid.NewString())
		abs := filepath.Join(cfg.Upload.Path, filepath.FromSlash(strings.TrimPrefix(key, "/profile/")))
		if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte("temporary report"), 0o600); err != nil {
			t.Fatal(err)
		}
		c, cancel := ctx()
		defer cancel()
		if err := service.RecordFile(c, key, "report.txt", "text/plain", int64(len("temporary report")), ownerID, "owner"); err != nil {
			_ = os.Remove(abs)
			t.Fatal(err)
		}
		return key
	}
	run("temporary_owner_tenant_and_partial_response_boundaries", func(t *testing.T) {
		key := writeTemporary()
		assertDenied(reader, namedURL(key, true))
		assertDenied(b.owner, namedURL(key, true))
		status, _, raw := fetch(a.owner, namedURL(key, true), map[string]string{"Range": "bytes=0-2"})
		if status != 206 || string(raw) != "tem" {
			t.Fatal("partial response changed")
		}
		assertPresent(key, []byte("temporary report"))
		_, headers, _ := fetch(a.owner, namedURL(key, false), nil)
		status, _, _ = fetch(a.owner, namedURL(key, true), map[string]string{"If-Modified-Since": headers.Get("Last-Modified")})
		if status != 304 {
			t.Fatal("expected conditional response")
		}
		assertPresent(key, []byte("temporary report"))
		status, headers, raw = fetch(a.owner, namedURL(key, true), nil)
		if status != 200 || string(raw) != "temporary report" || headers.Get("download-filename") == "" {
			t.Fatal("full temporary download failed")
		}
		c, cancel := ctx()
		defer cancel()
		f, err := repository.FindFile(c, key)
		if err != nil || f != nil {
			t.Fatal("temporary registration not removed", err)
		}
		abs := filepath.Join(cfg.Upload.Path, filepath.FromSlash(strings.TrimPrefix(key, "/profile/")))
		if _, err := os.Stat(abs); !os.IsNotExist(err) {
			t.Fatal("temporary file not removed", err)
		}
		assertDenied(a.owner, namedURL(key, true))
	})
	run("stale_file_identity_cannot_remove_current_registration", func(t *testing.T) {
		key := writeTemporary()
		c, cancel := ctx()
		defer cancel()
		f, err := repository.FindFile(c, key)
		if err != nil || f == nil {
			t.Fatal("fixture missing")
		}
		stale := *f
		stale.FileID++
		removed, err := service.ForgetDownloadedFile(c, &stale, ownerID, cfg.Upload.URLPrefix)
		if err != nil || removed {
			t.Fatal("stale identity removed registration")
		}
		assertPresent(key, []byte("temporary report"))
		removed, err = repository.DeleteOwnedFileRecord(c, f.FileID, key, ownerID+1)
		if err != nil || removed {
			t.Fatal("wrong uploader removed registration")
		}
		assertPresent(key, []byte("temporary report"))
	})
}
