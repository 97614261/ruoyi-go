package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
	"os"
	"path/filepath"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/tenantctx"
	"strings"
	"time"
	_ "time/tzdata"
)

func secret(n int) (string, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	input := flag.String("config", "configs/application.yml", "原配置，仅取得本地连接")
	output := flag.String("out", "configs/multitenant.local.yml", "独立开发配置")
	inspect := flag.Bool("inspect", false, "只读检查")
	flag.Parse()
	if !*inspect {
		inAbs, e := filepath.Abs(*input)
		if e != nil {
			return e
		}
		outAbs, e := filepath.Abs(*output)
		if e != nil {
			return e
		}
		if strings.EqualFold(inAbs, outAbs) {
			return fmt.Errorf("独立配置不能覆盖原配置")
		}
		if _, e = os.Stat(*output); e == nil {
			return fmt.Errorf("独立配置已存在，拒绝覆盖")
		}
		if _, e = os.Stat("logs/multitenant-accounts.local.json"); e == nil {
			return fmt.Errorf("登录资料已存在，拒绝覆盖")
		}
	}
	cfg, err := config.Load(*input)
	if err != nil {
		return err
	}
	base, err := os.ReadFile("sql/multitenant/base.sql")
	if err != nil {
		return err
	}
	delta, err := os.ReadFile("sql/multitenant/002_tenants.sql")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	dsn, err := repository.BootstrapLocalDatabase(ctx, cfg.MySQL.DSN, string(base), string(delta), *inspect)
	if err != nil {
		return err
	}
	if *inspect {
		fmt.Println(dsn)
		return nil
	}
	cfg.MySQL.DSN = dsn
	if err = repository.Init(cfg.MySQL); err != nil {
		return err
	}
	defer repository.Close()
	policyDDL, err := os.ReadFile("sql/multitenant/003_tenant_policy.sql")
	if err != nil {
		return err
	}
	if err = repository.EnsureTenantOperationsSchema(ctx, string(policyDDL)); err != nil {
		return err
	}
	passwords := map[string]string{}
	for _, code := range []string{"platform", "demo", "demo-b"} {
		passwords[code], err = secret(8)
		if err != nil {
			return err
		}
	}
	hash := func(p string) (string, error) {
		b, e := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
		return string(b), e
	}
	ph, err := hash(passwords["platform"])
	if err != nil {
		return err
	}
	if err = repository.BootstrapPlatformOwner(ctx, ph); err != nil {
		return err
	}
	if err = repository.EnsurePlatformConsole(ctx); err != nil {
		return err
	}
	dh, err := hash(passwords["demo"])
	if err != nil {
		return err
	}
	if err = repository.ResetUserPwd(tenantctx.With(ctx, 10000, false), 1, dh, "bootstrap", time.Now()); err != nil {
		return err
	}
	bh, err := hash(passwords["demo-b"])
	if err != nil {
		return err
	}
	if err = repository.ProvisionTenant(ctx, &model.SysTenant{TenantCode: "demo-b", TenantName: "示例企业 B", CreateBy: "bootstrap"}, "admin", bh); err != nil {
		return err
	}
	v := viper.New()
	v.SetConfigFile(*input)
	if err = v.ReadInConfig(); err != nil {
		return err
	}
	jwtSecret, err := secret(32)
	if err != nil {
		return err
	}
	v.Set("mysql.dsn", dsn)
	v.Set("server.port", 8081)
	v.Set("server.host", "127.0.0.1")
	v.Set("redis.db", 13)
	v.Set("jwt.secret", jwtSecret)
	v.Set("upload.path", "./uploadPath-mt")
	v.Set("log.level", "info")
	if err = v.WriteConfigAs(*output); err != nil {
		return err
	}
	if err = os.MkdirAll("logs", 0750); err != nil {
		return err
	}
	accounts := map[string]any{"username": "admin", "passwords": passwords, "frontend": "http://localhost:5173", "backend": "http://localhost:8081", "database": repository.TenantDatabaseName}
	b, err := json.MarshalIndent(accounts, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile("logs/multitenant-accounts.local.json", b, 0600); err != nil {
		return err
	}
	fmt.Printf("独立开发库创建成功；配置：%s；登录资料：logs/multitenant-accounts.local.json\n", *output)
	return nil
}
