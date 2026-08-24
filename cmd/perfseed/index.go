package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// candidateIndex 一条候选索引。
//
// 【全部是候选，不是既定方案】CLAUDE.md 写的是"表结构一行不改"。
// 索引对 Java 和 Go 都是透明的，加了不影响两版并行跑，
// 但要不要加得看 before / after 的实测数字，不能凭感觉。
// 所以这里做成可加可删的开关，用完能完整回滚。
type candidateIndex struct {
	name  string
	table string
	ddl   string
	why   string
}

var candidateIndexes = []candidateIndex{
	{
		name:  "idx_sys_user_name",
		table: "sys_user",
		ddl:   "CREATE INDEX idx_sys_user_name ON sys_user (user_name)",
		why:   "登录要 WHERE user_name = ?，现在是全表扫，每次登录都扫一遍 10 万行",
	},
	{
		name:  "idx_sys_user_dept",
		table: "sys_user",
		ddl:   "CREATE INDEX idx_sys_user_dept ON sys_user (dept_id, del_flag)",
		why:   "数据权限的 dept_id IN (...) 和部门树筛选都靠它。带上 del_flag 是因为每条查询都有这个条件",
	},
	{
		name:  "idx_sys_user_role_role",
		table: "sys_user_role",
		ddl:   "CREATE INDEX idx_sys_user_role_role ON sys_user_role (role_id)",
		why:   "主键是 (user_id, role_id)，只按 role_id 查用不上前缀，角色授权页的 NOT IN 子查询因此要扫全表",
	},
	{
		name:  "idx_sys_dept_parent",
		table: "sys_dept",
		ddl:   "CREATE INDEX idx_sys_dept_parent ON sys_dept (parent_id)",
		why:   "部门树按 parent_id 逐层查；部门数上千后有意义",
	},
}

// createIndexes 建立候选索引，已存在的跳过。
func createIndexes(db *sql.DB) error {
	fmt.Println("=== 建立候选索引 ===")
	for _, index := range candidateIndexes {
		exists, err := indexExists(db, index.table, index.name)
		if err != nil {
			return err
		}
		if exists {
			fmt.Printf("  %-24s 已存在，跳过\n", index.name)
			continue
		}

		start := time.Now()
		if _, err := db.Exec(index.ddl); err != nil {
			return fmt.Errorf("建立索引 %s 失败: %w", index.name, err)
		}
		fmt.Printf("  %-24s 建立成功，耗时 %s\n", index.name, time.Since(start).Round(time.Millisecond))
		fmt.Printf("  %-24s   %s\n", "", index.why)
	}

	fmt.Println("\n接着跑 explain 对比 before / after：")
	fmt.Println("  .\\scripts\\perf.ps1 explain")
	fmt.Println("不划算的话回滚：")
	fmt.Println("  .\\scripts\\perf.ps1 unindex")
	return nil
}

// dropIndexes 删除候选索引，回到原始表结构。
func dropIndexes(db *sql.DB) error {
	fmt.Println("=== 删除候选索引 ===")
	for _, index := range candidateIndexes {
		exists, err := indexExists(db, index.table, index.name)
		if err != nil {
			return err
		}
		if !exists {
			fmt.Printf("  %-24s 不存在，跳过\n", index.name)
			continue
		}
		if _, err := db.Exec(fmt.Sprintf("DROP INDEX %s ON %s", index.name, index.table)); err != nil {
			return fmt.Errorf("删除索引 %s 失败: %w", index.name, err)
		}
		fmt.Printf("  %-24s 已删除\n", index.name)
	}
	return nil
}

// listIndexes 打印相关表当前的索引，用来确认状态。
func listIndexes(db *sql.DB) error {
	fmt.Println("=== 当前索引 ===")
	for _, table := range []string{"sys_user", "sys_dept", "sys_user_role", "sys_oper_log"} {
		rows, err := db.Query(fmt.Sprintf("SHOW INDEX FROM %s", table))
		if err != nil {
			return fmt.Errorf("查询 %s 的索引失败: %w", table, err)
		}

		names, err := scanIndexNames(rows)
		rows.Close()
		if err != nil {
			return err
		}
		fmt.Printf("  %-16s %s\n", table, strings.Join(names, ", "))
	}
	return nil
}

func scanIndexNames(rows *sql.Rows) ([]string, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	keyNameIndex := -1
	for i, name := range columns {
		if strings.EqualFold(name, "Key_name") {
			keyNameIndex = i
			break
		}
	}
	if keyNameIndex < 0 {
		return nil, fmt.Errorf("SHOW INDEX 的结果里没有 Key_name 列")
	}

	seen := make(map[string]bool)
	var names []string
	for rows.Next() {
		cells := make([]sql.NullString, len(columns))
		pointers := make([]any, len(columns))
		for i := range cells {
			pointers[i] = &cells[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		// 复合索引每列一行，去重后只看索引名
		if name := cells[keyNameIndex].String; !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names, rows.Err()
}

func indexExists(db *sql.DB, table, name string) (bool, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.statistics
		WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?`, table, name).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("检查索引 %s 是否存在失败: %w", name, err)
	}
	return count > 0, nil
}
