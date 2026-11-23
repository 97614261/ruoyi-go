package repository

import (
	"context"
	"database/sql"
	"fmt"
	"net"

	"github.com/go-sql-driver/mysql"

	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	podsql "ruoyi-go/sql/pod"
)

type PODSeedUser struct{ UserName, NickName, PasswordHash string }

// localPODConnection is used only by the local development bootstrap command.
// Production migration and existing-database upgrades are intentionally separate.
func localPODConnection(cfg *config.Config) (*mysql.Config, error) {
	if err := config.ValidatePOD(cfg); err != nil {
		return nil, err
	}
	dsn, err := mysql.ParseDSN(cfg.MySQL.DSN)
	if err != nil {
		return nil, fmt.Errorf("MySQL 配置不合法")
	}
	host, _, err := net.SplitHostPort(dsn.Addr)
	if err != nil || dsn.Net != "tcp" || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return nil, fmt.Errorf("本地初始化工具只允许连接回环地址的 MySQL")
	}
	dsn.Timeout, dsn.ReadTimeout, dsn.WriteTimeout = cfg.MySQL.ConnectTimeout, cfg.MySQL.ReadTimeout, cfg.MySQL.WriteTimeout
	return dsn, nil
}

func PODDatabaseExists(ctx context.Context, cfg *config.Config) (bool, error) {
	dsn, err := localPODConnection(cfg)
	if err != nil {
		return false, err
	}
	dsn.DBName = ""
	conn, err := sql.Open("mysql", dsn.FormatDSN())
	if err != nil {
		return false, err
	}
	defer conn.Close()
	var count int
	err = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = ?", cfg.POD.DatabaseName).Scan(&count)
	return count != 0, err
}

// InstallPODFoundation creates a brand-new local database. It never drops a
// database, never imports into an existing database and never resets accounts.
// Only embedded, reviewed SQL is executed with multiStatements enabled.
func InstallPODFoundation(ctx context.Context, cfg *config.Config, adminHash string, users []PODSeedUser) error {
	dsn, err := localPODConnection(cfg)
	if err != nil {
		return err
	}
	if len(adminHash) < 50 || len(users) != 2 {
		return fmt.Errorf("初始化账号参数不完整")
	}
	serverDSN := *dsn
	serverDSN.DBName = ""
	server, err := sql.Open("mysql", serverDSN.FormatDSN())
	if err != nil {
		return err
	}
	defer server.Close()
	var exists int
	if err := server.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = ?", cfg.POD.DatabaseName).Scan(&exists); err != nil {
		return err
	}
	if exists != 0 {
		return fmt.Errorf("目标数据库已存在；拒绝覆盖或重置，请使用 check")
	}
	// DatabaseName is validated against a strict lowercase identifier regex.
	if _, err := server.ExecContext(ctx, "CREATE DATABASE `"+cfg.POD.DatabaseName+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		return fmt.Errorf("创建专用数据库失败: %w", err)
	}
	importDSN := *dsn
	importDSN.MultiStatements = true
	importer, err := sql.Open("mysql", importDSN.FormatDSN())
	if err != nil {
		return err
	}
	defer importer.Close()
	if _, err := importer.ExecContext(ctx, podsql.Foundation); err != nil {
		return fmt.Errorf("安装 POD 结构失败，已保留新库供检查: %w", err)
	}
	if _, err := importer.ExecContext(ctx, "INSERT INTO pod_schema_version(version,checksum,instance_id,state,create_by) VALUES(?,?,?,'installing','podsetup')", podsql.Version, podsql.Checksum(), cfg.POD.InstanceID); err != nil {
		return err
	}
	if _, err := importer.ExecContext(ctx, podsql.Baseline); err != nil {
		return fmt.Errorf("安装系统基线失败，已保留新库供检查: %w", err)
	}
	connection, err := sql.Open("mysql", dsn.FormatDSN())
	if err != nil {
		return err
	}
	defer connection.Close()
	tx, err := connection.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Seed passwords are rotated before a server can pass VerifyPODSchema.
	if _, err := tx.ExecContext(ctx, "UPDATE sys_user SET password=?, status=CASE WHEN user_id=1 THEN '0' ELSE '1' END, pwd_update_date=NOW()", adminHash); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE sys_job SET status='1'"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE sys_config SET config_value='false' WHERE config_key='sys.account.registerUser'"); err != nil {
		return err
	}
	roleResult, err := tx.ExecContext(ctx, "INSERT INTO sys_role(role_name,role_key,role_sort,data_scope,menu_check_strictly,dept_check_strictly,status,del_flag,create_by,create_time) VALUES('POD 创作者','pod_creator',20,'5',1,1,'0','0','podsetup',NOW())")
	if err != nil {
		return err
	}
	roleID, err := roleResult.LastInsertId()
	if err != nil {
		return err
	}
	menuResult, err := tx.ExecContext(ctx, "INSERT INTO sys_menu(menu_name,parent_id,order_num,path,menu_type,visible,status,perms,create_by,create_time) VALUES('POD 工作空间访问',0,100,'','F','1','0',?,'podsetup',NOW())", model.PODStudioAccess)
	if err != nil {
		return err
	}
	menuID, err := menuResult.LastInsertId()
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO sys_role_menu(role_id,menu_id) VALUES(?,?)", roleID, menuID); err != nil {
		return err
	}
	for _, user := range users {
		res, err := tx.ExecContext(ctx, "INSERT INTO sys_user(dept_id,user_name,nick_name,user_type,password,status,del_flag,pwd_update_date,create_by,create_time) VALUES(100,?,?,'00',?,'0','0',NOW(),'podsetup',NOW())", user.UserName, user.NickName, user.PasswordHash)
		if err != nil {
			return err
		}
		userID, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO sys_user_role(user_id,role_id) VALUES(?,?)", userID, roleID); err != nil {
			return err
		}
		res, err = tx.ExecContext(ctx, "INSERT INTO pod_workspace(owner_user_id,name,create_by) VALUES(?,?,'podsetup')", userID, user.NickName+"的空间")
		if err != nil {
			return err
		}
		workspaceID, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO pod_workspace_member(workspace_id,user_id,create_by) VALUES(?,?,'podsetup')", workspaceID, userID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE pod_schema_version SET state='ready',update_time=NOW(),update_by='podsetup' WHERE version=?", podsql.Version); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return MigratePODAssets(ctx, cfg)
}
