# 决议记录

按日期追加，**只增不改**。稳定的规则请写进 [CONVENTIONS.md](./CONVENTIONS.md)，这里只记"为什么当初这么定"。

每条格式：**当前边界 / 为什么 / 禁止回退 / 相关位置**。

---

## 2026-08-26 登录权限按角色批量查询，禁止恢复循环查库

**当前边界**：`GetMenuPermission` 先收集全部启用角色 ID，一次查询角色与菜单关联，
再按角色回填 `role.Permissions`。无角色用户仍按用户维度查询一次。

**为什么**：旧实现每个角色执行一次 SQL，登录和 `/getInfo` 的查询次数随角色数增长，
并在连接池繁忙时放大等待。批量查询不改变停用角色跳过、多角色权限取并集的语义。

**禁止回退**：不得为了贴近 Java 的调用形态恢复按角色循环查询；修改时必须同时验证
总权限集合和每个角色的 `Permissions`。

**相关位置**：`internal/service/permission.go`、`internal/repository/menu.go`

---

## 2026-08-26 在线会话使用 SCAN + MGET，日志不记录会话 key

**当前边界**：在线列表和权限刷新按 SCAN 批次执行 MGET；同一用户的权限在一次刷新中
只计算一次。Redis 读取或反序列化失败只记录错误和批次数量，不记录
`login_tokens:<uuid>`。

**为什么**：逐 key GET 会产生 O(N) 次网络往返；完整 key 含会话 UUID，写入日志违反
Token 不落日志的约束。MGET 保留 SCAN 的非阻塞特性，同时明显减少 Redis 往返。

**禁止回退**：不得改回 KEYS 或逐 key GET，也不得把完整会话 key 塞进日志或错误文本。
如果在线规模继续增长，再引入 user/role 到 session 的反向索引，不盲目增大批次。

**相关位置**：`pkg/redisx/redisx.go`、`internal/service/token.go`、
`internal/service/online.go`

---

## 2026-08-26 Java 契约响应改为逐字段构造

**当前边界**：`java_contract.go` 用强类型逐字段 map 构造器复制模型 JSON 字段，再应用
Java 特有的 null、空数组和 Fastjson 参数标记。

**为什么**：旧实现每次响应先 Marshal 再 Unmarshal，最终写响应时还要再次 Marshal；
两次转换错误都被忽略，模型出现不可序列化字段时会静默返回空对象。

**禁止回退**：不得恢复 JSON 往返转换。模型字段变化时同步更新构造器、
`CONVENTIONS.md` 的接口清单和契约测试。

**相关位置**：`internal/handler/java_contract.go`、
`internal/handler/java_contract_test.go`、`docs/CONVENTIONS.md`

---

## 2026-08-26 HTTP 与调度器使用独立关闭超时

**当前边界**：收到退出信号后先关闭 HTTP 监听并等待请求，再使用新的 timeout context
停止调度器；数据库和 Redis 在两者之后关闭。

**为什么**：旧实现共用一个 context，调度任务耗尽 deadline 后，HTTP Shutdown 会立刻
收到过期 context。独立预算保证其中一个组件超时不会跳过另一个组件的关闭流程。

**禁止回退**：不得让 HTTP 与调度器复用已经消耗过的 timeout context；即使 HTTP
关闭失败，也必须尝试停止调度器。

**相关位置**：`cmd/server/main.go`、`cmd/server/main_test.go`

---

## 2026-08-24 测试 Redis 清理必须精确登记并验证，不能扫全库

**当前边界**：接口测试在请求发生时登记本轮创建的会话、验证码和防重复提交 key；
密码错误计数只清 `pwd_err_cnt:zz_test_*`，限流只清 httptest 固定来源
`192.0.2.1` 对应的键。收尾逐个删除并用 `EXISTS` 验证，任一步失败都会把
`TestMain` 的退出码改为失败。`cmd/contractcheck` 同样记录两端自己的会话和验证码，
无论对拍成功还是发现差异都做精确清理。

**为什么**：旧实现只清限流键，完整回归会留下上百个 `login_tokens:*`；
第一次补丁改成逐个调用 `/logout`，但该接口为保证前端能退出，Redis 删除失败也固定
返回成功，测试仍然会假绿。另一方面，直接扫描并删除全部 `login_tokens:*`、
`captcha_codes:*` 会把同一开发 Redis 上真实用户的会话一起删掉。

**禁止回退**：

- 禁止在测试收尾使用 `FLUSHDB`、`KEYS + DEL` 或删除整个业务前缀。
- 禁止忽略清理错误；“有 TTL 最终会过期”不能作为测试污染的处理方式。
- 新增会写 Redis 的中间件时，测试辅助层必须同时登记它创建的精确 key。
- 双端对拍的 Java 与 Go 仍必须使用不同 Redis DB；精确清理不能替代序列化隔离。

**相关位置**：`test/main_test.go`、`test/helper_test.go`、`cmd/contractcheck/main.go`、
`scripts/test.ps1`、`docs/CONVENTIONS.md` 第 7 节

---

## 2026-08-24 分页必须有唯一兜底列，统一走 `pg.Stable`

**当前边界**：13 处分页查询全部保证行序唯一，分两种写法：

- **11 处**接受前端排序，用 `page.Query.Stable(fallback, tiebreaker)`：

  ```go
  db.Order(pg.Stable("r.role_sort, r.role_id", "r.role_id"))
  ```

  前端没传 `orderByColumn` → 用 `fallback`；传了 → `<用户指定的>, <tiebreaker>`。
  `tiebreaker` 必须是**唯一列**（通常是主键），单列、不带方向，
  **不接受空串**（没有"为空就不追加"的分支，那等于给绕过稳定排序开口子）。

- **2 处**不接受前端排序，直接在 SQL 里写死确定性排序：
  公告已读用户按 `read_time DESC, user_id` 两级排序（`read_time` 不唯一，
  必须补主键）；角色授权用户按唯一主键 `user_id` 单列排序（本身已确定，
  不需要再追加）。

**为什么**：`LIMIT + OFFSET` 在两种情况下行序都是未定义的 ——
没有 `ORDER BY`，以及**排序键存在并列值**。两种情况下翻页都可能
**重复某行或漏掉某行**。

症状极其难查：没有任何报错，用户只会说"某条记录在列表里找不到"，
翻回上一页又出现了。日志、监控、测试全是正常的。

并列不是小概率事件：`role_sort` / `dict_sort` / `post_sort` 新建时默认同值；
`sys_notice_read.read_time` 精度到秒，一条公告推送后大批人同时点开，
并列几乎必然发生。

**改之前的真实状况**（13 处分页）：

| | 状况 |
|---|---|
| 角色授权用户 | 始终按唯一 `user_id` 排序，**本来就是稳定的** |
| 其余 12 处 | 至少存在一条不稳定路径 |

那 12 处细分：

- **11 个支持前端排序的接口** —— 默认路径多数按唯一主键排（`config_id`、`job_id`
  这些本身就确定），但**用户一旦点表头改用非唯一字段排序**（`postSort`、
  `roleSort`、`createTime`…），就没有主键兜底了。风险在这条路径上，不在默认路径。
- **岗位列表** —— 更糟，前端没传排序时**完全不加 `ORDER BY`**，默认路径就不稳定。
- **公告已读用户** —— 只按非唯一的 `read_time` 排序，而且不接受前端排序，
  也就是说它**唯一的那条路径就是不稳定的**。

**与 Java 的差异是有意的**：Java 的 mapper 多数只写单列排序甚至完全不排序
（`SysPostMapper.xml` 全文没有 `order by`），并列时行序由执行计划决定。
加兜底列会让双端对拍在有并列值时报差异。

**禁止回退**：

- **不要靠去掉兜底列来消掉对拍差异。** 行序不属于接口契约 ——
  前端依赖的是字段和结构。复刻"未定义行为"不是对齐，是把缺陷搬过来。
- 不要为了少传一个参数去解析 `fallback` 字符串（见下一条）。

**一个判断方法**：Java 到底有没有排序，要看 **service 转调到哪条 SQL**，
不能只看 mapper 里有没有同名 select。`selectRoleAll()` 转调的是 `selectRoleList`，
那条才带 `order by r.role_sort`。曾据此误删过 `SelectRoleAll` 的排序，
**而且双端对拍没发现** —— 种子数据里 `role_id` 和 `role_sort` 恰好同序，
样本把差异盖住了。对应回归见 `test/role_test.go` 的 `TestRoleAllOrderedBySort`，
它刻意让两者反序。

**相关位置**：`pkg/page/page.go` 的 `Stable`、`pkg/page/page_test.go`、
`internal/repository/*.go`、`test/post_test.go` 的 `TestPostPagingStable`、
@CONVENTIONS.md 第四节

---

## 2026-08-24 不要从 SQL 片段里解析结构，让调用方显式传

**当前边界**：`Stable(fallback, tiebreaker string)` 收两个参数。
**禁止**改回单参数版本去推断兜底列。

**为什么**：单参数版本是这么写的 ——

```go
tiebreaker := strings.Fields(fallback)[0]   // 想从 "info_id DESC" 里取出 "info_id"
```

它对 `"info_id DESC"` 有效，对 `"r.role_sort, r.role_id"` 切出来是
**`"r.role_sort,"`**，带着逗号。拼完是：

```sql
ORDER BY r.role_sort asc, r.role_sort,
```

末尾多一个逗号，MySQL 直接语法错误，接口返回 500。
角色列表和字典数据列表只要前端点一下表头排序就会中。

**这个 bug 的形状值得记住**：注释里写下了一个假设（"fallback 是单列或单列带方向"），
然后在**同一次改动里**自己写了多列 fallback，破坏了自己的假设。
调用方本来就知道主键叫什么，多传一个参数的成本远低于解析的脆弱性。

**为什么全量测试没抓到**：`pkg/page` 当时**一个测试都没有**，
而接口测试里没有任何用例给角色/字典列表传 `orderByColumn` ——
默认路径走的是 `fallback` 分支，正好绕开了出错的那一半。

现已补 `pkg/page/page_test.go`，其中专门有一条断言：
**排序表达式不能以逗号结尾、不能有连续逗号**。

**禁止回退**：

- 不要为了"接口更简洁"把两个参数合成一个。
- 新增分页查询时两个参数都要显式写，即使它们相同（如 `Stable("post_id", "post_id")`）。
- 纯函数包（`pkg/` 下）新增可复用工具时必须同时补表格驱动单测 ——
  接口测试覆盖不到分支组合。

**相关位置**：`pkg/page/page.go`、`pkg/page/page_test.go`

---

## 2026-08-24 接口测试必须照前端的契约写，不能照自己的实现写

**当前边界**：写接口测试时，**参数放在哪（body / query）、叫什么名字，必须去翻
`RuoYi-Vue3-master/src/api/*.js`**，不能照着自己刚写的 handler 抄。

**为什么**：`PUT /system/user/profile/updatePwd` 的密码参数，前端发的是 JSON body
（`data: data`），Java 是 `@RequestBody Map<String, String>`，
而 Go 的 handler 读的是 `c.Query(...)`。

**结果是改密码功能在真实前端上从来没成功过** —— Go 拿到两个空串，
报"旧密码和新密码不能为空"，用户完全猜不到是参数没接上。

而 `TestUserProfile` 一直是绿的，因为它是照着 handler 写的：

```go
// 改密码：参数在 query string 里          ← 注释和实现都错，测试跟着错
request(http.MethodPut, ".../updatePwd?oldPassword=...&newPassword=...", token, nil)
```

同一个错误在 `markReadAll` 上又犯了一次：只测了不带 `ids` 的分支，
而前端 `markAllRead()` 永远带 `ids`。**测了一条前端根本不走的路径。**

**这类 bug 的特征**：接口返回 200、测试全绿、日志没有异常，
只有真人用真前端点一下才会发现。自动化测试在这里非但没帮上忙，
反而把错误固化下来了。

**禁止回退**：

- 不要因为"测试跑通了"就认为契约对了。测试证明的是"实现和测试一致"，
  不是"实现和前端一致"。
- 涉及参数位置的接口，测试里要加**反向断言**：
  比如 updatePwd 现在会断言"放在查询串里必须不生效"。
- 新增接口时，先打开 `src/api/` 下对应的 js 看清楚 `params` 还是 `data`，
  再写 handler。

**发现方式**：一份从"前端实际发什么"倒推的静态契约审计。
这正好补上了接口测试的盲区 —— 测试看不见前端，审计看不见运行时，两者互补。
（那份审计自己也错了一条：说 `dictType` 的 `@Pattern` 是 Go 加的，
实际 `SysDictType.java:61` 就有。**审计当线索用，每条都要回源码核。**）

**相关位置**：`internal/handler/profile.go`、`internal/model/sys_user.go` 的 `UpdatePwdBody`、
`test/user_test.go` 的 `TestUserProfile`、`test/notice_test.go` 的 `TestNoticeMarkReadAllByIDs`

---

## 2026-08-24 限流阈值按"一个 IP 后面有多少人"定

**当前边界**：登录 60/分钟、验证码 120/分钟、注册 5/分钟，均按 IP。
（原先是 20 / 60 / 5。）

**为什么**：这类后台系统基本都装在内网，**整个公司共用一个出口 IP**。
早上九点几十号人同时登录是常态 —— 阈值 20/分钟那会儿就集体被拦，
而且报的是"访问过于频繁"，运维根本想不到是自己人把自己人挤掉了。

验证码必须比登录更宽：每次登录前都要先取一次验证码，用户还会手动刷新。

暴力破解要试几千次，60/分钟拦得住；再叠加 `pwd_err_cnt`
（单个账号错 5 次锁 10 分钟），两层加起来够用。

**发现方式**：接口测试自己被限流拦了 —— httptest 的请求全部来自同一个 clientIP，
一轮测试里 `loginAs` 会被调几十次。这个失败不是"测试环境的特殊问题"，
**生产上一样会中**，只是没人在测试里撞见就不会想到。

**禁止回退**：

- 不要为了迁就测试去调阈值 —— 那是生产配置。测试自己在 setup 里清
  `rate_limit:*`（`purgeRateLimit`）。
- 不要把登录阈值调回几十以下，除非确认部署环境是每人独立公网 IP。
- 真要更精细，方向是"只对失败的登录计数"而不是继续压低阈值 ——
  但那需要中间件感知响应结果，复杂度上一个台阶，当前不值得。

**相关位置**：`internal/router/router.go` 的 `registerAnonymous`、
`test/main_test.go` 的 `purgeRateLimit`

---

## 2026-08-23 补上限流与防重复提交

**当前边界**：

- `middleware.RateLimit` 挂在三个匿名接口上（按 IP）：
  登录 20/分钟、注册 5/分钟、验证码 60/分钟。
- `middleware.RepeatSubmit` 挂在 **10 个新增路由**上，判定条件是
  「同一个人 + 同一 URL + 同样参数 + 5 秒内」。

**为什么**：Java 定义了 `@RateLimiter` 和 `@RepeatSubmit` 注解、切面、拦截器，
但**全仓库零处使用**，是彻底的摆设。所以这不是"对齐 Java"，是补一个两边都缺的真实防护。

登录走 bcrypt，实测并发 50 就把 CPU 打满、QPS 只有 200 ——
全站最容易被打垮的点。已有的 `pwd_err_cnt`（5 次锁 10 分钟）只挡单个账号，
挡不住拿一批账号轮着刷。

### 四个具体决定

**1. 限流用 Lua，不用 INCR + EXPIRE 两条命令。**
分开发的话，两条之间进程崩了或 EXPIRE 因网络抖动没执行，这个 key 就永不过期 ——
计数只增不减，那个 IP 从此被永久封死。

**2. Redis 挂了一律放行。**
限流和防重复都是防护不是业务。Redis 不可用时如果拒绝请求，
等于让缓存故障直接升级成全站不可用 —— 比被刷严重得多。

**3. 防重复提交只挂新增，不挂修改，不全局挂。**
重复新增会实打实多一条数据；重复修改是幂等的，挂上去只是徒增误伤面。
不全局挂是因为**导出走的是 POST** —— 同样的查询条件连点两次导两份完全合法，
全局挂会拦掉它，还报"不允许重复提交"，用户根本想不到是被防重拦了。

**4. 参数存哈希，不存原文（比 Java 多做的一件事）。**
Java 把请求体原样塞进 Redis 的 `repeatParams`。它自己没在任何接口上用这个注解，
所以没暴露问题；但只要挂到「新增用户」「重置密码」上，明文密码就进 Redis 了，
而且 TTL 期间一直躺在那。判重只需要知道"一样不一样"，不需要还原内容，
所以只存 sha256 前 16 字节。

**禁止回退**：

- 不要把限流的 Lua 拆成两条命令。
- 不要为了"更严格"在 Redis 故障时拒绝请求。
- 不要把防重复提交挂到整个 authed 组上。
- 不要为了排查方便把请求体原文存进 Redis。

**一个副作用**：写测试时，**不能原样重发同一个请求体**。
`TestDictTypeUnique` 和 `TestDeptUniqueName` 都因此挂过 ——
拿到的是"不允许重复提交"而不是"XX 已存在"。
正确做法是让一个无关字段不同，被测的唯一性字段保持一致。

**相关位置**：`pkg/redisx/ratelimit.go`、`internal/middleware/ratelimit.go`、
`internal/middleware/repeatsubmit.go`、`internal/router/router.go` 的 `noRepeat`

---

## 2026-08-23 导出上限由内存决定，不是由 xlsx 格式决定

**当前边界**：`service.MaxExportRows = 100000`、
`service.MaxConcurrentExports = 2`（`middleware.ExportLimit` 执行，拿不到名额等 5 秒后拒绝）。

**先说一个 bug**：这个常量原来的值是 **`1048575048575`** ——
多打了一串数字，比预期大 100 万倍。**导出上限从来没生效过**，
"有上限就报错"这条规范在代码里一直是空的。
测试测不出来：没有哪条用例会去造 100 万行数据。是 grep 时撞见的。

**实测依据**（`pkg/excelx` 基准，10 列贴近 SysUser）：

| 行数 | 堆峰值 | 分配总量 | 分配次数 | 耗时 |
|---|---|---|---|---|
| 1 万 | ~9 MB | 50 MB | 98 万 | 80 ms |
| 10 万 | ~90 MB | 321 MB | 980 万 | 796 ms |

完全线性：约 **900 字节/行常驻**、**每行 98 次分配**。
按 xlsx 格式上限 1048575 行外推是 **约 940 MB 堆峰值**，
而整个服务压测时常驻才 65 MB。一次导出内存翻十几倍，两三个人同时点就 OOM ——
**正好抵消掉换 Go 省下来的内存**。

**推翻了 CONVENTIONS 里的一句话**：原文说「数据仍是一次性装进切片的，
所以真实瓶颈在数据列表而非工作簿」。实测数据切片只占堆峰值的 **16%**，
另外 84% 是导出过程本身。StreamWriter 确实在流式写，它只是 churn 很凶，
堆峰值仍随行数线性增长。**改成分批查库只能省下那 16%，是白费力气。**

**为什么还要限并发**：只限行数不限并发，行数上限就是摆设 ——
10 个人各导 10 万行就是 900 MB。

**禁止回退**：

- 不要因为"xlsx 能装 100 万行"就把上限调回格式上限。那个数描述的是
  **文件格式能装多少**，不是**进程扛得住多少**。
- **绝不截断**。截断后照常返回文件是最坏的做法：用户拿到一份看起来正常、
  实际少了数据的表格，而且不会发现。宁可让他导不出来。
- 导出接口上的**每一条**错误路径都必须走 `response.FailDownload`。
  前端 `blobValidate()` 是严格相等比较 `data.type !== 'application/json'`，
  `c.JSON` 写的是 `application/json; charset=utf-8` —— 比较不相等，
  前端会把错误当文件存下来，用户拿到打不开的 `.xlsx` 且看不到任何提示。

**相关位置**：`internal/service/export.go`、`internal/middleware/exportlimit.go`、
`pkg/excelx/excelx_bench_test.go`、@CONVENTIONS.md 第一节第 5 条

---

## 2026-08-23 定时任务已实现：命名注册表 + robfig/cron

**当前边界**：`/monitor/job` 和 `/monitor/jobLog` 共 13 个接口已实现，
2026-08-21「定时任务暂不实现」那条作废。

架构：

- `internal/job` —— 任务注册表。`job.Register(name, func(ctx, args []string) error)`。
  不依赖数据库；需要访问数据的任务由 service 层注册进来，避免循环依赖。
- `pkg/cronx` —— Quartz 表达式到 robfig/cron 的适配层。
- `internal/service/job_scheduler.go` —— 调度器，进程内单例。

### 三个必须知道的限制

**1. `misfire_policy` 只有「放弃执行」是真实生效的。**
字段照常存、照常返回、前端照常能选「立即执行」「执行一次」，但内存调度器
没有"跨重启的错过触发"这个概念，那两个选项落不了地。
**Java 版默认配置同样如此** —— `ScheduleConfig` 整个被注释掉，走的是 RAMJobStore。
不要在文档或对外说明里声称支持。

**2. 单机调度，多实例部署会重复执行。** 同样与 Java 默认行为一致
（`QRTZ_*` 表在单独的 `quartz.sql` 里，本项目用的 `ry_20260417.sql` 没有）。
要多实例就得自己加分布式锁，那是另一个决定，不是"对齐 Java"的一部分。

**3. 新建任务默认是暂停（`status = '1'`）。** 与种子数据一致。
建完立刻开跑的话，用户还没来得及检查 cron 对不对就已经在执行了。

### 安全边界只有注册表这一条

Java 用反射调任意 Spring bean，为此堆了白名单（只允许 `com.ruoyi.quartz.task`）
+ 黑名单（`java.net.URL`、`javax.naming.InitialContext`、`org.yaml.snakeyaml`、
`org.springframework`、`org.apache` …）+ 禁 `rmi:` / `ldap:` / `http(s)`。

Go 侧查不到名字就是查不到，**没有"绕过去调到别的东西"这种可能**，
所以一个黑名单都不需要。`test/job_test.go` 里用 JNDI 注入和内网探测的写法验证过。

**禁止回退**：不要为了"像 Java 一样灵活"引入反射或插件式动态加载。
那正是 RuoYi 打了一圈补丁在堵的洞。

### Cron 表达式：星期编号差 1

**这是最危险的一处，改 `pkg/cronx` 前务必读懂。**

| | Quartz（前端生成器 / 数据库里的记录） | robfig/cron |
|---|---|---|
| 段数 | 6 或 7（秒在最前，年可选） | 6（开 Second 后） |
| 星期 | **1=周日 … 7=周六** | **0=周日 … 6=周六** |
| `?` | 支持 | 支持（内部按 `*` 处理） |
| `L` `W` `#` | 支持 | **不支持** |

前端选「每周一」生成 `2`，直接喂给 robfig 会变成**周二** ——
不报错、不告警，只是每周错一天，几乎不可能靠肉眼发现。
`cronx.Translate` 逐个数字减 1（只动纯数字，`MON`/`SUN` 这类名称两边含义相同）。

`L` / `W` / `#` 和「指定具体年份」**明确报错**，不做"尽力而为"的降级。
静默按错误的时间执行，比配不上去糟糕得多。

**禁止回退**：不要把 robfig 的原始报错直接抛给用户 ——
那里面是转换后的表达式，和他在界面上填的对不上，只会更困惑。

**相关位置**：`internal/job/`、`pkg/cronx/`、`internal/service/job_scheduler.go`、
`test/job_test.go`

---

## 2026-08-23 更正：定时任务的"能力差异"被高估了

**当前边界**：仍然不做定时任务（2026-08-21 那条的结论不变），
但那条里对 Java 版能力的描述是错的，在此更正。**做与不做的成本重新评估过，比原先记的低。**

**原文说**：「Java 的 `invoke_target` 靠 Spring 反射调用任意 bean，Go 没有等价机制，
只能预先注册命名任务，代价是**新增定时任务要改代码重新部署，不能像 Java 那样纯后台配置**。」

**实际读代码后**：

1. **Java 也必须改代码。** `Constants.JOB_WHITELIST_STR = { "com.ruoyi.quartz.task" }` ——
   `invoke_target` 只能指向这一个包里的类。想加新任务，照样得写代码重新部署。
   后台能改的只有「什么时候跑」和「传什么参数」。
   另有黑名单 `JOB_ERROR_STR`（`java.net.URL`、`javax.naming.InitialContext`、
   `org.yaml.snakeyaml`、`org.springframework`、`org.apache` …）以及禁止
   `rmi:` / `ldap:` / `http(s)`。**这一圈白名单加黑名单本身就是在堵 RCE**，
   不是可以照抄的设计。

2. **Java 默认也是单机内存调度。** `ScheduleConfig.java` 整个文件是注释掉的，
   注释写着「单机部署建议删除此类和 qrtz 数据库表，默认走内存会最高效」；
   `QRTZ_*` 表在单独的 `quartz.sql` 里，本项目用的 `ry_20260417.sql` 里没有。
   进程重启后由 `@PostConstruct` 从 `sys_job` 全量重建调度。
   **多实例部署会重复执行** —— Go 用 `robfig/cron/v3` 是同样的特性，
   这里也不存在差距。

3. 自带的 `RyTask` 三个方法全是 `System.out.println`，开箱即用的业务价值为零。

**结论**：Go 的命名任务注册表和 Java 的**实际可用能力基本等价**，而且没有反射那个洞。
真做的时候工作量是：13 个接口（job 8 + jobLog 5）、一个 cron 调度器、一张执行日志表。

**这条记录的教训**：那句判断是**从架构差异推出来的**（"Go 没有反射调 bean 的机制"），
推理没错，但没去读 Java 到底允许调什么。**评估"对方有什么能力"时要读它的限制条件，
不能只看它的机制。** 和 2026-08-23「find_in_set 只占 3.6%」是同一类错误 ——
凭原理推断，没有验证。

**禁止回退**：真要做时仍然**不要**用反射模拟任意调用，理由见上面第 1 条。
另外不要假设 Go 版需要额外补分布式锁才算"对齐" —— Java 默认也没有。

**相关位置**：`../RuoYi-Vue-master/ruoyi-quartz/`、
`ruoyi-common/.../constant/Constants.java` 的 `JOB_WHITELIST_STR`、
`ruoyi-quartz/.../config/ScheduleConfig.java`（整个被注释）

---

## 2026-08-23 Go 与 Java 版实测对比：收益是内存，不是性能

**当前边界**：`scripts/compare.ps1` 可以随时重跑这个对比。
同一台机器、同一个 MySQL、同一批数据（10 万用户 / 1000 部门 / 50 万操作日志）、
同一套索引，一次只跑一个实现，各自预热 300 个请求后再计时，并发 50。
Java 侧全部走命令行覆盖，`RuoYi-Vue-master` 一个字没改。

**内存（进程工作集）**

| | 空载 | 压测中 |
|---|---|---|
| Go | 40.5 MB | 65 MB |
| Java（`-Xmx512m`） | 414.6 MB | 907.8 MB |

**10 到 14 倍。** 注意 JVM 是钉死 512m 堆跑出来的 908MB ——
堆只是 RSS 的一部分，还有元空间、代码缓存、线程栈、直接内存、GC 开销。
**"给多少堆"和"占多少内存"是两回事**，按堆大小估算服务器规格会翻车。

**吞吐（QPS / P50）**

| 接口 | Go | Java |
|---|---|---|
| 登录 | 207 / 237ms | 75 / 661ms |
| getInfo | 5549 / 9ms | **6653 / 7ms** |
| 用户列表 | 404 / 119ms | 68 / 721ms ※ |
| 用户列表深翻页 | 346 / 141ms | 65 / 755ms ※ |
| 部门树 | 518 / 92ms | 442 / 107ms |
| 字典（走 Redis） | 6189 / 8ms | **8450 / 5ms** |
| 操作日志列表 | 109 / 390ms | 69 / 600ms |

**※ 这两项没有可比性。** Java 的分页 COUNT 带着 `LEFT JOIN sys_dept`，
就是本项目 2026-08-22 那条决议里干掉的 432ms 那条 SQL。
我们改了设计（事后 `attachDepts` 批量补部门），Java 没有 ——
**这是实现差异不是语言差异**，任何人都能在 Java 上做同样的优化。

**为什么这条结论重要**：**Java 在两个纯 Redis 接口上更快**（getInfo 快 20%、
字典快 36%）。JIT 预热后的 JVM 在热路径上是真的快，Go 没有天生的性能优势。
两边的慢接口慢在同一条 SQL 上，快接口快在同一个 Redis 上 ——
**瓶颈在数据库，不在 Web 框架**，这与 2026-08-20 选型时的判断一致，现在有数字了。

**结论**：这次移植换来的是**内存占用和部署体积**，不是吞吐。
选型理由要按这个来讲，不要对外说"换 Go 是为了性能" —— 数据不支持。

**禁止回退**：

- 不要拿这份数据去论证"Go 比 Java 快"。用户列表那两项是我们优化过的 SQL，
  不是语言差距；而纯计算+缓存的路径上 Java 反而更快。
- 重新对比时**必须预热**。JVM 冷启动前几百个请求慢一个数量级，
  不预热就是拿 Java 最差的状态去比，那不是对比是构陷。
- 必须一次只跑一个实现。两个同时开会抢 CPU 和 MySQL。
- JVM 堆必须显式钉住。默认堆是物理内存的 1/4，RSS 对比会毫无意义。

**这次对比的局限**（下次要补的）：单机同时跑 MySQL / Redis / 被测服务，
互相干扰；只测了并发 50 一个点；只测了一种数据规模；没测启动时间和长时间运行的稳定性。

**相关位置**：`scripts/compare.ps1`、`cmd/perfbench/`、
`test/results/compare-*.log`、`docs/PERF.md`

---

## 2026-08-23 两版的 Redis 会话不能互通（value 序列化格式不同）

**当前边界**：Go 版和 Java 版**不能共用同一份登录会话**。
key 名完全一致，但 value 格式不同，互相读不懂对方写的数据。

**为什么**：Java 的 `RedisConfig` 给 `RedisTemplate` 配的是
`FastJson2JsonRedisSerializer`，序列化时带 `JSONWriter.Feature.WriteClassName`，
所以：

- `LoginUser` 存进去带 `@type":"com.ruoyi.common.core.domain.model.LoginUser"`，
  字段结构也跟 Go 的不一样
- **连 String 也会被 JSON 化** —— 验证码 `1234` 存进 Redis 是 `"1234"`，**带引号**

CONVENTIONS 第三节写的是"Redis key 命名必须与 Java 端完全一致，否则两边不能共存"。
这句话只说对了一半：key 名对齐了，value 格式没对齐，所以**做不到
"Java 和 Go 同时在线、用户会话互通"**。

**影响**：

- 灰度/双跑时用户在两边之间跳转会掉登录，必须按用户或按域名整体切，不能混流
- 想真正互通，得让 Go 侧按 FastJson2 的格式读写（包括那个 `@type`），
  代价是把 Java 的类名硬编码进 Go —— 不值得
- `cmd/perfbench` 已经兼容两种验证码格式（读出来先剥引号），
  否则压 Java 版时每次登录都验证码错误

**禁止回退**：不要因为"key 名一样"就假设数据能互通。
真要做双跑互通，先写一个跨版本读写的验证用例，不要靠推断。

**相关位置**：`RuoYi-Vue-master/ruoyi-framework/.../config/RedisConfig.java`、
`FastJson2JsonRedisSerializer.java`、`cmd/perfbench/main.go` 的 `login`

---

## 2026-08-22 用户列表去掉 DISTINCT 和 LEFT JOIN sys_dept

**当前边界**：`repository.userListDB` 不 join `sys_dept`，
`SelectUserPage` / `SelectUserList` 用 `Select("u.*")` 而不是 `Distinct("u.*")`，
`COUNT` 也不再是 `COUNT(DISTINCT u.user_id)`。

配套改动：`service.userScope` 的 `DeptAlias` 从 `"d"` 改成 `"u"`，
`datascope.denyAll` 从 `dept_id = 0` 改成 `1 = 0`。

`authUserDB`（角色授权页）的两个 join 和 `DISTINCT` **保留** —— 那里
真的 join 了 `sys_user_role` / `sys_role`，会产生重复行，Java 版同样有 `distinct`。

**为什么**：10 万用户下实测（`docs/PERF.md`）：

| | 之前 | 之后 |
|---|---|---|
| 分页主查询 | 448ms（`Using temporary`） | 1.5ms |
| 分页 COUNT | 432ms | 归零（不再出现在慢 SQL 里） |
| 接口 QPS（并发 50） | 71 | 422 |
| 接口 P50 | 650ms | 113ms |

改之前并发 50 压测时用户列表 **100% 超时**，服务端 5214 条慢 SQL 警告全是那条 COUNT。

两处开销的成因：

- **DISTINCT 是 Go 侧自己加的**，Java 的 `selectUserList` 没有。
  这个查询只 join 了 `sys_dept` 的主键，一个用户至多一个部门，
  DISTINCT 去不掉任何行，只是让 MySQL 为整个结果集建了张临时表。
- **JOIN 在 Go 侧没有任何用途**。Java join 它是为了取 `d.dept_name` / `d.leader`，
  而我们是事后用 `attachDepts` 批量补部门的，从来没从 `d` 取过列。
  它唯一的存在理由是让数据权限能写 `d.dept_id`。

**别名替换的等价性证明**（这是能删 JOIN 的前提）：

JOIN 走的是 `sys_dept` 主键，所以每一行只有两种可能 ——
`d.dept_id` 等于 `u.dept_id`，或者部门行不存在、`d.dept_id` 为 NULL。
而所有数据范围的取值都来自 `sys_dept` 或 `sys_role_dept`，
**一个在 sys_dept 里不存在的 dept_id 永远不可能出现在允许列表中**。
所以 `d.dept_id IN (...)` 和 `u.dept_id IN (...)` 对每一行的判定完全一致。

唯一的例外是兜底条件：Java 写 `d.dept_id = 0`，靠 JOIN 出 NULL 才恒假。
换成 `u.dept_id = 0` 后，**dept_id 真的等于 0 的用户会被放行** —— 兜底反而漏人。
所以改成与别名无关的 `1 = 0`，任何情况下都恒假。

**禁止回退**：

- 不要"为了保险"给用户列表加回 DISTINCT。判断标准只有一条：
  **这个查询 join 的表会不会让一行变多行**。只 join 主键就不会。
- 不要把 `authUserDB` 的 DISTINCT 也一起去掉，那里是真需要。
- `denyAll` 不要改回 `dept_id = 0`。

**验证方式**：`test/datascope_test.go` 用真实登录逐个断言五种 data_scope 的可见范围。
改动数据权限 SQL 后必须跑它，纸上的等价性证明不能替代。

**相关位置**：`internal/repository/user.go`、`internal/service/user.go` 的 `userScope`、
`internal/datascope/datascope.go` 的 `denyAll`、`docs/PERF.md`

---

## 2026-08-22 补四个索引（推翻 find_in_set 的判断）

**当前边界**：`sql/001_perf_indexes.sql` 给原始表结构补四个索引：
`sys_user(user_name)`、`sys_user(dept_id, del_flag)`、
`sys_user_role(role_id)`、`sys_dept(parent_id)`。

`ry_20260417.sql` 一行不改，索引作为增量脚本单独维护。
回滚：`go run ./cmd/perfseed -unindex`。

**为什么不算违反"表结构一行不改"**：索引对 Java 和 Go 都是透明的 ——
不增删列、不改类型、不改约束语义，两版并排跑不受任何影响。
改的是执行计划，不是数据契约。

**实测依据**：`sys_user` 和 `sys_dept` 在原始 DDL 里**只有主键，一个二级索引都没有**。
10 万用户下登录的 `WHERE user_name = ?` 是全表扫，177ms；建索引后 1.5ms，**快 118 倍**。
登录在每个会话的关键路径上。

**⚠️ 这里推翻了 2026-08-21「数据权限暂不做 find_in_set 优化」里的判断。**
那条写着"`find_in_set` 走不了索引，部门数上千后是明确的性能问题"。实测：

| 写法 | 耗时 |
|---|---|
| `find_in_set`（Java 原版） | 448ms |
| `ancestors` 前缀匹配（候选方案） | 432ms |

**只差 3.6%**，等价性验证（COUNT / SUM / MIN / MAX 四个值）完全一致。
开销根本不在 `find_in_set` 身上，而在同一条 SQL 里的 DISTINCT 和 JOIN。
那两处去掉之后这条查询是 1.5ms，**更没有换写法的理由**。

**禁止回退**：不要再提"把 find_in_set 换成前缀匹配"。要重新提，
必须先拿出新的实测数字，并说明为什么 1.5ms 还不够。

**一个方法论教训**：那条决议是**从原理推出来的**（"函数调用用不了索引"），
推理本身没错，但它在整条 SQL 里的占比从来没量过。
性能判断必须有数字，"看起来像瓶颈"和"是瓶颈"是两回事。

**另一个**：索引让登录 SQL 快了 118 倍，但接口层 QPS 只从 206 变成 226 ——
因为登录的瓶颈是 bcrypt 的 CPU，不是那次查询。
**SQL 层的巨大提升不一定反映到接口层**，这就是两层都要测的理由。

**相关位置**：`sql/001_perf_indexes.sql`、`cmd/perfseed/index.go`、`docs/PERF.md`

---

## 2026-08-22 可观测性只做三件事：traceId、慢请求日志、真健康检查

**当前边界**：

1. `middleware.Trace()` 给每个请求分配 traceId，塞进 `context.Context`，
   并写进响应头 `X-Trace-Id`。日志由 `logx.ContextHandler` 自动补上该字段。
   反代传进来的 `X-Request-Id` 会被沿用，但要过滤（只允许字母数字和 `-_`，长度 ≤64）。
2. 请求耗时超过 `server.slowRequestThreshold`（默认 500ms）单独打 `warn`。
3. `GET /health` 真去 ping MySQL 和 Redis，并带上连接池状态；
   依赖不可用时返回 **HTTP 503**。

**不做**：Prometheus / Grafana / OpenTelemetry。

**为什么**：

- **traceId**：没有它时，一次请求的日志散落在几十条并发请求中间，只能靠时间戳猜。
  用户报"我刚才保存失败了"，连是哪一次请求都定位不到。
  用 `slog.Handler` 包一层而不是让业务层自己 `slog.With` ——
  后者要改几百个调用点，漏一个就断一次链，靠 review 保不住。
- **慢请求日志**：正常请求也在打日志，慢的那几条淹在里面看不见。
  提到 warn 级别，配上日志平台的等级过滤就是一份免费的性能报告。
  这是上监控系统之前性价比最高的手段。
- **健康检查返回 503**：进程活着不等于服务可用。数据库连不上时，恒返 200 的探活接口
  会让负载均衡器继续打流量，每个请求都在超时后报 500，比直接摘掉实例糟糕得多。
- **不上 Prometheus**：并发 100 以内的自用后台，指标系统的运维成本高于它带来的价值。
  真需要时再加，不要现在就背上。

**这里故意违反了「HTTP 状态码一律 200」**：那条规矩是为 RuoYi 前端定的（它只看 body 里的 `code`）。
但 `/health` 的消费者是负载均衡器和探针，它们只认 HTTP 状态码、根本不解析 body。
所以 `/health` 也不套 `AjaxResult`，直接返回裸结构。**除它之外没有第二个例外。**

**禁止回退**：

- 不要为了"少一个中间件"把 `Trace()` 挪到 `Recovery()` 后面 ——
  那样 panic 那条日志就没有 traceId，而那恰恰是最需要能追溯的一条。
- 不要无条件信任外部传入的 `X-Request-Id`。它是外部输入，
  超长会撑爆日志存储，带控制字符能做日志注入。
- 业务层写日志一律用 `slog.XxxContext(ctx, ...)`。用不带 ctx 的 `slog.Info`
  拿不到 traceId，链就断在那里。

**相关位置**：`pkg/logx/`、`internal/middleware/trace.go`、`internal/middleware/logger.go`、
`internal/service/health.go`、`docs/PERF.md`

---

## 2026-08-22 请求日志的 query string 必须脱敏

**当前边界**：`middleware.Logger` 打印 query 前先过 `desensitize()`，
与操作日志中间件共用同一套正则。

**为什么**：`Logger` 原来的注释写着"只记录元信息，不记录请求体 —— 密码在请求体里"。
推理没错，但**前提是错的**：`/system/user/profile/updatePwd` 的
`oldPassword` / `newPassword` 走的是查询串，不是请求体。
结果就是每次用户改密码，两个明文密码原样进应用日志，直接违反 CLAUDE.md 硬约束第 7 条。

操作日志那边一开始就做了两种格式的脱敏（JSON 体 + 查询串），请求日志漏了查询串这一半。

**这个 bug 的性质值得记一笔**：注释自证正确，代码也确实照注释做了，
但注释对事实的判断是错的。这类问题测试也测不出来 —— 除非专门去断言日志内容。
**改动涉及"哪些数据会被记录"时，要按数据实际所在的位置逐个核对，不要照着注释推。**

**禁止回退**：不要为了"日志更好查"把 query 原样打出来。
需要完整参数时去看操作日志表，那边同样是脱敏后的。

**相关位置**：`internal/middleware/logger.go`、`internal/middleware/operlog.go` 的 `desensitize`

---

## 2026-08-22 接口自动化测试取代"点一遍前端"作为完成标准

**当前边界**：`test/` 下是接口层自动化测试，用 httptest 直接打路由，连真实 MySQL + Redis。
`.\scripts\test.ps1 [用例名正则]` 运行，结果写进 `test/results/summary.log` 和 `latest.log`。
当前 272 个用例，全量约 7 秒。

模块做完的判定标准从"用前端点一遍"改成"对应的测试文件写完且全绿"。前端仍要点，
但那是补充不是主证据。

**为什么**：手工点前端有三类问题永远发现不了，而这三类恰恰是本项目最容易出错的地方：

1. **删除后再按主键查** —— UI 删完就刷新列表，没人会再去请求那个 ID。
   `SelectDeptByID` 不过滤 `del_flag` 因此能往已删除的部门下建子部门，
   就是这条用例挖出来的（见 2026-08-22 那条决议）。
2. **响应结构的细节** —— 平铺字段错位、叶子节点多出空 `children`、
   `status` 被建模成数字，前端很多时候只是"显示不太对"，不会报错。
3. **数据权限** —— 管理员自己怎么点都是全量。必须建部门树、建受限角色、
   建用户，再**用那个用户真的登录一次**，才能验证过滤条件生效。
   该失效时接口不报错，只是悄悄多返回别人部门的数据。

另外校验规则是逐字段的组合爆炸（少传 / 传空串 / 传纯空格 / 超一个字符 / 传字典外的值），
手工穷举不现实，而这些恰好是踩过最多次坑的地方（零值陷阱、指针字段的两个失败点、
`oneof` 硬编码字典）。

**禁止回退**：

- 不要为了跑测试去改数据库或配置（比如关掉验证码）。验证码答案直接从 Redis 读
  （`redisx.CaptchaKey(uuid)`），测试有 Redis 权限，不需要动运行环境。
- 不要只断言 `code == 200`。**改完密码要真的用新密码登录一次**，
  **停用账号后要真的登录失败一次** —— 密码存明文、存错列、bcrypt 没生效，
  接口一样返回成功。
- 断言"能看到哪些记录"时不要拉一页列表再比对，要带条件精确查询。
  用户数超过 pageSize 后想找的记录会翻到第二页，断言会时对时错。
- 新增模块必须同时补测试文件，照 `test/post_test.go` 的结构写：
  CRUD 一遍 + 契约断言 + 校验用例表（用 mutate 函数改单个字段，不要预先拼好整张 map，
  否则循环变量会把被测字段覆盖掉）。

**相关位置**：`test/`（`main_test.go` 装配与幂等清理、`helper_test.go` 断言工具）、`scripts/test.ps1`

---

## 2026-08-20 技术栈定为 Gin + GORM

**当前边界**：Web 用 gin，ORM 用 gorm，JWT 用 golang-jwt/v5，缓存 go-redis/v9，日志用标准库 slog。

**为什么**：本项目主要靠 AI 辅助开发，选型的首要标准是**训练数据充足、生成准确率高**，而不是框架本身的性能或优雅程度。Gin + GORM 是 Go 生态里资料最多的组合。相比之下 Fiber、Echo、ent、sqlc 都会显著提高 AI 写错的概率。

**禁止回退**：不要因为"Fiber 更快""sqlc 更类型安全"就替换。性能瓶颈在数据库，不在 Web 框架。要换必须先有实测依据。

---

## 2026-08-20 前端与数据库都不动，只替换中间层

**当前边界**：直接复用 `RuoYi-Vue3-master` 前端和 `ry_20260417.sql` 表结构，Go 后端严格对齐现有接口契约。

**为什么**：这是能低成本完成移植的关键。好处有三：省掉前端重写（工期减半）、每写完一个模块能立刻用真实前端验证、可以跟 Java 版并排跑对拍返回值。如果前端一起重写，就失去了唯一的正确性基准。

**禁止回退**：不要"顺手优化"响应结构（比如把 `total`/`rows` 收进 `data`、把平铺的 `token` 挪进 `data`）。前端不认就是错的，哪怕设计更合理。

**相关位置**：@CONVENTIONS.md 第一节

---

## 2026-08-20 密码沿用 bcrypt

**当前边界**：`golang.org/x/crypto/bcrypt`，直接校验 `sys_user.password` 里 Java 版生成的 hash。

**为什么**：Spring Security 用的就是 BCrypt（见 `SecurityConfig.bCryptPasswordEncoder()`），算法跨语言通用。这样现有账号数据零改动，也能随时切回 Java 版。

**禁止回退**：不要换成 argon2、scrypt 或自研加盐方案，会导致所有存量密码失效。

---

## 2026-08-20 会话沿用 JWT + Redis 双层设计

**当前边界**：JWT 只装一个随机 UUID（`login_user_key`），真实会话存 Redis 的 `login_tokens:<uuid>`。

**为什么**：初看多此一举——把用户信息直接放进 JWT 不是更省一次 Redis 查询吗？但那样会失去服务端的会话控制能力：在线用户列表、强制踢下线、改权限后即时生效、改密码后使旧 token 失效，全都依赖服务端能查到并删掉会话。RuoYi 的这个设计是对的。

**禁止回退**：不要为了"减少一次 Redis 查询"把用户信息、角色、权限塞进 JWT payload。这不是性能优化，是功能删除。

**相关位置**：@CONVENTIONS.md 第二节

---

## 2026-08-20 暂不做工作流

**当前边界**：不引入任何流程引擎。审批类需求先按状态机实现（业务表 `status` 字段 + 审批流水表）。

**为什么**：RuoYi 原版本身就没有工作流（全仓库无 flowable/activiti 依赖，20 张表里没有任何流程表），所以走 Go 不存在"丢掉已有能力"的问题。而当前业务尚未出现复杂审批需求，提前引入引擎是过度设计。

**禁止回退**：真需要时，按复杂度分档处理，不要一上来就自研引擎：

- 单人串行、固定节点 → 状态机，够用
- 可配置节点、简单条件分支、驳回、抄送 → 自己写小引擎（JSON 流程定义 + 解释器）
- **会签 / 并行网关 / 加签 / 动态审批人 / 流程版本管理** → 不要用 Go 自研。起一个独立的 Java + Flowable 服务，或直接对接钉钉/飞书审批

**约束**：不管走哪档，审批逻辑必须收在独立模块里。**禁止把 `status` 判断散落进各个业务 service**，否则将来换实现的代价会失控。

---

## 2026-08-22 按主键查询一律过滤 del_flag

**当前边界**：`SelectDeptByID` / `SelectUserByID` / `SelectRoleByID` 都加了
`del_flag = '0'` 条件。Java 版的对应 mapper 没有这个过滤。

**为什么**：不加的话，已逻辑删除的记录仍能按主键查到。后果不止"查得到"：

- `CreateDept` 用 `SelectDeptByID` 查父部门 → **可以往已删除的部门下建子部门**，
  建出来的分支在列表里根本看不见
- 已删除的用户仍能被改状态、重置密码
- 已删除的角色仍能被授权给用户

前端从不请求已删除的记录，所以这个过滤对前端完全无感，是纯粹的加固。

**发现方式**：接口自动化测试的"删除后再查应当失败"用例。这类问题手工点是点不出来的
——UI 删完就刷新列表了，没人会再去请求那个 ID。

**禁止回退**：不要因为"Java 没过滤"就去掉。列表查询本来就都带 del_flag 条件，
按主键查不带才是不一致。

---

## 2026-08-21 定时任务暂不实现

**当前边界**：`/monitor/job` 和 `/monitor/jobLog` 不做。菜单里的"定时任务"点进去会 404，
需要的话先把该菜单隐藏（`sys_menu` 里把对应记录的 `visible` 改成 `1`）。

**为什么**：Java 的 `invoke_target` 是 `ryTask.ryParams('ry')` 这种字符串，
靠 Spring 反射调用任意 bean。Go 没有等价机制，只能预先注册命名任务：

```go
job.Register("ryTask.ryNoParams", func(ctx context.Context, args []string) error { ... })
```

代价是**新增定时任务要改代码重新部署**，不能像 Java 那样纯后台配置。
这个能力差异是真实的，当前没有定时任务需求，不值得为此先把框架搭起来。

**禁止回退**：真要做时，**不要**试图用反射去模拟 Java 的任意调用。
RuoYi 自己为了防 RCE 专门加了 `invoke_target` 白名单校验——反射调用任意方法本身就是个洞。
Go 侧的注册表方案是更安全的形态，接受"要改代码"这个代价。

**相关位置**：`../RuoYi-Vue-master/ruoyi-quartz/`

---

## 2026-08-21 代码生成器推迟，先靠 vibe coding 铺模块

**当前边界**：不做代码生成器。业务模块由 AI 照着岗位管理（`model/sys_post.go` +
`repository/post.go` + `service/post.go` + `handler/post.go`）这套参考实现手写。

**为什么**：模板还没稳定。岗位模块做完之后又连续改了 4 次（零值陷阱、字典 oneof、
notblank、导出的 Content-Type），每次都是模板级的改动。现在固化生成器，等于把
还在变的东西刻进石头，后面每改一次规范都要回头改模板 + 重新生成所有模块。

**禁止回退**：CLAUDE.md 里"模块化 CRUD 一律用生成器产出，不要手写"这条**暂时不生效**。
等模块铺到 5 个以上、连续两个模块不需要改动参考实现时，再回来做生成器。

**相关位置**：@../CLAUDE.md 的"工作方式"一节

---

## 2026-08-21 数据权限暂不做 find_in_set 优化

**当前边界**：`internal/datascope` 忠实复刻 Java 的
`dept_id IN (SELECT dept_id FROM sys_dept WHERE dept_id = ? OR find_in_set(?, ancestors))`。

**为什么**：之前记的"改用 `ancestors LIKE '0,100,%'` 前缀匹配"是对的方向，但要先拿到
当前部门的完整 ancestors 路径才能拼前缀，多一层依赖；而且两种写法的结果等价性
需要真实数据比对才能确认。sys_dept 通常只有几十到几百行，子查询会被 MySQL 物化，
现阶段不是瓶颈。先保证正确，别过早优化。

**禁止回退**：部门数上千、列表查询确实变慢时再优化，且必须先验证两种写法返回的
记录集合完全一致（同一用户、同一角色配置、多角色取并集）。

---

## 2026-08-21 平铺字段清单化，禁止凭直觉设计响应结构

**当前边界**：@CONVENTIONS.md 第一节列出了全部 13 个带平铺字段的接口，其中 `/system/user/{userId}`、`/system/user/profile`、`/system/notice/listTop` 三个是 `data` 与平铺字段并存的混合形态。

**为什么**：RuoYi 的 `AjaxResult` 继承 `HashMap`，`ajax.put()` 直接往顶层塞键。这不是设计出来的规范，是实现方式的副产物，所以毫无规律可循——同样是树选择接口，`deptTree` 用 `depts`+`checkedKeys` 平铺，`getRouters` 却把树放进 `data`。任何"看起来应该是这样"的推断都会错。

**禁止回退**：
- 不要把平铺字段"整理"进 `data`，哪怕结构明显更干净
- 不要假设同类接口有同样的形态
- 新接口实现前，**必须打开 Java 版对应 Controller 确认它 `put` 了什么**

**相关位置**：`../RuoYi-Vue-master/ruoyi-admin/src/main/java/com/ruoyi/web/controller/`，搜 `ajax.put(`

---

## 2026-08-21 Redis 遍历一律用 SCAN，禁止 KEYS

**当前边界**：`pkg/redisx.ScanKeys` 用游标迭代，业务代码禁止调用 `KEYS`。

**为什么**：Java 版 `TokenService.refreshPermissionByRoleId` 用 `redisCache.keys("login_tokens:*")` 扫全部在线会话。`KEYS` 是 O(N) 且**阻塞整个 Redis 实例**，在线用户上千时，一次角色权限变更就能让全站卡顿数秒。

**禁止回退**：不要因为"SCAN 要写循环、KEYS 一行搞定"就改回去。SCAN 的弱一致性（迭代期间新增的 key 可能漏掉）对"刷新在线用户权限"这个场景完全可接受。

**相关位置**：`../RuoYi-Vue-master/ruoyi-framework/src/main/java/com/ruoyi/framework/web/service/TokenService.java:243`

---

## 2026-08-21 repository 用包级变量而非依赖注入

**当前边界**：`repository.Init()` 初始化包级 `db`，业务通过 `repository.DB(ctx)` 取带 ctx 的会话。不做构造函数注入，不传 `*gorm.DB`。

**为什么**：本项目绝大多数模块是生成器产出的同构 CRUD，一致性比可测试性重要。DI 的样板代码越多，AI 生成时的花样越多（有的注入、有的全局、有的开新会话），最终维护成本更高。包级访问器只有一种写法，不会写歪。

**禁止回退**：如果将来确实需要 mock 数据库做单测，用 `sqlmock` 或 docker 起真实 MySQL，不要为此重构成 DI。

---

## 2026-08-20 数据权限允许改用前缀匹配，但结果必须一致

**当前边界**：`pkg/datascope` 返回 GORM Scope。"本部门及以下"允许用 `ancestors LIKE '0,100,%'` 替代 Java 版的 `find_in_set`。

**为什么**：`find_in_set(deptId, ancestors)` 走不了索引，每次列表查询都要扫一遍 `sys_dept`。部门数上千后是明确的性能问题。

**禁止回退**：不要为了性能改变过滤语义。切换实现前必须验证：同一用户、同一角色配置下，Go 版和 Java 版返回的记录集合完全相同。多角色场景取并集，别漏。

**相关位置**：`../RuoYi-Vue-master/ruoyi-framework/src/main/java/com/ruoyi/framework/aspectj/DataScopeAspect.java:114`
