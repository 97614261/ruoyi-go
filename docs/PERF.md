# 性能测试

**顺序很重要，倒着做会浪费时间。** 并发 100 以内的后台系统，瓶颈几乎一定在数据库不在 Go，
所以先看 SQL，最后才轮到 pprof。

---

## 一、三条命令

```powershell
.\scripts\perf.ps1 seed      # 灌数据：10 万用户 / 1000 部门 / 50 万条操作日志
.\scripts\perf.ps1 explain   # 跑执行计划清单，结果同时写进 test/results/perf.log
.\scripts\perf.ps1 clean     # 清干净
```

想要小一点的数据集：

```powershell
.\scripts\perf.ps1 seed -Users 20000 -Depts 200 -OperLogs 50000
```

关于数据本身：

- 直接用 `configs/application.yml` 指的那个库，**也就是开发库**
- 所有主键从 `900000` 起算，`clean` 按主键区间删，不靠名字猜
- 账号 `perf_0` .. `perf_99999`，密码统一 `perf123456`
- 部门是一棵真树（`ancestors` 算对了），否则"本部门及以下"压出来的数字没意义
- 密码哈希只算一次复用 —— bcrypt 单次约 60ms，逐个算 10 万次要一个多小时

### ⚠️ 压测数据在库里期间，接口测试会挂

主要是 `TestRoleAuthUser`：它要在"未分配用户"列表里找刚建的测试账号，
而那个列表现在有 10 万行，测试账号的 `user_id` 最大、排在最后，
`pageSize=100` 的第一页里根本找不到。

其余用例大多只是变慢（`sys_user` 没有二级索引，全表扫 10 万行）。

**所以顺序是：跑测试 → 灌数据 → 压测 → `clean` → 再跑测试。**
不要在压测数据还在库里的时候去看测试结果。

`clean` 之后 `sys_user` 的 `auto_increment` 会停在 100 万以上，这没有影响。

---

## 二、先看索引（这一步就能定性）

`ry_20260417.sql` 里，**`sys_user` 和 `sys_dept` 只有主键，一个二级索引都没有**：

| 表 | 索引 |
|---|---|
| `sys_user` | `primary key (user_id)` —— 仅此而已 |
| `sys_dept` | `primary key (dept_id)` —— 仅此而已 |
| `sys_oper_log` | PK + `business_type` + `status` + `oper_time` |
| `sys_logininfor` | PK + `status` + `login_time` |

推论（灌完数据用下面的 EXPLAIN 验证）：

- **登录**要 `WHERE user_name = ?`，无索引 → 每次登录全表扫 `sys_user`
- **数据权限**要 `WHERE dept_id IN (...)`，无索引 → 每次列表查询全表扫
- `find_in_set(deptId, ancestors)` 本身就用不了索引，叠加上面两条

> ⚠️ 这是 RuoYi 原版就有的问题，不是 Go 版引入的。Java 版同样是全表扫。
>
> **加索引需要你拍板**：CLAUDE.md 写的是"表结构一行不改"。但索引对 Java 和 Go 都是透明的，
> 加了不影响两版并行跑，严格说不算改表结构。真要加的话是这三条：
>
> ```sql
> ALTER TABLE sys_user ADD KEY idx_sys_user_name (user_name);
> ALTER TABLE sys_user ADD KEY idx_sys_user_dept (dept_id, del_flag);
> ALTER TABLE sys_dept ADD KEY idx_sys_dept_parent (parent_id);
> ```
>
> 别凭感觉加，**先跑完第三节拿到 before / after 数字再决定**。

---

## 三、EXPLAIN 清单

```powershell
.\scripts\perf.ps1 explain
```

八条查询全部**照抄 `repository` 里真实执行的语句**，包括 `DISTINCT` 和 `LEFT JOIN` ——
"简化一下"的 SQL 执行计划完全不同，分析出来的结论对不上线上情况。

| # | 查询 | 为什么值得看 |
|---|---|---|
| ① | 登录：按账号查用户 | 每次登录都跑，`user_name` 无索引 |
| ② | 用户列表 COUNT（本部门及以下） | 每次分页都跑一次，和主查询一样贵 |
| ③ | 用户列表 分页（`find_in_set`） | 最重的读，就是 DECISIONS 里挂着的那条 |
| ④ | 对照组：`ancestors` 前缀匹配 | 候选优化方案，跟 ③ 比耗时 |
| ⑤ | 用户名模糊查询 | 前导 `%` 必然全表扫 |
| ⑥ | 按部门筛选（点部门树） | **另一处独立的 `find_in_set`**，和数据权限那处不是同一条路径 |
| ⑦ | 未分配用户列表 | 带 `NOT IN` 子查询，用户量大时最容易出事 |
| ⑧ | 操作日志深翻页 | `LIMIT 100000,10` 要先扫过前 10 万行 |

每条都打印**真实耗时**和执行计划。`EXPLAIN` 只给估算，实际快慢还得跑一遍。

**看什么**：`type` 是不是 `ALL`（全表扫）、`rows` 有多大、`Extra` 里有没有
`Using filesort`（排序没走索引）、`Using temporary`（用了临时表）。

### 等价性验证

最后一步自动比对 ③ 和 ④ 的结果集（`COUNT` / `SUM` / `MIN` / `MAX` 四个值）。

**这一步不能省。** 前缀匹配有个容易踩空的地方：直属子部门的 `ancestors`
**正好等于** prefix，没有后面那个逗号。只写 `LIKE 'prefix,%'` 会把直属子部门整个漏掉 ——
而且漏得很安静，列表少一批人，没有任何报错。所以工具里的条件是
`dept_id = ? OR ancestors = ? OR ancestors LIKE ?` 三段。

四个值全部相同才谈得上替换。

### 顺便开慢查询日志

```sql
SET GLOBAL slow_query_log = 1;
SET GLOBAL long_query_time = 0.1;
SHOW VARIABLES LIKE 'slow_query_log_file';   -- 日志落在哪
```

然后去前端点一遍，能抓到清单之外的慢 SQL。

### 关于用户名模糊查询（⑤）

前导 `%` 加索引也救不了。真要优化只能改成前缀匹配（`LIKE 'perf_5%'`）或上全文索引，
**但那会改变前端的搜索语义** —— 属于产品决策不是技术决策，不要自己改。

---

## 四、接口压测

```bash
go install github.com/rakyll/hey@latest
```

先拿一个 token（验证码开着的话从 Redis 读答案，或临时把 `sys.account.captchaEnabled` 改成 false）：

```bash
hey -n 2000 -c 100 -H "Authorization: Bearer <token>" \
    "http://localhost:8080/system/user/list?pageNum=1&pageSize=10"
```

值得测的四个：

| 接口 | 为什么 |
|---|---|
| `/system/user/list` | 带数据权限子查询，最重的读 |
| `/getInfo` | 每次刷页面都调，走 Redis，验证缓存路径 |
| `/login` | bcrypt 故意很慢（约 60ms），是唯一的 CPU 大头，也是最容易被打爆的点 |
| `/system/dept/list` | 树形构建，纯 CPU |

**`/login` 单独说**：bcrypt 慢是设计如此，不是 bug，不要为了压测数字好看去调低 cost。
真被刷的话应该上限流（`rate_limit:` 这个 key 前缀已经预留了），而不是削弱密码强度。

压测时同时看这三处：

- 慢请求日志（`slowRequestThreshold` 默认 500ms，超了打 `warn`）
- MySQL 慢查询日志
- `GET /health` 返回的 `db` 字段：`inUse` 长期贴着 `maxOpen`、`waitCount` 持续涨，
  就是连接池不够或者连接没归还

---

## 四点五、和 Java 版对拍

```powershell
.\scripts\compare.ps1 -JavaHome 'C:\Program Files\Eclipse Adoptium\jdk-21...' 
.\scripts\compare.ps1 -JavaHome '...' -SkipBuild      # jar 已经编译过
```

同一台机器、同一个 MySQL、同一批数据、同一套索引，**一次只跑一个实现**，
各自预热后再计时。Java 侧全部走命令行覆盖，`RuoYi-Vue-master` 一个字不改。

**环境要求**：JDK 17+（RuoYi 3.9.2 是 Spring Boot 4.1.0）、Maven 3.6.3+
（pom 里的 `maven-compiler-plugin:3.13.0` 硬性要求）。
两者都可以用 `-JavaHome` / `-MavenHome` 单独指定，不必改系统环境变量。

**这个对比最容易做假的四个地方**，脚本里都处理了：

| 坑 | 不处理会怎样 | 怎么处理的 |
|---|---|---|
| 不预热 | JVM 冷启动前几百个请求慢一个数量级 | 每个接口先跑 300 个丢弃（`-warmup`） |
| 两个同时跑 | 抢 CPU 和 MySQL，测的是互相干扰 | 串行，一次只起一个 |
| JVM 堆不限 | 默认堆是物理内存的 1/4，RSS 对比毫无意义 | 钉死 `-Xmx512m` |
| 接口不对等 | Java 没有 `/health`，会全部计为失败 | `-cross` 模式自动跳过 |

结果见 `test/results/compare-*.log`，结论见
[DECISIONS.md](./DECISIONS.md) 的 2026-08-23 那条。

---

## 五、pprof（只在前面几步指向 Go 侧时才用）

`cmd/server/main.go` 里加一行，**只在压测时加，不要提交**：

```go
import _ "net/http/pprof"
```

```bash
go tool pprof http://localhost:8080/debug/pprof/profile?seconds=30   # CPU
go tool pprof http://localhost:8080/debug/pprof/heap                 # 内存
go tool pprof -http=:9090 <上面下载的文件>                            # 浏览器看火焰图
```

⚠️ **绝不能暴露到公网**。pprof 会把完整的函数名、调用栈和运行时状态吐出来。

---

## 六、记录结果

测完把数字填进来，否则下次还得重测一遍。空着的格子就是还没做。

### 2026-08-22 实测（10 万用户 / 1000 部门 / 50 万日志，并发 50）

改动前后对比。「改前」= 有 DISTINCT + 无用的 JOIN + 无索引。

| 接口 | 改前 | 改后 |
|---|---|---|
| `/login` | 200 QPS / P50 244ms | 226 / 215ms |
| `/getInfo` | 7718 / 6ms | 8755 / 5ms |
| **`/system/user/list`** | **1000 个全超时** | **422 / 113ms** |
| 用户列表深翻页 | **1000 个全 500** | 368 / 129ms |
| `/system/dept/list` | 985 失败 / P50 17s※ | 623 / 79ms |
| `/system/dict/data/type/*` | 1613 / 23ms※ | 19130 / 3ms |
| `/monitor/operlog/list` | 17 / P99 24.8s※ | 115 / 312ms |
| `/health` | 3961 / 11ms | 25003 / 1ms |

※ 打星的三行改前的数字**不能信**：压测工具当时没有在目标之间等系统恢复，
它们是被前一个目标的余波打死的，不是自身有问题。已修（`waitUntilCalm`）。

单条 SQL：

| 查询 | 改前 | 改后 |
|---|---|---|
| 登录按账号查用户 | 177ms（`type=ALL`） | 1.5ms（`type=ref`） |
| 用户列表分页 | 448ms（`Using temporary`） | 1.5ms |
| 用户列表 COUNT | 432ms | 不再进慢 SQL |
| `find_in_set` vs `ancestors` 前缀 | 448ms vs 432ms | 差 3.6%，不值得换 |

结论已写进 DECISIONS 的 2026-08-22 两条。

### 2026-08-23 Go vs Java 对拍（同库同数据同索引，并发 50，各自预热 300）

**内存（进程工作集）**

| | 空载 | 压测中 |
|---|---|---|
| Go | **40.5 MB** | **65 MB** |
| Java（`-Xmx512m`） | 414.6 MB | 907.8 MB |

⚠️ JVM 是钉死 512m 堆跑出来的 908MB —— 堆只是 RSS 的一部分，
还有元空间、代码缓存、线程栈、直接内存、GC 开销。
**按堆大小估算服务器规格会翻车。**

**吞吐（QPS / P50）**

| 接口 | Go | Java | |
|---|---|---|---|
| 登录 | **207 / 237ms** | 75 / 661ms | Go 2.8× |
| getInfo | 5549 / 9ms | **6653 / 7ms** | **Java 快 20%** |
| 用户列表 | **404 / 119ms** | 68 / 721ms | 5.9× ※ |
| 用户列表深翻页 | **346 / 141ms** | 65 / 755ms | 5.3× ※ |
| 部门树 | **518 / 92ms** | 442 / 107ms | Go 1.2× |
| 字典（走 Redis） | 6189 / 8ms | **8450 / 5ms** | **Java 快 36%** |
| 操作日志列表 | **109 / 390ms** | 69 / 600ms | Go 1.6× |

**※ 这两项没有可比性。** Java 的分页 COUNT 带着 `LEFT JOIN sys_dept`，
就是上面 2026-08-22 干掉的那条 432ms 的 SQL。我们改了设计，Java 没有 ——
**实现差异，不是语言差异**。

**结论：移植换来的是内存和部署体积，不是吞吐。**
Java 在两个纯 Redis 接口上反而更快。两边的慢接口慢在同一条 SQL 上，
快接口快在同一个 Redis 上 —— 瓶颈在数据库，不在 Web 框架。

### 还没测的

| 场景 | 说明 |
|---|---|
| 启动时间 | Go 几十毫秒 vs Spring Boot 十几秒，没正式计过 |
| 并发梯度 | 只测了 50 一个点。10 / 100 / 200 各是什么样不知道 |
| 长时间运行 | 连接泄漏、goroutine 泄漏、JVM 老年代增长，要跑几小时 |
| 用户导出 10 万行 | 内存峰值多少？会不会 OOM？CONVENTIONS 第一节说"真实瓶颈在数据列表而非工作簿"，同样是没量过的推断 |
| `sys_oper_log` 到 500 万行 | 现在 50 万行时 COUNT 已经是最慢的读了 |
| 长时间运行 | 连接泄漏、goroutine 泄漏，要跑几小时才看得出来 |
