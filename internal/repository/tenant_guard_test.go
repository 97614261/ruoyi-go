package repository

import (
	"context"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/tenantctx"
	"strings"
	"testing"
)

func guardDB(t *testing.T) *gorm.DB {
	t.Helper()
	g, err := gorm.Open(mysql.New(mysql.Config{DSN: "root@tcp(127.0.0.1:3306)/dryrun", SkipInitializeWithVersion: true}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = registerTenantGuard(g); err != nil {
		t.Fatal(err)
	}
	return g
}
func TestTenantGuardRejectsMissingContext(t *testing.T) {
	g := guardDB(t)
	var rows []model.SysUser
	if err := g.Find(&rows).Error; err == nil {
		t.Fatal("missing identity accepted")
	}
}

func TestTenantPolicyCannotBeWrittenOrGeneratedByCustomer(t *testing.T) {
	g := guardDB(t).WithContext(tenantctx.With(context.Background(), 10, false))
	if err := g.Create(&model.TenantPolicy{MaxUsers: 0}).Error; err == nil {
		t.Fatal("customer created an unlimited policy")
	}
	if err := g.Model(&model.TenantPolicy{}).Updates(map[string]any{"max_users": 0}).Error; err == nil {
		t.Fatal("customer changed its quota")
	}
	if err := TenantTable(g, "sys_tenant_policy").Error; err == nil {
		t.Fatal("generator accepted platform policy table")
	}
}
func TestTenantGuardAliasesJoinsAndOr(t *testing.T) {
	g := guardDB(t).WithContext(tenantctx.With(context.Background(), 10, false))
	var rows []model.SysUser
	q := g.Table("sys_user u").Joins("LEFT JOIN sys_dept d ON d.dept_id = u.dept_id").Where("u.user_name = ?", "none").Or("1=1").Find(&rows)
	if q.Error != nil {
		t.Fatal(q.Error)
	}
	sql := q.Statement.SQL.String()
	if !strings.Contains(sql, "`u`.`tenant_id` = ?") || !strings.Contains(sql, "d.tenant_id = 10 AND") || !strings.Contains(sql, "(u.user_name = ? OR 1=1)") {
		t.Fatalf("tenant guard missing or OR bypass possible: %s", sql)
	}
}
func TestTenantGuardCreateAssignsAndRejectsForeignTenant(t *testing.T) {
	g := guardDB(t).WithContext(tenantctx.With(context.Background(), 10, false))
	u := model.SysUser{UserName: "local"}
	if err := g.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	if u.TenantID != 10 {
		t.Fatal("tenant not assigned")
	}
	foreign := model.SysUser{TenantRecord: model.TenantRecord{TenantID: 20}, UserName: "foreign"}
	if err := g.Create(&foreign).Error; err == nil {
		t.Fatal("foreign tenant write accepted")
	}
}
func TestTenantGuardPlatformIsNotBusinessBypass(t *testing.T) {
	g := guardDB(t).WithContext(tenantctx.With(context.Background(), 1, true))
	var rows []model.SysUser
	q := g.Find(&rows)
	if q.Error != nil {
		t.Fatal(q.Error)
	}
	if !strings.Contains(q.Statement.SQL.String(), "tenant_id") {
		t.Fatal("platform bypassed business filter")
	}
}
func TestTenantGuardRejectsTenantMutationAndGlobalWrite(t *testing.T) {
	g := guardDB(t).WithContext(tenantctx.With(context.Background(), 10, false))
	if err := g.Model(&model.SysUser{}).Where("user_id=7").Updates(map[string]any{"tenant_id": 20}).Error; err == nil {
		t.Fatal("tenant migration accepted")
	}
	if err := g.Create(&model.SysMenu{MenuName: "foreign"}).Error; err == nil {
		t.Fatal("customer wrote platform menu")
	}
	if err := g.Exec("TRUNCATE TABLE sys_job_log").Error; err == nil {
		t.Fatal("customer raw SQL accepted")
	}
}
func TestTenantMenuReadsRespectEntitlements(t *testing.T) {
	g := guardDB(t).WithContext(tenantctx.With(context.Background(), 10, false))
	var rows []model.SysMenu
	q := g.Find(&rows)
	if q.Error != nil {
		t.Fatal(q.Error)
	}
	if !strings.Contains(q.Statement.SQL.String(), "sys_tenant_menu WHERE tenant_id = ?") {
		t.Fatal("tenant menu whitelist missing")
	}
}

func TestGeneratedBusinessScalarAndMapQueriesStayScoped(t *testing.T) {
	g := guardDB(t).WithContext(tenantctx.With(context.Background(), 10, false))
	var count int64
	q := TenantTable(g, "biz_order").Count(&count)
	if q.Error != nil {
		t.Fatal(q.Error)
	}
	if !strings.Contains(q.Statement.SQL.String(), "`biz_order`.`tenant_id` = ?") {
		t.Fatal("scalar business query missed tenant scope")
	}
	q = TenantTable(g, "biz_order").Where("order_id=7").Updates(map[string]any{"order_name": "changed"})
	if q.Error != nil {
		t.Fatal(q.Error)
	}
	if !strings.Contains(q.Statement.SQL.String(), "tenant_id") {
		t.Fatal("map update missed tenant scope")
	}
}

func TestTenantJoinOrIsGroupedAndUnknownJoinsFailClosed(t *testing.T) {
	g := guardDB(t).WithContext(tenantctx.With(context.Background(), 10, false))
	var rows []model.SysUser
	q := g.Table("sys_user u").Joins("LEFT JOIN sys_dept d ON d.dept_id=u.dept_id OR d.status='0'").Find(&rows)
	if q.Error != nil {
		t.Fatal(q.Error)
	}
	if !strings.Contains(q.Statement.SQL.String(), "d.tenant_id = 10 AND (d.dept_id=u.dept_id OR d.status='0')") {
		t.Fatal("join OR escaped tenant constraint")
	}
	q = g.Table("sys_user u").Joins("LEFT JOIN unknown_table x ON x.id=u.user_id").Find(&rows)
	if q.Error == nil {
		t.Fatal("unknown join did not fail closed")
	}
}
