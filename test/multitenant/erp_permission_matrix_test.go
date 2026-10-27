package multitenant_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"github.com/xuri/excelize/v2"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/service"
	"ruoyi-go/test/permissionmatrix"
)

func TestERPPermissionMatrix(t *testing.T) {
	root := os.Getenv("MT_TEST_ROOT")
	if root == "" {
		t.Skip("explicit dedicated local acceptance required")
	}
	cfg, err := config.Load(filepath.Join(root, "configs/multitenant.local.yml"))
	if err != nil {
		t.Fatal("config unavailable")
	}
	d, err := mysql.ParseDSN(cfg.MySQL.DSN)
	if err != nil {
		t.Fatal("invalid DSN")
	}
	host, _, e := net.SplitHostPort(d.Addr)
	if e != nil || d.Net != "tcp" || d.DBName != repository.TenantDatabaseName || (host != "127.0.0.1" && host != "localhost") {
		t.Fatal("dedicated local database only")
	}
	rh, _, e := net.SplitHostPort(cfg.Redis.Addr)
	if e != nil || (rh != "127.0.0.1" && rh != "localhost") || cfg.Redis.DB != 13 {
		t.Fatal("local Redis13 only")
	}
	if repository.Init(cfg.MySQL) != nil {
		t.Fatal("repository unavailable")
	}
	t.Cleanup(func() { repository.Close() })
	cache := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	t.Cleanup(func() { cache.Close() })
	s := &suite{t: t, root: root, base: "http://127.0.0.1:8081", client: &http.Client{Timeout: 20 * time.Second}, redis: cache, passwords: loadTestAccounts(t, root)}
	p := s.login("platform")
	t.Cleanup(func() { s.req("POST", "/logout", p, nil) })
	a, b := newOperationsFixture(t, s, p), newOperationsFixture(t, s, p)
	t.Cleanup(func() { s.req("POST", "/logout", a.owner, nil); s.req("POST", "/logout", b.owner, nil) })
	t.Logf("permission matrix fixture tenantIds=%d,%d", a.tenant.TenantID, b.tenant.TenantID)
	obj := func(v any) map[string]any { return v.(map[string]any) }
	data := func(v map[string]any) map[string]any { s.ok(v); return obj(v["data"]) }
	id := func(v map[string]any, k string) int64 { return int64(v[k].(float64)) }
	createUser := func(name string, roles []int64) (int64, string) {
		s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": name, "nickName": "权限矩阵专用员工", "password": operationsPassword, "status": "0", "roleIds": roles}))
		v := s.req("GET", "/system/user/list?userName="+name, a.owner, nil)
		s.ok(v)
		uid := id(obj(v["rows"].([]any)[0]), "userId")
		token := s.loginAs(a.code, name, operationsPassword)
		t.Cleanup(func() { s.req("POST", "/logout", token, nil) })
		return uid, token
	}
	_, empty := createUser("matrixempty", []int64{})
	info := s.req("GET", "/getInfo", empty, nil)
	s.ok(info)
	for _, perm := range info["permissions"].([]any) {
		if strings.HasPrefix(perm.(string), "erp:") || perm == "*:*:*" || perm == "system:role:add" {
			t.Fatal("empty employee unexpectedly authorized")
		}
	}
	routes, err := permissionmatrix.Routes()
	if err != nil {
		t.Fatal(err)
	}
	for _, actor := range []struct {
		name, token string
		code        float64
	}{{"anonymous", "", 401}, {"platform", p, 403}, {"no_ERP_permissions", empty, 403}} {
		t.Run("all_routes/"+actor.name, func(t *testing.T) {
			old := s.t
			s.t = t
			defer func() { s.t = old }()
			for _, r := range routes {
				var body any
				if r.Method != "GET" {
					body = map[string]any{}
				}
				v := s.req(r.Method, r.ProbePath(), actor.token, body)
				if v["code"] != actor.code {
					t.Fatalf("%s: expected code %.0f, got %v", r.Key(), actor.code, v["code"])
				}
				if v["data"] != nil || v["rows"] != nil {
					t.Fatalf("%s exposed business payload on denial", r.Key())
				}
			}
			t.Logf("%d registered ERP function gates rejected this actor", len(routes))
		})
	}

	t.Run("real_import_file_results_export_and_old_token_revocation", func(t *testing.T) {
		old := s.t
		s.t = t
		defer func() { s.t = old }()
		menus := []int64{6000, 7100, 7300, 7301, 7304, 7305, 7400, 7401}
		s.ok(s.req("POST", "/system/role", a.owner, map[string]any{"roleName": "权限矩阵导入员", "roleKey": "matriximport", "roleSort": 1, "status": "0", "dataScope": "1", "menuIds": menus}))
		rr := s.req("GET", "/system/role/list?roleKey=matriximport", a.owner, nil)
		s.ok(rr)
		role := obj(rr["rows"].([]any)[0])
		rid := id(role, "roleId")
		uid, token := createUser("matriximport", []int64{rid})
		peerID, peer := createUser("matrixpeer", []int64{rid})
		shop := data(s.req("POST", "/erp/shop", a.owner, map[string]any{"shopCode": "MATRIX-S", "shopName": "权限矩阵专用店", "channelCode": "MANUAL", "siteCode": "CN", "currency": "CNY", "status": "0"}))
		shopID := id(shop, "shopId")
		grant := func(user int64, ids []int64) {
			path := fmt.Sprintf("/erp/shop/access/%d", user)
			v := data(s.req("GET", path, a.owner, nil))
			s.ok(s.req("PUT", path, a.owner, map[string]any{"version": v["version"], "shopIds": ids}))
		}
		grant(uid, []int64{shopID})
		grant(peerID, []int64{shopID})
		defer grant(uid, []int64{})
		defer grant(peerID, []int64{})
		setMenus := func(ids []int64) {
			v := data(s.req("GET", fmt.Sprintf("/system/role/%d", rid), a.owner, nil))
			v["menuIds"] = ids
			s.ok(s.req("PUT", "/system/role", a.owner, v))
		}
		template, e := service.ERPOrderImportTemplate()
		if e != nil {
			t.Fatal(e)
		}
		book, e := excelize.OpenReader(bytes.NewReader(template))
		if e != nil {
			t.Fatal(e)
		}
		const private = "MATRIX_PRIVATE_RECIPIENT"
		for i, row := range [][]string{
			{"MATRIX-OK", "01", "SKU-MATRIX", "合成商品", "1", private, "0012345", "CN", "", "", "00100", "MATRIX_PRIVATE_ADDRESS", "CNY", "", "2026-10-26 10:00:00"},
			{"MATRIX-BAD", "01", "SKU-MATRIX", "合成商品", "0", private, "0012345", "CN", "", "", "00100", "MATRIX_PRIVATE_ADDRESS", "CNY", "", "2026-10-26 10:00:00"},
		} {
			for j, cell := range row {
				coord, _ := excelize.CoordinatesToCellName(j+1, i+2)
				if e = book.SetCellStr("订单导入", coord, cell); e != nil {
					t.Fatal(e)
				}
			}
		}
		buffer, e := book.WriteToBuffer()
		book.Close()
		if e != nil {
			t.Fatal(e)
		}
		original := append([]byte(nil), buffer.Bytes()...)
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		writer.WriteField("shopId", fmt.Sprint(shopID))
		writer.WriteField("requestKey", "matrix-original-upload")
		part, e := writer.CreateFormFile("file", "matrix-private.xlsx")
		if e != nil {
			t.Fatal(e)
		}
		part.Write(original)
		writer.Close()
		req, e := http.NewRequest("POST", s.base+"/erp/order/import", &body)
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		res, e := s.client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		var uploaded map[string]any
		e = json.NewDecoder(res.Body).Decode(&uploaded)
		res.Body.Close()
		if e != nil {
			t.Fatal("upload JSON")
		}
		jid := id(data(uploaded), "jobId")
		base := fmt.Sprintf("/erp/importJob/%d", jid)
		wait := func() map[string]any {
			until := time.Now().Add(25 * time.Second)
			for time.Now().Before(until) {
				v := data(s.req("GET", base, a.owner, nil))
				if v["status"] != "PENDING" && v["status"] != "RUNNING" {
					return v
				}
				time.Sleep(100 * time.Millisecond)
			}
			t.Fatal("import did not finish")
			return nil
		}
		job := wait()
		if job["status"] != "PARTIAL_FAILED" || job["createdOrders"] != float64(1) || job["failedOrders"] != float64(1) {
			t.Fatal("fixture must create one actual order and one retriable failure")
		}
		replay := map[string]any{"version": job["version"], "requestKey": "matrix-successful-retry"}
		s.ok(s.req("POST", base+"/retry", token, replay))
		wait()
		s.ok(s.req("POST", base+"/retry", token, replay))
		file := func(actor string, allowed bool) {
			r, e := http.NewRequest("GET", s.base+base+"/file", nil)
			if e != nil {
				t.Fatal(e)
			}
			r.Header.Set("Authorization", "Bearer "+actor)
			v, e := s.client.Do(r)
			if e != nil {
				t.Fatal(e)
			}
			got, e := io.ReadAll(io.LimitReader(v.Body, 2<<20))
			v.Body.Close()
			if e != nil {
				t.Fatal(e)
			}
			if allowed {
				if v.StatusCode != 200 || !bytes.Equal(got, original) || !strings.Contains(v.Header.Get("Cache-Control"), "no-store") {
					t.Fatal("authorized original download mismatch")
				}
				return
			}
			var denied map[string]any
			if json.Unmarshal(got, &denied) != nil || (denied["code"] != float64(403) && denied["code"] != float64(500)) || bytes.Contains(got, []byte(private)) || bytes.HasPrefix(got, []byte("PK")) {
				t.Fatal("private original exposed on denial")
			}
		}
		deny := func(actor string) {
			for _, suffix := range []string{"", "/results"} {
				v := s.req("GET", base+suffix, actor, nil)
				if (v["code"] != float64(403) && v["code"] != float64(500)) || v["data"] != nil || v["rows"] != nil {
					t.Fatal("job or results exposed", suffix)
				}
			}
			file(actor, false)
			if s.req("POST", base+"/retry", actor, replay)["code"] == float64(200) {
				t.Fatal("authorized retry replay bypassed current boundary")
			}
		}
		file(token, true)
		file(a.owner, true)
		results := s.req("GET", base+"/results", token, nil)
		s.ok(results)
		encodedResults, _ := json.Marshal(results)
		if results["total"] != float64(2) || bytes.Contains(encodedResults, []byte(private)) {
			t.Fatal("positive result control missing or raw recipient leaked")
		}
		deny(peer)
		deny(b.owner)
		peerList := s.req("GET", "/erp/importJob/list", peer, nil)
		s.ok(peerList)
		if peerList["total"] != float64(0) {
			t.Fatal("same-shop peer sees another actor's job")
		}
		orders := s.req("GET", "/erp/order/list?sourceOrderNo=MATRIX-OK", token, nil)
		s.ok(orders)
		if orders["total"] != float64(1) {
			t.Fatal("positive order control missing")
		}
		oid := id(obj(orders["rows"].([]any)[0]), "orderId")
		s.ok(s.req("GET", fmt.Sprintf("/erp/order/%d", oid), token, nil))
		export := func(expectedRows int) {
			r, e := http.NewRequest("POST", s.base+fmt.Sprintf("/erp/order/export?shopId=%d", shopID), nil)
			if e != nil {
				t.Fatal(e)
			}
			r.Header.Set("Authorization", "Bearer "+token)
			v, e := s.client.Do(r)
			if e != nil {
				t.Fatal(e)
			}
			got, e := io.ReadAll(io.LimitReader(v.Body, 2<<20))
			v.Body.Close()
			if e != nil {
				t.Fatal(e)
			}
			f, e := excelize.OpenReader(bytes.NewReader(got))
			if e != nil {
				t.Fatal("actual export is not XLSX")
			}
			defer f.Close()
			rows, e := f.GetRows(f.GetSheetList()[0])
			if e != nil || len(rows) != expectedRows {
				t.Fatalf("scoped export rows: got %d want %d", len(rows), expectedRows)
			}
			raw, _ := json.Marshal(rows)
			if bytes.Contains(raw, []byte(private)) {
				t.Fatal("summary export leaked private recipient")
			}
			if expectedRows == 2 && !bytes.Contains(raw, []byte("MATRIX-OK")) {
				t.Fatal("positive export has no actual fixture")
			}
		}
		export(2)
		grant(uid, []int64{})
		deny(token)
		for _, path := range []string{"/erp/order/list", "/erp/importJob/list"} {
			v := s.req("GET", path, token, nil)
			s.ok(v)
			if v["total"] != float64(0) {
				t.Fatal("old session retained revoked shop rows")
			}
		}
		if s.req("GET", fmt.Sprintf("/erp/order/%d", oid), token, nil)["code"] == float64(200) {
			t.Fatal("old session retained private order")
		}
		export(1)
		grant(uid, []int64{shopID})
		file(token, true)
		// Same successful replay body: status/version rejection cannot falsely
		// prove either of the retry route's two independent function gates.
		setMenus([]int64{6000, 7100, 7300, 7301, 7304, 7305, 7400})
		deny(token)
		setMenus([]int64{6000, 7100, 7300, 7301, 7305, 7400, 7401})
		file(token, true)
		if s.req("POST", base+"/retry", token, replay)["code"] != float64(403) {
			t.Fatal("retry did not require import permission independently")
		}
		setMenus([]int64{6000, 7100, 7300, 7301, 7304, 7400, 7401})
		file(token, true)
		if s.req("POST", "/erp/order/export", token, nil)["code"] != float64(403) {
			t.Fatal("old token retained independent export permission")
		}
		setMenus([]int64{})
		if s.req("GET", "/erp/importJob/list", token, nil)["code"] != float64(403) {
			t.Fatal("old token retained job-list function")
		}
		if v := data(s.req("GET", base, a.owner, nil)); v["attempt"] != float64(2) || v["createdOrders"] != float64(1) {
			t.Fatal("denied probes changed import progress")
		}
	})
}
