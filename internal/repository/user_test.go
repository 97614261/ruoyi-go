package repository

import (
	"testing"

	"ruoyi-go/internal/model"
)

func TestUserPageRowBuildsContractDept(t *testing.T) {
	deptID := int64(103)
	deptName := "研发部门"
	leader := "负责人"

	row := userPageRow{
		SysUser:          model.SysUser{UserID: 7, DeptID: &deptID},
		JoinedDeptID:     &deptID,
		JoinedDeptName:   &deptName,
		JoinedDeptLeader: &leader,
	}
	user := row.user()
	if user.Dept == nil {
		t.Fatal("分页 JOIN 命中部门时必须填充用户的部门对象")
	}
	if user.Dept.DeptID != deptID || user.Dept.DeptName != deptName ||
		user.Dept.Leader == nil || *user.Dept.Leader != leader {
		t.Fatalf("分页部门字段映射错误：%#v", user.Dept)
	}
}

func TestUserPageRowKeepsMissingDeptEmpty(t *testing.T) {
	missingDeptID := int64(999999)
	user := (userPageRow{SysUser: model.SysUser{UserID: 8, DeptID: &missingDeptID}}).user()
	if user.DeptID == nil || *user.DeptID != missingDeptID {
		t.Fatalf("LEFT JOIN 未命中时不能改写用户的 deptId：%v", user.DeptID)
	}
	if user.Dept != nil {
		t.Fatalf("LEFT JOIN 未命中时不应伪造部门对象：%#v", user.Dept)
	}
}
