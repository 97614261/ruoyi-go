package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/modelcatalog"
	"ruoyi-go/pkg/redisx"
	"ruoyi-go/pkg/storage"
)

type bundleEntry struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}
type bundleManifest struct {
	Format     int           `json:"format"`
	InstanceID string        `json:"instanceId"`
	Database   string        `json:"database"`
	CreatedAt  time.Time     `json:"createdAt"`
	Files      []bundleEntry `json:"files"`
}

var bundleAssetName = regexp.MustCompile(`^[a-f0-9]{32}\.(png|jpg|webp)$`)

func bundleOffline(ctx context.Context, cfg *config.Config) error {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", fmt.Sprint(cfg.Server.Port)), 400*time.Millisecond)
	if err == nil {
		conn.Close()
		return errors.New("请先停止本实例 API 和 Worker，避免备份期间写入")
	}
	if err = redisx.Init(ctx, redisx.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB}); err != nil {
		return errors.New("无法核对 Redis 归属和 Worker 状态")
	}
	defer redisx.Close()
	owner, err := redisx.C().Get(ctx, redisx.PODInstanceKey).Result()
	if err != nil || owner != cfg.POD.InstanceID {
		return errors.New("Redis 归属不匹配")
	}
	heartbeat, err := redisx.C().Exists(ctx, "pod:worker:heartbeat:"+cfg.POD.InstanceID).Result()
	if err != nil || heartbeat != 0 {
		return errors.New("请等待 Worker 停止后的 10 秒心跳过期再备份")
	}
	return nil
}
func copyBundleFile(ctx context.Context, source, target string) (bundleEntry, error) {
	var out bundleEntry
	info, err := os.Lstat(source)
	if err != nil || !info.Mode().IsRegular() {
		return out, errors.New("备份源不是普通文件")
	}
	if info.Size() > 256<<20 {
		return out, errors.New("单文件超过 256 MiB，本地交付备份不支持")
	}
	in, err := os.Open(source)
	if err != nil {
		return out, err
	}
	defer in.Close()
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return out, err
	}
	defer file.Close()
	hash := sha256.New()
	n, err := storage.Copy(ctx, io.MultiWriter(file, hash), io.LimitReader(in, info.Size()+1))
	if err != nil {
		return out, err
	}
	if n != info.Size() {
		return out, errors.New("备份期间文件发生变化")
	}
	if err = file.Sync(); err != nil {
		return out, err
	}
	return bundleEntry{Bytes: n, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}
func backupBundle(ctx context.Context, cfg *config.Config, configPath, executable string) error {
	if err := bundleOffline(ctx, cfg); err != nil {
		return err
	}
	var dump string
	if err := backupSQL(ctx, cfg, executable, &dump); err != nil {
		return err
	}
	root, err := os.MkdirTemp(".local/backups", "bundle-"+time.Now().Format("20060102-150405")+"-*.partial")
	if err != nil {
		return err
	}
	manifest := bundleManifest{Format: 1, InstanceID: cfg.POD.InstanceID, Database: cfg.POD.DatabaseName, CreatedAt: time.Now().UTC(), Files: []bundleEntry{}}
	add := func(source, name string) error {
		target := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		entry, err := copyBundleFile(ctx, source, target)
		if err != nil {
			return err
		}
		entry.Path = name
		manifest.Files = append(manifest.Files, entry)
		return nil
	}
	if err = add(dump, "database.sql"); err != nil {
		return err
	}
	if err = add(configPath, "config.snapshot.yml"); err != nil {
		return err
	}
	if _, err = os.Stat(".local/accounts.json"); err == nil {
		if err = add(".local/accounts.json", "accounts.local.json"); err != nil {
			return err
		}
	}
	files, err := bundleStorageFiles(cfg.POD.StorageRoot)
	if err != nil {
		return err
	}
	for _, name := range files {
		if err = add(filepath.Join(cfg.POD.StorageRoot, filepath.FromSlash(name)), "storage/"+name); err != nil {
			return err
		}
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(root, "manifest.json"), raw, 0600); err != nil {
		return err
	}
	completed := strings.TrimSuffix(root, ".partial")
	if err = os.Rename(root, completed); err != nil {
		return err
	}
	fmt.Printf("完整本地备份已生成：%s（%d 个校验文件，含私有配置和素材）。请妥善保管；运行 restore-check 验证独立恢复。\n", completed, len(manifest.Files))
	return nil
}

func allowedBundlePath(name string) bool {
	return name == "database.sql" || name == "config.snapshot.yml" || name == "accounts.local.json" || (strings.HasPrefix(name, "storage/") && bundleAssetName.MatchString(strings.TrimPrefix(name, "storage/"))) || (strings.HasPrefix(name, "storage/catalog/") && bundleAssetName.MatchString(strings.TrimPrefix(name, "storage/catalog/"))) || (strings.HasPrefix(name, "storage/"+storage.ReceiptDirectory+"/") && storage.ValidReceiptName(strings.TrimPrefix(name, "storage/"+storage.ReceiptDirectory+"/")))
}

// Each private storage namespace is flat; other directories are ignored.
func bundleStorageFiles(root string) ([]string, error) {
	files := []string{}
	var total int64
	var visit func(string) error
	visit = func(prefix string) error {
		entries, err := os.ReadDir(filepath.Join(root, prefix))
		if os.IsNotExist(err) && prefix == "" {
			return nil
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			name := filepath.Join(prefix, entry.Name())
			info, err := os.Lstat(filepath.Join(root, name))
			if err != nil {
				return err
			}
			if prefix == "" && (entry.Name() == "catalog" || entry.Name() == storage.ReceiptDirectory) && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
				if err := visit(entry.Name()); err != nil {
					return err
				}
				continue
			}
			if !allowedBundlePath("storage/"+filepath.ToSlash(name)) || !info.Mode().IsRegular() {
				return errors.New("素材目录存在非预期文件，请核对后再备份")
			}
			total += info.Size()
			files = append(files, filepath.ToSlash(name))
			if len(files) > 10000 || total > 2<<30 {
				return errors.New("素材超过 10000 文件或 2 GiB，请使用运维级备份")
			}
		}
		return nil
	}
	err := visit("")
	return files, err
}
func verifyBundle(ctx context.Context, path string) (*bundleManifest, error) {
	raw, err := os.ReadFile(filepath.Join(path, "manifest.json"))
	if err != nil || len(raw) > 4<<20 {
		return nil, errors.New("备份清单不可用")
	}
	var manifest bundleManifest
	if json.Unmarshal(raw, &manifest) != nil || manifest.Format != 1 || len(manifest.Files) < 2 || len(manifest.Files) > 10003 {
		return nil, errors.New("备份清单格式不合法")
	}
	seen := map[string]bool{}
	var total int64
	for _, entry := range manifest.Files {
		if !allowedBundlePath(entry.Path) || seen[entry.Path] || entry.Bytes < 0 || entry.Bytes > 256<<20 {
			return nil, errors.New("备份清单路径或大小不合法")
		}
		seen[entry.Path] = true
		total += entry.Bytes
		if total > 3<<30 {
			return nil, errors.New("备份超过本地恢复检查上限")
		}
		filePath := filepath.Join(path, filepath.FromSlash(entry.Path))
		if strings.HasPrefix(entry.Path, "storage/") {
			parents := []string{filepath.Join(path, "storage")}
			if strings.HasPrefix(entry.Path, "storage/catalog/") {
				parents = append(parents, filepath.Join(path, "storage", "catalog"))
			}
			if strings.HasPrefix(entry.Path, "storage/"+storage.ReceiptDirectory+"/") {
				parents = append(parents, filepath.Join(path, "storage", storage.ReceiptDirectory))
			}
			for _, parent := range parents {
				info, err := os.Lstat(parent)
				if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
					return nil, errors.New("备份素材目录不合法")
				}
			}
		}
		info, err := os.Lstat(filePath)
		if err != nil || !info.Mode().IsRegular() || info.Size() != entry.Bytes {
			return nil, errors.New("备份文件缺失或大小不匹配")
		}
		file, err := os.Open(filePath)
		if err != nil {
			return nil, err
		}
		hash := sha256.New()
		n, copyErr := storage.Copy(ctx, hash, io.LimitReader(file, entry.Bytes+1))
		file.Close()
		if copyErr != nil || n != entry.Bytes || hex.EncodeToString(hash.Sum(nil)) != entry.SHA256 {
			return nil, errors.New("备份文件校验失败")
		}
	}
	if !seen["database.sql"] || !seen["config.snapshot.yml"] {
		return nil, errors.New("备份缺少数据库或配置")
	}
	return &manifest, nil
}

// Always creates a new local database and directory. It never attaches a queue or starts a worker.
func restoreCheck(ctx context.Context, cfg *config.Config, path string, upgradeAdmin, upgradeCatalog bool, creatorUpgrade ...bool) error {
	upgradeCreators := len(creatorUpgrade) > 0 && creatorUpgrade[0]
	upgradeManaged := len(creatorUpgrade) > 1 && creatorUpgrade[1]
	upgradeOfficial := len(creatorUpgrade) > 2 && creatorUpgrade[2]
	manifest, err := verifyBundle(ctx, path)
	if err != nil {
		return err
	}
	if manifest.InstanceID != cfg.POD.InstanceID || manifest.Database != cfg.POD.DatabaseName {
		return errors.New("备份不属于当前配置实例，拒绝恢复检查")
	}
	dsn, err := mysql.ParseDSN(cfg.MySQL.DSN)
	if err != nil {
		return errors.New("恢复连接配置不合法")
	}
	host, _, err := net.SplitHostPort(dsn.Addr)
	if err != nil || dsn.Net != "tcp" || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return errors.New("仅允许本地恢复检查")
	}
	target := "pod_studio_restore_" + time.Now().Format("20060102_150405") + "_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	original := *dsn
	dsn.DBName = ""
	server, err := sql.Open("mysql", dsn.FormatDSN())
	if err != nil {
		return errors.New("恢复服务器连接不可用")
	}
	defer server.Close()
	// No IF NOT EXISTS: even a name collision must fail without touching the existing database.
	if _, err = server.ExecContext(ctx, "CREATE DATABASE `"+target+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		return errors.New("新恢复库创建失败，未覆盖既有库")
	}
	fmt.Printf("独立恢复目标：%s；失败时保留供排查。\n", target)
	original.DBName = target
	original.MultiStatements = true
	db, err := sql.Open("mysql", original.FormatDSN())
	if err != nil {
		return errors.New("新恢复库连接失败")
	}
	defer db.Close()
	info, err := os.Stat(filepath.Join(path, "database.sql"))
	if err != nil || info.Size() > 32<<20 {
		return errors.New("本地自动恢复检查仅支持 32 MiB 内 SQL；更大备份请用 mysql 客户端")
	}
	script, err := os.ReadFile(filepath.Join(path, "database.sql"))
	if err != nil {
		return err
	}
	if _, err = db.ExecContext(ctx, string(script)); err != nil {
		return errors.New("新恢复库导入失败，保留供排查；原库未修改")
	}
	restoreDir, err := os.MkdirTemp(".local", target+"-*")
	if err != nil {
		return err
	}
	assetDir := filepath.Join(restoreDir, "storage")
	if err = os.Mkdir(assetDir, 0700); err != nil {
		return err
	}
	for _, entry := range manifest.Files {
		if strings.HasPrefix(entry.Path, "storage/") {
			target := filepath.Join(assetDir, filepath.FromSlash(strings.TrimPrefix(entry.Path, "storage/")))
			if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return err
			}
			if _, err = copyBundleFile(ctx, filepath.Join(path, filepath.FromSlash(entry.Path)), target); err != nil {
				return err
			}
		}
	}
	// Restore using the copied storage, not paths from the source configuration.
	restored := cfg.MySQL
	original.MultiStatements = false
	restored.DSN = original.FormatDSN()
	if err = repository.Init(restored); err != nil {
		return errors.New("恢复库验证连接失败")
	}
	defer repository.Close()
	var latest string
	if err = repository.DB(ctx).Table("pod_schema_version").Select("version").Order("version DESC").Limit(1).Scan(&latest).Error; err != nil {
		return errors.New("恢复版本读取失败")
	}
	if latest == "007" {
		err = repository.VerifyPODSchema007(ctx, manifest.InstanceID)
	} else if latest == "008" {
		err = repository.VerifyPODSchema008(ctx, manifest.InstanceID)
	} else if latest == "009" {
		err = repository.VerifyPODSchema009(ctx, manifest.InstanceID)
	} else if latest == "010" {
		err = repository.VerifyPODSchema010(ctx, manifest.InstanceID)
	} else if latest == "011" {
		err = repository.VerifyPODSchema011(ctx, manifest.InstanceID)
	} else if latest == "012" {
		err = repository.VerifyPODSchema012(ctx, manifest.InstanceID)
	} else if latest == "013" {
		err = repository.VerifyPODSchema013(ctx, manifest.InstanceID)
	} else if latest == "014" {
		err = repository.VerifyPODSchema014(ctx, manifest.InstanceID)
	} else if latest == "015" {
		err = repository.VerifyPODSchema(ctx, manifest.InstanceID)
	} else {
		return errors.New("恢复包版本不受支持")
	}
	if err != nil {
		return errors.New("恢复库 schema 校验失败")
	}
	if upgradeAdmin || upgradeCatalog || upgradeCreators || upgradeManaged || upgradeOfficial {
		copyConfig := *cfg
		copyConfig.MySQL = restored
		copyConfig.POD.DatabaseName = target
		copyConfig.POD.InstanceID = manifest.InstanceID
		if upgradeOfficial {
			err = repository.MigratePODOfficialTemplates(ctx, &copyConfig)
		} else if upgradeManaged {
			err = repository.MigratePODManaged(ctx, &copyConfig)
		} else if upgradeCreators {
			err = repository.MigratePODCreators(ctx, &copyConfig)
		} else if upgradeCatalog {
			err = repository.MigratePODModelCatalog(ctx, &copyConfig)
		} else {
			err = repository.MigratePODAdminFoundation(ctx, &copyConfig)
		}
		if err != nil {
			return fmt.Errorf("副本升级失败，保留现场: %w", err)
		}
		if upgradeOfficial {
			err = repository.VerifyPODSchema012(ctx, manifest.InstanceID)
			if latest < "012" {
				latest = "012"
			}
		} else if upgradeManaged {
			err = repository.VerifyPODSchema011(ctx, manifest.InstanceID)
			latest = max(latest, "011")
		} else if upgradeCreators {
			err = repository.VerifyPODSchema010(ctx, manifest.InstanceID)
			latest = max(latest, "010")
		} else if upgradeCatalog {
			err = repository.VerifyPODSchema009(ctx, manifest.InstanceID)
			latest = max(latest, "009")
		} else {
			err = repository.VerifyPODSchema008(ctx, manifest.InstanceID)
			if latest < "008" {
				latest = "008"
			}
		}
		if err != nil {
			return errors.New("副本升级验证失败")
		}
	}
	var files []model.PODAssetFile
	if err = repository.DB(ctx).Table("pod_asset_file").Find(&files).Error; err != nil {
		return errors.New("恢复素材记录读取失败")
	}
	for _, record := range files {
		input, err := (storage.Local{Path: assetDir}).Open(ctx, record.StorageKey)
		if err != nil {
			return errors.New("恢复素材文件缺失")
		}
		hash := sha256.New()
		n, copyErr := storage.Copy(ctx, hash, io.LimitReader(input, record.ByteSize+1))
		input.Close()
		if copyErr != nil || n != record.ByteSize || hex.EncodeToString(hash.Sum(nil)) != record.SHA256 {
			return errors.New("恢复素材与数据库摘要不一致")
		}
	}
	var officialFiles []model.PODAssetFile
	var officialCounts *repository.PODOfficialRestoreCounts
	verifiedPendingFiles := 0
	if latest >= "012" {
		officialCounts, err = repository.PODVerifyOfficialData(ctx)
		if err != nil {
			return err
		}
		if err = repository.DB(ctx).Table("pod_catalog_asset_file").Find(&officialFiles).Error; err != nil {
			return err
		}
		for _, record := range officialFiles {
			input, err := (storage.Local{Path: filepath.Join(assetDir, "catalog")}).Open(ctx, record.StorageKey)
			if err != nil {
				return errors.New("恢复平台素材缺失")
			}
			hash := sha256.New()
			n, copyErr := storage.Copy(ctx, hash, io.LimitReader(input, record.ByteSize+1))
			input.Close()
			if copyErr != nil || n != record.ByteSize || hex.EncodeToString(hash.Sum(nil)) != record.SHA256 {
				return errors.New("恢复平台素材与数据库摘要不一致")
			}
		}
		planned, err := repository.PODOfficialPendingCopyFiles(ctx)
		if err != nil {
			return err
		}
		for _, record := range planned {
			input, err := (storage.Local{Path: assetDir}).Open(ctx, record.StorageKey)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return errors.New("恢复中断复制文件不可用")
			}
			hash := sha256.New()
			n, copyErr := storage.Copy(ctx, hash, io.LimitReader(input, record.ByteSize+1))
			input.Close()
			if copyErr != nil || n != record.ByteSize || hex.EncodeToString(hash.Sum(nil)) != record.SHA256 {
				return errors.New("恢复中断复制文件与计划摘要不一致")
			}
			verifiedPendingFiles++
		}
	}
	var counts struct {
		Projects      int64
		Runs          int64
		References    int64
		Batches       int64
		AdminAudits   int64
		Providers     int64
		Credentials   int64
		Models        int64
		ModelVersions int64
	}
	if err = repository.DB(ctx).Raw("SELECT (SELECT COUNT(*) FROM pod_project) projects,(SELECT COUNT(*) FROM pod_run) runs,((SELECT COUNT(*) FROM pod_project_asset_ref)+(SELECT COUNT(*) FROM pod_template_asset_ref)+(SELECT COUNT(*) FROM pod_run_asset_ref)+(SELECT COUNT(*) FROM pod_batch_asset_ref)) `references`,(SELECT COUNT(*) FROM pod_batch) batches").Scan(&counts).Error; err != nil {
		return errors.New("恢复行数核验失败")
	}
	if latest >= "008" {
		if err = repository.DB(ctx).Table("pod_admin_audit").Count(&counts.AdminAudits).Error; err != nil {
			return errors.New("恢复审计行数核验失败")
		}
	}
	verifiedVersions, decryptedCredentials := 0, 0
	if latest >= "009" {
		// Scan resets the destination struct; keep earlier counts in a separate value.
		var catalogCounts struct{ Providers, Credentials, Models, ModelVersions int64 }
		if err = repository.DB(ctx).Raw("SELECT (SELECT COUNT(*) FROM pod_provider) providers,(SELECT COUNT(*) FROM pod_provider_credential_version) credentials,(SELECT COUNT(*) FROM pod_model) models,(SELECT COUNT(*) FROM pod_model_version) model_versions").Scan(&catalogCounts).Error; err != nil {
			return errors.New("恢复模型目录计数失败")
		}
		counts.Providers, counts.Credentials, counts.Models, counts.ModelVersions = catalogCounts.Providers, catalogCounts.Credentials, catalogCounts.Models, catalogCounts.ModelVersions
		vault, err := modelcatalog.NewVault(cfg.POD.Catalog.KeyringFile, cfg.JWT.Secret, cfg.POD.Catalog.CredentialEnvs)
		if err != nil {
			return errors.New("恢复校验所需的独立主密钥不可用")
		}
		verifiedVersions, decryptedCredentials, err = repository.PODCatalogVerifyRestore(ctx, vault)
		if err != nil {
			return err
		}
	}
	if latest >= "010" {
		if err := repository.PODVerifyCreatorAccounts(ctx); err != nil {
			return err
		}
	}
	report := map[string]any{"database": target, "storage": assetDir, "schema": "001-" + latest, "adminUpgradeRehearsed": upgradeAdmin, "catalogUpgradeRehearsed": upgradeCatalog, "verifiedFiles": len(files), "counts": counts, "workerStarted": false, "queueConnected": false, "sourceUntouched": true}
	report["creatorUpgradeRehearsed"] = upgradeCreators
	report["creatorAccountsVerified"] = latest >= "010"
	if latest >= "011" {
		if err := repository.PODVerifyManagedAccounts(ctx); err != nil {
			return err
		}
	}
	report["officialUpgradeRehearsed"] = upgradeOfficial
	report["officialDataVerified"] = latest >= "012"
	report["officialCounts"] = officialCounts
	report["verifiedOfficialFiles"] = len(officialFiles)
	report["verifiedPendingCopyFiles"] = verifiedPendingFiles
	report["managedUpgradeRehearsed"] = upgradeManaged
	report["managedAccountsVerified"] = latest >= "011"
	report["verifiedModelVersions"] = verifiedVersions
	report["decryptedCredentialVersions"] = decryptedCredentials
	report["environmentCredentialValuesVerified"] = false
	data, _ := json.MarshalIndent(report, "", "  ")
	if err = os.WriteFile(filepath.Join(restoreDir, "restore-report.json"), data, 0600); err != nil {
		return err
	}
	fmt.Printf("独立恢复检查通过：%d 个私人素材、%d 个平台素材、%d 个中断复制文件逐一 SHA256 校验；报告 %s。副本未连接 Redis、未启动服务。\n", len(files), len(officialFiles), verifiedPendingFiles, filepath.Join(restoreDir, "restore-report.json"))
	return nil
}
