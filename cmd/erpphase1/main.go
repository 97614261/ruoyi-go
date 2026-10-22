// Command erpphase1 inspects by default; CREATE requires explicit confirmation
// and verified local backup evidence. No business records or grants are seeded.
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
	configPath := flag.String("config", "configs/multitenant.local.yml", "dedicated local configuration")
	apply := flag.Bool("apply", false, "explicitly install the reviewed phase-1 schema")
	confirm := flag.String("confirm-database", "", "must match the dedicated database for writes")
	backup := flag.String("backup-report", "", "verified native backup/restore report, no older than 24h")
	flag.Parse()
	cfg, err := config.Load(*configPath)
	if err != nil {
		return errors.New("读取独立本地配置失败")
	}
	if err = validatePhase1DSN(cfg.MySQL.DSN); err != nil {
		return err
	}
	if *apply {
		if *confirm != repository.TenantDatabaseName {
			return errors.New("写入必须显式确认独立库名")
		}
		if err = validatePhase1Backup(*backup, time.Now().UTC()); err != nil {
			return err
		}
	}
	parsed, _ := mysql.ParseDSN(cfg.MySQL.DSN)
	parsed.MultiStatements = false
	cfg.MySQL.DSN = parsed.FormatDSN()
	if err = repository.Init(cfg.MySQL); err != nil {
		return errors.New("连接本地独立多租户库失败，未执行一期迁移")
	}
	defer repository.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	var result *repository.ERPPhase1SchemaStatus
	if *apply {
		result, err = repository.InstallERPPhase1(ctx)
	} else {
		result, err = repository.InspectERPPhase1(ctx)
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Apply bool `json:"apply"`
		*repository.ERPPhase1SchemaStatus
	}{*apply, result})
}
func validatePhase1DSN(raw string) error {
	c, err := mysql.ParseDSN(raw)
	if err != nil || c.Net != "tcp" || c.DBName != repository.TenantDatabaseName {
		return errors.New("仅允许本地ruoyi_go_mt，拒绝原库、恢复副本和未知连接")
	}
	host, _, err := net.SplitHostPort(c.Addr)
	if err != nil || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		return errors.New("仅允许本地MySQL")
	}
	return nil
}

// Evidence gate, not a replacement for a maintenance window. The operator must
// keep writers stopped and compare source snapshots before/after the upgrade.
func validatePhase1Backup(path string, now time.Time) error {
	if path == "" {
		return errors.New("先备份并恢复核验，再提供-backup-report")
	}
	f, err := os.Open(path)
	if err != nil {
		return errors.New("备份报告不可读")
	}
	defer f.Close()
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
	if json.NewDecoder(io.LimitReader(f, 16<<20)).Decode(&r) != nil {
		return errors.New("备份报告格式错误")
	}
	if r.Source != repository.TenantDatabaseName || len(r.Restored) > 64 || !regexp.MustCompile(`^ruoyi_go_mt_restore_[a-zA-Z0-9_]{8,64}$`).MatchString(r.Restored) || r.Failure != "" || r.Tables < 1 {
		return errors.New("备份源/恢复范围或结果不符合要求")
	}
	if r.Completed.IsZero() || r.Completed.After(now.Add(time.Minute)) || now.Sub(r.Completed) > 24*time.Hour {
		return errors.New("备份报告过期或时间不合法，请重新备份")
	}
	for _, key := range []string{"nativeTransactionalDump", "sourceStableDuringBackup", "allTableSchemasAndRowHashes", "uploadCopyHashes", "registeredAttachmentsPresentAndExact"} {
		if !r.Checks[key] {
			return errors.New("备份/恢复/附件核验未全部通过")
		}
	}
	dump, err := os.Open(filepath.Join(filepath.Dir(path), "database.sql"))
	if err != nil {
		return errors.New("备份SQL不存在")
	}
	defer dump.Close()
	h := sha256.New()
	n, err := io.Copy(h, dump)
	if err != nil || n <= 0 || n != r.DumpBytes || hex.EncodeToString(h.Sum(nil)) != r.DumpSHA {
		return errors.New("备份SQL大小或摘要不符")
	}
	return nil
}
