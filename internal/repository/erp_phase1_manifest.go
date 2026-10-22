package repository

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"

	erpsql "ruoyi-go/sql/erp"
)

const ERPPhase1Version = "003_order_fulfillment"

var phase1BusinessTables = []string{
	"erp_shop", "erp_shop_access", "erp_shop_policy", "erp_shop_item", "erp_shop_sku_mapping",
	"erp_order", "erp_order_line", "erp_order_event", "erp_package", "erp_package_line",
	"erp_import_job", "erp_import_result",
}

type phase1Column struct {
	Name      string
	Type      string
	Nullable  bool
	Default   *string
	Auto      bool
	Collation string
}
type phase1Index struct {
	Columns string
	Unique  bool
}
type phase1Table struct {
	Name, DDL   string
	Columns     []phase1Column
	Indexes     map[string]phase1Index
	ForeignKeys map[string]string
}

func ERPPhase1SHA256() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ReplaceAll(erpsql.Phase1, "\r\n", "\n"))))
}

// This parses only the deliberately narrow, embedded CREATE manifest, never
// user SQL. One definition per line; no ALTER/DML or arbitrary identifiers.
func phase1Manifest() ([]phase1Table, error) { return parsePhase1Manifest(erpsql.Phase1) }
func parsePhase1Manifest(script string) ([]phase1Table, error) {
	var clean []string
	for _, line := range strings.Split(script, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			clean = append(clean, line)
		}
	}
	allowed := map[string]bool{"erp_schema_migration": true}
	for _, name := range phase1BusinessTables {
		allowed[name] = true
	}
	header := regexp.MustCompile(`^CREATE TABLE ([a-z0-9_]+) \(\n([\s\S]+)\n\) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci$`)
	col := regexp.MustCompile(`^([a-z0-9_]+) ([A-Z]+(?:\([0-9,]+\))?)(.*)$`)
	index := regexp.MustCompile(`^(PRIMARY KEY|UNIQUE KEY [a-z0-9_]+|KEY [a-z0-9_]+) \(([a-z0-9_,]+)\)$`)
	fk := regexp.MustCompile(`^CONSTRAINT ([a-z0-9_]+) FOREIGN KEY \(([a-z0-9_,]+)\) REFERENCES ([a-z0-9_]+)\(([a-z0-9_,]+)\)$`)
	def := regexp.MustCompile(`\bDEFAULT ('[^']*'|[0-9]+|NULL)`)
	coll := regexp.MustCompile(`\bCOLLATE ([a-z0-9_]+)`)
	var tables []phase1Table
	for _, statement := range strings.Split(strings.ReplaceAll(strings.Join(clean, "\n"), "\r\n", "\n"), ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		m := header.FindStringSubmatch(statement)
		if m == nil || !allowed[m[1]] {
			return nil, fmt.Errorf("一期DDL必须是唯一白名单CREATE语句")
		}
		delete(allowed, m[1])
		table := phase1Table{Name: m[1], DDL: statement, Indexes: map[string]phase1Index{}, ForeignKeys: map[string]string{}}
		seen := map[string]bool{}
		for _, line := range strings.Split(m[2], "\n") {
			line = strings.TrimSuffix(strings.TrimSpace(line), ",")
			if k := index.FindStringSubmatch(line); k != nil {
				name := "PRIMARY"
				if k[1] != "PRIMARY KEY" {
					parts := strings.Fields(k[1])
					name = parts[len(parts)-1]
				}
				if _, exists := table.Indexes[name]; exists {
					return nil, fmt.Errorf("重复索引")
				}
				table.Indexes[name] = phase1Index{Columns: k[2], Unique: !strings.HasPrefix(k[1], "KEY ")}
			} else if k := fk.FindStringSubmatch(line); k != nil {
				if _, exists := table.ForeignKeys[k[1]]; exists {
					return nil, fmt.Errorf("重复外键")
				}
				table.ForeignKeys[k[1]] = k[2] + ">" + k[3] + "(" + k[4] + ")"
			} else if k := col.FindStringSubmatch(line); k != nil {
				if seen[k[1]] {
					return nil, fmt.Errorf("重复列")
				}
				seen[k[1]] = true
				c := phase1Column{Name: k[1], Type: strings.ToLower(k[2]), Nullable: !strings.Contains(k[3], "NOT NULL"), Auto: strings.Contains(k[3], "AUTO_INCREMENT")}
				if d := def.FindStringSubmatch(k[3]); d != nil && d[1] != "NULL" {
					value := strings.Trim(d[1], "'")
					c.Default = &value
				}
				if strings.HasPrefix(c.Type, "varchar") || strings.HasPrefix(c.Type, "char") {
					c.Collation = "utf8mb4_unicode_ci"
					if d := coll.FindStringSubmatch(k[3]); d != nil {
						c.Collation = d[1]
					}
				}
				table.Columns = append(table.Columns, c)
			} else {
				return nil, fmt.Errorf("不支持的一期DDL定义")
			}
		}
		if len(table.Columns) == 0 || len(table.Indexes) == 0 {
			return nil, fmt.Errorf("一期DDL不完整")
		}
		tables = append(tables, table)
	}
	if len(allowed) != 0 {
		return nil, fmt.Errorf("一期DDL缺少白名单表")
	}
	return tables, nil
}
