// Command perfseed 往压测库里灌数据。
//
// 【为什么需要它】
// 空库压测的数字没有任何意义 —— sys_user 只有十几行时，
// 全表扫和走索引都是零点几毫秒，看不出任何差别。
// DECISIONS 里挂着的两条"以后再优化"（find_in_set 走不走索引、
// 导出一次性装内存会不会炸），不灌数据就永远只是推测。
//
// 【安全设计】
//   - 不加 -confirm 只打印计划，不写任何数据
//   - 所有数据的主键都从 idBase 起算，-clean 按主键区间删干净，
//     不靠名称前缀去猜哪些是造出来的
//
// 【注意】默认直接用 configs/application.yml，也就是开发库。
// 压测数据在库里期间，接口测试会有几个用例失败（详见 docs/PERF.md），
// 测完记得 -clean。
//
// 用法：
//
//	go run ./cmd/perfseed -users 100000 -depts 1000            # 只看计划
//	go run ./cmd/perfseed -users 100000 -depts 1000 -confirm   # 真灌
//	go run ./cmd/perfseed -explain                             # 跑 EXPLAIN 清单
//	go run ./cmd/perfseed -clean -confirm                      # 清干净
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"

	"ruoyi-go/internal/config"
)

// idBase 压测数据的主键起点。
//
// 真实数据的主键远小于它，所以按 id >= idBase 删除既干净又安全，
// 不用靠名称前缀去猜哪些是造出来的。
const idBase = 900000

// batchSize 单条 INSERT 的行数。
//
// 太小则往返次数爆炸，太大会撞 max_allowed_packet（默认 4MB）。
// 1000 行的用户数据大约 200KB，留足了余量。
const batchSize = 1000

// perfPassword 压测账号的明文密码，登录压测时要用。
const perfPassword = "perf123456"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "失败: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		configPath  = flag.String("config", "configs/application.yml", "配置文件路径")
		users       = flag.Int("users", 100000, "生成的用户数")
		depts       = flag.Int("depts", 1000, "生成的部门数")
		fanout      = flag.Int("fanout", 10, "部门树的分叉数，决定树有多深")
		operLogs    = flag.Int("operlogs", 500000, "生成的操作日志条数，0 表示不生成")
		clean       = flag.Bool("clean", false, "删除所有压测数据后退出")
		explainOnly = flag.Bool("explain", false, "只跑 EXPLAIN 清单，不写任何数据")
		addIndex    = flag.Bool("index", false, "建立候选索引（可用 -unindex 回滚）")
		dropIndex   = flag.Bool("unindex", false, "删除候选索引，回到原始表结构")
		confirm     = flag.Bool("confirm", false, "真正执行；不加只打印计划")
	)
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	db, err := sql.Open("mysql", cfg.MySQL.DSN)
	if err != nil {
		return fmt.Errorf("连接数据库失败: %w", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		return fmt.Errorf("数据库不可达: %w", err)
	}

	// EXPLAIN 是只读的，不需要 -confirm
	if *explainOnly {
		if err := listIndexes(db); err != nil {
			return err
		}
		return explainAll(db)
	}
	// 索引可以完整回滚，也不用 -confirm
	if *addIndex {
		return createIndexes(db)
	}
	if *dropIndex {
		return dropIndexes(db)
	}

	fmt.Printf("目标库: %s\n", databaseName(cfg.MySQL.DSN))
	if *clean {
		fmt.Printf("动作   : 删除 id >= %d 的全部压测数据（sys_user / sys_dept / sys_user_role / sys_oper_log）\n", idBase)
	} else {
		fmt.Printf("动作   : 灌入 %d 个部门（分叉 %d）、%d 个用户、%d 条操作日志\n", *depts, *fanout, *users, *operLogs)
		fmt.Printf("账号   : perf_0 .. perf_%d，密码统一 %s\n", *users-1, perfPassword)
	}

	if !*confirm {
		fmt.Println("\n以上只是计划。确认无误后加 -confirm 重新执行。")
		return nil
	}

	if *clean {
		return cleanAll(db)
	}
	return seed(db, *depts, *fanout, *users, *operLogs)
}

func seed(db *sql.DB, deptCount, fanout, userCount, logCount int) error {
	start := time.Now()

	// 先清一遍，保证可重复执行不撞主键
	if err := cleanAll(db); err != nil {
		return err
	}

	deptIDs, err := seedDepts(db, deptCount, fanout)
	if err != nil {
		return err
	}
	fmt.Printf("部门     : %d 个，耗时 %s\n", len(deptIDs), time.Since(start).Round(time.Millisecond))

	if userCount > 0 {
		t := time.Now()
		if err := seedUsers(db, userCount, deptIDs); err != nil {
			return err
		}
		fmt.Printf("用户     : %d 个，耗时 %s\n", userCount, time.Since(t).Round(time.Millisecond))
	}

	if logCount > 0 {
		t := time.Now()
		if err := seedOperLogs(db, logCount); err != nil {
			return err
		}
		fmt.Printf("操作日志 : %d 条，耗时 %s\n", logCount, time.Since(t).Round(time.Millisecond))
	}

	fmt.Printf("\n完成，总耗时 %s\n", time.Since(start).Round(time.Millisecond))
	fmt.Println("接下来看 docs/PERF.md 的 EXPLAIN 清单。")
	return nil
}

// seedDepts 造一棵部门树，返回所有部门 ID。
//
// 树形结构必须造对：ancestors 写错的话，"本部门及以下"的过滤结果就是错的，
// 压测出来的数字也就没有意义。
func seedDepts(db *sql.DB, count, fanout int) ([]int64, error) {
	if fanout < 2 {
		fanout = 2
	}

	// 挂在 RuoYi 初始数据的根部门 100 下面，它的 ancestors 是 "0"
	const rootID int64 = 100
	ancestors := map[int64]string{rootID: "0"}

	ids := make([]int64, 0, count)
	values := make([]string, 0, batchSize)
	args := make([]any, 0, batchSize*8)

	flush := func() error {
		if len(values) == 0 {
			return nil
		}
		query := `INSERT INTO sys_dept
			(dept_id, parent_id, ancestors, dept_name, order_num, leader, phone, email,
			 status, del_flag, create_by, create_time, update_by, update_time)
			VALUES ` + strings.Join(values, ",")
		if _, err := db.Exec(query, args...); err != nil {
			return fmt.Errorf("插入部门失败: %w", err)
		}
		values = values[:0]
		args = args[:0]
		return nil
	}

	for i := 0; i < count; i++ {
		id := int64(idBase + i)

		// 前 fanout 个挂在根下，之后依次挂到已生成的部门上，形成多层树
		parent := rootID
		if i >= fanout {
			parent = ids[i/fanout-1]
		}
		ancestors[id] = ancestors[parent] + "," + fmt.Sprint(parent)

		values = append(values, "(?,?,?,?,?,?,?,?,'0','0','perfseed',NOW(),'perfseed',NOW())")
		args = append(args, id, parent, ancestors[id], fmt.Sprintf("perf_部门_%d", i),
			i%100, "负责人", "13800000000", "dept@example.com")
		ids = append(ids, id)

		if len(values) >= batchSize {
			if err := flush(); err != nil {
				return nil, err
			}
		}
	}
	return ids, flush()
}

// seedUsers 造用户，均匀铺在所有部门上。
//
// 【密码只算一次】bcrypt 故意很慢（约 60ms/次），
// 10 万个用户逐个算要一个多小时。压测数据不需要每个人密码不同。
func seedUsers(db *sql.DB, count int, deptIDs []int64) error {
	hashed, err := bcrypt.GenerateFromPassword([]byte(perfPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("生成密码哈希失败: %w", err)
	}
	password := string(hashed)

	values := make([]string, 0, batchSize)
	args := make([]any, 0, batchSize*8)
	roleValues := make([]string, 0, batchSize)
	roleArgs := make([]any, 0, batchSize*2)

	flush := func() error {
		if len(values) == 0 {
			return nil
		}
		userQuery := `INSERT INTO sys_user
			(user_id, dept_id, user_name, nick_name, user_type, email, phonenumber, sex,
			 avatar, password, status, del_flag, login_ip, login_date,
			 create_by, create_time, update_by, update_time, remark)
			VALUES ` + strings.Join(values, ",")
		if _, err := db.Exec(userQuery, args...); err != nil {
			return fmt.Errorf("插入用户失败: %w", err)
		}

		// 每个压测账号都挂上普通角色（role_id=2），
		// 否则"分配用户"和数据权限相关的查询压出来的数字不真实
		roleQuery := `INSERT INTO sys_user_role (user_id, role_id) VALUES ` + strings.Join(roleValues, ",")
		if _, err := db.Exec(roleQuery, roleArgs...); err != nil {
			return fmt.Errorf("插入用户角色失败: %w", err)
		}

		values, args = values[:0], args[:0]
		roleValues, roleArgs = roleValues[:0], roleArgs[:0]
		return nil
	}

	for i := 0; i < count; i++ {
		id := int64(idBase + i)
		deptID := deptIDs[i%len(deptIDs)]

		values = append(values,
			"(?,?,?,?,'00',?,?,?,'',?,'0','0','127.0.0.1',NOW(),'perfseed',NOW(),'perfseed',NOW(),'压测数据')")
		args = append(args, id, deptID,
			fmt.Sprintf("perf_%d", i), fmt.Sprintf("压测用户%d", i),
			fmt.Sprintf("perf_%d@example.com", i),
			fmt.Sprintf("1%010d", i%10000000000),
			fmt.Sprint(i%3), password)

		roleValues = append(roleValues, "(?,?)")
		roleArgs = append(roleArgs, id, 2)

		if len(values) >= batchSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

// seedOperLogs 造操作日志。
//
// sys_oper_log 是唯一会无限增长的表，真实环境里几百万行很常见。
// 压测它才能看出"操作日志列表"这个页面在数据量上来后还能不能打开。
func seedOperLogs(db *sql.DB, count int) error {
	values := make([]string, 0, batchSize)
	args := make([]any, 0, batchSize*6)

	flush := func() error {
		if len(values) == 0 {
			return nil
		}
		query := `INSERT INTO sys_oper_log
			(oper_id, title, business_type, method, request_method, operator_type,
			 oper_name, dept_name, oper_url, oper_ip, oper_location, oper_param,
			 json_result, status, error_msg, oper_time, cost_time)
			VALUES ` + strings.Join(values, ",")
		if _, err := db.Exec(query, args...); err != nil {
			return fmt.Errorf("插入操作日志失败: %w", err)
		}
		values, args = values[:0], args[:0]
		return nil
	}

	for i := 0; i < count; i++ {
		values = append(values,
			"(?,'压测数据',?,'perfseed','POST',1,?,'研发部门','/system/user','127.0.0.1','内网IP','{}','{}',0,'',DATE_SUB(NOW(), INTERVAL ? SECOND),?)")
		args = append(args, int64(idBase+i), i%7, fmt.Sprintf("perf_%d", i%1000), i, i%500)

		if len(values) >= batchSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

// cleanAll 按主键区间删除全部压测数据。
func cleanAll(db *sql.DB) error {
	// 顺序有讲究：先删关联表，再删主表
	statements := []string{
		fmt.Sprintf("DELETE FROM sys_user_role WHERE user_id >= %d", idBase),
		fmt.Sprintf("DELETE FROM sys_user_post WHERE user_id >= %d", idBase),
		fmt.Sprintf("DELETE FROM sys_user WHERE user_id >= %d", idBase),
		fmt.Sprintf("DELETE FROM sys_dept WHERE dept_id >= %d", idBase),
		fmt.Sprintf("DELETE FROM sys_oper_log WHERE oper_id >= %d", idBase),
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("清理失败(%s): %w", stmt, err)
		}
	}
	return nil
}

// databaseName 从 DSN 里抠出库名，仅用于打印确认。
func databaseName(dsn string) string {
	slash := strings.LastIndex(dsn, "/")
	if slash < 0 {
		return "(无法识别)"
	}
	name := dsn[slash+1:]
	if question := strings.Index(name, "?"); question >= 0 {
		name = name[:question]
	}
	if name == "" {
		return "(无法识别)"
	}
	return name
}
