// portalinit defaults to a read-only plan. -confirm only installs a never-existing local portal database.
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"go.yaml.in/yaml/v3"
	"golang.org/x/crypto/bcrypt"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/repository"
	portalsql "ruoyi-go/sql/portal"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "门户安装停止：", err)
		os.Exit(1)
	}
}
func randomSecret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func run() error {
	source := flag.String("admin-config", "", "已有本地配置，仅读取连接凭证，不读业务数据")
	confirm := flag.Bool("confirm", false, "显式创建全新 ruoyi_go_portal，不允许复用/重置已有库")
	flag.Parse()
	if !*confirm {
		fmt.Printf("预览：只可新建本地 ruoyi_go_portal，导入20张单租户基线表和8张CMS表并记录结构版本6；SQL SHA256=%s；未连接数据库。\n", portalsql.BaselineSHA256())
		return nil
	}
	if *source == "" {
		return fmt.Errorf("需要 -admin-config 指向本地私有配置")
	}
	if _, err := os.Stat("configs/portal.local.yml"); !os.IsNotExist(err) {
		return fmt.Errorf("私有门户配置已存在，拒绝覆盖")
	}
	if _, err := os.Stat("logs/portal-accounts.local.json"); !os.IsNotExist(err) {
		return fmt.Errorf("门户账号记录已存在，拒绝覆盖")
	}
	cfg, err := config.Load(*source)
	if err != nil {
		return fmt.Errorf("无法加载本地连接配置")
	}
	dsn, err := mysql.ParseDSN(cfg.MySQL.DSN)
	if err != nil {
		return fmt.Errorf("数据库连接配置无效")
	}
	host, _, err := net.SplitHostPort(dsn.Addr)
	if err != nil || dsn.Net != "tcp" || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		return fmt.Errorf("安装只允许本机MySQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	redisHost, _, redisErr := net.SplitHostPort(cfg.Redis.Addr)
	if redisErr != nil || (redisHost != "127.0.0.1" && redisHost != "localhost" && redisHost != "::1") {
		return fmt.Errorf("安装只允许本机Redis")
	}
	cache := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: 14, DialTimeout: 3 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second})
	defer cache.Close()
	n, err := cache.DBSize(ctx).Result()
	if err != nil {
		return fmt.Errorf("专用Redis检查失败")
	}
	if n != 0 {
		return fmt.Errorf("Redis DB14非空，停止而不清理")
	}
	dsn.DBName = ""
	dsn.MultiStatements = false
	dsn.Timeout = 5 * time.Second
	dsn.ReadTimeout = 30 * time.Second
	dsn.WriteTimeout = 30 * time.Second
	admin, err := sql.Open("mysql", dsn.FormatDSN())
	if err != nil {
		return fmt.Errorf("数据库连接失败")
	}
	defer admin.Close()
	var exists int
	if err = admin.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='ruoyi_go_portal'").Scan(&exists); err != nil {
		return fmt.Errorf("无法检查目标库")
	}
	if exists != 0 {
		return fmt.Errorf("ruoyi_go_portal已存在；不会覆盖、清空或自动重建")
	}
	password, err := randomSecret(15)
	if err != nil {
		return err
	}
	jwt, err := randomSecret(48)
	if err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	// Persist this new installation's credentials before nontransactional DDL, so failures remain recoverable.
	pending, err := json.MarshalIndent(map[string]any{"database": "ruoyi_go_portal", "password": password, "jwt": jwt, "baselineSHA256": portalsql.BaselineSHA256(), "createdAt": time.Now().UTC()}, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll("logs", 0700); err != nil {
		return err
	}
	pendingFile, err := os.OpenFile("logs/portal-install.pending.local.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("存在未完成安装记录，请先人工核对")
	}
	_, err = pendingFile.Write(pending)
	closeErr := pendingFile.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	// Exclusive create is the ownership boundary. Failure afterwards is retained for inspection, never dropped/replayed.
	if _, err = admin.ExecContext(ctx, "CREATE DATABASE ruoyi_go_portal CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		return fmt.Errorf("新库创建失败")
	}
	dsn.DBName = "ruoyi_go_portal"
	dsn.MultiStatements = true
	target, err := sql.Open("mysql", dsn.FormatDSN())
	if err != nil {
		return err
	}
	defer target.Close()
	target.SetMaxOpenConns(1)
	if _, err = target.ExecContext(ctx, portalsql.InstallSQL()); err != nil {
		return fmt.Errorf("新库基线安装失败；保留现场，禁止重放")
	}
	if _, err = target.ExecContext(ctx, "UPDATE sys_user SET password=?,nick_name='门户管理员',email='',phonenumber='',pwd_update_date=NOW() WHERE user_id=1", string(hash)); err != nil {
		return fmt.Errorf("新库管理员初始化失败")
	}
	if _, err = target.ExecContext(ctx, "UPDATE sys_user SET status='1' WHERE user_id<>1"); err != nil {
		return err
	}
	if _, err = target.ExecContext(ctx, "UPDATE sys_job SET status='1'"); err != nil {
		return err
	}
	if _, err = target.ExecContext(ctx, `CREATE TABLE portal_schema_version(version INT NOT NULL PRIMARY KEY,baseline_sha256 CHAR(64) NOT NULL,structure_sha256 CHAR(64) NOT NULL,installed_at DATETIME NOT NULL)`); err != nil {
		return err
	}
	digest, err := repository.PortalStructureVersion(ctx, target, 1)
	if err != nil {
		return err
	}
	if _, err = target.ExecContext(ctx, "INSERT INTO portal_schema_version VALUES(1,?,?,NOW())", portalsql.BaselineSHA256(), digest); err != nil {
		return err
	}
	if err = repository.ValidatePortalVersion(ctx, target, 1); err != nil {
		return err
	}

	for version := 2; version <= portalsql.CurrentVersion; version++ {
		if _, err = target.ExecContext(ctx, portalsql.MigrationSQL(version)); err != nil {
			return fmt.Errorf("CMS基线安装失败，保留现场")
		}
		digest, err = repository.PortalStructureVersion(ctx, target, version)
		if err != nil {
			return err
		}
		if _, err = target.ExecContext(ctx, "INSERT INTO portal_schema_version VALUES(?,?,?,NOW())", version, portalsql.SourceSHA256(version), digest); err != nil {
			return err
		}
	}
	if err = repository.ValidatePortalConnection(ctx, target); err != nil {
		return err
	}
	dsn.MultiStatements = false
	private := map[string]any{"server": map[string]any{"port": 8085, "mode": "debug"}, "mysql": map[string]any{"dsn": dsn.FormatDSN()}, "redis": map[string]any{"addr": cfg.Redis.Addr, "password": cfg.Redis.Password, "db": 14}, "jwt": map[string]any{"secret": jwt}, "upload": map[string]any{"path": "./uploadPath-portal-public", "urlPrefix": "/profile"}, "portal": map[string]any{"enabled": true, "theme": "business", "siteName": "门户开发站", "runJobs": false}}
	yamlBody, err := yaml.Marshal(private)
	if err != nil {
		return err
	}
	record := map[string]any{"database": "ruoyi_go_portal", "url": "http://127.0.0.1:8085/admin/", "username": "admin", "password": password, "baselineSHA256": portalsql.BaselineSHA256(), "structureSHA256": digest, "schemaVersion": portalsql.CurrentVersion, "createdAt": time.Now().UTC()}
	jsonBody, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	for name, body := range map[string][]byte{"configs/portal.local.yml": yamlBody, "logs/portal-accounts.local.json": jsonBody} {
		if err = os.MkdirAll(filepath.Dir(name), 0700); err != nil {
			return err
		}
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, writeErr := f.Write(body)
		closeErr := f.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	if err = os.Remove("logs/portal-install.pending.local.json"); err != nil {
		return err
	}
	fmt.Println("新门户库安装并只读结构核验通过；20张基线表+8张CMS表+1张版本表；配置和新账号已保存至忽略文件，未输出密码。旧库未写入。")
	return nil
}
