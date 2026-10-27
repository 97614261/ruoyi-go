package multitenant_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/erpctx"
	"ruoyi-go/pkg/tenantctx"
	"strings"
	"testing"
	"time"
)

func TestERPOrders(t *testing.T) {
	root := os.Getenv("MT_TEST_ROOT")
	if root == "" {
		t.Skip("explicit dedicated local acceptance only")
	}
	cfg, err := config.Load(filepath.Join(root, "configs/multitenant.local.yml"))
	if err != nil {
		t.Fatal("local config unavailable")
	}
	dsn, err := mysql.ParseDSN(cfg.MySQL.DSN)
	if err != nil {
		t.Fatal("invalid local config")
	}
	host, _, e := net.SplitHostPort(dsn.Addr)
	if e != nil || dsn.DBName != repository.TenantDatabaseName || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		t.Fatal("remote/original database refused")
	}
	if err = repository.Init(cfg.MySQL); err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	cache := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	defer cache.Close()
	s := &suite{t: t, base: "http://127.0.0.1:8081", root: root, client: &http.Client{Timeout: 30 * time.Second}, redis: cache, passwords: loadTestAccounts(t, root)}
	platform := s.login("platform")
	t.Cleanup(func() { s.req("POST", "/logout", platform, nil) })
	a, b := newOperationsFixture(t, s, platform), newOperationsFixture(t, s, platform)
	deny := func(r map[string]any) {
		t.Helper()
		if r["code"] == float64(200) {
			t.Fatal("unexpected authorization/safety success")
		}
	}
	shop := func(f operationsFixture, code string) int64 {
		r := s.req("POST", "/erp/shop", f.owner, map[string]any{"shopCode": code, "shopName": "订单测试", "channelCode": "MANUAL", "siteCode": "CN", "currency": "CNY", "status": "0"})
		s.ok(r)
		return int64(r["data"].(map[string]any)["shopId"].(float64))
	}
	shopA, shopB := shop(a, "ORDER-A"), shop(b, "ORDER-A")
	input := func(shop int64, no, key string) map[string]any {
		return map[string]any{"shopId": shop, "sourceOrderNo": no, "requestKey": key, "receiverName": "PRIVATE_RECEIVER_882", "receiverPhone": "PRIVATE_PHONE_882", "receiverCountry": "CN", "receiverAddress": "PRIVATE_ADDRESS_882", "currency": "CNY", "originalAmount": "0", "orderedAt": "2026-10-21 10:00:00", "lines": []map[string]any{{"sourceLineNo": "0001", "shopItemCode": "0002", "sourceTitle": "验收商品", "quantity": 2}}}
	}
	ctxFor := func(f operationsFixture) context.Context {
		return erpctx.WithUser(tenantctx.With(context.Background(), f.tenant.TenantID, false), *f.tenant.OwnerUserID)
	}
	detail := func(id int64) map[string]any {
		r := s.req("GET", fmt.Sprintf("/erp/order/%d", id), a.owner, nil)
		s.ok(r)
		return r["data"].(map[string]any)
	}
	var orderID int64
	t.Run("create_replay_edit_concurrency_and_privacy", func(t *testing.T) {
		in := input(shopA, "0000001", "order-test-create-01")
		r := s.req("POST", "/erp/order", a.owner, in)
		s.ok(r)
		orderID = int64(r["data"].(map[string]any)["orderId"].(float64))
		d := detail(orderID)
		if d["status"] != "PENDING_REVIEW" || d["originalAmount"] != "0.0000" || d["stockDocumentId"] != nil {
			t.Fatal("order create contract")
		}
		for i := 0; i < 2; i++ {
			r = s.req("POST", "/erp/order", a.owner, in)
			s.ok(r)
			if !r["data"].(map[string]any)["existing"].(bool) {
				t.Fatal("duplicate order")
			}
		}
		edit := input(shopA, "0000001", "order-test-edit-01")
		edit["version"] = 1
		edit["receiverName"] = "PRIVATE_EDITED_882"
		results := parallelJSON(s, 5, func(i int) (string, string, string, any) {
			body := map[string]any{}
			for k, v := range edit {
				body[k] = v
			}
			body["requestKey"] = fmt.Sprintf("order-concurrent-%d", i)
			return "PUT", fmt.Sprintf("/erp/order/%d", orderID), a.owner, body
		})
		success := 0
		for _, r := range results {
			if r.err != nil {
				t.Fatal(r.err)
			}
			if r.code == 200 {
				success++
			}
		}
		if success != 1 || detail(orderID)["version"] != float64(2) {
			t.Fatal("CAS did not serialize", success)
		}
		r = s.req("POST", "/erp/order", a.owner, in)
		s.ok(r)
		if detail(orderID)["receiverName"] != "PRIVATE_EDITED_882" {
			t.Fatal("replay overwrote edit")
		}
		in["receiverAddress"] = "changed"
		deny(s.req("POST", "/erp/order", a.owner, in))
		r = s.req("GET", "/erp/order/list?pageSize=10000", a.owner, nil)
		s.ok(r)
		encoded, _ := json.Marshal(r)
		if bytes.Contains(encoded, []byte("PRIVATE_")) {
			t.Fatal("list leaked recipient")
		}
		ctx, cancel := context.WithTimeout(ctxFor(a), 10*time.Second)
		defer cancel()
		var logs []model.SysOperLog
		if err = repository.DB(ctx).Where("title='ERP订单'").Find(&logs).Error; err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(logs)
		if bytes.Contains(raw, []byte("PRIVATE_")) {
			t.Fatal("audit leaked recipient")
		}
		var stock, ledger, packages int64
		repository.DB(ctx).Model(&model.ERPStock{}).Count(&stock)
		repository.DB(ctx).Model(&model.ERPLedger{}).Count(&ledger)
		repository.TenantTable(repository.DB(ctx), "erp_package").Count(&packages)
		if stock+ledger+packages != 0 {
			t.Fatal("phase5 changed fulfillment")
		}
	})
	t.Run("create_key_source_match_sets_live_scope_and_unique_races", func(t *testing.T) {
		prior := s.t
		s.t = t
		defer func() { s.t = prior }()
		expectRejected := func(v map[string]any) {
			t.Helper()
			if v["code"] == float64(200) || v["data"] != nil {
				t.Fatal("conflicting or unauthorized create returned data")
			}
		}
		made := func(body map[string]any) int64 {
			t.Helper()
			v := s.req("POST", "/erp/order", a.owner, body)
			s.ok(v)
			return int64(v["data"].(map[string]any)["orderId"].(float64))
		}
		first := input(shopA, "MATCH-SOURCE-A", "match-request-a")
		second := input(shopA, "MATCH-SOURCE-B", "match-request-b")
		aid, bid := made(first), made(second)
		if aid == bid {
			t.Fatal("distinct keys/sources did not create distinct orders")
		}
		replay := s.req("POST", "/erp/order", a.owner, first)
		s.ok(replay)
		if replay["data"].(map[string]any)["existing"] != true || int64(replay["data"].(map[string]any)["orderId"].(float64)) != aid {
			t.Fatal("two conditions matching one row were not deduplicated")
		}
		expectRejected(s.req("POST", "/erp/order", a.owner, input(shopA, "MATCH-ABSENT", "match-request-a")))
		sourceReplay := s.req("POST", "/erp/order", a.owner, input(shopA, "MATCH-SOURCE-A", "match-absent-key"))
		s.ok(sourceReplay)
		if sourceReplay["data"].(map[string]any)["existing"] != true || int64(sourceReplay["data"].(map[string]any)["orderId"].(float64)) != aid {
			t.Fatal("identical source/content did not preserve original replay with an alternate request key")
		}
		sourceChanged := input(shopA, "MATCH-SOURCE-A", "match-new-content-key")
		sourceChanged["receiverName"] = "different source payload"
		expectRejected(s.req("POST", "/erp/order", a.owner, sourceChanged))
		expectRejected(s.req("POST", "/erp/order", a.owner, input(shopA, "MATCH-SOURCE-B", "match-request-a")))
		changed := input(shopA, "MATCH-SOURCE-A", "match-request-a")
		changed["receiverName"] = "changed fingerprint"
		expectRejected(s.req("POST", "/erp/order", a.owner, changed))
		otherShop := shop(a, "MATCH-SECOND")
		hidden := input(otherShop, "MATCH-SOURCE-A", "match-hidden-shop")
		made(hidden)
		// Identical identifiers in another tenant must neither block nor replay A.
		foreign := s.req("POST", "/erp/order", b.owner, input(shopB, "MATCH-SOURCE-A", "match-request-a"))
		s.ok(foreign)
		if int64(foreign["data"].(map[string]any)["orderId"].(float64)) == aid {
			t.Fatal("foreign tenant key collision leaked a result")
		}
		s.ok(s.req("POST", "/system/role", a.owner, map[string]any{"roleName": "查重等价员工", "roleKey": "creatematches", "roleSort": 1, "status": "0", "dataScope": "1", "menuIds": []int64{6000, 7100, 7300, 7301, 7302}}))
		roleList := s.req("GET", "/system/role/list?roleKey=creatematches", a.owner, nil)
		s.ok(roleList)
		rid := roleList["rows"].([]any)[0].(map[string]any)["roleId"]
		s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": "creatematches", "nickName": "查重等价员工", "password": operationsPassword, "status": "0", "roleIds": []any{rid}}))
		users := s.req("GET", "/system/user/list?userName=creatematches", a.owner, nil)
		s.ok(users)
		uid := int64(users["rows"].([]any)[0].(map[string]any)["userId"].(float64))
		employee := s.loginAs(a.code, "creatematches", operationsPassword)
		defer s.req("POST", "/logout", employee, nil)
		grant := func(ids []int64) {
			path := fmt.Sprintf("/erp/shop/access/%d", uid)
			v := s.req("GET", path, a.owner, nil)
			s.ok(v)
			s.ok(s.req("PUT", path, a.owner, map[string]any{"version": v["data"].(map[string]any)["version"], "shopIds": ids}))
		}
		grant([]int64{shopA})
		defer grant([]int64{})
		s.ok(s.req("GET", fmt.Sprintf("/erp/order/%d", aid), employee, nil))
		// An invisible key in the same tenant must still fail its unique insert,
		// without revealing hidden metadata or leaving a partially created order.
		expectRejected(s.req("POST", "/erp/order", employee, input(shopA, "MATCH-NOT-CREATED", "match-hidden-shop")))
		check := s.req("GET", "/erp/order/list?sourceOrderNo=MATCH-NOT-CREATED", a.owner, nil)
		s.ok(check)
		if check["total"] != float64(0) {
			t.Fatal("hidden-key conflict left an order")
		}
		grant([]int64{})
		expectRejected(s.req("POST", "/erp/order", employee, first))
		for _, kind := range []string{"same_key_same_source", "same_source_different_keys", "same_source_different_content", "same_key_different_sources"} {
			results := parallelJSON(s, 6, func(i int) (string, string, string, any) {
				key, source := "match-race-"+kind, "MATCH-RACE-"+kind
				if kind == "same_source_different_keys" || kind == "same_source_different_content" {
					key += fmt.Sprint(i)
				}
				if kind == "same_key_different_sources" {
					source += fmt.Sprint(i)
				}
				body := input(shopA, source, key)
				if kind == "same_source_different_content" {
					body["receiverName"] = fmt.Sprintf("race-content-%d", i)
				}
				return "POST", "/erp/order", a.owner, body
			})
			winners := 0
			for _, r := range results {
				if r.err != nil {
					t.Fatal(r.err)
				}
				if r.code == 200 {
					winners++
				}
			}
			want := 1
			if kind == "same_key_same_source" || kind == "same_source_different_keys" {
				want = 6
			}
			if winners != want {
				t.Fatalf("%s successes=%d want=%d", kind, winners, want)
			}
			ctx, cancel := context.WithTimeout(ctxFor(a), 10*time.Second)
			var rows []model.ERPOrder
			if err := repository.DB(ctx).Where("source_order_no LIKE ?", "MATCH-RACE-"+kind+"%").Find(&rows).Error; err != nil {
				cancel()
				t.Fatal(err)
			}
			if len(rows) != 1 {
				cancel()
				t.Fatal("race persisted multiple orders", kind)
			}
			var events, lines int64
			e1 := repository.DB(ctx).Model(&model.ERPOrderEvent{}).Where("order_id=? AND action='CREATE'", rows[0].OrderID).Count(&events).Error
			e2 := repository.DB(ctx).Model(&model.ERPOrderLine{}).Where("order_id=?", rows[0].OrderID).Count(&lines).Error
			cancel()
			if e1 != nil || e2 != nil || events != 1 || lines != 1 {
				t.Fatal("race duplicated audit or lines", kind)
			}
		}
	})
	t.Run("tenant_isolation_and_invalid_json", func(t *testing.T) {
		deny(s.req("GET", fmt.Sprintf("/erp/order/%d", orderID), b.owner, nil))
		deny(s.req("GET", "/erp/order/list", platform, nil))
		deny(s.req("POST", "/erp/order", a.owner, input(shopB, "0000001", "foreign-shop-key-01")))
		s.ok(s.req("POST", "/erp/order", b.owner, input(shopB, "0000001", "order-test-create-01")))
		in := input(shopA, "bad", "invalid-amount-key")
		in["originalAmount"] = 1.2
		deny(s.req("POST", "/erp/order", a.owner, in))
	})
	t.Run("atomic_audit_failure_rolls_back", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(ctxFor(a), 10*time.Second)
		defer cancel()
		hook := "stage5:audit_fault"
		db := repository.DB(ctx)
		if err = db.Callback().Create().Before("gorm:create").Register(hook, func(tx *gorm.DB) {
			if tx.Statement.Table == "sys_oper_log" {
				tx.AddError(errors.New("injected audit fault"))
			}
		}); err != nil {
			t.Fatal(err)
		}
		var in model.ERPOrderWrite
		raw, _ := json.Marshal(input(shopA, "ROLLBACK-ONLY", "rollback-order-key"))
		json.Unmarshal(raw, &in)
		_, e := service.CreateERPOrder(ctx, in, "test")
		db.Callback().Create().Remove(hook)
		if e == nil {
			t.Fatal("audit fault accepted")
		}
		var n int64
		if err = repository.DB(ctx).Model(&model.ERPOrder{}).Where("source_order_no='ROLLBACK-ONLY'").Count(&n).Error; err != nil || n != 0 {
			t.Fatal("partial order persisted")
		}
	})
	workbook := func(rows [][]string) []byte {
		t.Helper()
		data, err := service.ERPOrderImportTemplate()
		if err != nil {
			t.Fatal(err)
		}
		f, err := excelize.OpenReader(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		for i, row := range rows {
			for j, v := range row {
				c, _ := excelize.CoordinatesToCellName(j+1, i+2)
				f.SetCellStr("订单导入", c, v)
			}
		}
		out, err := f.WriteToBuffer()
		if err != nil {
			t.Fatal(err)
		}
		return out.Bytes()
	}
	upload := func(token, key string, shopID int64, data []byte) map[string]any {
		t.Helper()
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		w.WriteField("shopId", fmt.Sprint(shopID))
		w.WriteField("requestKey", key)
		f, _ := w.CreateFormFile("file", "private-order.xlsx")
		f.Write(data)
		w.Close()
		req, _ := http.NewRequest("POST", s.base+"/erp/order/import", &buf)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", w.FormDataContentType())
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal("upload did not return JSON")
		}
		return out
	}
	waitJob := func(id int64) map[string]any {
		t.Helper()
		until := time.Now().Add(20 * time.Second)
		for time.Now().Before(until) {
			r := s.req("GET", fmt.Sprintf("/erp/importJob/%d", id), a.owner, nil)
			s.ok(r)
			d := r["data"].(map[string]any)
			if d["status"] != "PENDING" && d["status"] != "RUNNING" {
				return d
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal("import did not finish")
		return nil
	}
	row := func(no, line string) []string {
		return []string{no, line, "0002", "测试商品", "2", "PRIVATE_IMPORT_882", "0012345", "CN", "", "", "00100", "PRIVATE_IMPORT_ADDRESS_882", "CNY", "", "2026-10-21 10:00:00"}
	}
	t.Run("durable_partial_import_replay_retry_private_file_and_quota", func(t *testing.T) {
		data := workbook([][]string{row("000IMPORT-01", "01"), row("000IMPORT-02", "01"), row("000IMPORT-02", "01")})
		r := upload(a.owner, "import-partial-key-01", shopA, data)
		s.ok(r)
		id := int64(r["data"].(map[string]any)["jobId"].(float64))
		d := waitJob(id)
		if d["status"] != "PARTIAL_FAILED" || d["createdOrders"] != float64(1) || d["failedOrders"] != float64(1) {
			t.Fatal("partial group contract", d)
		}
		out := s.req("GET", fmt.Sprintf("/erp/importJob/%d/results?pageSize=1000", id), a.owner, nil)
		s.ok(out)
		raw, _ := json.Marshal(out)
		if bytes.Contains(raw, []byte("PRIVATE_")) {
			t.Fatal("result leaked private row")
		}
		deny(s.req("GET", fmt.Sprintf("/erp/importJob/%d", id), b.owner, nil))
		deny(s.req("GET", fmt.Sprintf("/erp/importJob/%d/results", id), b.owner, nil))
		r = upload(a.owner, "import-partial-key-01", shopA, data)
		s.ok(r)
		if int64(r["data"].(map[string]any)["jobId"].(float64)) != id {
			t.Fatal("upload replay duplicated task")
		}
		for _, bad := range []string{"fileKey", "fileSha256", "requestHash", "requestKey"} {
			if _, ok := r["data"].(map[string]any)[bad]; ok {
				t.Fatal("private metadata exposed")
			}
		}
		retry := map[string]any{"version": d["version"], "requestKey": "retry-partial-key-01"}
		s.ok(s.req("POST", fmt.Sprintf("/erp/importJob/%d/retry", id), a.owner, retry))
		s.ok(s.req("POST", fmt.Sprintf("/erp/importJob/%d/retry", id), a.owner, retry))
		d = waitJob(id)
		if d["createdOrders"] != float64(1) || d["failedOrders"] != float64(1) || d["attempt"] != float64(2) {
			t.Fatal("retry changed success accounting", d)
		}
		req, _ := http.NewRequest("GET", fmt.Sprintf("%s/erp/importJob/%d/file", s.base, id), nil)
		req.Header.Set("Authorization", "Bearer "+a.owner)
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !bytes.Equal(data, got) {
			t.Fatal("private file mismatch")
		}
		ctx, cancel := context.WithTimeout(ctxFor(a), 10*time.Second)
		defer cancel()
		var j model.ERPImportJob
		if err = repository.DB(ctx).Where("job_id=?", id).Take(&j).Error; err != nil {
			t.Fatal(err)
		}
		if _, err = service.AccessibleFile(ctx, model.ERPImportFilePrefix+j.FileKey); err == nil {
			t.Fatal("public attachment bypass")
		}
		var n int64
		repository.DB(ctx).Model(&model.SysFile{}).Where("storage_key=?", model.ERPImportFilePrefix+j.FileKey).Count(&n)
		if n != 1 {
			t.Fatal("quota metadata missing")
		}
		// Expire this dedicated fixture only; ordinary metadata release is idempotent.
		if err = repository.DB(ctx).Model(&model.ERPImportJob{}).Where("job_id=?", id).Update("retain_until", time.Now().Add(-time.Minute)).Error; err != nil {
			t.Fatal(err)
		}
		deny(s.req("POST", fmt.Sprintf("/erp/importJob/%d/retry", id), a.owner, map[string]any{"version": d["version"], "requestKey": "expired-retry-key"}))
	})
	t.Run("live_employee_scope_revocation_interrupts_and_retry_resumes", func(t *testing.T) {
		menus := []int64{6000, 7100, 7300, 7301, 7302, 7303, 7304, 7305, 7400, 7401}
		s.ok(s.req("POST", "/system/role", a.owner, map[string]any{"roleName": "订单验收员工", "roleKey": "order_test_operator", "roleSort": 0, "status": "0", "dataScope": "1", "menuIds": menus}))
		r := s.req("GET", "/system/role/list?roleKey=order_test_operator", a.owner, nil)
		s.ok(r)
		role := r["rows"].([]any)[0].(map[string]any)["roleId"]
		s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": "orderemployee", "nickName": "订单验收员工", "password": operationsPassword, "status": "0", "roleIds": []any{role}}))
		r = s.req("GET", "/system/user/list?userName=orderemployee", a.owner, nil)
		s.ok(r)
		uid := int64(r["rows"].([]any)[0].(map[string]any)["userId"].(float64))
		employee := s.loginAs(a.code, "orderemployee", operationsPassword)
		grant := func(ids []int64) {
			p := fmt.Sprintf("/erp/shop/access/%d", uid)
			v := s.req("GET", p, a.owner, nil)
			s.ok(v)
			s.ok(s.req("PUT", p, a.owner, map[string]any{"version": v["data"].(map[string]any)["version"], "shopIds": ids}))
		}
		deny(s.req("GET", fmt.Sprintf("/erp/order/%d", orderID), employee, nil))
		grant([]int64{shopA})
		s.ok(s.req("GET", fmt.Sprintf("/erp/order/%d", orderID), employee, nil))
		source := make([][]string, 100)
		for i := range source {
			source[i] = row(fmt.Sprintf("REVOKE-%04d", i), "01")
		}
		r = upload(employee, "employee-import-revoke", shopA, workbook(source))
		s.ok(r)
		id := int64(r["data"].(map[string]any)["jobId"].(float64))
		grant([]int64{})
		d := waitJob(id)
		if d["status"] != "INTERRUPTED" || d["createdOrders"].(float64) >= 100 {
			t.Fatal("revoked worker kept writing")
		}
		deny(s.req("GET", fmt.Sprintf("/erp/importJob/%d", id), employee, nil))
		deny(s.req("GET", fmt.Sprintf("/erp/order/%d", orderID), employee, nil))
		grant([]int64{shopA})
		s.ok(s.req("POST", fmt.Sprintf("/erp/importJob/%d/retry", id), a.owner, map[string]any{"version": d["version"], "requestKey": "owner-resume-retry"}))
		d = waitJob(id)
		if d["status"] != "SUCCEEDED" || d["createdOrders"].(float64) != 100 {
			t.Fatal("retry did not resume deduplicated orders", d)
		}
		// Function revocation matters as well as shop scope. Keep a failed job's
		// original actor; owner retry must not impersonate an owner for that actor.
		invalid := row("FUNCTION-REVOKE-01", "01")
		invalid[4] = "0"
		r = upload(employee, "employee-import-role-revoke", shopA, workbook([][]string{invalid}))
		s.ok(r)
		jobID := int64(r["data"].(map[string]any)["jobId"].(float64))
		d = waitJob(jobID)
		if d["status"] != "FAILED" {
			t.Fatal("fixture not failed")
		}
		get := s.req("GET", fmt.Sprintf("/system/role/%v", role), a.owner, nil)
		s.ok(get)
		roleInput := get["data"].(map[string]any)
		roleInput["menuIds"] = []int64{6000, 7100, 7300, 7301, 7400, 7401}
		s.ok(s.req("PUT", "/system/role", a.owner, roleInput))
		s.ok(s.req("POST", fmt.Sprintf("/erp/importJob/%d/retry", jobID), a.owner, map[string]any{"version": d["version"], "requestKey": "owner-retry-revoked-actor"}))
		d = waitJob(jobID)
		if d["status"] != "INTERRUPTED" {
			t.Fatal("worker ignored function revocation", d)
		}
	})
	t.Run("import_respects_shared_attachment_storage_quota", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(tenantctx.System(context.Background()), 10*time.Second)
		defer cancel()
		var p model.TenantPolicy
		if err := repository.DB(ctx).Where("tenant_id=?", a.tenant.TenantID).Take(&p).Error; err != nil {
			t.Fatal(err)
		}
		var used int64
		if err := repository.DB(ctx).Model(&model.SysFile{}).Where("tenant_id=?", a.tenant.TenantID).Select("COALESCE(SUM(size_bytes),0)").Scan(&used).Error; err != nil {
			t.Fatal(err)
		}
		if err := repository.DB(ctx).Model(&model.TenantPolicy{}).Where("tenant_id=?", a.tenant.TenantID).Update("max_storage_bytes", used+1).Error; err != nil {
			t.Fatal(err)
		}
		defer repository.DB(ctx).Model(&model.TenantPolicy{}).Where("tenant_id=?", a.tenant.TenantID).Update("max_storage_bytes", p.MaxStorageBytes)
		data := workbook([][]string{row("QUOTA-REFUSED", "01")})
		deny(upload(a.owner, "quota-rejected-import", shopA, data))
		var n int64
		repository.DB(ctx).Model(&model.ERPImportJob{}).Where("tenant_id=? AND request_key='quota-rejected-import'", a.tenant.TenantID).Count(&n)
		if n != 0 {
			t.Fatal("quota rejection left admitted job")
		}
	})
	t.Run("invalid_file_is_durable_failure_without_raw_error", func(t *testing.T) {
		r := upload(a.owner, "invalid-file-key-01", shopA, []byte("PRIVATE_BROKEN_882"))
		s.ok(r)
		id := int64(r["data"].(map[string]any)["jobId"].(float64))
		d := waitJob(id)
		if d["status"] != "FAILED" {
			t.Fatal("invalid workbook not failed", d)
		}
		r = s.req("GET", fmt.Sprintf("/erp/importJob/%d/results", id), a.owner, nil)
		raw, _ := json.Marshal(r)
		if strings.Contains(string(raw), "PRIVATE_") {
			t.Fatal("parser error leaked raw input")
		}
	})
	t.Run("read_pages_counts_projection_and_live_scope", func(t *testing.T) { checkOrderReadPages(t, s, platform) })

}
