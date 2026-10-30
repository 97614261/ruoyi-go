package portaltest

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// Explicit recovery of this suite's interrupted fixtures; never selected by the normal test run.
func TestCMSCleanupInterruptedFixtures(t *testing.T) {
	if !strings.EqualFold(os.Getenv("PORTAL_CLEANUP_INTERRUPTED"), "1") {
		t.Skip("explicit interrupted-fixture cleanup only")
	}
	c := cmsSetup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, e := c.db.QueryContext(ctx, `SELECT u.user_id,u.user_name,r.role_id,r.role_key FROM sys_user u JOIN sys_user_role ur ON ur.user_id=u.user_id JOIN sys_role r ON r.role_id=ur.role_id WHERE u.user_name LIKE 'cv%' AND u.nick_name='CMS只读测试' AND u.create_by='admin' AND u.create_time>='2026-10-29 20:30:00' AND u.status='0' AND r.role_key LIKE 'cms_%'`)
	if e != nil {
		t.Fatal(e)
	}
	type fixture struct {
		userID  int64
		name    string
		roleID  int64
		roleKey string
	}
	fixtures := []fixture{}
	for rows.Next() {
		var f fixture
		if e = rows.Scan(&f.userID, &f.name, &f.roleID, &f.roleKey); e != nil {
			rows.Close()
			t.Fatal(e)
		}
		fixtures = append(fixtures, f)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		t.Fatal(e)
	}
	for _, f := range fixtures {
		if f.userID <= 2 || f.roleID <= 2 || len(f.name) > 20 || !strings.HasPrefix(f.roleKey, "cms_") {
			t.Fatal("fixture ownership mismatch")
		}
		c.ok("PUT", "/system/user", map[string]any{"userId": f.userID, "userName": f.name, "nickName": "CMS测试停用", "deptId": 103, "status": "1", "roleIds": []int64{}, "postIds": []int64{}})
		c.ok("PUT", "/system/role", map[string]any{"roleId": f.roleID, "roleName": f.roleKey, "roleKey": f.roleKey, "roleSort": 1, "status": "1", "dataScope": "1", "menuIds": []int64{}, "menuCheckStrictly": true, "deptCheckStrictly": true})
		var access int
		if e = c.db.QueryRowContext(ctx, "SELECT (SELECT COUNT(*) FROM sys_user_role WHERE user_id=?)+(SELECT COUNT(*) FROM sys_role_menu WHERE role_id=?)", f.userID, f.roleID).Scan(&access); e != nil || access != 0 {
			t.Fatal("fixture permissions not revoked")
		}
		t.Logf("disabled interrupted CMS fixture user %d / role %d; scopes zero", f.userID, f.roleID)
	}
}
