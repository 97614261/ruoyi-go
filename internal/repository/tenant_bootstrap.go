package repository

import (
	"context"
	"database/sql"
	"fmt"
	drivermysql "github.com/go-sql-driver/mysql"
	"net"
	"strings"
	"time"
)

const TenantDatabaseName = "ruoyi_go_mt"

// Never connect to the old database for writes; never overwrite an existing database.
func BootstrapLocalDatabase(ctx context.Context, dsn, baseSQL, tenantSQL string, inspect bool) (string, error) {
	cfg, err := drivermysql.ParseDSN(dsn)
	if err != nil {
		return "", fmt.Errorf("数据库连接配置不合法")
	}
	host, _, err := net.SplitHostPort(cfg.Addr)
	if err != nil || (host != "localhost" && host != "127.0.0.1" && host != "::1") {
		return "", fmt.Errorf("初始化仅允许本地MySQL")
	}
	cfg.DBName = ""
	cfg.MultiStatements = true
	cfg.Timeout = 5 * time.Second
	cfg.ReadTimeout = 2 * time.Minute
	cfg.WriteTimeout = 2 * time.Minute
	conn, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return "", fmt.Errorf("打开本地MySQL失败")
	}
	defer conn.Close()
	var version string
	if err = conn.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil {
		return "", err
	}
	var count int
	if err = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name=?", TenantDatabaseName).Scan(&count); err != nil {
		return "", err
	}
	if inspect {
		return fmt.Sprintf("MySQL %s; %s exists=%t", version, TenantDatabaseName, count > 0), nil
	}
	if count != 0 {
		return "", fmt.Errorf("目标库 %s 已存在，拒绝覆盖", TenantDatabaseName)
	}
	if !strings.Contains(tenantSQL, "CREATE TABLE sys_tenant") || !strings.Contains(baseSQL, "create table sys_user") {
		return "", fmt.Errorf("初始化SQL不完整")
	}
	if _, err = conn.ExecContext(ctx, "CREATE DATABASE `ruoyi_go_mt` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		return "", err
	}
	cfg.DBName = TenantDatabaseName
	dest, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return "", err
	}
	defer dest.Close()
	if _, err = dest.ExecContext(ctx, baseSQL); err != nil {
		return "", fmt.Errorf("新库基线初始化失败（原库未修改）: %w", err)
	}
	if _, err = dest.ExecContext(ctx, tenantSQL); err != nil {
		return "", fmt.Errorf("新库租户结构初始化失败（原库未修改）: %w", err)
	}
	cfg.MultiStatements = false
	return cfg.FormatDSN(), nil
}
