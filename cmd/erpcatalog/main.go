// Command erpcatalog is read-only unless explicitly confirmed for the
// dedicated local database with fresh full database + both-file-roots evidence.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
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
	path := flag.String("config", "configs/multitenant.local.yml", "dedicated local configuration")
	apply := flag.Bool("apply", false, "explicitly install reviewed catalog CREATE schema")
	confirm := flag.String("confirm-database", "", "must be ruoyi_go_mt for writes")
	backup := flag.String("backup-report", "", "verified database + public/private files restore report within 24h")
	flag.Parse()
	if flag.NArg() != 0 {
		return errors.New("不接受额外位置参数")
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return errors.New("读取独立本地配置失败")
	}
	if err = validateDSN(cfg.MySQL.DSN); err != nil {
		return err
	}
	if *apply {
		if *confirm != repository.TenantDatabaseName {
			return errors.New("写入必须显式确认独立库名")
		}
		if err = validateBackup(*backup, time.Now().UTC()); err != nil {
			return err
		}
	}
	dsn, _ := mysql.ParseDSN(cfg.MySQL.DSN)
	dsn.MultiStatements = false
	cfg.MySQL.DSN = dsn.FormatDSN()
	if err = repository.Init(cfg.MySQL); err != nil {
		return errors.New("连接本地独立多租户库失败，未执行商品基础迁移")
	}
	defer repository.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	var result *repository.ERPPhase1SchemaStatus
	if *apply {
		result, err = repository.InstallERPCatalog(ctx)
	} else {
		result, err = repository.InspectERPCatalog(ctx)
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Apply bool `json:"apply"`
		*repository.ERPPhase1SchemaStatus
	}{*apply, result})
}

func validateDSN(raw string) error {
	d, err := mysql.ParseDSN(raw)
	if err != nil || d.Net != "tcp" || d.DBName != repository.TenantDatabaseName {
		return errors.New("仅允许本地ruoyi_go_mt，拒绝原库、恢复副本和未知连接")
	}
	host, _, err := net.SplitHostPort(d.Addr)
	if err != nil || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		return errors.New("仅允许本地MySQL")
	}
	return nil
}

// An operator evidence gate, not a signed proof or a maintenance mechanism.
// The operator still stops writers and compares fresh pre/post snapshots.
func validateBackup(path string, now time.Time) error {
	if path == "" {
		return errors.New("先备份并恢复核验，再提供-backup-report")
	}
	f, err := os.Open(path)
	if err != nil {
		return errors.New("备份报告不可读")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16<<20 {
		return errors.New("备份报告大小或文件类型错误")
	}
	var r struct {
		Source    string          `json:"sourceDatabase"`
		Restored  string          `json:"restoredDatabase"`
		Completed time.Time       `json:"completed"`
		Failure   string          `json:"failure"`
		DumpSHA   string          `json:"dumpSHA256"`
		DumpBytes int64           `json:"dumpBytes"`
		Tables    int             `json:"tablesVerified"`
		Checks    map[string]bool `json:"checks"`
	}
	dec := json.NewDecoder(f)
	if dec.Decode(&r) != nil || dec.Decode(new(any)) != io.EOF {
		return errors.New("备份报告格式错误")
	}
	if r.Source != repository.TenantDatabaseName || len(r.Restored) > 64 || !regexp.MustCompile(`^ruoyi_go_mt_restore_[a-zA-Z0-9_]{8,64}$`).MatchString(r.Restored) || r.Failure != "" || r.Tables < 1 {
		return errors.New("备份源/恢复范围或结果不符合要求")
	}
	if r.Completed.IsZero() || r.Completed.After(now.Add(time.Minute)) || now.Sub(r.Completed) > 24*time.Hour {
		return errors.New("备份报告过期或时间不合法，请重新备份")
	}
	for _, key := range []string{"nativeTransactionalDump", "sourceStableDuringBackup", "allTableSchemasAndRowHashes", "uploadCopyHashes", "registeredAttachmentsPresentAndExact", "privateUploadCopyHashes", "registeredPrivateAttachmentsPresentAndExact"} {
		if !r.Checks[key] {
			return errors.New("数据库/公开及私有附件恢复核验未全部通过")
		}
	}
	dump, err := os.Open(filepath.Join(filepath.Dir(path), "database.sql"))
	if err != nil {
		return errors.New("备份SQL不存在")
	}
	defer dump.Close()
	stat, err := dump.Stat()
	if err != nil || !stat.Mode().IsRegular() {
		return errors.New("备份SQL文件类型错误")
	}
	h := sha256.New()
	n, err := io.Copy(h, dump)
	if err != nil || n <= 0 || n != r.DumpBytes || hex.EncodeToString(h.Sum(nil)) != r.DumpSHA {
		return errors.New("备份SQL大小或摘要不符")
	}
	return nil
}
