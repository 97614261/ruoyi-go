package repository

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
	"time"

	mydriver "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"ruoyi-go/pkg/tenantctx"
)

type ERPPhase1SchemaStatus struct {
	Version         string   `json:"version"`
	DDLHash         string   `json:"ddlHash"`
	InstalledTables int      `json:"installedTables"`
	MissingTables   []string `json:"missingTables"`
	Recorded        bool     `json:"recorded"`
}
type phase1ActualColumn struct {
	TableName, ColumnName, ColumnType, IsNullable, Extra string
	ColumnDefault                                        *string
	CollationName                                        *string
}
type phase1ActualIndex struct {
	TableName, IndexName, ColumnName, IndexType string
	SeqInIndex, NonUnique                       int
	SubPart                                     *int
}
type phase1ActualFK struct {
	TableName, ConstraintName, ColumnName, ReferencedTableName, ReferencedColumnName, DeleteRule, UpdateRule string
	SameSchema                                                                                               int
	OrdinalPosition                                                                                          int
}

func InspectERPPhase1(ctx context.Context) (*ERPPhase1SchemaStatus, error) {
	return inspectPhase1(DB(tenantctx.System(ctx)))
}

// Optional until explicitly installed. Any partial installation or drift fails
// closed. No CREATE/ALTER/seed occurs on the server startup path.
func VerifyERPPhase1Schema(ctx context.Context) error {
	s, err := InspectERPPhase1(ctx)
	if err != nil {
		return err
	}
	if s.InstalledTables == 0 {
		return nil
	}
	if len(s.MissingTables) != 0 || !s.Recorded {
		return fmt.Errorf("ERP一期结构未完整登记，请先备份并显式运行erpphase1")
	}
	return nil
}

var phase1IntegerWidth = regexp.MustCompile(`^(bigint|int)\([0-9]+\)`)

func phase1Type(s string) string {
	return phase1IntegerWidth.ReplaceAllString(strings.ToLower(s), "$1")
}
func equalPhase1Column(want phase1Column, got phase1ActualColumn) bool {
	sameDefault := (want.Default == nil && got.ColumnDefault == nil) || (want.Default != nil && got.ColumnDefault != nil && *want.Default == *got.ColumnDefault)
	coll := ""
	if got.CollationName != nil {
		coll = *got.CollationName
	}
	extra := ""
	if want.Auto {
		extra = "auto_increment"
	}
	return want.Name == got.ColumnName && want.Type == phase1Type(got.ColumnType) && want.Nullable == (got.IsNullable == "YES") && sameDefault && want.Collation == coll && got.Extra == extra
}

func inspectPhase1(q *gorm.DB) (*ERPPhase1SchemaStatus, error) {
	manifest, err := phase1Manifest()
	if err != nil {
		return nil, err
	}
	return inspectERPCreateSchema(q, manifest, ERPPhase1Version, ERPPhase1SHA256())
}

// Shared read-only metadata comparison, not an arbitrary SQL migration engine.
func inspectERPCreateSchema(q *gorm.DB, manifest []phase1Table, version, hash string) (*ERPPhase1SchemaStatus, error) {
	var err error
	names := make([]string, len(manifest))
	for i, t := range manifest {
		names[i] = t.Name
	}
	s := &ERPPhase1SchemaStatus{Version: version, DDLHash: hash, MissingTables: []string{}}
	var tables []struct{ TableName, Engine, TableCollation string }
	if err = q.Raw("SELECT table_name,engine,table_collation FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ?", names).Scan(&tables).Error; err != nil {
		return nil, err
	}
	present := map[string]bool{}
	for _, t := range tables {
		if t.Engine != "InnoDB" || t.TableCollation != "utf8mb4_unicode_ci" {
			return nil, fmt.Errorf("一期表引擎或字符序漂移：%s", t.TableName)
		}
		present[t.TableName] = true
	}
	s.InstalledTables = len(tables)
	for _, name := range names {
		if !present[name] {
			s.MissingTables = append(s.MissingTables, name)
		}
	}
	if len(tables) == 0 && version == ERPPhase1Version {
		return s, nil
	}
	var columns []phase1ActualColumn
	if err = q.Raw("SELECT table_name,column_name,column_type,is_nullable,column_default,extra,collation_name FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name IN ? ORDER BY table_name,ordinal_position", names).Scan(&columns).Error; err != nil {
		return nil, err
	}
	cols := map[string][]phase1ActualColumn{}
	for _, c := range columns {
		cols[c.TableName] = append(cols[c.TableName], c)
	}
	var indexes []phase1ActualIndex
	if err = q.Raw("SELECT table_name,index_name,column_name,seq_in_index,non_unique,sub_part,index_type FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name IN ? ORDER BY table_name,index_name,seq_in_index", names).Scan(&indexes).Error; err != nil {
		return nil, err
	}
	idx := map[string]map[string]phase1Index{}
	for _, v := range indexes {
		if v.SubPart != nil || v.IndexType != "BTREE" {
			return nil, fmt.Errorf("一期索引类型或前缀漂移：%s.%s", v.TableName, v.IndexName)
		}
		if idx[v.TableName] == nil {
			idx[v.TableName] = map[string]phase1Index{}
		}
		x := idx[v.TableName][v.IndexName]
		if x.Columns != "" {
			x.Columns += ","
		}
		x.Columns += v.ColumnName
		x.Unique = v.NonUnique == 0
		idx[v.TableName][v.IndexName] = x
	}
	var foreign []phase1ActualFK
	if err = q.Raw(`SELECT table_name,constraint_name,column_name,referenced_table_name,referenced_column_name,ordinal_position,(referenced_table_schema=DATABASE()) AS same_schema
 FROM information_schema.key_column_usage WHERE constraint_schema=DATABASE() AND referenced_table_name IS NOT NULL AND table_name IN ? ORDER BY table_name,constraint_name,ordinal_position`, names).Scan(&foreign).Error; err != nil {
		return nil, err
	}
	// MySQL 5.7 materializes metadata joins poorly when many restore databases
	// exist. Fetch each bounded schema set once and combine in memory.
	var rules []struct{ TableName, ConstraintName, DeleteRule, UpdateRule string }
	if err = q.Raw("SELECT table_name,constraint_name,delete_rule,update_rule FROM information_schema.referential_constraints WHERE constraint_schema=DATABASE() AND table_name IN ?", names).Scan(&rules).Error; err != nil {
		return nil, err
	}
	ruleMap := map[string][2]string{}
	for _, r := range rules {
		ruleMap[r.TableName+"."+r.ConstraintName] = [2]string{r.DeleteRule, r.UpdateRule}
	}
	fks := map[string]map[string]string{}
	left := map[string]string{}
	right := map[string]string{}
	refs := map[string]string{}
	for _, f := range foreign {
		rule, ok := ruleMap[f.TableName+"."+f.ConstraintName]
		if !ok {
			return nil, fmt.Errorf("一期外键规则缺失：%s", f.ConstraintName)
		}
		f.DeleteRule, f.UpdateRule = rule[0], rule[1]
		if f.SameSchema != 1 {
			return nil, fmt.Errorf("一期外键引用了其他数据库：%s", f.ConstraintName)
		}
		if (f.DeleteRule != "RESTRICT" && f.DeleteRule != "NO ACTION") || (f.UpdateRule != "RESTRICT" && f.UpdateRule != "NO ACTION") {
			return nil, fmt.Errorf("一期外键级联规则漂移：%s", f.ConstraintName)
		}
		k := f.TableName + "." + f.ConstraintName
		if left[k] != "" {
			left[k] += ","
			right[k] += ","
		}
		left[k] += f.ColumnName
		right[k] += f.ReferencedColumnName
		refs[k] = f.ReferencedTableName
		if fks[f.TableName] == nil {
			fks[f.TableName] = map[string]string{}
		}
		fks[f.TableName][f.ConstraintName] = left[k] + ">" + refs[k] + "(" + right[k] + ")"
	}
	for _, t := range manifest {
		if !present[t.Name] {
			continue
		}
		if len(cols[t.Name]) != len(t.Columns) {
			return nil, fmt.Errorf("一期列数量漂移：%s", t.Name)
		}
		for i, c := range t.Columns {
			if !equalPhase1Column(c, cols[t.Name][i]) {
				return nil, fmt.Errorf("一期字段漂移：%s.%s", t.Name, c.Name)
			}
		}
		if len(idx[t.Name]) != len(t.Indexes) {
			return nil, fmt.Errorf("一期索引数量漂移：%s", t.Name)
		}
		for name, want := range t.Indexes {
			if got, ok := idx[t.Name][name]; !ok || got != want {
				return nil, fmt.Errorf("一期索引漂移：%s.%s", t.Name, name)
			}
		}
		if len(fks[t.Name]) != len(t.ForeignKeys) {
			return nil, fmt.Errorf("一期外键数量漂移：%s", t.Name)
		}
		for name, want := range t.ForeignKeys {
			if fks[t.Name][name] != want {
				return nil, fmt.Errorf("一期外键漂移：%s.%s", t.Name, name)
			}
		}
	}
	metadataPresent := present["erp_schema_migration"]
	if !metadataPresent && version != ERPPhase1Version {
		var count int64
		if err = q.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='erp_schema_migration' AND table_type='BASE TABLE'").Scan(&count).Error; err != nil {
			return nil, err
		}
		metadataPresent = count == 1
	}
	if metadataPresent {
		var rows []struct {
			DDLHash string `gorm:"column:ddl_sha256"`
		}
		if err = q.Raw("SELECT ddl_sha256 FROM erp_schema_migration WHERE version_key=?", version).Scan(&rows).Error; err != nil {
			return nil, err
		}
		if len(rows) > 0 {
			if rows[0].DDLHash != s.DDLHash {
				return nil, fmt.Errorf("一期迁移指纹不匹配，禁止改写已发布DDL")
			}
			s.Recorded = true
		}
	}
	return s, nil
}

var phase1RestoreName = regexp.MustCompile(`^ruoyi_go_mt_restore_[a-zA-Z0-9_]{8,64}$`)

func phase1DatabaseAllowed(name string) bool {
	return len(name) <= 64 && (name == TenantDatabaseName || phase1RestoreName.MatchString(name))
}

func phase1LocalConnection(q *gorm.DB) error {
	dialect, ok := q.Dialector.(*gormmysql.Dialector)
	if !ok {
		return fmt.Errorf("一期升级仅允许本地MySQL连接")
	}
	c, err := mydriver.ParseDSN(dialect.DSN)
	if err != nil || c.Net != "tcp" || !phase1DatabaseAllowed(c.DBName) {
		return fmt.Errorf("一期升级拒绝原库和未知连接")
	}
	host, _, err := net.SplitHostPort(c.Addr)
	if err != nil || (host != "localhost" && host != "127.0.0.1" && host != "::1") {
		return fmt.Errorf("一期升级拒绝远程连接")
	}
	return nil
}

// The CLI validates loopback DSN, explicit confirmation and backup evidence.
// Only reviewed CREATE statements execute; existing tables are verified, never
// repaired/overwritten. DDL is not transactional: failures retain partial state.
func InstallERPPhase1(ctx context.Context) (*ERPPhase1SchemaStatus, error) {
	q := DB(tenantctx.System(ctx))
	if err := phase1LocalConnection(q); err != nil {
		return nil, err
	}
	var name string
	if err := q.Raw("SELECT DATABASE()").Scan(&name).Error; err != nil {
		return nil, err
	}
	if !phase1DatabaseAllowed(name) {
		return nil, fmt.Errorf("一期升级拒绝原库或未知数据库")
	}
	if err := VerifyTenantSchema(ctx); err != nil {
		return nil, err
	}
	if err := VerifyERPControls(ctx); err != nil {
		return nil, err
	}
	var coreTables int64
	if err := q.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('erp_sku','erp_warehouse','erp_document','erp_document_line','erp_stock','erp_stock_ledger','erp_warehouse_access','erp_warehouse_policy')").Scan(&coreTables).Error; err != nil {
		return nil, err
	}
	if coreTables != 8 {
		return nil, fmt.Errorf("请先显式安装完整ERP库存底座")
	}
	var result *ERPPhase1SchemaStatus
	err := q.Connection(func(conn *gorm.DB) error {
		// Connection callbacks receive a clone=0 handle. Keep a clean session
		// so a failed CREATE cannot poison the RELEASE_LOCK statement.
		conn = conn.Session(&gorm.Session{NewDB: true})
		var locked int
		lockName := name + ":erp-phase1"
		if err := conn.Raw("SELECT GET_LOCK(?,0)", lockName).Scan(&locked).Error; err != nil {
			return err
		}
		if locked != 1 {
			return fmt.Errorf("另一个一期升级正在运行")
		}
		defer func() {
			release, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			conn.WithContext(tenantctx.System(release)).Exec("DO RELEASE_LOCK(?)", lockName)
		}()
		state, err := inspectPhase1(conn)
		if err != nil {
			return err
		}
		missing := map[string]bool{}
		for _, n := range state.MissingTables {
			missing[n] = true
		}
		manifest, err := phase1Manifest()
		if err != nil {
			return err
		}
		for _, t := range manifest {
			if missing[t.Name] {
				if err = conn.Exec(t.DDL).Error; err != nil {
					return fmt.Errorf("一期建表失败：%s；保留部分结构，请核对后重跑: %w", t.Name, err)
				}
			}
		}
		state, err = inspectPhase1(conn)
		if err != nil {
			return err
		}
		if len(state.MissingTables) != 0 {
			return fmt.Errorf("一期结构仍不完整")
		}
		if !state.Recorded {
			if err = conn.Exec("INSERT INTO erp_schema_migration(version_key,ddl_sha256,create_by,create_time,remark) VALUES (?,?,?,NOW(),?)", ERPPhase1Version, ERPPhase1SHA256(), "erpphase1", "explicit local migration; no grants or business seeds").Error; err != nil {
				return err
			}
		}
		result, err = inspectPhase1(conn)
		return err
	})
	return result, err
}

// Sorted names are useful for diagnostics/tests, without exposing connections.
func ERPPhase1TableNames() []string {
	names := append([]string(nil), phase1BusinessTables...)
	names = append(names, "erp_schema_migration")
	sort.Strings(names)
	return names
}
