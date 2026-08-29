package service

import (
	"context"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"ruoyi-go/internal/datascope"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/types"
)

// 部门相关的权限标识，数据权限按权限维度过滤角色时要用。
const (
	PermDeptList = "system:dept:list"
)

// deptScope 构造部门查询的数据权限过滤器。
func deptScope(user *model.SysUser, permission string) func(*gorm.DB) *gorm.DB {
	return datascope.Scope(datascope.Params{
		User:       user,
		DeptAlias:  repository.DeptTableAlias,
		Permission: permission,
	})
}

// ListDepts 查部门列表（带数据权限）。
func ListDepts(ctx context.Context, user *model.SysUser, query model.DeptQuery) ([]model.SysDept, error) {
	return repository.SelectDeptList(ctx, query, deptScope(user, PermDeptList))
}

// ListDeptsExcludeChild 查部门列表并排除指定部门及其子孙。
//
// 用于"修改部门"时的上级部门下拉：不能把自己或自己的下级选成上级，
// 否则会形成环。
func ListDeptsExcludeChild(ctx context.Context, user *model.SysUser, deptID int64) ([]model.SysDept, error) {
	list, err := ListDepts(ctx, user, model.DeptQuery{})
	if err != nil {
		return nil, err
	}

	excluded := make([]model.SysDept, 0, len(list))
	target := strconv.FormatInt(deptID, 10)
	for _, dept := range list {
		if dept.DeptID == deptID {
			continue
		}
		// ancestors 形如 "0,100,101"，按逗号切分精确匹配，
		// 不能用 strings.Contains —— "10" 会误命中 "100"
		if containsID(dept.Ancestors, target) {
			continue
		}
		excluded = append(excluded, dept)
	}
	return excluded, nil
}

func containsID(ancestors, id string) bool {
	for _, part := range strings.Split(ancestors, ",") {
		if strings.TrimSpace(part) == id {
			return true
		}
	}
	return false
}

// GetDept 查部门详情，并校验数据权限。
func GetDept(ctx context.Context, user *model.SysUser, deptID int64) (*model.SysDept, error) {
	if err := CheckDeptDataScope(ctx, user, deptID); err != nil {
		return nil, err
	}
	dept, err := repository.SelectDeptByID(ctx, deptID)
	if err != nil {
		return nil, err
	}
	if dept == nil {
		return nil, errs.New("部门不存在")
	}
	return dept, nil
}

// CheckDeptDataScope 校验当前用户是否有权访问该部门。
//
// 对齐 Java 版 checkDeptDataScope：超级管理员跳过；
// 其余用户拿数据权限查一次，查不到就拒绝。
func CheckDeptDataScope(ctx context.Context, user *model.SysUser, deptID int64) error {
	if user == nil || user.IsAdmin() || deptID == 0 {
		return nil
	}
	list, err := repository.SelectDeptList(ctx,
		model.DeptQuery{DeptID: deptID}, deptScope(user, PermDeptList))
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return errs.New("没有权限访问部门数据！")
	}
	return nil
}

// CreateDept 新增部门。
func CreateDept(ctx context.Context, user *model.SysUser, dept *model.SysDept, operator string) error {
	if _, err := checkDeptIDs(ctx, user, []int64{dept.ParentID}); err != nil {
		return err
	}
	count, err := repository.CountDeptByNameAndParent(ctx, dept.DeptName, dept.ParentID, 0)
	if err != nil {
		return err
	}
	if count > 0 {
		return errs.Newf("新增部门'%s'失败，部门名称已存在", dept.DeptName)
	}

	parent, err := repository.SelectDeptByID(ctx, dept.ParentID)
	if err != nil {
		return err
	}
	if parent == nil {
		return errs.New("上级部门不存在")
	}
	if parent.Status != model.DeptNormal {
		return errs.New("部门停用，不允许新增")
	}

	dept.DeptID = 0
	dept.Ancestors = parent.Ancestors + "," + strconv.FormatInt(parent.DeptID, 10)
	dept.DelFlag = model.DelFlagExist
	dept.CreateBy = operator
	dept.CreateTime = types.Now()
	return repository.InsertDept(ctx, dept)
}

// UpdateDept 修改部门。
//
// 改了上级部门就要同步重算自己和所有子孙的 ancestors；
// 把部门改成正常状态时，其所有上级也要一并启用（对齐 Java）。
func UpdateDept(ctx context.Context, user *model.SysUser, dept *model.SysDept, operator string) error {
	if dept.DeptID == 0 {
		return errs.New("部门ID不能为空")
	}
	if err := CheckDeptDataScope(ctx, user, dept.DeptID); err != nil {
		return err
	}
	if dept.ParentID != 0 {
		if _, err := checkDeptIDs(ctx, user, []int64{dept.ParentID}); err != nil {
			return err
		}
	}

	count, err := repository.CountDeptByNameAndParent(ctx, dept.DeptName, dept.ParentID, dept.DeptID)
	if err != nil {
		return err
	}
	if count > 0 {
		return errs.Newf("修改部门'%s'失败，部门名称已存在", dept.DeptName)
	}
	if dept.ParentID == dept.DeptID {
		return errs.Newf("修改部门'%s'失败，上级部门不能是自己", dept.DeptName)
	}
	if dept.Status == model.DeptDisable {
		normalChildren, err := repository.CountNormalChildrenDeptByID(ctx, dept.DeptID)
		if err != nil {
			return err
		}
		if normalChildren > 0 {
			return errs.New("该部门包含未停用的子部门！")
		}
	}

	oldDept, err := repository.SelectDeptByID(ctx, dept.DeptID)
	if err != nil {
		return err
	}
	if oldDept == nil {
		return errs.New("部门不存在")
	}
	newParent, err := repository.SelectDeptByID(ctx, dept.ParentID)
	if err != nil {
		return err
	}

	var children []model.SysDept
	if newParent != nil {
		dept.Ancestors = newParent.Ancestors + "," + strconv.FormatInt(newParent.DeptID, 10)
		children, err = rebuildChildrenAncestors(ctx, dept.DeptID, oldDept.Ancestors, dept.Ancestors)
		if err != nil {
			return err
		}
	} else {
		dept.Ancestors = oldDept.Ancestors
	}

	// 启用某个部门时，把它所有上级也启用，避免出现"父停用子正常"的断链
	var enableParentIDs []int64
	if dept.Status == model.DeptNormal && dept.Ancestors != "" && dept.Ancestors != "0" {
		enableParentIDs = parseAncestorIDs(dept.Ancestors)
	}

	dept.UpdateBy = operator
	dept.UpdateTime = types.Now()
	return repository.UpdateDept(ctx, dept, children, enableParentIDs)
}

// rebuildChildrenAncestors 把子孙部门的 ancestors 前缀从旧路径换成新路径。
func rebuildChildrenAncestors(ctx context.Context, deptID int64, oldAncestors, newAncestors string) ([]model.SysDept, error) {
	if oldAncestors == newAncestors {
		return nil, nil
	}
	children, err := repository.SelectChildrenDeptByID(ctx, deptID)
	if err != nil {
		return nil, err
	}
	for i := range children {
		// 只替换开头的一段，对齐 Java 的 replaceFirst
		children[i].Ancestors = strings.Replace(children[i].Ancestors, oldAncestors, newAncestors, 1)
	}
	return children, nil
}

func parseAncestorIDs(ancestors string) []int64 {
	var ids []int64
	for _, part := range strings.Split(ancestors, ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || id == 0 {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

// DeleteDept 删除部门。
func DeleteDept(ctx context.Context, user *model.SysUser, deptID int64) error {
	hasChild, err := repository.HasChildByDeptID(ctx, deptID)
	if err != nil {
		return err
	}
	if hasChild {
		return errs.New("存在下级部门,不允许删除")
	}

	hasUser, err := repository.CheckDeptExistUser(ctx, deptID)
	if err != nil {
		return err
	}
	if hasUser {
		return errs.New("部门存在用户,不允许删除")
	}

	if err := CheckDeptDataScope(ctx, user, deptID); err != nil {
		return err
	}
	return repository.DeleteDeptByID(ctx, deptID)
}

// UpdateDeptSort 保存部门排序。
//
// 传输格式与菜单排序一致，解析逻辑共用 parseSortPairs。
func UpdateDeptSort(ctx context.Context, user *model.SysUser, body model.DeptSortBody) error {
	sorts, err := parseSortPairs(body.DeptIDs, body.OrderNums)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(sorts))
	for id := range sorts {
		ids = append(ids, id)
	}
	if _, err := checkDeptIDs(ctx, user, ids); err != nil {
		return err
	}
	return repository.UpdateDeptSort(ctx, sorts)
}
