package multitenant_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
	"golang.org/x/crypto/bcrypt"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/tenantctx"
)

type operationsFixture struct {
	code   string
	tenant *model.SysTenant
	owner  string
}

const operationsPassword = "Operations_782!"

func newOperationsFixture(t *testing.T, s *suite, p string) operationsFixture {
	t.Helper()
	code := fmt.Sprintf("ops-%d", time.Now().UnixNano()%1000000000000)
	s.ok(s.req("POST", "/platform/tenant", p, map[string]any{"tenantCode": code, "tenantName": "生命周期与配额验收", "userName": "owner", "password": operationsPassword, "status": "0"}))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tenant, err := repository.ResolveTenant(ctx, code)
	if err != nil || tenant == nil {
		t.Fatal("operations fixture initialization failed")
	}
	t.Cleanup(func() {
		value := s.req("GET", fmt.Sprintf("/platform/tenant/%d", tenant.TenantID), p, nil)
		s.ok(value)
		current := value["data"].(map[string]any)
		if current["status"] == "3" || current["status"] == "4" {
			action := "rejectCancellation"
			if current["status"] == "4" {
				action = "restore"
			}
			s.ok(s.req("PUT", fmt.Sprintf("/platform/tenant/%d/lifecycle", tenant.TenantID), p, map[string]any{"version": current["version"], "confirmTenantCode": code, "action": action, "reason": "验收结束，保留停用记录"}))
		}
		s.ok(s.updateTenant(p, map[string]any{"tenantId": tenant.TenantID, "tenantCode": code, "tenantName": tenant.TenantName, "status": "1"}))
	})
	return operationsFixture{code: code, tenant: tenant, owner: s.loginAs(code, "owner", operationsPassword)}
}

type concurrentResult struct {
	code    float64
	message string
	err     error
	index   int
}

func parallelJSON(s *suite, n int, makeRequest func(int) (string, string, string, any)) []concurrentResult {
	results := make([]concurrentResult, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			method, path, token, body := makeRequest(i)
			raw, err := json.Marshal(body)
			if err != nil {
				results[i] = concurrentResult{err: err}
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			r, err := http.NewRequestWithContext(ctx, method, s.base+path, bytes.NewReader(raw))
			if err != nil {
				results[i] = concurrentResult{err: err}
				return
			}
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+token)
			response, err := s.client.Do(r)
			if err != nil {
				results[i] = concurrentResult{err: err}
				return
			}
			defer response.Body.Close()
			var v map[string]any
			err = json.NewDecoder(response.Body).Decode(&v)
			code, _ := v["code"].(float64)
			message, _ := v["msg"].(string)
			results[i] = concurrentResult{code: code, message: message, err: err, index: i}
		}(i)
	}
	wg.Wait()
	return results
}

func multipartRequest(s *suite, path, token, field string, names []string, contents [][]byte) map[string]any {
	s.t.Helper()
	var data bytes.Buffer
	w := multipart.NewWriter(&data)
	for i, name := range names {
		part, err := w.CreateFormFile(field, name)
		if err != nil {
			s.t.Fatal(err)
		}
		if _, err = part.Write(contents[i]); err != nil {
			s.t.Fatal(err)
		}
	}
	_ = w.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, "POST", s.base+path, &data)
	if err != nil {
		s.t.Fatal(err)
	}
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", w.FormDataContentType())
	res, err := s.client.Do(r)
	if err != nil {
		s.t.Fatal(err)
	}
	defer res.Body.Close()
	var result map[string]any
	if err = json.NewDecoder(res.Body).Decode(&result); err != nil {
		s.t.Fatal("multipart response invalid")
	}
	return result
}

func runTenantOperations(t *testing.T, parent *suite, other, p string) {
	t.Run("concurrent_platform_changes_have_one_winner", func(t *testing.T) {
		s := *parent
		s.t = t
		f := newOperationsFixture(t, &s, p)
		s.ok(s.req("POST", "/system/user", f.owner, map[string]any{"userName": "nextowner", "nickName": "交接验收", "password": operationsPassword, "status": "0"}))
		users := s.req("GET", "/system/user/list?userName=nextowner", f.owner, nil)
		s.ok(users)
		nextID := int64(users["rows"].([]any)[0].(map[string]any)["userId"].(float64))
		menus := s.req("GET", fmt.Sprintf("/platform/tenant/%d/menus", f.tenant.TenantID), p, nil)
		s.ok(menus)
		settings := menus["data"].(map[string]any)
		results := parallelJSON(&s, 3, func(i int) (string, string, string, any) {
			switch i {
			case 0:
				return "PUT", fmt.Sprintf("/platform/tenant/%d/menus", f.tenant.TenantID), p, map[string]any{"version": settings["version"], "confirmTenantCode": f.code, "reason": "并发授权验收", "menuIds": settings["checkedKeys"]}
			case 1:
				return "PUT", "/platform/tenant", p, map[string]any{"tenantId": f.tenant.TenantID, "version": settings["version"], "tenantCode": f.code, "tenantName": f.tenant.TenantName, "status": "0", "ownerUserId": nextID}
			default:
				return "PUT", fmt.Sprintf("/platform/tenant/%d/resetOwnerPwd", f.tenant.TenantID), p, map[string]any{"version": settings["version"], "ownerUserId": *f.tenant.OwnerUserID, "confirmTenantCode": f.code, "newPassword": "Concurrent_New_927!", "confirmPassword": "Concurrent_New_927!", "operatorPassword": s.passwords["platform"], "reason": "并发重置验收"}
			}
		})
		winners := 0
		for _, result := range results {
			if result.err != nil {
				t.Fatal(result.err)
			}
			if result.code == 200 {
				winners++
			}
		}
		if winners != 1 {
			t.Fatal("same-version security changes must have exactly one winner")
		}
		current := s.req("GET", fmt.Sprintf("/platform/tenant/%d", f.tenant.TenantID), p, nil)
		s.ok(current)
		if current["data"].(map[string]any)["version"] != float64(2) {
			t.Fatal("concurrent changes advanced the version more than once")
		}
		if s.req("GET", "/getInfo", f.owner, nil)["code"] != float64(401) {
			t.Fatal("old session survived concurrent change")
		}
		s.ok(s.req("GET", "/getInfo", other, nil))
		s.ok(s.req("GET", "/getInfo", p, nil))
	})
	t.Run("quota_concurrency_import_and_upload", func(t *testing.T) {
		s := *parent
		s.t = t
		f := newOperationsFixture(t, &s, p)
		quotaPath := fmt.Sprintf("/platform/tenant/%d/quota", f.tenant.TenantID)
		quota := func() map[string]any {
			v := s.req("GET", quotaPath, p, nil)
			s.ok(v)
			return v["data"].(map[string]any)
		}
		set := func(users, storage, upload int64) {
			v := quota()
			s.ok(s.req("PUT", quotaPath, p, map[string]any{"version": v["version"], "confirmTenantCode": f.code, "reason": "配额并发验收", "maxUsers": users, "maxStorageBytes": storage, "maxUploadBytes": upload}))
			f.owner = s.loginAs(f.code, "owner", operationsPassword)
		}
		set(3, 8, 8)
		results := parallelJSON(&s, 8, func(i int) (string, string, string, any) {
			return "POST", "/system/user", f.owner, map[string]any{"userName": fmt.Sprintf("apiq_%d", i), "nickName": "并发名额验收", "password": operationsPassword, "status": "0"}
		})
		created := 0
		for _, result := range results {
			if result.err != nil {
				t.Fatal(result.err)
			}
			if result.code == 200 {
				created++
			}
		}
		if created != 2 {
			t.Fatal("HTTP account creation exceeded/missed the last two seats")
		}
		set(5, 8, 8)
		hash, _ := bcrypt.GenerateFromPassword([]byte(operationsPassword), bcrypt.DefaultCost)
		var wg sync.WaitGroup
		direct := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(tenantctx.With(context.Background(), f.tenant.TenantID, false), 15*time.Second)
				defer cancel()
				direct <- repository.InsertUser(ctx, &model.SysUser{UserName: fmt.Sprintf("repoq_%d", i), NickName: "数据库名额验收", Password: string(hash), UserType: "00", Status: "0", DelFlag: "0"})
			}(i)
		}
		wg.Wait()
		close(direct)
		created = 0
		for err := range direct {
			if err == nil {
				created++
			}
		}
		if created != 2 {
			t.Fatal("database quota row lock did not enforce the last two seats")
		}
		if quota()["usage"].(map[string]any)["userCount"] != float64(5) {
			t.Fatal("committed user quota count incorrect")
		}
		book := excelize.NewFile()
		defer book.Close()
		headers := []any{"部门编号", "登录名称", "用户名称", "用户邮箱", "手机号码", "用户性别", "账号状态"}
		row := []any{"", "quota_import", "导入配额验收", "", "", "", ""}
		_ = book.SetSheetRow("Sheet1", "A1", &headers)
		_ = book.SetSheetRow("Sheet1", "A2", &row)
		encoded, err := book.WriteToBuffer()
		if err != nil {
			t.Fatal(err)
		}
		imported := multipartRequest(&s, "/system/user/importData?updateSupport=false", f.owner, "file", []string{"quota.xlsx"}, [][]byte{encoded.Bytes()})
		if imported["code"] == float64(200) || !strings.Contains(fmt.Sprint(imported["msg"]), "上限") {
			t.Fatal("Excel import bypassed account quota or did not reach quota validation")
		}
		users := s.req("GET", "/system/user/list?userName=apiq_", f.owner, nil)
		s.ok(users)
		deleteID := int64(users["rows"].([]any)[0].(map[string]any)["userId"].(float64))
		s.ok(s.req("DELETE", fmt.Sprintf("/system/user/%d", deleteID), f.owner, nil))
		s.ok(s.req("POST", "/system/user", f.owner, map[string]any{"userName": "apiq_reuse", "nickName": "释放名额复用", "password": operationsPassword, "status": "0"}))
		var fileWG sync.WaitGroup
		uploads := make(chan map[string]any, 6)
		for i := 0; i < 6; i++ {
			fileWG.Add(1)
			go func(i int) {
				defer fileWG.Done()
				var data bytes.Buffer
				w := multipart.NewWriter(&data)
				part, _ := w.CreateFormFile("file", fmt.Sprintf("quota-%d.txt", i))
				_, _ = io.WriteString(part, "123456")
				_ = w.Close()
				r, _ := http.NewRequest("POST", s.base+"/common/upload", &data)
				r.Header.Set("Authorization", "Bearer "+f.owner)
				r.Header.Set("Content-Type", w.FormDataContentType())
				response, err := s.client.Do(r)
				if err != nil {
					uploads <- map[string]any{"code": float64(0)}
					return
				}
				defer response.Body.Close()
				var value map[string]any
				_ = json.NewDecoder(response.Body).Decode(&value)
				uploads <- value
			}(i)
		}
		fileWG.Wait()
		close(uploads)
		files := 0
		for value := range uploads {
			if value["code"] == float64(200) {
				files++
			}
		}
		if files != 1 {
			t.Fatal("concurrent attachment quota must accept only one 6-byte file")
		}
		if quota()["usage"].(map[string]any)["storageBytes"] != float64(6) {
			t.Fatal("failed uploads leaked quota metadata")
		}
		oversize := multipartRequest(&s, "/common/upload", f.owner, "file", []string{"large.txt"}, [][]byte{[]byte("123456789")})
		if oversize["code"] == float64(200) {
			t.Fatal("single-file quota bypassed")
		}
		png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jw1sAAAAASUVORK5CYII=")
		avatar := multipartRequest(&s, "/system/user/profile/avatar", f.owner, "avatarfile", []string{"avatar.png"}, [][]byte{png})
		if avatar["code"] == float64(200) {
			t.Fatal("avatar bypassed per-file quota")
		}
		set(5, 12, 8)
		batch := multipartRequest(&s, "/common/uploads", f.owner, "files", []string{"part-a.txt", "part-b.txt"}, [][]byte{[]byte("1234"), []byte("5678")})
		if batch["code"] == float64(200) || quota()["usage"].(map[string]any)["storageBytes"] != float64(6) {
			t.Fatal("failed batch did not roll back its earlier file metadata")
		}
		state := quota()
		bad := s.req("PUT", quotaPath, p, map[string]any{"version": state["version"], "confirmTenantCode": f.code, "reason": "降额负例", "maxUsers": 4, "maxStorageBytes": 5, "maxUploadBytes": 8})
		if bad["code"] == float64(200) {
			t.Fatal("quota lowered below committed usage")
		}
		set(5, 0, 8)
		t.Run("quota_edit_and_attachment_registration_share_lock_order", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			platform, err := repository.ResolveTenant(ctx, "platform")
			if err != nil {
				t.Fatal(err)
			}
			platformCtx := tenantctx.With(ctx, platform.TenantID, true)
			tenantCtx := tenantctx.With(ctx, f.tenant.TenantID, false)
			actor := model.PlatformActor{TenantID: platform.TenantID, UserID: *platform.OwnerUserID, UserName: "admin", IP: "127.0.0.1"}
			errors := make(chan error, 24)
			var workers sync.WaitGroup
			workers.Add(2)
			go func() {
				defer workers.Done()
				for i := 0; i < 12; i++ {
					q, err := repository.CustomerQuotaSettings(platformCtx, f.tenant.TenantID)
					if err == nil {
						err = repository.UpdateCustomerQuota(platformCtx, f.tenant.TenantID, q.Version, f.code, "并发锁顺序验收", model.TenantPolicy{MaxUsers: 5, MaxStorageBytes: 0, MaxUploadBytes: 8}, actor)
					}
					errors <- err
				}
			}()
			go func() {
				defer workers.Done()
				for i := 0; i < 12; i++ {
					key := fmt.Sprintf("/profile/tenants/%d/upload/lock-check-%d.txt", f.tenant.TenantID, i)
					errors <- repository.InsertFile(tenantCtx, &model.SysFile{StorageKey: key, OriginalName: "lock-check.txt", SizeBytes: 3, UploaderUserID: *f.tenant.OwnerUserID})
				}
			}()
			workers.Wait()
			close(errors)
			// These rows test metadata locking, not filesystem IO. Remove only their
			// exact fixture keys; keep the real uploaded attachment and audit records.
			for i := 0; i < 12; i++ {
				_ = repository.DeleteFileRecord(tenantCtx, fmt.Sprintf("/profile/tenants/%d/upload/lock-check-%d.txt", f.tenant.TenantID, i))
			}
			for err := range errors {
				if err != nil {
					t.Fatal("quota editor/attachment registrar failed under concurrent lock order")
				}
			}
		})
		f.owner = s.loginAs(f.code, "owner", operationsPassword)
		if s.req("GET", quotaPath, other, nil)["code"] != float64(403) {
			t.Fatal("customer read another tenant's quota")
		}
		workspace := s.req("GET", "/tenant/workspace?tenantId=10000", f.owner, nil)
		s.ok(workspace)
		if workspace["data"].(map[string]any)["tenantId"] != float64(f.tenant.TenantID) {
			t.Fatal("workspace query accepted injected tenant ID")
		}
		s.ok(s.req("GET", "/getInfo", other, nil))
	})
	t.Run("cancellation_archive_restore_preserves_data", func(t *testing.T) {
		s := *parent
		s.t = t
		f := newOperationsFixture(t, &s, p)
		detailPath := fmt.Sprintf("/platform/tenant/%d", f.tenant.TenantID)
		lifePath := detailPath + "/lifecycle"
		s.ok(s.req("POST", "/system/user", f.owner, map[string]any{"userName": "member", "nickName": "归档验收员工", "password": operationsPassword, "status": "0"}))
		member := s.loginAs(f.code, "member", operationsPassword)
		upload := multipartRequest(&s, "/common/upload", f.owner, "file", []string{"retained.txt"}, [][]byte{[]byte("retained")})
		s.ok(upload)
		resource := upload["fileName"].(string)
		current := func() map[string]any {
			v := s.req("GET", detailPath, p, nil)
			s.ok(v)
			return v["data"].(map[string]any)
		}
		body := func(action string) map[string]any {
			return map[string]any{"version": current()["version"], "confirmTenantCode": f.code, "action": action, "reason": "生命周期验收，保留数据"}
		}
		if s.req("POST", "/tenant/cancellation", member, body("requestCancellation"))["code"] != float64(403) {
			t.Fatal("ordinary employee could request cancellation")
		}
		s.ok(s.req("POST", "/tenant/cancellation", f.owner, body("requestCancellation")))
		if current()["status"] != "3" {
			t.Fatal("cancellation did not freeze workspace")
		}
		if s.req("GET", "/getInfo", f.owner, nil)["code"] != float64(401) {
			t.Fatal("cancellation retained old session")
		}
		s.ok(s.req("PUT", lifePath, p, body("archive")))
		if current()["status"] != "4" {
			t.Fatal("archive transition failed")
		}
		if s.updateTenant(p, map[string]any{"tenantId": f.tenant.TenantID, "tenantCode": f.code, "tenantName": f.tenant.TenantName, "status": "0"})["code"] == float64(200) {
			t.Fatal("ordinary update bypassed archive restore")
		}
		ctx, cancel := context.WithTimeout(tenantctx.With(context.Background(), 1, true), 5*time.Second)
		defer cancel()
		usage, err := repository.CustomerQuotaSettings(ctx, f.tenant.TenantID)
		if err != nil || usage.Usage.UserCount != 2 || usage.Usage.StorageBytes != 8 {
			t.Fatal("archiving deleted tenant data")
		}
		s.ok(s.req("PUT", lifePath, p, body("restore")))
		if current()["status"] != "1" {
			t.Fatal("restore must remain disabled")
		}
		if s.req("POST", "/login", "", map[string]any{"tenantCode": f.code, "username": "owner", "password": operationsPassword})["code"] == float64(200) {
			t.Fatal("restored tenant was auto-enabled")
		}
		s.ok(s.updateTenant(p, map[string]any{"tenantId": f.tenant.TenantID, "tenantCode": f.code, "tenantName": f.tenant.TenantName, "status": "0"}))
		if s.req("GET", "/getInfo", f.owner, nil)["code"] != float64(401) {
			t.Fatal("old pre-archive session resurrected")
		}
		fresh := s.loginAs(f.code, "owner", operationsPassword)
		s.ok(s.req("GET", "/system/user/list", fresh, nil))
		r, _ := http.NewRequest("GET", s.base+resource, nil)
		r.Header.Set("Authorization", "Bearer "+fresh)
		res, err := s.client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		data, _ := io.ReadAll(res.Body)
		if string(data) != "retained" {
			t.Fatal("restored tenant lost its attachment")
		}
		s.ok(s.req("GET", "/getInfo", other, nil))
	})
}
