package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"ruoyi-go/internal/model"
)

// Called once while 008 is installing, under the instance migration lock.
// A collision is evidence to investigate, never permission to reuse another role.
func installPODAdminMenus(ctx context.Context, conn *sql.Conn) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sys_menu WHERE perms LIKE 'podAdmin:%' OR (parent_id=0 AND path='pod-admin')").Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return errors.New("POD 管理菜单标识已被占用，拒绝合并授权")
	}
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sys_role WHERE role_key IN ('pod_admin','pod_operator')").Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return errors.New("POD 管理角色标识已被占用，拒绝扩大既有角色权限")
	}
	insertMenu := func(name, path, component, kind, perm, icon string, parent int64, sort int) (int64, error) {
		r, err := tx.ExecContext(ctx, `INSERT INTO sys_menu(menu_name,parent_id,order_num,path,component,is_frame,is_cache,menu_type,visible,status,perms,icon,create_by,create_time) VALUES(?,?,?,?,?,1,1,?,'0','0',?,?,'podsetup',NOW())`, name, parent, sort, path, component, kind, perm, icon)
		if err != nil {
			return 0, err
		}
		return r.LastInsertId()
	}
	root, err := insertMenu("POD 管理", "pod-admin", "", "M", model.PODAdminAccess, "dashboard", 0, 5)
	if err != nil {
		return err
	}
	ids := map[string]int64{model.PODAdminAccess: root}
	for i, item := range []struct{ name, path, permission, icon string }{
		{"运营概览", "overview", model.PODAdminOverviewQuery, "chart"},
		{"创作者管理", "creators", "podAdmin:creator:list", "user"},
		{"模型管理", "models", "podAdmin:model:list", "component"},
		{"任务中心", "tasks", "podAdmin:task:list", "job"},
		{"官方模板", "templates", "podAdmin:template:list", "example"},
		{"用量统计", "usage", "podAdmin:usage:query", "monitor"},
	} {
		id, err := insertMenu(item.name, item.path, "pod/admin/"+item.path+"/index", "C", item.permission, item.icon, root, i+1)
		if err != nil {
			return err
		}
		ids[item.permission] = id
	}
	for i, perm := range model.PODAdminPermissions {
		if _, ok := ids[perm]; ok {
			continue
		}
		id, err := insertMenu(perm, "", "", "F", perm, "#", root, i+10)
		if err != nil {
			return err
		}
		ids[perm] = id
	}
	for _, role := range []struct {
		name, key string
		sort      int
	}{{"POD 平台管理员", "pod_admin", 21}, {"POD 只读运营", "pod_operator", 22}} {
		r, err := tx.ExecContext(ctx, `INSERT INTO sys_role(role_name,role_key,role_sort,data_scope,menu_check_strictly,dept_check_strictly,status,del_flag,create_by,create_time) VALUES(?,?,?,'5',1,1,'0','0','podsetup',NOW())`, role.name, role.key, role.sort)
		if err != nil {
			return err
		}
		roleID, err := r.LastInsertId()
		if err != nil {
			return err
		}
		for _, perm := range model.PODAdminPermissions {
			readOnly := perm == model.PODAdminAccess || perm == model.PODAdminOverviewQuery || perm == "podAdmin:usage:query" || ((strings.HasPrefix(perm, "podAdmin:creator:") || strings.HasPrefix(perm, "podAdmin:task:")) && (strings.HasSuffix(perm, ":list") || strings.HasSuffix(perm, ":query")))
			if role.key == "pod_operator" && !readOnly {
				continue
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO sys_role_menu(role_id,menu_id) VALUES(?,?)", roleID, ids[perm]); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
