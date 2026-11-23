// Command podsetup prepares and initializes an isolated, local POD environment.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
	_ "time/tzdata"

	"github.com/go-sql-driver/mysql"
	"go.yaml.in/yaml/v3"
	"golang.org/x/crypto/bcrypt"

	"ruoyi-go/internal/config"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/redisx"
)

type localAccount struct {
	UserName string `json:"userName"`
	Password string `json:"password"`
	Purpose  string `json:"purpose"`
}
type accountFile struct {
	State    string         `json:"state"`
	Accounts []localAccount `json:"accounts"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "POD 初始化:", err)
		os.Exit(1)
	}
}
func run() error {
	mode := flag.String("mode", "check", "prepare / init / migrate / check / doctor / claim-cache / stop-cache / backup / bundle / restore-check / seed-templates")
	path := flag.String("config", "configs/pod.local.yml", "独立配置路径")
	source := flag.String("source", "configs/application.yml", "prepare 时仅复用本机数据库连接资料")
	redisDB := flag.Int("redis-db", 15, "独立 Redis 逻辑库")
	dumpPath := flag.String("mysqldump", "mysqldump", "backup 使用的 mysqldump 可执行文件")
	bundlePath := flag.String("bundle", "", "restore-check 使用本工具生成的可信完整备份目录")
	upgradeAdmin := flag.Bool("upgrade-admin", false, "仅 restore-check：在新恢复副本验证 008 管理基础升级，不修改源库")
	upgradeCatalog := flag.Bool("upgrade-catalog", false, "仅 restore-check：在新恢复副本验证 009 模型目录，不修改源库")
	upgradeCreators := flag.Bool("upgrade-creators", false, "仅 restore-check：在独立副本验证 010 用户权益与点数")
	upgradeOfficial := flag.Bool("upgrade-official", false, "仅 restore-check：在独立副本验证 012 官方模板与平台素材")
	upgradeManaged := flag.Bool("upgrade-managed", false, "仅 restore-check：在独立副本验证 011 正式模型执行与点数结算")
	flag.Parse()
	upgrades := 0
	for _, enabled := range []bool{*upgradeAdmin, *upgradeCatalog, *upgradeCreators, *upgradeManaged, *upgradeOfficial} {
		if enabled {
			upgrades++
		}
	}
	if upgrades > 1 || upgrades > 0 && *mode != "restore-check" {
		return errors.New("upgrade 参数只用于新恢复副本，且不可合用")
	}
	if *mode == "prepare" {
		return prepare(*source, *path, *redisDB)
	}
	if *mode == "doctor" {
		return doctorFile(*path, os.Stdout)
	}
	if *mode != "init" && *mode != "migrate" && *mode != "check" && *mode != "backup" && *mode != "bundle" && *mode != "restore-check" && *mode != "claim-cache" && *mode != "stop-cache" && *mode != "seed-templates" {
		return errors.New("未知 mode")
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	if err := config.ValidatePOD(cfg); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if *mode == "bundle" {
		return backupBundle(ctx, cfg, *path, *dumpPath)
	}
	if *mode == "restore-check" {
		return restoreCheck(ctx, cfg, *bundlePath, *upgradeAdmin, *upgradeCatalog, *upgradeCreators, *upgradeManaged, *upgradeOfficial)
	}
	if *mode == "backup" {
		return backup(ctx, cfg, *dumpPath)
	}
	if *mode == "stop-cache" {
		return stopCache(ctx, cfg)
	}
	if *mode == "seed-templates" {
		return seedTemplates(ctx, cfg)
	}
	if *mode == "migrate" {
		if err := bundleOffline(ctx, cfg); err != nil {
			return err
		}
		if err := redisx.Init(ctx, redisx.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB}); err != nil {
			return err
		}
		defer redisx.Close()
		owner, err := redisx.C().Get(ctx, redisx.PODInstanceKey).Result()
		if err != nil || owner != cfg.POD.InstanceID {
			return errors.New("Redis 实例归属不匹配，拒绝迁移")
		}
		if err := repository.MigratePODAssets(ctx, cfg); err != nil {
			return err
		}
		fmt.Println("本地迁移检查完成：001—015 已就绪；未覆盖账号、项目、旧模型密钥或费用。")
		return nil
	}
	if *mode == "check" || *mode == "claim-cache" {
		if err := repository.Init(cfg.MySQL); err != nil {
			return err
		}
		defer repository.Close()
		if err := repository.VerifyPODSchema(ctx, cfg.POD.InstanceID); err != nil {
			return err
		}
		if err := redisx.Init(ctx, redisx.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB}); err != nil {
			return err
		}
		defer redisx.Close()
		if cfg.POD.Execution.Mode == "simulation" || cfg.POD.Execution.Mode == "qwen" || cfg.POD.Execution.Mode == "managed" {
			if err := redisx.VerifyPODQueueRedis(ctx); err != nil {
				return err
			}
		}
		if *mode == "claim-cache" {
			if err := redisx.ClaimPODInstance(ctx, cfg.POD.InstanceID); err != nil {
				return err
			}
			fmt.Println("已核对 POD 数据库并认领独立空 Redis 库，未迁移或清理旧缓存。")
			return nil
		}
		owner, err := redisx.C().Get(ctx, redisx.PODInstanceKey).Result()
		if err != nil || owner != cfg.POD.InstanceID {
			return errors.New("Redis 归属标记缺失或不匹配")
		}
		fmt.Printf("检查通过：数据库 %s，schema 001—015，Redis DB %d；未执行迁移或重置。\n", cfg.POD.DatabaseName, cfg.Redis.DB)
		return nil
	}
	exists, err := repository.PODDatabaseExists(ctx, cfg)
	if err != nil {
		return err
	}
	if exists {
		return errors.New("目标数据库已存在，拒绝覆盖；请运行 -mode check")
	}
	if err := redisx.Init(ctx, redisx.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB}); err != nil {
		return err
	}
	defer redisx.Close()
	if err := redisx.ClaimPODInstance(ctx, cfg.POD.InstanceID); err != nil {
		return err
	}
	accounts := accountFile{State: "prepared"}
	users := []repository.PODSeedUser{}
	adminHash := ""
	for _, entry := range []struct{ name, nickname, purpose string }{{"admin", "管理员", "专用 POD 开发库的系统管理员；不自动获得个人空间"}, {"pod_creator", "创作者", "主要创作测试账号"}, {"pod_creator_b", "创作者 B", "隔离验证账号"}} {
		password, err := randomString(12)
		if err != nil {
			return err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		accounts.Accounts = append(accounts.Accounts, localAccount{entry.name, password, entry.purpose})
		if entry.name == "admin" {
			adminHash = string(hash)
		} else {
			users = append(users, repository.PODSeedUser{UserName: entry.name, NickName: entry.nickname, PasswordHash: string(hash)})
		}
	}
	if err := os.MkdirAll(".local", 0700); err != nil {
		return err
	}
	accountPath := filepath.Join(".local", "accounts.json")
	data, err := json.MarshalIndent(accounts, "", "  ")
	if err != nil {
		return err
	}
	if err := writeNew(accountPath, data); err != nil {
		return err
	}
	if err := repository.InstallPODFoundation(ctx, cfg, adminHash, users); err != nil {
		return err
	}
	accounts.State = "ready"
	data, err = json.MarshalIndent(accounts, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(accountPath, data, 0600); err != nil {
		return err
	}
	fmt.Printf("初始化完成：数据库 %s，Redis DB %d。账号仅写入 %s，未打印密码。\n", cfg.POD.DatabaseName, cfg.Redis.DB, accountPath)
	return nil
}

func prepare(source, path string, redisDB int) error {
	if redisDB < 1 {
		return errors.New("Redis DB 必须大于 0")
	}
	if _, err := os.Stat(path); err == nil {
		return errors.New("本地配置已存在，拒绝覆盖")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	legacy, err := config.Load(source)
	if err != nil {
		return err
	}
	dsn, err := mysql.ParseDSN(legacy.MySQL.DSN)
	if err != nil {
		return errors.New("源 MySQL 连接格式不合法")
	}
	host, _, err := net.SplitHostPort(dsn.Addr)
	if err != nil || dsn.Net != "tcp" || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return errors.New("prepare 只允许复用本机 MySQL 连接")
	}
	redisHost, _, err := net.SplitHostPort(legacy.Redis.Addr)
	if err != nil || net.ParseIP(redisHost) == nil || !net.ParseIP(redisHost).IsLoopback() {
		return errors.New("prepare 只允许复用本机 Redis 地址")
	}
	dsn.DBName = "pod_studio_dev"
	dsn.MultiStatements = false
	dsn.ParseTime = true
	secret, err := randomString(48)
	if err != nil {
		return err
	}
	instanceSuffix, err := randomString(6)
	if err != nil {
		return err
	}
	// Instance ID uses a normalized random suffix, never a credential.
	instanceID := fmt.Sprintf("pod-local-%x", []byte(instanceSuffix))
	privateRoot, err := filepath.Abs(".local/assets")
	if err != nil {
		return err
	}
	publicRoot, err := filepath.Abs(".local/public")
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(map[string]any{
		"pod":    map[string]any{"enabled": true, "instanceId": instanceID, "databaseName": dsn.DBName, "redisDB": redisDB, "storageRoot": privateRoot},
		"server": map[string]any{"host": "127.0.0.1", "port": 18110, "mode": "release"},
		"mysql":  map[string]any{"dsn": dsn.FormatDSN(), "maxOpenConns": 10, "maxIdleConns": 3},
		"redis":  map[string]any{"addr": legacy.Redis.Addr, "password": legacy.Redis.Password, "db": redisDB, "poolSize": 8},
		"jwt":    map[string]any{"secret": secret}, "upload": map[string]any{"path": publicRoot},
	})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if err := writeNew(path, data); err != nil {
		return err
	}
	if _, err := config.Load(path); err != nil {
		return fmt.Errorf("生成配置未通过检查: %w", err)
	}
	fmt.Printf("已准备独立配置 %s；尚未连接或创建数据库。\n", path)
	return nil
}

func randomString(bytes int) (string, error) {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
func writeNew(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(data)
	return err
}
