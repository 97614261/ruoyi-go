package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	portalsql "ruoyi-go/sql/portal"
)

// PortalStructure fingerprints stable schema metadata, excluding changing row counts and sequences.
// The same read-only implementation is used at explicit installation and server startup.
func PortalStructure(ctx context.Context, conn *sql.DB) (string, error) {
	return PortalStructureVersion(ctx, conn, portalsql.CurrentVersion)
}

func PortalStructureVersion(ctx context.Context, conn *sql.DB, version int) (string, error) {
	if version < 1 || version > portalsql.CurrentVersion {
		return "", fmt.Errorf("未知门户结构版本")
	}
	var database string
	if err := conn.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&database); err != nil {
		return "", err
	}
	if database != "ruoyi_go_portal" {
		return "", fmt.Errorf("结构检查仅限门户库")
	}
	rows, err := conn.QueryContext(ctx, "SHOW TABLES FROM ruoyi_go_portal")
	if err != nil {
		return "", err
	}
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return "", err
		}
		names = append(names, name)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", err
	}
	expectedTables := 21
	if version == 2 {
		expectedTables = 24
	}
	if version >= 3 {
		expectedTables = 26
	}
	if version >= 5 {
		expectedTables = 28
	}
	if version >= 6 {
		expectedTables = 29
	}
	if len(names) != expectedTables {
		return "", fmt.Errorf("门户v%d应有%d张表，当前%d", version, expectedTables, len(names))
	}
	sort.Strings(names)
	definitions := []string{"portal-schema-show-create-v1"}
	sequence := regexp.MustCompile(` AUTO_INCREMENT=[0-9]+`)
	// Startup-only bounded metadata inspection; no business-row N+1 queries.
	for _, name := range names {
		var actual, ddl string
		quoted := "`" + strings.ReplaceAll(name, "`", "``") + "`"
		if err := conn.QueryRowContext(ctx, "SHOW CREATE TABLE ruoyi_go_portal."+quoted).Scan(&actual, &ddl); err != nil {
			return "", err
		}
		definitions = append(definitions, sequence.ReplaceAllString(ddl, ""))
	}
	body, err := json.Marshal(definitions)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(body)
	return hex.EncodeToString(hash[:]), nil
}

// ValidatePortalConnection only reads metadata and the installed manifest, and never performs DDL or repair.
func ValidatePortalConnection(ctx context.Context, conn *sql.DB) error {
	return ValidatePortalVersion(ctx, conn, portalsql.CurrentVersion)
}

func ValidatePortalVersion(ctx context.Context, conn *sql.DB, version int) error {
	var database, baseline, expected string
	if err := conn.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&database); err != nil {
		return err
	}
	if database != "ruoyi_go_portal" {
		return fmt.Errorf("门户结构校验拒绝非专用库")
	}
	var count int
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM portal_schema_version").Scan(&count); err != nil {
		return fmt.Errorf("门户未安装结构版本，请运行显式安装工具")
	}
	if count != version {
		return fmt.Errorf("门户结构版本记录异常")
	}
	if err := conn.QueryRowContext(ctx, "SELECT baseline_sha256,structure_sha256 FROM portal_schema_version WHERE version=?", version).Scan(&baseline, &expected); err != nil {
		return fmt.Errorf("门户结构版本不完整")
	}
	if baseline != portalsql.SourceSHA256(version) {
		return fmt.Errorf("门户SQL基线与运行程序不一致")
	}
	actual, err := PortalStructureVersion(ctx, conn, version)
	if err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("门户结构已漂移，请核对显式迁移；不会自动修改数据库")
	}
	return nil
}

func ValidatePortalSchema(ctx context.Context) error {
	if db == nil {
		return fmt.Errorf("数据库未初始化")
	}
	conn, err := db.DB()
	if err != nil {
		return err
	}
	return ValidatePortalConnection(ctx, conn)
}
