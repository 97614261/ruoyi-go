package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"ruoyi-go/internal/config"
	"ruoyi-go/internal/repository"
)

func backup(ctx context.Context, cfg *config.Config, executable string) error {
	var complete string
	if err := backupSQL(ctx, cfg, executable, &complete); err != nil {
		return err
	}
	fmt.Printf("数据库备份已生成：%s。尚未验证恢复；完整素材和配置请使用 -mode bundle。\n", complete)
	return nil
}

func backupSQL(ctx context.Context, cfg *config.Config, executable string, complete *string) error {
	if err := repository.Init(cfg.MySQL); err != nil {
		return err
	}
	defer repository.Close()
	if err := repository.VerifyPODFoundation(ctx, cfg.POD.InstanceID); err != nil {
		return err
	}
	dsn, err := mysql.ParseDSN(cfg.MySQL.DSN)
	if err != nil {
		return errors.New("无法解析备份连接配置")
	}
	host, port, err := net.SplitHostPort(dsn.Addr)
	if err != nil || dsn.Net != "tcp" || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return errors.New("本地备份命令只允许回环地址的 MySQL")
	}
	bin, err := exec.LookPath(executable)
	if err != nil {
		return errors.New("未找到 mysqldump，请通过 -mysqldump 指定已安装的工具")
	}
	dir, err := filepath.Abs(".local/backups")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	optionFile, err := os.CreateTemp(dir, ".mysql-options-*.cnf")
	if err != nil {
		return err
	}
	defer os.Remove(optionFile.Name())
	// Credentials stay in a temporary private option file, never command arguments.
	options := "[client]\nprotocol=tcp\nhost=" + mysqlOption(host) + "\nport=" + port + "\nuser=" + mysqlOption(dsn.User) + "\npassword=" + mysqlOption(dsn.Passwd) + "\n"
	if _, err := optionFile.WriteString(options); err != nil {
		optionFile.Close()
		return err
	}
	if err := optionFile.Close(); err != nil {
		return err
	}
	output, err := os.CreateTemp(dir, cfg.POD.DatabaseName+"-"+time.Now().Format("20060102-150405")+"-*.sql.partial")
	if err != nil {
		return err
	}
	defer output.Close()
	command := exec.CommandContext(ctx, bin, "--defaults-extra-file="+optionFile.Name(), "--single-transaction", "--skip-lock-tables", "--no-tablespaces", "--set-gtid-purged=OFF", "--default-character-set=utf8mb4", cfg.POD.DatabaseName)
	command.Stdout = output
	// Tool errors may contain paths or connection details; retain only safe status.
	if err := command.Run(); err != nil {
		return errors.New("备份未完成，已保留不完整文件；请勿用于恢复")
	}
	if err := output.Sync(); err != nil {
		return err
	}
	info, err := output.Stat()
	if err != nil || info.Size() == 0 {
		return errors.New("备份文件为空")
	}
	if err := output.Close(); err != nil {
		return err
	}
	completePath := strings.TrimSuffix(output.Name(), ".partial")
	if err := os.Rename(output.Name(), completePath); err != nil {
		return err
	}
	*complete = completePath
	return nil
}

func mysqlOption(value string) string {
	return `"` + strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n", "\r", "\\r", "\t", "\\t").Replace(value) + `"`
}
