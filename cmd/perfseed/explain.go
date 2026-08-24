package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// explainCase 一条待分析的查询。
//
// SQL 全部照抄 repository 里真实执行的语句，**不要"简化一下"** ——
// 少一个 DISTINCT、少一个 LEFT JOIN，执行计划就完全变了，
// 分析出来的结论对不上线上的实际情况。
type explainCase struct {
	name string
	why  string
	sql  string
	args []any
}

// explainAll 跑一遍执行计划清单。
//
// 只读，不改任何数据。
func explainAll(db *sql.DB) error {
	deptID, ancestors, err := pickDept(db)
	if err != nil {
		return err
	}
	// 该部门的所有后代，ancestors 都以 prefix 开头
	prefix := ancestors + "," + fmt.Sprint(deptID)

	if err := printScale(db); err != nil {
		return err
	}
	fmt.Printf("\n分析用的部门: dept_id=%d, ancestors=%q\n", deptID, ancestors)

	cases := []explainCase{
		{
			name: "① 登录：按账号查用户",
			why:  "每次登录都跑。sys_user 上没有 user_name 索引，看是不是全表扫",
			sql:  "SELECT * FROM sys_user WHERE user_name = ? AND del_flag = '0'",
			args: []any{"perf_50000"},
		},
		{
			name: "② 用户列表 COUNT（本部门及以下）",
			why:  "每次分页都要跑一次，和主查询一样贵",
			sql: `SELECT COUNT(DISTINCT u.user_id) FROM sys_user u
			      LEFT JOIN sys_dept d ON u.dept_id = d.dept_id
			      WHERE u.del_flag = '0'
			        AND (u.dept_id IN (SELECT dept_id FROM sys_dept WHERE dept_id = ? OR find_in_set(?, ancestors)))`,
			args: []any{deptID, deptID},
		},
		{
			name: "③ 用户列表 分页（本部门及以下，find_in_set）",
			why:  "最重的读。DECISIONS「数据权限暂不做 find_in_set 优化」说的就是这条",
			sql: `SELECT DISTINCT u.* FROM sys_user u
			      LEFT JOIN sys_dept d ON u.dept_id = d.dept_id
			      WHERE u.del_flag = '0'
			        AND (u.dept_id IN (SELECT dept_id FROM sys_dept WHERE dept_id = ? OR find_in_set(?, ancestors)))
			      ORDER BY u.user_id LIMIT 10`,
			args: []any{deptID, deptID},
		},
		{
			name: "③b 同上，但去掉 DISTINCT",
			why:  "Java 版 selectUserList 没有 distinct，是 Go 侧自己加的。只 join 了 sys_dept 主键，1:1 不会产生重复行，DISTINCT 纯属白花钱",
			sql: `SELECT u.* FROM sys_user u
			      LEFT JOIN sys_dept d ON u.dept_id = d.dept_id
			      WHERE u.del_flag = '0'
			        AND (u.dept_id IN (SELECT dept_id FROM sys_dept WHERE dept_id = ? OR find_in_set(?, ancestors)))
			      ORDER BY u.user_id LIMIT 10`,
			args: []any{deptID, deptID},
		},
		{
			name: "④ 对照组：同样的过滤改成 ancestors 前缀匹配",
			why:  "候选优化方案。快多少要看这里，但先看下面的等价性验证",
			sql: `SELECT DISTINCT u.* FROM sys_user u
			      LEFT JOIN sys_dept d ON u.dept_id = d.dept_id
			      WHERE u.del_flag = '0'
			        AND (d.dept_id = ? OR d.ancestors = ? OR d.ancestors LIKE ?)
			      ORDER BY u.user_id LIMIT 10`,
			args: []any{deptID, prefix, prefix + ",%"},
		},
		{
			name: "⑤ 用户名模糊查询",
			why:  "前导 % 必然全表扫，加索引也救不了。Java 版一样",
			sql:  "SELECT * FROM sys_user u WHERE u.del_flag = '0' AND u.user_name LIKE ? LIMIT 10",
			args: []any{"%perf_5%"},
		},
		{
			name: "⑥ 按部门筛选（前端点部门树）",
			why:  "这里还有一处 find_in_set，和数据权限那处是独立的两条路径",
			sql: `SELECT u.* FROM sys_user u WHERE u.del_flag = '0'
			      AND (u.dept_id = ? OR u.dept_id IN (SELECT t.dept_id FROM sys_dept t WHERE find_in_set(?, t.ancestors)))
			      LIMIT 10`,
			args: []any{deptID, deptID},
		},
		{
			name: "⑦ 未分配用户列表（角色授权页）",
			why:  "带 NOT IN 子查询，用户量大时最容易出事的一条",
			sql: `SELECT DISTINCT u.* FROM sys_user u
			      LEFT JOIN sys_dept d ON u.dept_id = d.dept_id
			      LEFT JOIN sys_user_role ur ON u.user_id = ur.user_id
			      LEFT JOIN sys_role r ON r.role_id = ur.role_id
			      WHERE u.del_flag = '0' AND (r.role_id <> ? OR r.role_id IS NULL)
			        AND u.user_id NOT IN (
			          SELECT u2.user_id FROM sys_user u2
			          INNER JOIN sys_user_role ur2 ON u2.user_id = ur2.user_id AND ur2.role_id = ?)
			      LIMIT 10`,
			args: []any{2, 2},
		},
		{
			name: "⑧ 操作日志深翻页",
			why:  "LIMIT 100000,10 要先扫过前 10 万行再丢掉。实际没人翻这么深，优先级低",
			sql:  "SELECT * FROM sys_oper_log ORDER BY oper_id DESC LIMIT 100000, 10",
		},
		{
			name: "⑨ 用户名模糊查询（查不到任何结果）",
			why:  "⑤ 用的关键字第一行就命中，LIMIT 10 提前收工，那个数字是假的。这条查不到东西，必须扫完全表才能确认，才是真实上限",
			sql:  "SELECT * FROM sys_user u WHERE u.del_flag = '0' AND u.user_name LIKE ? LIMIT 10",
			args: []any{"%这个关键字一定查不到%"},
		},
	}

	for _, c := range cases {
		if err := runCase(db, c); err != nil {
			return err
		}
	}

	return verifyEquivalence(db, deptID, prefix)
}

// printScale 打印各表当前行数，没有它后面的数字没法解读。
func printScale(db *sql.DB) error {
	fmt.Println("=== 数据量 ===")
	for _, table := range []string{"sys_user", "sys_dept", "sys_oper_log", "sys_user_role"} {
		var count int64
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			return fmt.Errorf("统计 %s 失败: %w", table, err)
		}
		fmt.Printf("  %-16s %d\n", table, count)
	}
	return nil
}

// pickDept 挑一个有大量后代的部门来分析。
//
// 优先用压测数据里的第一个部门（挂在根下，后代最多）；
// 没灌过数据就退回初始数据里的部门，此时数字只能看趋势不能下结论。
func pickDept(db *sql.DB) (int64, string, error) {
	var (
		deptID    int64
		ancestors string
	)
	err := db.QueryRow(
		`SELECT dept_id, ancestors FROM sys_dept WHERE dept_id >= ? ORDER BY dept_id LIMIT 1`, idBase).
		Scan(&deptID, &ancestors)
	if err == nil {
		return deptID, ancestors, nil
	}
	if err != sql.ErrNoRows {
		return 0, "", fmt.Errorf("挑选部门失败: %w", err)
	}

	fmt.Println("⚠ 没有找到压测部门数据，退回初始数据。此时的执行计划只能看趋势，不能下结论。")
	fmt.Println("  先跑：go run ./cmd/perfseed -confirm")
	err = db.QueryRow(`SELECT dept_id, ancestors FROM sys_dept WHERE del_flag = '0' ORDER BY dept_id LIMIT 1`).
		Scan(&deptID, &ancestors)
	if err != nil {
		return 0, "", fmt.Errorf("挑选部门失败: %w", err)
	}
	return deptID, ancestors, nil
}

// runCase 打印一条查询的执行计划和真实耗时。
func runCase(db *sql.DB, c explainCase) error {
	fmt.Printf("\n=== %s ===\n%s\n", c.name, c.why)

	// 真实耗时：EXPLAIN 只给估算，实际快慢还得跑一遍
	start := time.Now()
	rows, err := db.Query(c.sql, c.args...)
	if err != nil {
		return fmt.Errorf("%s 执行失败: %w", c.name, err)
	}
	for rows.Next() { // 必须读完，否则计时只算到第一行
	}
	rows.Close()
	fmt.Printf("实际耗时: %s\n", time.Since(start).Round(time.Microsecond))

	return printExplain(db, c.sql, c.args)
}

// printExplain 输出 EXPLAIN 结果里最关键的几列。
//
// 关注三样：
//   - type = ALL 表示全表扫
//   - rows 是预估要检查的行数
//   - Extra 里的 Using filesort（排序没走索引）、Using temporary（用了临时表）
func printExplain(db *sql.DB, query string, args []any) error {
	rows, err := db.Query("EXPLAIN "+query, args...)
	if err != nil {
		return fmt.Errorf("EXPLAIN 失败: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return err
	}
	index := make(map[string]int, len(columns))
	for i, name := range columns {
		index[strings.ToLower(name)] = i
	}

	wanted := []string{"select_type", "table", "type", "possible_keys", "key", "rows", "extra"}
	fmt.Printf("%-13s %-10s %-8s %-16s %-16s %-10s %s\n",
		"select_type", "table", "type", "possible_keys", "key", "rows", "Extra")

	for rows.Next() {
		cells := make([]sql.NullString, len(columns))
		pointers := make([]any, len(columns))
		for i := range cells {
			pointers[i] = &cells[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return err
		}

		values := make([]string, 0, len(wanted))
		for _, name := range wanted {
			i, ok := index[name]
			if !ok || !cells[i].Valid {
				values = append(values, "-")
				continue
			}
			values = append(values, cells[i].String)
		}
		fmt.Printf("%-13s %-10s %-8s %-16s %-16s %-10s %s\n",
			values[0], values[1], values[2], values[3], values[4], values[5], values[6])
	}
	return rows.Err()
}

// verifyEquivalence 验证 find_in_set 和 ancestors 前缀匹配的结果集是否一致。
//
// 【这一步不能省】前缀匹配有个容易踩空的地方：
// 直属子部门的 ancestors **正好等于** prefix，没有后面那个逗号，
// 只写 LIKE 'prefix,%' 会把直属子部门整个漏掉 —— 而且漏得很安静，
// 列表少了一批人，没有任何报错。
//
// 用 COUNT + SUM + MIN + MAX 四个值比对。理论上不同的集合也可能凑出相同的
// COUNT 和 SUM，但同时撞上 MIN 和 MAX 的概率低到可以忽略，
// 而代价远低于逐行做差集。
func verifyEquivalence(db *sql.DB, deptID int64, prefix string) error {
	fmt.Printf("\n=== 等价性验证：find_in_set vs ancestors 前缀匹配 ===\n")

	const stats = `SELECT COUNT(*), COALESCE(SUM(user_id),0), COALESCE(MIN(user_id),0), COALESCE(MAX(user_id),0) FROM (%s) t`

	findInSet := fmt.Sprintf(stats, `
		SELECT u.user_id FROM sys_user u WHERE u.del_flag = '0'
		  AND u.dept_id IN (SELECT dept_id FROM sys_dept WHERE dept_id = ? OR find_in_set(?, ancestors))`)

	likePrefix := fmt.Sprintf(stats, `
		SELECT u.user_id FROM sys_user u
		LEFT JOIN sys_dept d ON u.dept_id = d.dept_id
		WHERE u.del_flag = '0'
		  AND (d.dept_id = ? OR d.ancestors = ? OR d.ancestors LIKE ?)`)

	var a, b [4]int64
	if err := db.QueryRow(findInSet, deptID, deptID).Scan(&a[0], &a[1], &a[2], &a[3]); err != nil {
		return fmt.Errorf("find_in_set 统计失败: %w", err)
	}
	if err := db.QueryRow(likePrefix, deptID, prefix, prefix+",%").Scan(&b[0], &b[1], &b[2], &b[3]); err != nil {
		return fmt.Errorf("前缀匹配统计失败: %w", err)
	}

	labels := []string{"COUNT", "SUM", "MIN", "MAX"}
	same := true
	for i := range labels {
		mark := "OK"
		if a[i] != b[i] {
			mark = "!! 不一致"
			same = false
		}
		fmt.Printf("  %-6s find_in_set=%-14d 前缀匹配=%-14d %s\n", labels[i], a[i], b[i], mark)
	}

	if same {
		fmt.Println("\n结论: 两种写法结果一致，可以考虑替换（还要看上面 ③ 和 ④ 的耗时差多少）。")
	} else {
		fmt.Println("\n结论: 结果不一致，**不能替换**。先查清是哪一类部门被漏掉或多算了。")
	}
	return nil
}
