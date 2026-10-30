// portalrecoverycheck only creates a new local recovery copy. It never drops or overwrites a database.
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	mysql "github.com/go-sql-driver/mysql"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/repository"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "恢复验收停止:", err)
		os.Exit(1)
	}
}
func run() error {
	confirm := flag.Bool("confirm", false, "服务已停，创建全新本地恢复副本并核验；不替换现库")
	clientDir := flag.String("mysql-bin", `C:\Program Files\MySQL\MySQL Server 5.7\bin`, "官方MySQL客户端目录")
	flag.Parse()
	if !*confirm {
		fmt.Println("预览：仅本地ruoyi_go_portal，停服务后备份SQL/私有配置/素材到logs，再创建全新ruoyi_go_portal_restore_时间戳副本并校验；不会覆盖、删除或启动恢复副本。")
		return nil
	}
	cfg, err := config.Load("configs/portal.local.yml")
	if err != nil {
		return fmt.Errorf("无法读取门户私有配置")
	}
	d, err := mysql.ParseDSN(cfg.MySQL.DSN)
	if err != nil || !recoveryTargetAllowed(cfg, d) {
		return fmt.Errorf("仅允许本地专用门户库")
	}
	if conn, e := net.DialTimeout("tcp", "127.0.0.1:8085", time.Second); e == nil {
		conn.Close()
		return fmt.Errorf("请先停止本地门户服务")
	}
	d.Addr = "127.0.0.1:3306"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db, err := sql.Open("mysql", d.FormatDSN())
	if err != nil {
		return err
	}
	defer db.Close()
	if err = repository.ValidatePortalConnection(ctx, db); err != nil {
		return err
	}
	stamp := fmt.Sprint(time.Now().UnixNano())
	folder := filepath.Join("logs", "portal-step56", "recovery-"+stamp)
	if err = os.MkdirAll(folder, 0700); err != nil {
		return err
	}
	dump := filepath.Join(folder, "database.sql")
	clientFile, err := filepath.Abs(filepath.Join(folder, "mysql-client.local.cnf"))
	if err != nil {
		return err
	}
	if err = os.WriteFile(clientFile, recoveryClientOptions(d), 0600); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, filepath.Join(*clientDir, "mysqldump"), "--defaults-file="+clientFile, "--host=127.0.0.1", "--port=3306", "--protocol=tcp", "--single-transaction", "--default-character-set=utf8mb4", "--hex-blob", "--set-gtid-purged=OFF", "--result-file="+dump, "ruoyi_go_portal")
	if err = cmd.Run(); err != nil {
		return fmt.Errorf("官方mysqldump备份失败（凭证未输出）")
	}
	manifest := map[string]string{}
	hashFile := func(path string) (string, error) {
		f, e := os.Open(path)
		if e != nil {
			return "", e
		}
		defer f.Close()
		h := sha256.New()
		_, e = io.Copy(h, f)
		return hex.EncodeToString(h.Sum(nil)), e
	}
	manifest["mysql-client.local.cnf"], err = hashFile(clientFile)
	if err != nil {
		return err
	}
	copyFile := func(source, target string) error {
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		in, err := os.Open(source)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, e := io.Copy(out, in)
		ce := out.Close()
		if e != nil {
			return e
		}
		return ce
	}
	for _, source := range []string{"configs/portal.local.yml", "logs/portal-accounts.local.json"} {
		target := filepath.Join(folder, "private", filepath.Base(source))
		if err = copyFile(source, target); err != nil {
			return err
		}
		digest, e := hashFile(target)
		if e != nil {
			return e
		}
		manifest[filepath.ToSlash(strings.TrimPrefix(target, folder+string(filepath.Separator)))] = digest
	}
	root := cfg.Portal.PrivateAssetPath
	if root == "" {
		root = "uploadPath-portal-private"
	}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("素材目录包含符号链接，停止")
		}
		if entry.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(root, path)
		if e != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("素材越界")
		}
		target := filepath.Join(folder, "assets", rel)
		if e = copyFile(path, target); e != nil {
			return e
		}
		digest, e := hashFile(target)
		if e != nil {
			return e
		}
		manifest["assets/"+filepath.ToSlash(rel)] = digest
		return nil
	})
	if err != nil {
		return err
	}
	manifest["database.sql"], err = hashFile(dump)
	if err != nil {
		return err
	}
	targetDB := "ruoyi_go_portal_restore_" + stamp
	if !regexp.MustCompile(`^ruoyi_go_portal_restore_[0-9]+$`).MatchString(targetDB) {
		return fmt.Errorf("恢复库名称不合法")
	}
	if _, err = db.ExecContext(ctx, "CREATE DATABASE `"+targetDB+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		return fmt.Errorf("恢复库创建失败，不覆盖现有库")
	}
	input, err := os.Open(dump)
	if err != nil {
		return err
	}
	restore := exec.CommandContext(ctx, filepath.Join(*clientDir, "mysql"), "--defaults-file="+clientFile, "--host=127.0.0.1", "--port=3306", "--protocol=tcp", "--default-character-set=utf8mb4", targetDB)
	restore.Stdin = input
	err = restore.Run()
	input.Close()
	if err != nil {
		return fmt.Errorf("导入新恢复副本失败，保留现场")
	}
	rows, err := db.QueryContext(ctx, "SHOW TABLES")
	if err != nil {
		return err
	}
	tables := []string{}
	for rows.Next() {
		var table string
		if err = rows.Scan(&table); err != nil {
			rows.Close()
			return err
		}
		tables = append(tables, table)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	seq := regexp.MustCompile(` AUTO_INCREMENT=[0-9]+`)
	for _, table := range tables {
		if !regexp.MustCompile(`^[a-z0-9_]+$`).MatchString(table) {
			return fmt.Errorf("表名不合法")
		}
		var n, a, b string
		if err = db.QueryRowContext(ctx, "SHOW CREATE TABLE `ruoyi_go_portal`.`"+table+"`").Scan(&n, &a); err != nil {
			return err
		}
		if err = db.QueryRowContext(ctx, "SHOW CREATE TABLE `"+targetDB+"`.`"+table+"`").Scan(&n, &b); err != nil {
			return err
		}
		if seq.ReplaceAllString(a, "") != seq.ReplaceAllString(b, "") {
			return fmt.Errorf("恢复表定义不一致: %s", table)
		}
		var left, right sql.NullInt64
		if err = db.QueryRowContext(ctx, "CHECKSUM TABLE `ruoyi_go_portal`.`"+table+"` EXTENDED").Scan(&n, &left); err != nil {
			return err
		}
		if err = db.QueryRowContext(ctx, "CHECKSUM TABLE `"+targetDB+"`.`"+table+"` EXTENDED").Scan(&n, &right); err != nil {
			return err
		}
		if !left.Valid || !right.Valid || left.Int64 != right.Int64 {
			return fmt.Errorf("恢复表数据校验不一致: %s", table)
		}
	}
	for path, digest := range manifest {
		if strings.HasPrefix(path, "assets/") {
			source := filepath.Join(folder, filepath.FromSlash(path))
			target := filepath.Join(folder, "restored-assets", strings.TrimPrefix(filepath.FromSlash(path), "assets"+string(filepath.Separator)))
			if err = copyFile(source, target); err != nil {
				return err
			}
			got, e := hashFile(target)
			if e != nil || got != digest {
				return fmt.Errorf("附件恢复校验失败")
			}
		}
	}
	report := map[string]any{"database": targetDB, "source": "ruoyi_go_portal", "tables": len(tables), "schemaEqual": true, "tableChecksumsEqual": true, "fileSHA256": manifest, "createdAt": time.Now().UTC(), "note": "副本未启动服务；备份含私有配置和账号，必须限制访问"}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(folder, "manifest.local.json"), body, 0600); err != nil {
		return err
	}
	fmt.Printf("PASS 恢复副本=%s，%d表定义与数据校验一致，附件SHA256一致；备份目录=%s。未覆盖源库，未启动副本。\n", targetDB, len(tables), folder)
	return nil
}

func recoveryTargetAllowed(cfg *config.Config, d *mysql.Config) bool {
	return cfg != nil && d != nil && d.Net == "tcp" && d.DBName == "ruoyi_go_portal" && (d.Addr == "127.0.0.1:3306" || d.Addr == "localhost:3306") && cfg.Redis.DB == 14 && cfg.Server.Port == 8085 && cfg.Portal.ListenHost == "127.0.0.1" && !strings.ContainsRune(d.User, 0) && !strings.ContainsRune(d.Passwd, 0)
}
func recoveryClientOptions(d *mysql.Config) []byte {
	quote := func(s string) string {
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`).Replace(s) + `"`
	}
	return []byte("[client]\nuser=" + quote(d.User) + "\npassword=" + quote(d.Passwd) + "\nhost=127.0.0.1\nport=3306\nprotocol=tcp\n")
}
