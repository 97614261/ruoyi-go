package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	mysql "github.com/go-sql-driver/mysql"
	"os"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/repository"
	portalsql "ruoyi-go/sql/portal"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "门户升级停止：", err)
		os.Exit(1)
	}
}
func run() error {
	confirm := flag.Bool("confirm", false, "显式逐版本升级门户专用库到当前版本；先停门户服务并核对备份")
	flag.Parse()
	if !*confirm {
		fmt.Printf("预览：只逐版本升级本地ruoyi_go_portal至v%d；新增门户表和菜单，不改sys表结构。执行前停门户服务并备份。\n", portalsql.CurrentVersion)
		return nil
	}
	cfg, err := config.Load("configs/portal.local.yml")
	if err != nil {
		return fmt.Errorf("无法加载门户专用配置")
	}
	d, err := mysql.ParseDSN(cfg.MySQL.DSN)
	if err != nil || d.DBName != "ruoyi_go_portal" || (d.Addr != "127.0.0.1:3306" && d.Addr != "localhost:3306") {
		return fmt.Errorf("仅允许本机专用门户库")
	}
	d.MultiStatements = true
	d.Timeout = 5 * time.Second
	d.ReadTimeout = 30 * time.Second
	d.WriteTimeout = 30 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, err := sql.Open("mysql", d.FormatDSN())
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	var installed int
	if err = db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version),0) FROM portal_schema_version").Scan(&installed); err != nil {
		return err
	}
	if err = repository.ValidatePortalVersion(ctx, db, installed); err != nil {
		return err
	}
	if installed >= portalsql.CurrentVersion {
		return fmt.Errorf("已是当前结构版本，不重复执行")
	}
	digest := ""
	for next := installed + 1; next <= portalsql.CurrentVersion; next++ {
		low, high := 6000, 6014
		if next == 3 {
			low, high = 6020, 6035
		}
		if next == 4 {
			low, high = 6040, 6045
		}
		if next == 5 {
			low, high = 6050, 6053
		}
		if next <= 5 {
			var conflicts int
			if err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sys_menu WHERE menu_id BETWEEN ? AND ?", low, high).Scan(&conflicts); err != nil {
				return err
			}
			if conflicts != 0 {
				return fmt.Errorf("新菜单ID冲突，停止")
			}
		}
		if _, err = db.ExecContext(ctx, portalsql.MigrationSQL(next)); err != nil {
			return fmt.Errorf("显式升级失败，保留现场；禁止重放或自动删除: %w", err)
		}
		digest, err = repository.PortalStructureVersion(ctx, db, next)
		if err != nil {
			return err
		}
		if _, err = db.ExecContext(ctx, "INSERT INTO portal_schema_version VALUES(?,?,?,NOW())", next, portalsql.SourceSHA256(next), digest); err != nil {
			return err
		}
		if err = repository.ValidatePortalVersion(ctx, db, next); err != nil {
			return err
		}
	}
	if err = repository.ValidatePortalConnection(ctx, db); err != nil {
		return err
	}
	path := "logs/portal-accounts.local.json"
	if body, readErr := os.ReadFile(path); readErr == nil {
		record := map[string]any{}
		if err = json.Unmarshal(body, &record); err != nil {
			return err
		}
		record["schemaVersion"] = portalsql.CurrentVersion
		record["structureSHA256"] = digest
		body, err = json.MarshalIndent(record, "", "  ")
		if err != nil {
			return err
		}
		if err = os.WriteFile(path, body, 0600); err != nil {
			return err
		}
	}
	fmt.Printf("门户v%d显式升级并只读结构核验通过；原sys表结构/角色授权未改变。\n", portalsql.CurrentVersion)
	return nil
}
