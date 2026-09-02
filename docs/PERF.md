# 性能测试

> 2026-08-31 目标机部署回归结果见 [VALIDATION_REPORT_2026-08-31.md](./VALIDATION_REPORT_2026-08-31.md)。
> 本轮没有复现 2026-08-29 的约 3000 QPS：旧制品和当前制品在同一窗口都只有约
> 580～650 QPS，所以不能判定为当前代码回退，也不能继续把旧容量数字当作已复验的上线承诺。
> 在相同制品、相同压测程序连续三次复现前，容量规划采用新报告中的保守值。

2026-08-31 固定环境连续三轮 c500 分别为 595.8、612.1、639.2 QPS，P95 为
1451.5、1394.9、1377.8ms，全部 0 错误；最终源码制品另跑一轮为 659.0 QPS / P95 1326.2ms。
目标机 cgroup v1 没有 CPU quota（`quota=-1`、`nr_throttled=0`），但压测时主机总 CPU 平均
98% 以上，当前瓶颈是 2 vCPU 已饱和，不是容器节流。

同日完成 30 分钟 c100 混合读写：1,303,269 个读请求，724.0 QPS，P95/P99 为
222.0/267.2ms，0 错误；161 轮参数增改删、32 次用户 XLSX 导出、53 次健康检查也全部通过。
服务 RSS 67.0 -> 76.5 MiB、峰值 77.1 MiB，末段稳定在约 76 MiB；Redis 池有高并发排队但
0 超时。2C2G 生产容量按 50～80 个同时在途请求留余量，c100 是持续压力档，c500 仅是短测档。

**顺序很重要，倒着做会浪费时间。** 并发 100 以内的后台系统，瓶颈几乎一定在数据库不在 Go，
所以先看 SQL，最后才轮到 pprof。

> **环境边界：本文件只用于隔离的压测数据库。** `index`、`unindex` 和 `all` 会修改数据库索引，
> `seed` 和 `clean` 会批量写删压测数据；禁止对生产库执行。开始前必须核对配置文件、数据库实例和库名。

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
> **以下索引只用于隔离压测库的 before / after 实验，禁止在生产库执行。** 索引属于数据库
> 结构冻结范围；即使不改变接口字段，也不能据此进入生产基线。压测候选包括：
>
> ```sql
> ALTER TABLE sys_user ADD KEY idx_sys_user_name (user_name);
> ALTER TABLE sys_user ADD KEY idx_sys_user_dept (dept_id, del_flag);
> ALTER TABLE sys_dept ADD KEY idx_sys_dept_parent (parent_id);
> ```
>
> 执行前先核对目标实例和库名，实验后用 `unindex` 回滚。压测数字只能用于说明瓶颈，不能视为
> 已批准的生产结构变更。

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
| 数小时运行 | **已完成**：2026-09-01 在 2C2G 上连续 10 小时、2815 万请求零错误，RSS 约 97.0→99.3MiB，详见 `OPTIMIZATION_VALIDATION_2026-09-02.md` |
| 用户导出 10 万行 | **已完成内存实测**：数据切片约 14.5MiB，导出堆峰值约 89.6MiB；10 万行和并发 2 的现有限制保留 |
| `sys_oper_log` 到 500 万行 | **仍未测**：当前目标库仅 892 行；不向现有库灌 500 万条测试数据，必须在隔离副本验证 |

### 2026-08-29 当前 Go 远程回归（2C2G，小库混合读）

复用 2026-08-26 同一台服务器、同一小库和同一份 `extremebench` 混合读模型，只重新部署并
测试当前 Go；Java 和旧 Go 使用保存下来的服务器本机回环结果，不重复消耗服务器。当前制品
SHA-256 为 `1f7058996b01528c197b6541d80b3101e471b870d3838c8ea97a3313a0dfbfa6`。

| 并发 | Java QPS / P95 | 旧 Go QPS / P95 | 当前 Go QPS / P95 |
|---:|---:|---:|---:|
| 100 | 728 / 270.8ms | 2823 / 62.9ms | **3072 / 55.2ms** |
| 500 | 877 / 970.3ms | 2852 / 380.8ms | **3029 / 290.3ms** |
| 1200 | 996 / 1884.9ms | 2699 / 997.1ms | **2821 / 760.1ms** |
| 2000 | 1004 / 2711.7ms | 2642 / 1704.8ms | **2688 / 1372.7ms** |
| 3000 | 1090 / 3678.9ms | 2440 / 2867.5ms | **2641 / 2060.5ms** |
| 4000 | 未测 | 2443 / 3723.9ms | **2630 / 2699.6ms** |

- 17 个共同档位全部 0 业务错误。当前 Go 相对旧 Go 的 QPS 提升 1.8%～15.2%，平均 7.7%；
  混合 P95 改善 11.0%～29.1%，平均 22.1%。跨日期复测存在系统状态噪声，这只能证明没有
  明显回退，不能把全部提升归因于某一处代码。
- 当前 Go 的 P95 1 秒边界仍是 1200 并发，但由旧版 997ms 降为 760ms；P95 3 秒短时边界
  从旧 Go 的 3000 提升到 4000，4500 并发首次跨线（3064ms）。6000 并发仍为 0 错误，
  但 P95 已达 4023ms、RSS 488MiB，没有生产价值。
- 空载 RSS 约 38MiB；同为 4000 并发时当前 Go 为 343.5MiB，旧 Go 为 325.9MiB，增加
  约 5.4%。2C2G 长期仍建议 300～500 个同时在途请求，不能把短测极限当生产配置。
- 总体指标改善不代表所有接口都改善。并发 500/1200 时，`getInfo` P95 改善约 18%～22%，
  但用户、部门、操作日志和深分页四条数据库路径 P95 回退约 10%～24%。`/health` 最终记录
  `waitCount=68972`、`waitMs=654182`、`maxIdleClose=116413`；该现象已经促成下一节的隔离
  A/B，最终选择 `maxIdleConns=20`，没有扩大 `maxOpenConns=50`。
- 验证码在测试结束后恢复为开启，本次会话、登录日志和限流 key 已清理，Go 服务保持健康；
  远端 `/opt/perf-current` 和本地临时辅助程序均已清理；完整报告和原始结果保存在本机
  `D:\WorkSpace\CodeX\ruoyi-perf-artifacts`。

### 2026-08-29 定向优化 A/B（2C2G，不改数据库）

本轮先补齐观测，再逐项隔离变量。压测器增加固定速率模式和单接口模型，worker 独立聚合，
避免压测端结果锁成为瓶颈；服务监控把被测进程与压测进程 CPU/RSS 分开记录；`/health`
增加 Redis 连接池命中、等待、超时、连接数和 pending 指标。所有候选均在同一服务器、同一
数据库与 Redis、同一二进制构建参数下运行，每组 3 轮，每轮 15 秒，表中延迟和资源为三轮
中位数（连接池等待/关闭是三轮累计）。固定速率全部按计划发出，业务错误和丢弃均为 0。

**MySQL 空闲连接池：`maxIdleConns=10/20/30`**

| 空闲连接 | 模型 | P95 | P99 | DB 等待次数 / 时间 | 空闲连接关闭 | 服务 CPU |
|---:|---|---:|---:|---:|---:|---:|
| 10 | 混合读 2500 RPS | 10.778ms | 24.851ms | 1020 / 8324ms | 2059 | 74.11% |
| **20** | 混合读 2500 RPS | **8.169ms** | **17.584ms** | **0 / 0ms** | 118 | 72.84% |
| 30 | 混合读 2500 RPS | 9.283ms | 18.120ms | 59 / 141ms | **65** | 72.90% |
| 10 | 用户列表 800 RPS | **1.888ms** | 5.022ms | 0 / 0ms | 50 | 44.26% |
| 20 | 用户列表 800 RPS | 2.012ms | 5.581ms | 0 / 0ms | 8 | **43.85%** |
| 30 | 用户列表 800 RPS | 2.123ms | 5.123ms | 0 / 0ms | **6** | 44.55% |

最终选择 **20**：相对 10，混合 P95/P99 分别降低 24.2%/29.2%，DB 等待归零，空闲连接
抖动减少 94.3%；30 没有继续改善延迟，反而出现少量等待。用户列表在 10 下快 0.124ms，
但这是无 DB 争用的单接口微测，不足以抵消混合负载结果。`maxOpenConns` 保持 50。

**鉴权会话：两次 GET 与单次 Lua 对比（`getInfo` 3000 RPS）**

| 实现 | P50 | P95 | P99 | 服务 CPU | 峰值 RSS | Redis 等待次数 / 时间 |
|---|---:|---:|---:|---:|---:|---:|
| 两次 GET（基线） | **0.829ms** | **3.201ms** | **7.637ms** | 61.50% | 66.31MiB | 3429 / 2681ms |
| 单次 Lua + `cjson.decode` | 0.839ms | 3.511ms | 8.283ms | **60.08%** | **59.89MiB** | **2656 / 1880ms** |

Lua 少一次网络往返且降低池等待，但 P95/P99 反而回退 9.7%/8.5%；用户列表模型也出现同向
回退。原因是 generation key 依赖会话 JSON 内的 `userId`，不能用普通 MGET，必须在 Redis
执行 JSON 解码。按“可观察延迟优先、微小 CPU/RSS 差异不足以换复杂度”的准则，**不保留
Lua 候选，继续使用两次 GET**。旧会话 generation key 缺失按 0 处理的回归测试保留。

**用户分页部门加载：批量补查与分页取数 JOIN 对比（1400 RPS）**

这组只比较同样采用 Lua 鉴权的两个候选，隔离部门加载变量。COUNT 仍不 JOIN；候选仅在
分页取数 SQL 中 `LEFT JOIN sys_dept`，取 Java 列表契约实际使用的
`dept_id/dept_name/leader`，把“分页数据 + 部门批量补查”两次查询合成一次。导出仍批量补齐
完整部门对象。

| 部门加载 | P50 | P95 | P99 | 服务 CPU | 峰值 RSS | 空闲连接关闭 |
|---|---:|---:|---:|---:|---:|---:|
| 分页后批量补查 | 1.278ms | 5.633ms | 13.181ms | 68.81% | **59.48MiB** | 35 |
| **分页取数 JOIN** | **1.088ms** | **4.251ms** | **9.347ms** | **58.00%** | 61.62MiB | **8** |

JOIN 候选的 P95/P99 降低 24.5%/29.1%，服务 CPU 降低 15.7%，代价是本轮峰值 RSS 增加
2.14MiB；收益明确，**保留 JOIN 候选**。这里没有恢复已经删除的慢查询：COUNT 和基础查询
仍不 JOIN，也没有 DISTINCT；JOIN 只发生在带 LIMIT 的分页取数阶段。

原始数据及机器可读汇总位于本机
`D:\WorkSpace\CodeX\ruoyi-perf-artifacts\pool-ab-summary.json` 和
`D:\WorkSpace\CodeX\ruoyi-perf-artifacts\code-ab-summary.json`。
