// tenantconsole updates only the dedicated local development database's menu
// data and the explicit tenant-policy migration. It never changes passwords.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"time"
	_ "time/tzdata"

	"github.com/go-sql-driver/mysql"

	"ruoyi-go/internal/config"
	"ruoyi-go/internal/repository"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	path := flag.String("config", "configs/multitenant.local.yml", "独立多租户开发配置")
	flag.Parse()
	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	if err = validateLocalDSN(cfg.MySQL.DSN); err != nil {
		return err
	}
	if err = repository.Init(cfg.MySQL); err != nil {
		return err
	}
	defer repository.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ddl, err := os.ReadFile("sql/multitenant/003_tenant_policy.sql")
	if err != nil {
		return err
	}
	if err = repository.EnsureTenantOperationsSchema(ctx, string(ddl)); err != nil {
		return err
	}
	if err = repository.EnsurePlatformConsole(ctx); err != nil {
		return err
	}
	fmt.Println("平台管理 / 租户管理菜单已就绪；沿用 platform 企业现有超级管理员和原密码。")
	return nil
}

func validateLocalDSN(dsn string) error {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil || cfg.DBName != repository.TenantDatabaseName || cfg.Net != "tcp" {
		return fmt.Errorf("仅允许独立本地 ruoyi_go_mt 开发库")
	}
	host, _, err := net.SplitHostPort(cfg.Addr)
	if err != nil || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		return fmt.Errorf("仅允许本地MySQL连接")
	}
	return nil
}
