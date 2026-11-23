package podtest

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/redisx"
	"ruoyi-go/pkg/types"
)

func exercisePODAdmin(t *testing.T, ctx context.Context, call func(string, string, string, any) map[string]json.RawMessage, tokens map[string]string) {
	expect := func(t *testing.T, p map[string]json.RawMessage, want int) {
		t.Helper()
		if string(p["code"]) != strconv.Itoa(want) {
			t.Fatalf("business code %s want %d", p["code"], want)
		}
	}
	admin, creator := tokens["admin"], tokens["pod_creator"]
	t.Run("authentication_and_creator_denial", func(t *testing.T) {
		for _, path := range []string{"/pod-admin/bootstrap", "/pod-admin/overview", "/pod-admin/audits", "/pod-admin/audits/1"} {
			expect(t, call("GET", path, "", nil), 401)
			expect(t, call("GET", path, creator, nil), 403)
		}
	})
	t.Run("admin_without_workspace_and_honest_capabilities", func(t *testing.T) {
		before, err := repository.PODAdminState(ctx)
		if err != nil {
			t.Fatal("read platform state before bootstrap failed")
		}
		p := call("GET", "/pod-admin/bootstrap", admin, nil)
		expect(t, p, 200)
		var view model.PODAdminBootstrap
		if json.Unmarshal(p["data"], &view) != nil {
			t.Fatal("bootstrap DTO invalid")
		}
		if view.SchemaVersion != "012" || view.Stage != 7 || view.ManagedEnabled != before.ManagedEnabled || !view.Capabilities.Overview || !view.Capabilities.Audits || !view.Capabilities.PrivateContent || !view.Capabilities.Tasks || !view.Capabilities.Usage || !view.Capabilities.Templates || !view.Capabilities.Models || !view.Capabilities.Creators {
			t.Fatal("capabilities advertise future functionality")
		}
		expect(t, call("GET", "/pod/bootstrap", admin, nil), 403)
		var asset model.PODAsset
		if err := repository.DB(ctx).Select("asset_id").Order("asset_id").Take(&asset).Error; err == nil {
			expect(t, call("GET", "/pod/assets/"+strconv.FormatInt(asset.AssetID, 10)+"/content", admin, nil), 403)
		}
		after, err := repository.PODAdminState(ctx)
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatal("read-only bootstrap changed persisted platform state")
		}
	})
	t.Run("overview_metadata_totals_and_date_limits", func(t *testing.T) {
		p := call("GET", "/pod-admin/overview", admin, nil)
		expect(t, p, 200)
		var view model.PODAdminOverview
		if json.Unmarshal(p["data"], &view) != nil {
			t.Fatal("overview DTO invalid")
		}
		var storage int64
		if err := repository.DB(ctx).Table("pod_asset_file").Select("COALESCE(SUM(byte_size),0)").Scan(&storage).Error; err != nil || storage != view.StorageBytes {
			t.Fatal("storage summary differs from retained files")
		}
		var creators int64
		if err := repository.DB(ctx).Table("pod_workspace").Where("status='0'").Count(&creators).Error; err != nil || creators != view.Creators {
			t.Fatal("creator summary invalid")
		}
		for _, term := range []string{"graph", "prompt", "receipt", "storageKey", "apiKey", "password", "endpoint"} {
			if strings.Contains(string(p["data"]), term) {
				t.Fatal("overview leaked private fields")
			}
		}
		for _, query := range []string{"beginDate=invalid", "beginDate=2000-01-01&endDate=2026-11-13", "beginDate=2026-11-13&endDate=2026-11-12", "endDate=2099-01-01"} {
			expect(t, call("GET", "/pod-admin/overview?"+query, admin, nil), 400)
		}
	})
	t.Run("audits_are_real_filtered_stable_and_safe", func(t *testing.T) {
		var fixture model.PODAdminAudit
		fixture = model.PODAdminAudit{ActorUserID: 1, ActionType: "admin.access", ResourceType: "platform", ResourceID: 1, BeforeJSON: json.RawMessage(`{"password":"never-return"}`), AfterJSON: json.RawMessage(`{"schemaVersion":"008","apiKey":"never-return","prompt":"private-content"}`), TraceID: "admin-test-" + uuid.NewString(), Outcome: "success", CreateBy: "pod-admin-test", CreateTime: types.Now()}
		if err := repository.DB(ctx).Create(&fixture).Error; err != nil {
			t.Fatal("create audit fixture failed")
		}
		t.Cleanup(func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := repository.DB(cleanup).Where("audit_id=? AND create_by='pod-admin-test'", fixture.AuditID).Delete(&model.PODAdminAudit{}).Error; err != nil {
				t.Error("audit fixture cleanup failed")
			}
		})
		p := call("GET", "/pod-admin/audits?actorUserId=1&pageSize=999", admin, nil)
		expect(t, p, 200)
		var rows []model.PODAdminAuditView
		if json.Unmarshal(p["rows"], &rows) != nil || rows == nil || len(rows) > 100 || len(rows) < 1 {
			t.Fatal("audit pagination invalid")
		}
		for _, row := range rows {
			if row.ActorUserID != 1 || row.Before != nil || row.After != nil {
				t.Fatal("audit list ignores filter or loads snapshots")
			}
		}
		p1 := call("GET", "/pod-admin/audits?pageNum=1&pageSize=1", admin, nil)
		p2 := call("GET", "/pod-admin/audits?pageNum=2&pageSize=1", admin, nil)
		var first, second []model.PODAdminAuditView
		_ = json.Unmarshal(p1["rows"], &first)
		_ = json.Unmarshal(p2["rows"], &second)
		if len(first) != 1 || len(second) != 1 || first[0].AuditID == second[0].AuditID {
			t.Fatal("stable pagination repeated or lost an audit")
		}
		detail := call("GET", "/pod-admin/audits/"+strconv.FormatInt(fixture.AuditID, 10), admin, nil)
		expect(t, detail, 200)
		for _, term := range []string{"password", "apiKey", "prompt", "never-return", "private-content"} {
			if strings.Contains(string(detail["data"]), term) {
				t.Fatal("snapshot leaked fields outside allowlist")
			}
		}
		for _, query := range []string{"actorUserId=0", "ownerUserId=0001", "actionType=anything", "outcome=unknown", "beginDate=invalid"} {
			expect(t, call("GET", "/pod-admin/audits?"+query, admin, nil), 400)
		}
		expect(t, call("GET", "/pod-admin/audits/0001", admin, nil), 400)
		expect(t, call("GET", "/pod-admin/audits/9223372036854775807", admin, nil), 404)
		var denied int64
		if err := repository.DB(ctx).Table("pod_admin_audit").Where("outcome='denied'").Count(&denied).Error; err != nil || denied == 0 {
			t.Fatal("denied admin entry was not audited")
		}
	})
	t.Run("readonly_operator_and_live_revocation", func(t *testing.T) {
		var role model.SysRole
		if err := repository.DB(ctx).Where("role_key='pod_operator'").Take(&role).Error; err != nil {
			t.Fatal("operator role missing")
		}
		password := uuid.NewString()[:18]
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
		if err != nil {
			t.Fatal(err)
		}
		dept := int64(100)
		user := model.SysUser{DeptID: &dept, UserName: "poda_" + uuid.NewString()[:8], NickName: "管理边界测试", UserType: "00", Password: string(hash), Status: "0", DelFlag: "0", PwdUpdateDate: types.Now(), CreateBy: "pod-admin-test", CreateTime: types.Now()}
		if err := repository.DB(ctx).Create(&user).Error; err != nil {
			t.Fatal("operator fixture creation failed")
		}
		t.Cleanup(func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = service.RefreshOnlineUsersByID(cleanup, user.UserID)
			if err := repository.DB(cleanup).Exec("DELETE FROM sys_user_role WHERE user_id=?", user.UserID).Error; err != nil {
				t.Error(err)
			}
			if err := repository.DB(cleanup).Where("user_id=? AND create_by='pod-admin-test'", user.UserID).Delete(&model.SysUser{}).Error; err != nil {
				t.Error(err)
			}
		})
		if err := repository.DB(ctx).Exec("INSERT INTO sys_user_role(user_id,role_id) VALUES(?,?)", user.UserID, role.RoleID).Error; err != nil {
			t.Fatal(err)
		}
		captchaID := "pod-admin-test-" + uuid.NewString()
		if err := redisx.C().Set(ctx, redisx.CaptchaKey(captchaID), "2468", time.Minute).Err(); err != nil {
			t.Fatal(err)
		}
		p := call("POST", "/login", "", model.LoginBody{Username: user.UserName, Password: password, UUID: captchaID, Code: "2468"})
		expect(t, p, 200)
		var token string
		_ = json.Unmarshal(p["token"], &token)
		if token == "" {
			t.Fatal("operator login failed")
		}
		defer call("POST", "/logout", token, nil)
		p = call("GET", "/pod-admin/bootstrap", token, nil)
		expect(t, p, 200)
		var bootstrap model.PODAdminBootstrap
		_ = json.Unmarshal(p["data"], &bootstrap)
		for _, permission := range bootstrap.Permissions {
			if strings.HasPrefix(permission, "system:") || permission == "podAdmin:model:edit" || permission == "podAdmin:private:read" || permission == model.PODAdminAuditList {
				t.Fatal("operator received excess permission")
			}
		}
		expect(t, call("GET", "/pod-admin/overview", token, nil), 200)
		for _, path := range []string{"/pod-admin/runs", "/pod-admin/batches", "/pod-admin/usage"} {
			expect(t, call("GET", path, token, nil), 200)
		}
		for _, path := range []string{"/pod-admin/private-grants", "/pod-admin/private/runs/1", "/pod-admin/catalog/templates"} {
			expect(t, call("GET", path, token, nil), 403)
		}
		expect(t, call("POST", "/pod-admin/runs/1/cancel", token, model.PODOperationWrite{RequestID: uuid.NewString(), Reason: "readonly fixture"}), 403)
		expect(t, call("GET", "/pod-admin/audits", token, nil), 403)
		expect(t, call("GET", "/system/user/list", token, nil), 403)
		expect(t, call("GET", "/pod/bootstrap", token, nil), 403)
		if err := repository.DB(ctx).Exec("DELETE FROM sys_user_role WHERE user_id=? AND role_id=?", user.UserID, role.RoleID).Error; err != nil {
			t.Fatal(err)
		}
		// Emulate a cold script cache for this fixture only. Do not SCRIPT FLUSH:
		// that would affect unrelated scripts on the development Redis instance.
		redisx.C().AddHook(coldAdminPermissionHook{key: redisx.LoginUserPermissionVersionKey(user.UserID)})
		if err := service.RefreshOnlineUsersByID(ctx, user.UserID); err != nil {
			t.Fatalf("permission revocation failed: %v", err)
		}
		expect(t, call("GET", "/pod-admin/bootstrap", token, nil), 403)
		expect(t, call("GET", "/pod-admin/overview", token, nil), 403)
	})
}

type coldAdminPermissionHook struct{ key string }

func (h coldAdminPermissionHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) { return next(ctx, network, addr) }
}
func (h coldAdminPermissionHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook { return next }
func (h coldAdminPermissionHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, commands []redis.Cmder) error {
		for _, cmd := range commands {
			args := cmd.Args()
			if len(args) >= 4 && args[0] == "evalsha" && args[3] == h.key {
				err := errors.New("NOSCRIPT No matching script. Please use EVAL.")
				cmd.SetErr(err)
				return err
			}
		}
		return next(ctx, commands)
	}
}
