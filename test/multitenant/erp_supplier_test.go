package multitenant_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
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

func TestERPSuppliers(t *testing.T) {
	root := os.Getenv("MT_TEST_ROOT")
	if root == "" {
		t.Skip("explicit local supplier acceptance required")
	}
	cfg, err := config.Load(filepath.Join(root, "configs/multitenant.local.yml"))
	if err != nil {
		t.Fatal("local config unavailable")
	}
	dsn, err := mysql.ParseDSN(cfg.MySQL.DSN)
	if err != nil {
		t.Fatal("invalid DSN")
	}
	host, _, err := net.SplitHostPort(dsn.Addr)
	if err != nil || dsn.Net != "tcp" || dsn.DBName != repository.TenantDatabaseName || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		t.Fatal("original/remote DB refused")
	}
	if err = repository.Init(cfg.MySQL); err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	cache := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	defer cache.Close()
	s := &suite{t: t, base: "http://127.0.0.1:8081", root: root, client: &http.Client{Timeout: 20 * time.Second}, redis: cache, passwords: loadTestAccounts(t, root)}
	platform := s.login("platform")
	t.Cleanup(func() { s.req("POST", "/logout", platform, nil) })
	a, b := newOperationsFixture(t, s, platform), newOperationsFixture(t, s, platform)
	body := func(code string) map[string]any {
		return map[string]any{"supplierCode": code, "supplierName": "供应商验收", "status": "0", "contactName": "private-contact-test", "phone": "private-phone-test", "email": "supplier@example.test", "address": "private-address-test", "remark": "private-remark-test"}
	}
	deny := func(r map[string]any) {
		t.Helper()
		if r["code"] == float64(200) {
			t.Fatal("unexpected success")
		}
	}
	create := func(token, code string) int64 {
		t.Helper()
		r := s.req("POST", "/erp/supplier", token, body(code))
		s.ok(r)
		d := r["data"].(map[string]any)
		if len(d) != 2 || d["version"] != float64(1) {
			t.Fatal("write leaked details or client version")
		}
		return int64(d["supplierId"].(float64))
	}
	detail := func(token string, id int64) map[string]any {
		t.Helper()
		r := s.req("GET", fmt.Sprintf("/erp/supplier/%d", id), token, nil)
		s.ok(r)
		return r["data"].(map[string]any)
	}
	id, foreign := create(a.owner, "00001"), create(b.owner, "00001")
	role := func(key string, ids []int64) (int64, map[string]any) {
		t.Helper()
		input := map[string]any{"roleName": key, "roleKey": key, "roleSort": 0, "status": "0", "dataScope": "1", "menuIds": ids}
		s.ok(s.req("POST", "/system/role", a.owner, input))
		r := s.req("GET", "/system/role/list?roleKey="+key, a.owner, nil)
		s.ok(r)
		rid := int64(r["rows"].([]any)[0].(map[string]any)["roleId"].(float64))
		input["roleId"] = rid
		return rid, input
	}
	listRole, listRoleInput := role("supplier_list", []int64{6000, 7600})
	editRole, _ := role("supplier_editor", []int64{6000, 7600, 7601, 7602, 7603})
	user := func(name string, roleID int64) string {
		t.Helper()
		s.ok(s.req("POST", "/system/user", a.owner, map[string]any{"userName": name, "nickName": name, "password": operationsPassword, "status": "0", "roleIds": []int64{roleID}}))
		return s.loginAs(a.code, name, operationsPassword)
	}
	listOnly, editor := user("supplier-list", listRole), user("supplier-editor", editRole)
	ctx, cancel := context.WithTimeout(erpctx.WithUser(tenantctx.With(context.Background(), a.tenant.TenantID, false), *a.tenant.OwnerUserID), time.Minute)
	defer cancel()
	t.Run("tenant_platform_function_and_projection", func(t *testing.T) {
		if detail(a.owner, id)["contactName"] != "private-contact-test" {
			t.Fatal("detail lost contact")
		}
		deny(s.req("GET", fmt.Sprintf("/erp/supplier/%d", foreign), a.owner, nil))
		edit := body("00001")
		edit["version"] = 1
		deny(s.req("PUT", fmt.Sprintf("/erp/supplier/%d", foreign), a.owner, edit))
		for _, token := range []string{platform, ""} {
			deny(s.req("GET", "/erp/supplier/list", token, nil))
			deny(s.req("POST", "/erp/supplier", token, body("denied")))
		}
		list := s.req("GET", "/erp/supplier/list", listOnly, nil)
		s.ok(list)
		if list["data"] != nil || list["total"] != float64(1) {
			t.Fatal("shared enterprise list or shape incorrect")
		}
		row := list["rows"].([]any)[0].(map[string]any)
		for _, key := range []string{"tenantId", "contactName", "phone", "email", "address", "remark"} {
			if _, ok := row[key]; ok {
				t.Fatal("list leaked", key)
			}
		}
		deny(s.req("GET", fmt.Sprintf("/erp/supplier/%d", id), listOnly, nil))
		deny(s.req("POST", "/erp/supplier", listOnly, body("no-add")))
		deny(s.req("PUT", fmt.Sprintf("/erp/supplier/%d", id), listOnly, edit))
		forged := body("FORGED")
		forged["tenantId"] = b.tenant.TenantID
		forged["supplierId"] = foreign
		forged["version"] = 99
		forged["createBy"] = "forged"
		r := s.req("POST", "/erp/supplier", a.owner, forged)
		s.ok(r)
		newID := int64(r["data"].(map[string]any)["supplierId"].(float64))
		d := detail(a.owner, newID)
		if d["version"] != float64(1) || d["createBy"] != "owner" || newID == foreign {
			t.Fatal("server-controlled fields bypassed")
		}
		deny(s.req("GET", fmt.Sprintf("/erp/supplier/%d", newID), b.owner, nil))
	})
	t.Run("validation_optional_fields_duplicate_and_types", func(t *testing.T) {
		for key, values := range map[string][]any{"supplierCode": {" ", "a/b", strings.Repeat("a", 65)}, "supplierName": {" ", strings.Repeat("中", 121)}, "contactName": {strings.Repeat("a", 65), "a\nb"}, "email": {"bad", "X <x@example.test>"}, "phone": {"a\nb"}, "address": {strings.Repeat("中", 501)}, "status": {"", nil, "2", 0}} {
			for _, value := range values {
				v := body("INVALID")
				v[key] = value
				deny(s.req("POST", "/erp/supplier", a.owner, v))
			}
		}
		s.ok(s.req("POST", "/erp/supplier", a.owner, map[string]any{"supplierCode": "OPTIONAL", "supplierName": "空联系人合法", "status": "0"}))
		deny(s.req("POST", "/erp/supplier", a.owner, body("00001")))
		for _, version := range []any{nil, 0, -1, 1.5, "1"} {
			v := body("00001")
			v["version"] = version
			deny(s.req("PUT", fmt.Sprintf("/erp/supplier/%d", id), a.owner, v))
		}
		for _, bad := range []string{"0", "-1", "bad", "1.1"} {
			deny(s.req("GET", "/erp/supplier/"+bad, a.owner, nil))
		}
	})
	t.Run("immutable_code_enable_disable_CAS_and_employee_write", func(t *testing.T) {
		v := body("CHANGED")
		v["version"] = 1
		deny(s.req("PUT", fmt.Sprintf("/erp/supplier/%d", id), editor, v))
		v = body("00001")
		v["version"] = 1
		v["status"] = "1"
		r := s.req("PUT", fmt.Sprintf("/erp/supplier/%d", id), editor, v)
		s.ok(r)
		if len(r["data"].(map[string]any)) != 2 {
			t.Fatal("edit leaked contacts")
		}
		if d := detail(editor, id); d["version"] != float64(2) || d["status"] != "1" {
			t.Fatal("disable/CAS failed")
		}
		deny(s.req("PUT", fmt.Sprintf("/erp/supplier/%d", id), editor, v))
		results := parallelJSON(s, 2, func(i int) (string, string, string, any) {
			in := body("00001")
			in["version"] = 2
			in["status"] = "0"
			in["supplierName"] = fmt.Sprintf("CAS-%d", i)
			return "PUT", fmt.Sprintf("/erp/supplier/%d", id), editor, in
		})
		wins := 0
		for _, r := range results {
			if r.err != nil {
				t.Fatal(r.err)
			}
			if r.code == 200 {
				wins++
			}
		}
		if wins != 1 || detail(editor, id)["version"] != float64(3) {
			t.Fatal("one CAS winner required")
		}
		for _, pair := range [][2]string{{"DELETE", fmt.Sprintf("/erp/supplier/%d", id)}, {"POST", "/erp/supplier/export"}} {
			req, _ := http.NewRequest(pair[0], s.base+pair[1], nil)
			req.Header.Set("Authorization", "Bearer "+a.owner)
			res, e := s.client.Do(req)
			if e != nil {
				t.Fatal(e)
			}
			res.Body.Close()
			if res.StatusCode != 404 {
				t.Fatal("forbidden delete/export exposed")
			}
		}
	})
	t.Run("audit_failure_rolls_back_create_and_edit", func(t *testing.T) {
		db := repository.DB(ctx)
		fault := errors.New("injected supplier audit failure")
		hook := "supplier-test:audit-failure"
		if err := db.Callback().Create().Before("gorm:create").Register(hook, func(tx *gorm.DB) {
			if tx.Statement.Table == "sys_oper_log" {
				tx.AddError(fault)
			}
		}); err != nil {
			t.Fatal(err)
		}
		_, err := service.CreateErpSupplier(ctx, model.ERPSupplierWrite{SupplierCode: "FAULT-CREATE", SupplierName: "fault", Status: "0"}, "supplier-fault")
		if !errors.Is(err, fault) {
			db.Callback().Create().Remove(hook)
			t.Fatal("create audit failure not reached", err)
		}
		_, err = service.UpdateErpSupplier(ctx, id, model.ERPSupplierEdit{ERPSupplierWrite: model.ERPSupplierWrite{SupplierCode: "00001", SupplierName: "must rollback", Status: "1"}, Version: 3}, "supplier-fault")
		db.Callback().Create().Remove(hook)
		if !errors.Is(err, fault) {
			t.Fatal("edit audit failure not reached", err)
		}
		var n int64
		if err := repository.DB(ctx).Model(&model.ErpSupplier{}).Where("supplier_code='FAULT-CREATE'").Count(&n).Error; err != nil || n != 0 {
			t.Fatal("failed create persisted", err)
		}
		if d := detail(a.owner, id); d["version"] != float64(3) || d["supplierName"] == "must rollback" || d["phone"] != "private-phone-test" {
			t.Fatal("failed edit partially committed")
		}
	})
	t.Run("pagination_sort_privacy_headers_and_audit", func(t *testing.T) {
		r := s.req("GET", "/erp/supplier/list?pageSize=1&orderByColumn=status", a.owner, nil)
		s.ok(r)
		if len(r["rows"].([]any)) != 1 {
			t.Fatal("page limit")
		}
		r = s.req("GET", "/erp/supplier/list?pageSize=999999&orderByColumn=phone%20desc;DROP%20TABLE%20erp_supplier", a.owner, nil)
		s.ok(r)
		if len(r["rows"].([]any)) > 100 {
			t.Fatal("page cap")
		}
		var logs []model.SysOperLog
		if err := repository.DB(ctx).Where("method LIKE 'erp.supplier.%'").Find(&logs).Error; err != nil || len(logs) < 3 {
			t.Fatal("missing atomic audits", err)
		}
		raw, _ := json.Marshal(logs)
		for _, value := range []string{"private-contact-test", "private-phone-test", "supplier@example.test", "private-address-test", "private-remark-test"} {
			if strings.Contains(string(raw), value) {
				t.Fatal("PII in audit")
			}
		}
		for _, path := range []string{"/erp/supplier/list", fmt.Sprintf("/erp/supplier/%d", id)} {
			req, _ := http.NewRequest("GET", s.base+path, nil)
			req.Header.Set("Authorization", "Bearer "+a.owner)
			res, err := s.client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			if res.Header.Get("Cache-Control") != "no-store" {
				t.Fatal("private response cacheable")
			}
		}
	})
	t.Run("old_session_loses_revoked_function", func(t *testing.T) {
		listRoleInput["menuIds"] = []int64{6000}
		s.ok(s.req("PUT", "/system/role", a.owner, listRoleInput))
		deny(s.req("GET", "/erp/supplier/list", listOnly, nil))
	})
}
