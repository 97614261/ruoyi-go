package repository

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/tenantctx"
)

func TestTenantActionsRollBackWhenSuccessAuditFails(t *testing.T) {
	root := os.Getenv("MT_TEST_ROOT")
	if root == "" {
		t.Skip("explicit dedicated local database only")
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
	if err != nil || dsn.DBName != TenantDatabaseName || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		t.Fatal("refusing original or remote database")
	}
	if err = Init(cfg.MySQL); err != nil {
		t.Fatal(err)
	}
	defer Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	p, err := ResolveTenant(ctx, "platform")
	if err != nil || p == nil || p.OwnerUserID == nil {
		t.Fatal("platform fixture unavailable")
	}
	platformCtx := tenantctx.With(ctx, p.TenantID, true)
	actor := model.PlatformActor{TenantID: p.TenantID, UserID: *p.OwnerUserID, UserName: "admin", IP: "127.0.0.1"}
	hash, _ := bcrypt.GenerateFromPassword([]byte("Fault_Check_782!"), bcrypt.DefaultCost)
	newHash, _ := bcrypt.GenerateFromPassword([]byte("Fault_New_927!"), bcrypt.DefaultCost)
	fixture := &model.SysTenant{TenantCode: fmt.Sprintf("fault-%d", time.Now().UnixNano()%1000000000000), TenantName: "审计回滚验收", CreateBy: "acceptance"}
	if err = ProvisionTenant(platformCtx, fixture, "owner", string(hash)); err != nil {
		t.Fatal(err)
	}
	candidate := &model.SysUser{UserName: "candidate", NickName: "交接回滚验收", UserType: "00", Password: string(hash), Status: "0", DelFlag: "0"}
	if err = InsertUser(tenantctx.With(ctx, fixture.TenantID, false), candidate); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = db.Callback().Create().Remove("acceptance:audit-failure")
		_ = DB(tenantctx.System(ctx)).Model(&model.SysTenant{}).Where("tenant_id=?", fixture.TenantID).Update("status", "1").Error
	}()
	failure := errors.New("injected audit failure")
	if err = db.Callback().Create().Before("gorm:create").Register("acceptance:audit-failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "sys_oper_log" {
			tx.AddError(failure)
		}
	}); err != nil {
		t.Fatal(err)
	}
	_, _, before, err := CustomerTenantMenuState(platformCtx, fixture.TenantID, 2000)
	if err != nil {
		t.Fatal(err)
	}
	operations := []struct {
		name string
		run  func() error
	}{
		{"handoff", func() error {
			return UpdateTenant(platformCtx, fixture.TenantID, "不应提交的名称", "0", fixture.ExpireTime, candidate.UserID, actor, 1)
		}},
		{"grant", func() error {
			return ReplaceCustomerTenantMenus(platformCtx, fixture.TenantID, 1, fixture.TenantCode, []int64{}, "audit fault", actor)
		}},
		{"password", func() error {
			return ResetCustomerOwnerPassword(platformCtx, fixture.TenantID, 1, *fixture.OwnerUserID, fixture.TenantCode, string(newHash), "audit fault", actor)
		}},
		{"quota", func() error {
			return UpdateCustomerQuota(platformCtx, fixture.TenantID, 1, fixture.TenantCode, "audit fault", model.TenantPolicy{MaxUsers: 2, MaxStorageBytes: 1024, MaxUploadBytes: 512}, actor)
		}},
		{"lifecycle", func() error {
			return ChangeCustomerLifecycle(platformCtx, fixture.TenantID, 1, fixture.TenantCode, "requestCancellation", "audit fault", actor)
		}},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			if err := operation.run(); !errors.Is(err, failure) {
				t.Fatal("operation did not fail on transactional audit")
			}
			current, _, keys, err := CustomerTenantMenuState(platformCtx, fixture.TenantID, 2000)
			if err != nil || current.Version != 1 || current.Status != "0" || current.TenantName != fixture.TenantName || current.OwnerUserID == nil || *current.OwnerUserID != *fixture.OwnerUserID || len(keys) != len(before) {
				t.Fatal("partial tenant change escaped rollback")
			}
			user, err := SelectUserByID(tenantctx.With(ctx, fixture.TenantID, false), *fixture.OwnerUserID)
			if err != nil || user.Password != string(hash) || user.UserType != "01" {
				t.Fatal("password change escaped rollback")
			}
			quota, err := CustomerQuotaSettings(platformCtx, fixture.TenantID)
			if err != nil || quota.Policy.MaxUsers != 100 {
				t.Fatal("quota change escaped rollback")
			}
		})
	}
}
