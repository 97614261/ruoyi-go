// Command erpinit explicitly installs the ERP schema into the dedicated local DB.
package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"net"
	"os"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/repository"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	path := flag.String("config", "configs/multitenant.local.yml", "local multitenant config")
	grantDemo := flag.Bool("grant-demo", false, "explicitly enable ERP for local demo/demo-b customers")
	flag.Parse()
	cfg, err := config.Load(*path)
	if err != nil {
		return fmt.Errorf("读取本地配置失败")
	}
	dsn, err := mysql.ParseDSN(cfg.MySQL.DSN)
	if err != nil {
		return fmt.Errorf("连接配置不合法")
	}
	host, _, err := net.SplitHostPort(dsn.Addr)
	if err != nil || dsn.DBName != repository.TenantDatabaseName || (host != "localhost" && host != "127.0.0.1" && host != "::1") {
		return fmt.Errorf("仅允许本地独立多租户库，禁止修改原库")
	}
	script, err := os.ReadFile("sql/erp/001_core.sql")
	if err != nil {
		return err
	}
	if err = repository.Init(cfg.MySQL); err != nil {
		return err
	}
	defer repository.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err = repository.InstallERPSchema(ctx, string(script)); err != nil {
		return err
	}
	if err = repository.InstallERPConsole(ctx, *grantDemo); err != nil {
		return err
	}
	fmt.Println("ERP_SCHEMA_READY: dedicated local database only")
	return nil
}
