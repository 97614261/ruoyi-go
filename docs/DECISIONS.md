# 决议记录

按日期追加，**历史理由只增不改**。稳定的规则请写进 [CONVENTIONS.md](./CONVENTIONS.md)，
这里只记"为什么当初这么定"。允许在旧条目标题下补充状态导航指针，说明该条已被哪项
后续决议取代、更正或缩小范围；导航指针不算改写历史，修改旧正文的理由和结论才算。

每条格式：**当前边界 / 为什么 / 禁止回退 / 相关位置**。

---

## 2026-09-02 Redis 连接池采用 32，并保留外部 A/B 证据

**当前边界**：单实例默认和部署配置的 `redis.poolSize` 从 20 调整为 32，目标是当前 2C2G
部署。`mysql.maxIdleConns=20` 和 `mysql.maxOpenConns=50` 不随本条修改；两类连接池的数字
来自不同 A/B，不能因为名字相似一起扩大。

**为什么**：从独立 Windows 压测端经公网直连目标服务，按交错顺序对 20/32/40 各跑 3 轮
`c80 × 60s`。三轮中位数如下：

| Redis 连接数 | QPS 中位数 | P95 中位数 | Redis 等待次数中位数 | 错误/超时 |
|---:|---:|---:|---:|---:|
| 20 | 836.10 | 136.30ms | 135917 | 0/0 |
| **32** | **865.41** | **135.96ms** | **60565** | **0/0** |
| 40 | 857.40 | 138.88ms | 14981 | 0/0 |

32 相对 20 的中位 QPS 提升约 3.5%，P95 没有回退，池等待减少约 55%；40 虽继续减少等待，
但吞吐和 P95 都没有继续改善。选择 32 是当前硬件下的折中，不是“连接越多越快”。随后用 32
补跑 `c80 × 10m`，完成 505431 次请求、842.38 QPS、P95 140.67ms、错误 0。

**禁止回退/外推**：不要凭单轮数字改回 20，也不要只看等待次数把连接池继续扩大。更换 CPU、
Redis 部署方式、网络或实例数后必须重新 A/B；多实例还须把所有实例的连接总数一起计算。相关
证据见 `docs/OPTIMIZATION_VALIDATION_2026-09-02.md`。

---

## 2026-08-31 全路由场景覆盖不等于行为全等，差异按收益登记

**当前边界**：Java 的 130 条非代码生成器路由与 Go 的方法、结构化路径和权限标识全部匹配，
每条共同路由现在至少有一个自动双端场景。新增补充场景为 52/57 精确或语义一致；破坏性接口
中的一部分只验证双方都拒绝未登录请求，因此 130/130 表示“有自动执行入口”，不表示每条
成功路径、所有参数组合、响应字段和副作用都已全等。

本轮发现的响应形状差异必须修复：个人资料统一使用 Java 登录会话对象投影；角色已分配/未分配
用户统一使用 Java 窄列查询的投影；用户授权角色页统一复用 Java 兼容用户/角色转换。修复后
三个场景精确一致。下列可观察差异保留并纳入当前人工差异基线：

- 缺失文件下载时 Go 返回结构化业务错误，Java 暴露原始异常文本；缺失上传附件同理，Go 返回
  可读的白名单消息，不复刻 Java 的空指针或 multipart 内部异常。
- Go 缓存监控有意不展示 `login_tokens:`，且清理和值读取只允许白名单前缀，避免持有缓存权限的
  用户读取完整会话、权限或误删其他业务 key；Java 会展示会话分类并对未知清理静默成功。
- 不存在缓存值的空串/`null`、部分新增后详情的空串/`null`、校验错误文案不同，不改变已知 Vue
  分支、字段类型或成功业务结果。
- 查询已删除或不存在对象时 Go 返回明确失败，Java 部分 mapper 因未过滤 `del_flag` 或 controller
  不检查空值而返回成功；Go 的严格不存在语义保留。
- Go 更新公告时保存 `remark`；Java `updateNotice` mapper 漏写该列。Go 创建定时任务时校验
  `jobName`；Java 虽声明 `@NotBlank`，但新增入口没有启用 `@Validated`。两项都保留 Go 行为。
- 稳定分页的唯一兜底排序、数据权限安全加固、会话即时失效和更具体的错误消息继续按既有决议
  保留，不为了把 `different` 归零而删除。

**为什么**：补齐 57 个遗漏场景后，未测试过的投影错误确实被发现并修复，但也再次证明“与
Java 不同”不等于“Go 有缺陷”。原始异常泄露、可浏览完整会话、删除后仍返回对象、漏写字段和
漏校验都不值得复刻。需要固定的是消费者依赖的契约和安全不变量，而不是 Java 的每个缺陷。

**禁止回退**：不得把路由覆盖率写成成功行为一致率；不得把未登录拒绝场景写成已验证成功
副作用；不得为了提高精确一致百分比重新暴露会话缓存、吞掉不存在错误、漏写公告备注或放过
空任务名。`-AllowDifferences` 仍只适合人工复核；机器可读差异基线实现前，新增差异必须逐条
查看日志，不能因为命令退出 0 就视为已批准。

**相关位置**：`cmd/contractcheck/supplemental.go`、`cmd/routeaudit/main.go`、
`internal/handler/java_contract.go`、`internal/handler/profile.go`、`internal/handler/role.go`、
`internal/handler/user.go`、`docs/API_ROUTE_COVERAGE_2026-08-25.md`、
`docs/VALIDATION_REPORT_2026-08-31.md`

---

## 2026-08-28 目标从"复刻 Java"改为"超越 Java"，冻结契约面

**当前边界**：项目目标是做出比 Java 版更好的后端，同时冻结已有消费者依赖的契约。
边界分三层，完整表述见 [../CLAUDE.md](../CLAUDE.md) 开头：

- **冻结层**：前端源码不改；请求路径、方法、Content-Type、参数位置 / 名字 / 类型和
  前端依赖的必填、默认语义不改；响应字段的名字、是否出现、层级、类型、空值语义以及
  下载格式不改。普通 RuoYi 业务响应 HTTP 状态保持 200，只保留已登记的 CORS 预检 204、
  请求体超限 413、健康检查失败 503。生产数据库冻结表、列、类型、可空性、默认值、
  字符集 / collation、约束和索引；运行期业务数据不属于结构冻结，但被 Go、Java 参考版、
  Vue、部署脚本或测试按名字 / 固定 ID 引用的标识符属于契约数据。当前明确包括
  `sys_config.config_key`、`sys_dict_type.dict_type`、`sys_menu.perms`、
  `sys_user.user_id = 1` 和 `sys_role.role_id = 1`，新增同类标识符自动纳入冻结层。
  `pkg/redisx/keys.go` 中的 Redis Key 前缀同样属于运维契约，改名必须同步部署清理、
  会话迁移和测试清理。
- **超越层**：不改变冻结契约和正常业务语义的内部保障，包括原子性、并发控制、事务、
  缓存一致性、批量查询、超时、连接池、背压、日志和追踪。
  如果加固会改变返回值、行集、顺序或成功 / 失败语义，则同时进入受控差异层，必须经过
  第三层审查，不能只按内部实现处理。
- **受控差异层**：请求 / 响应形状不变，但返回值、行集、行序、错误文案或授权结果与
  Java 不同。必须有安全或正确性收益、不破坏任何已知消费者、登记本决议并补定向测试。

**为什么**：原来的目标是"复刻"，于是「Java 也有这个问题」成了免死金牌 ——
一份 22 条的运行时审计里，有 11 条被归进"Java 同源、建议修复"，等于把
"Java 的缺陷"和"我们的欠债"混在一张待办里，读的人分不清哪些是必须补的、
哪些是主动加固。而这些条目（会话不失效、写操作缺数据权限、无数据库超时、
唯一性并发竞争）恰恰是安全和正确性上最该修的。

同时"复刻"这个词也在误导实现：Java 的 `find_in_set`、无序分页、
明文密码进操作日志、`GET + DEL` 非原子，照抄过来都不是对齐，是把缺陷搬过来。

冻结契约面是为了让现有 Vue 无需改动即可切换后端，但 **Vue 只是用户界面兼容性的首要
基准，不是唯一正确性基准**。下载工具、部署和压测脚本、运维探针也会直接消费接口；
认证、授权和数据权限等安全不变量更不能用"前端是否报错"判断。Java 提供参考行为，
API 约定、已知消费者和安全 / 正确性约束共同决定哪个实现正确。

**数据库连索引一起冻结**：2026-08-22 补的四个索引是独立增量脚本
`sql/001_perf_indexes.sql`，没有进入 `ry_20260417.sql`。该脚本只允许用于隔离的压测环境，
**不得在生产库执行**；本文不对任何现有生产库是否曾执行过它作事实判断，部署时应另行核对。
本条取代 2026-08-22“索引不算违反表结构冻结”的旧判断；旧条目仅作为历史过程保留，
不得再作为生产执行依据。
代价要说清楚：登录的 `WHERE user_name = ?` 是全表扫，10 万用户下实测 177ms
（建索引后 1.5ms）。线性外推 1 万用户约 18ms、1000 用户约 2ms ——
**几千用户以内没有实际影响**，自用后台正是这个量级。
真正的触发线是**用户数上万**，届时症状是"登录转圈"、不报错，很难联想到索引。
不得拿仓库里存在该脚本当成生产库"已经补了四个索引"的证据。

**禁止回退**：

- 不得因为"Java 也这样"就把安全或并发缺陷标成"已对齐、不修"。
  归类时必须回答的是"哪个对"，不是"哪个一样"。
- 不得为了消掉双端对拍差异去掉已登记的加固（唯一兜底排序、`del_flag` 过滤、
  更具体的错误文案）。目标不再是所有探针 `different=0`，而是每一条差异都有登记的理由。
  **当前 `-AllowDifferences` 会放过全部差异，并不能识别"已登记"和"新增"；**在实现机器可读
  差异基线前，它只能用于人工复核，不能宣称 CI 已能阻止未登记差异。无已知差异的核心探针
  仍应按严格模式运行。
- 不得以"超越 Java"为由改动冻结层。想改前端或数据库，先改这条决议。
- 受控差异不得只写在代码注释里，必须登记进本文件 ——
  否则下一轮审计会把它当成缺陷"修"回去。

**相关位置**：`../CLAUDE.md` 开头三层边界、`docs/RUNTIME_AUDIT.md`、
`cmd/contractcheck/`、`pkg/redisx/keys.go`、`sql/001_perf_indexes.sql`

---

## 2026-08-28 会话续期原地合并，权限更新使用 revision CAS

> ⚠️ **状态导航：具体写入方式已被后续 2026-08-30「权限传播、入口顺序与退出排空」取代。**
> generation/revision CAS 和禁止旧快照覆盖新权限仍然有效，但不再经 Lua 解码后重编码 JSON。

**当前边界**：`RefreshToken` 通过 Lua 读取 Redis 中当前会话，只更新时间字段和 token、
用户索引、generation 三个 TTL，不再整体写回请求持有的会话快照。权限更新在 Redis 会话
中维护 `sessionRevision`：Lua 同时比较 generation 和 revision，成功后只替换最新用户、
部门和权限并递增 revision。revision 冲突会重新读取 Redis、重新查询数据库权限，最多
重试 3 次；仍冲突或会话内容异常时删除该会话，用户资料/权限无法加载时撤销该用户全部
会话。批量刷新继续处理其余会话，但最终返回首个错误，不静默宣告全部成功。

**为什么**：旧实现的鉴权续期和权限刷新都会把较早读出的完整 `LoginUser` 重新 SET，
可能在管理员撤权后把旧权限覆盖回来并随活跃续期长期保留。只执行 `PEXPIRE` 又会让 JSON
中的 `ExpireTime` 停留在旧值，进入刷新窗口后每个请求都重复续期。Lua 原地更新时间既
保持 Java 可观察的时间语义，又不接触权限；revision CAS 则让多 Go 实例间的权限写入也
能检测冲突。Java 参考实现没有同类并发保护，这是有意修复安全缺陷，不是接口差异。

**禁止回退**：普通续期不得接收或整体写回 `User/Roles/Permissions`；权限写入不得跳过
generation/revision 比较，不得在冲突后沿用原数据库快照。无法确认最终权限时必须安全
撤销，不能仅记日志继续保留旧会话。修改时必须保留旧请求续期不覆盖新权限、revision
冲突重算、异常状态撤销、多个 token 独立 revision 和三个 TTL 对齐测试。

**边界**：该方案解决 Redis 内同一会话的并发覆盖，不把 MySQL 提交与 Redis 刷新变成
分布式事务。生产仍要求 Java 停止写入同一 Redis DB；Redis 故障或数据库提交后进程崩溃
需要通过接口错误、监控和后续补偿处理，不能宣称绝对原子。

**相关位置**：`internal/model/login.go`、`internal/service/token.go`、
`internal/handler/login.go`、`test/session_test.go`

---

## 2026-08-28 在线用户内存分页使用唯一会话 ID 兜底

**当前边界**：在线用户列表完整读取并过滤会话后，先按 `LoginTime DESC` 排列；登录时间
相同时按唯一的 `TokenID ASC` 兜底，建立全序后才按 `pageNum/pageSize` 切片。Token ID
只参与内存排序和既有强退接口，不新增日志或响应字段。

**为什么**：Redis SCAN 的输入顺序不稳定，登录与续期时间又可能在同一毫秒并列。只按
登录时间排序时，并列记录会继承每次不同的 SCAN 顺序，连续请求不同页可能重复或漏掉
会话。`sort.SliceStable` 只能稳定本次输入，无法稳定跨请求输入；唯一兜底才能形成确定顺序。

**禁止回退**：内存分页和 SQL 分页一样，切页前必须包含唯一兜底键；不得只换成稳定排序，
也不得把 Token ID 写入日志。修改在线排序时必须保留并列时间、不同输入排列、跨页无重漏
和越界空页测试。

**相关位置**：`internal/service/online.go`、`internal/service/online_test.go`、
`docs/CONVENTIONS.md`

---

## 2026-08-28 定向刷新和会话撤销只信任新版用户索引

**当前边界**：`RefreshOnlineUsersByID` 先用 Redis Pipeline 批量读取
`login_user_sessions:<userId>`，去重 token UUID 后按每批 100 个 MGET 会话；每个有在线
会话的用户只重新加载一次资料和权限。单用户刷新委托给同一批量实现。过期 token 的索引
成员会被 SREM；`RevokeUserSessions` 只删除索引内会话并递增会话代数，不再执行兼容
`SCAN login_tokens:*`。

**为什么**：旧实现批量授权 N 个用户会执行 N 遍全量 SCAN，Redis 工作量随“用户数 ×
全站会话数”增长。生产已经确定采用 Java 停机硬切 Go，并在恢复流量前清理三个会话前缀，
因此运行期所有合法会话都由新版 Go 建立反向索引，不再需要永久支付旧会话兼容成本。
会话代数仍能拒绝意外残留的无索引 Token，但残余 key 的物理清理由部署检查单负责。

**禁止回退**：批量用户刷新不得在用户循环中调用 SCAN，也不得恢复撤销路径的兼容全量
扫描；不得自动在普通启动时清理会话。修改索引读取时必须保留多用户、多会话、重复用户
ID、失效索引清理和无索引残余不参与刷新测试。若未来改为无停机混合部署，必须先重新设计
兼容方案，不能默默依赖本实现读取 Java 会话。

**相关位置**：`internal/service/token.go`、`internal/service/role.go`、
`test/session_test.go`、`docs/DEPLOYMENT.md`

---

## 2026-08-28 Java 硬切 Go 时清理旧会话，之后只接受新版索引会话

**当前边界**：生产部署采用 Go 完全替换 Java 的停机切换时，先停止入口流量、Java 和旧
Go 实例，再分批清理 `login_tokens:*`、`login_user_sessions:*` 和
`login_user_generation:*`。恢复流量后所有用户重新登录，所有新会话都由新版 Go 建立
用户反向索引。普通 Go 重启、滚动升级和双端对拍不执行该清理。

**为什么**：旧 Go 和 Java 会话可能没有 `login_user_sessions:<userId>`，长期保留兼容
扫描会让已经存在的索引失去主要性能价值。硬切时主动失效全部旧 Token，既能消除无索引
残余，也能避免旧权限快照跨版本继续使用；代价是切换当天所有在线用户重新登录一次。

**禁止回退**：不得在旧服务仍接收请求时清理，不得自动绑定到每次启动，不得使用
`FLUSHDB`、`FLUSHALL` 或阻塞 Redis 的 `KEYS`。只能在核对 Redis DB 后用 `SCAN` 配合
分批 `UNLINK/DEL` 删除三个会话前缀；验证码、密码锁定、限流和业务缓存不得进入范围。
Java 对拍必须使用独立 Redis DB，不能在生产切换后继续写入 Go 的会话空间。

**相关位置**：`docs/DEPLOYMENT.md`、`internal/service/token.go`、
`pkg/redisx/keys.go`

---

## 2026-08-28 普通请求体入口统一限流，并只读取一次

> ⚠️ **状态导航：中间件位置已被后续 2026-08-30「权限传播、入口顺序与退出排空」取代。**
> 大小上限、完整读取一次和 HTTP 413 语义不变，但不再位于鉴权和匿名限流之前。

**当前边界**：非 multipart 请求体在全局路由入口通过 `http.MaxBytesReader` 限制，默认
上限为 `server.maxRequestBodyMB=2`。入口实际读取完整内容，因此没有 `Content-Length` 的
chunked 请求同样受限；通过后缓存原始字节并还原给 handler，防重复提交和操作日志复用
该缓存。multipart 不在内存中预读，继续使用 `upload.maxSizeMB=10` 的独立文件规则。
超限响应保持统一 `code/msg` JSON，同时使用真实 HTTP 413。

**为什么**：原来的防重复提交和操作日志都会无上限 `io.ReadAll` 后再还原请求体，同一个
请求会形成多份完整副本。读取超大 JSON/form 时，并发请求可能引发严重 GC 压力或 OOM。
只检查 `Content-Length` 可被 chunked 绕过；只截断中间件读取又会把残缺 JSON 交给
handler，重现富文本保存失败的历史问题。因此必须在所有业务中间件之前完整、限量读取。

**禁止回退**：不得只信任 `Content-Length`，不得用 `LimitReader` 截一段后继续调用
handler；防重复提交或操作日志未取得入口缓存时只能跳过或记录固定省略提示，不得自行
无上限读取。普通请求体和 multipart
文件上限必须继续分开；调整默认值时必须保留边界、chunked、并发和 multipart 回归测试。

**相关位置**：`internal/middleware/bodylimit.go`、`internal/router/router.go`、
`internal/middleware/repeatsubmit.go`、`internal/middleware/operlog.go`、
`internal/config/config.go`、`internal/middleware/bodylimit_test.go`

---

## 2026-08-27 调度条目变更串行化，并与任务运行状态分锁

**当前边界**：`jobScheduler` 使用独立的 `scheduleMu` 串行执行同一进程内的 cron 条目
替换和删除，`AddFunc`、`Remove` 与 `entries` 更新处于同一个临界区。新增也采用按
`jobID` 替换语义，重复装载不会留下旧 entry。任务是否正在运行由独立的 `runMu` 管理，
调度配置变化不会与“禁止并发”状态互相阻塞。

**为什么**：原来的 `reschedule` 是锁外 `remove -> add`，而 `AddFunc` 成功后才单独加锁
登记 entry。并发修改同一任务可能在 cron 中留下两条记录，`entries` 却只能保存最后一条，
使前一条无法暂停或删除并持续重复执行。该问题是跨多步操作的业务竞态，即使各组件没有
Go data race，Race Detector 也不一定报告。

**禁止回退**：不得把 cron 条目操作和 `entries` 更新拆到不同临界区，不得让同一 `jobID`
的新增绕过替换语义；不得复用任务运行状态锁。修改调度器时必须保留并发重排后唯一条目、
并发暂停无残留以及两类锁相互独立的回归测试。

**相关位置**：`internal/service/job_scheduler.go`、
`internal/service/job_scheduler_test.go`

---

## 2026-08-27 验证码通过 Redis Lua 原子读取并消费

**当前边界**：验证码校验通过单段 Redis Lua 读取 key，并在 key 存在时立即删除后返回
答案。正确答案、错误答案都会消费验证码；key 不存在仍返回“验证码已失效”，字段缺失和
答案不匹配仍返回“验证码错误”。登录、注册、验证码接口及 Redis key 格式均不变。

**为什么**：原来的 `GET -> DEL` 允许并发请求在删除前读到同一答案，使一次性验证码
可能被成功使用多次。Java 参考实现也分两条命令，但这是实现竞态，不是接口契约；Lua
既消除窗口，又兼容没有 `GETDEL` 的 Redis 6.0 及更早版本，不需要新增依赖。

**禁止回退**：不得拆回独立的读取和删除命令，不得忽略验证码消费失败；修改校验逻辑时
必须保留错误尝试消费、成功后不可重用，以及并发请求只能成功一次的回归测试。

**相关位置**：`internal/service/captcha.go`、`test/captcha_security_test.go`

---

## 2026-08-27 登录错误计数原子化，并以数据库命中的账号为键

**当前边界**：密码错误通过 Redis Lua 原子执行 `INCR + PEXPIRE`，每次失败都重置
10 分钟 TTL，保持原有滑动窗口语义。登录查询命中用户后，以数据库返回的 `user_name`
生成计数键；登录成功也通过 Lua 原子检查是否已达到 5 次阈值，未锁定时才清除计数。
后台解锁使用同样的规范账号规则。当前库实测 `sys_user.user_name` 为
`utf8mb4_general_ci`，`ADMIN` 会命中 `admin`，因此两种写法共用 `pwd_err_cnt:admin`。

**为什么**：原来的 `GET -> 内存 +1 -> SET` 会让并发失败互相覆盖；直接使用请求原文
又会在大小写不敏感的数据库中为同一账号创建多个 Redis key。自行 `ToLower` 不能准确
模拟不同数据库的 collation，也会误伤大小写敏感库中两个真实存在的不同账号。使用查询
实际返回的账号同时适配当前库和后续更换数据库排序规则，不需要修改表结构或数据。

**禁止回退**：不得拆回多条 Redis 命令，不得根据请求原文或自行猜测大小写规则生成已
存在用户的计数键；正确密码路径不得无条件删除已经达到锁定阈值的计数。修改锁定逻辑时
必须保留并发累计、滑动 TTL、大小写匹配、阈值和解锁回归测试。

**相关位置**：`internal/service/login.go`、`internal/service/log.go`、
`internal/repository/user.go`、`test/login_security_test.go`

---

## 2026-08-27 操作日志按参数语法解析后递归脱敏

**当前边界**：操作日志中的 JSON 请求体先完整解码，再递归遍历对象和数组，使用与
`encoding/json` 绑定一致的大小写不敏感规则识别密码字段；表单和查询串先通过
`url.ParseQuery` 解码再脱敏。敏感值无论是字符串、数字、对象还是数组都整体替换为
`******`。解析失败或无法判断结构的请求体只记录固定省略提示，原始内容不落库。

**为什么**：在原始字符串上跑正则无法覆盖字段大小写、转义引号、非字符串值、嵌套
结构和百分号编码。Go JSON 绑定却接受大小写变体，导致请求可以成功执行，而明文密码
进入可查询、可导出的 `sys_oper_log.oper_param`。结构化解析同时消除了这些旁路，并且
只改日志副本，不影响 handler 接收的原始请求体和接口契约。

**禁止回退**：不得恢复基于正则的 JSON 或查询串脱敏；新增密码字段必须加入集中清单并
补大小写、嵌套、非字符串和编码变体测试。参数解析失败时不得为了“方便排查”记录原文。

**相关位置**：`internal/middleware/operlog.go`、`internal/middleware/logger.go`、
`internal/middleware/operlog_test.go`、`test/operlog_test.go`

---

## 2026-08-27 公告富文本按 Quill 能力白名单净化，导入提示只信任固定 HTML

**当前边界**：公告正文写入前使用 `bluemonday` 的定制策略净化，详情和管理列表读出时
再次净化以覆盖历史数据；保留当前 Quill 编辑器使用的标题、强调、引用、代码、列表、
缩进、字号、字体、对齐、颜色、链接、上传图片和视频 iframe。链接及资源只接受相对地址
和 HTTP/HTTPS，内联样式只接受严格格式的 `color`、`background-color`。用户导入结果仍以
固定 `<br/>` 分行，但账号、行错误和业务错误文案全部做 HTML 转义。

**为什么**：公告详情和导入结果都由前端 `v-html` 渲染。公告若整体转义会破坏富文本，
只在写入时过滤又无法保护库中已有的危险内容；导入结果则只有后端固定的 `<br/>` 需要
作为 HTML，其余动态值都不应获得生成标签的能力。读时净化无需修改数据库或迁移数据，
也不改变现有请求、响应结构。

**禁止回退**：不得把公告正文改成纯文本转义，不得放开 `script`、事件属性、任意 CSS、
`javascript:`、`vbscript:` 或 `data:` 资源；新增 Quill 格式时必须同时更新白名单和正反
安全测试。不得把未经转义的 Excel 单元格值或错误文本拼进导入结果 HTML。

**相关位置**：`pkg/htmlx/richtext.go`、`internal/service/notice.go`、
`internal/service/user_import.go`、`test/notice_test.go`、`test/user_test.go`

---

## 2026-08-27 用户安全状态变化立即撤销 Redis 会话，不改数据库

**当前边界**：登录会话继续存放在 `login_tokens:<uuid>`，同时用
`login_user_sessions:<userId>` 维护用户到会话的反向索引，并用
`login_user_generation:<userId>` 阻止撤销期间的旧认证结果重新写回。停用、删除、
管理员重置密码和个人修改密码成功后，撤销该用户全部会话；重新启用不会恢复旧 Token。

**为什么**：Java 和旧 Go 实现只更新数据库，Redis 中已签发的权限快照会继续有效。
数据库结构是既定契约，不能增加 `auth_version`；会话索引和代数全部放在 Redis，既不改表，
也能用 Lua 原子处理“递增代数、删除会话、清理索引”。撤销时保留一次 SCAN，兼容升级前
以及 Java 版创建的无索引会话；批量删除多个用户只扫描一次。

**禁止回退**：不得只删除当前 Token，不得在停用、删除或改密后继续刷新旧会话；
不得用 Redis `KEYS` 代替兼容 SCAN。新增登录写入必须同时维护索引并校验会话代数。

**相关位置**：`internal/service/token.go`、`internal/service/login.go`、
`internal/service/user.go`、`internal/service/profile.go`、`pkg/redisx/keys.go`

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

> ⚠️ **状态导航：部分内容已被后续决议取代。** 本条对 Java 能力差异的更正仍有效；
> 其中“仍然不做定时任务”的状态已被 2026-08-23「定时任务已实现：命名注册表 +
> robfig/cron」取代。下文保留为历史评估过程，不代表当前实现状态。

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

> ⚠️ **状态导航：范围已被 2026-08-28 决议缩小。** 本条关于 `find_in_set` 只占 3.6%、
> 没有新实测就不切换写法的结论仍有效；“索引不算结构变更”的判断已被 2026-08-28
> 「目标从复刻 Java 改为超越 Java，冻结契约面」取代。候选索引仅限隔离压测库，禁止生产。

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

> ⚠️ **状态导航：当前结论已被取代，部分理由已被更正。** “暂不实现”已被 2026-08-23
> 「定时任务已实现：命名注册表 + robfig/cron」取代；对 Java 能力差异的描述已被同日
> 「更正：定时任务的能力差异被高估了」更正。下文仅保留历史过程，不得作为当前边界。

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

> ⚠️ **状态导航：理由和重新评估门槛已被更正。** “当前不切换”的结果仍有效，但
> “部门数上千、列表变慢后再优化”的性能推断已被 2026-08-22「补四个索引（推翻
> find_in_set 的判断）」实测取代：前缀匹配只快 3.6%，去掉 DISTINCT 和无用 JOIN 后
> 整条查询约 1.5ms。没有新的实测数字不得重新提出切换。

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

> ⚠️ **状态导航：切换结论已被 2026-08-22 实测取代。** 前缀匹配只快 3.6%，真正开销
> 来自 DISTINCT 和无用 JOIN；移除后整条查询约 1.5ms，当前继续使用 `find_in_set`，没有
> 新的实测证据不得重新提出切换。本条“如果未来切换，必须验证结果集完全一致”的约束仍有效。

**当前边界**：`pkg/datascope` 返回 GORM Scope。"本部门及以下"允许用 `ancestors LIKE '0,100,%'` 替代 Java 版的 `find_in_set`。

**为什么**：`find_in_set(deptId, ancestors)` 走不了索引，每次列表查询都要扫一遍 `sys_dept`。部门数上千后是明确的性能问题。

**禁止回退**：不要为了性能改变过滤语义。切换实现前必须验证：同一用户、同一角色配置下，Go 版和 Java 版返回的记录集合完全相同。多角色场景取并集，别漏。

**相关位置**：`../RuoYi-Vue-master/ruoyi-framework/src/main/java/com/ruoyi/framework/aspectj/DataScopeAspect.java:114`

---

## 2026-08-28 调度异常信息按 UTF-8 字节线性截断

**当前边界**：`sys_job_log.exception_info` 写入前最多保留 2000 个 UTF-8 字节，截断只能
停在完整字符边界；非法 UTF-8 转成替换字符。实现必须单次正向遍历，不得按字符逐个缩短后
反复构造字符串。

**为什么**：原实现最坏为 O(n²)，大错误文本会在失败日志路径产生大量分配。数据库列是
`varchar(2000)`，其上限按字符计算，但这里保留既有的 2000 字节限制，避免一次性能修复
顺带改变日志长度语义；它更保守，不会增加数据库写入风险。

**禁止回退**：不得用字节切片直接截断 UTF-8，不得恢复循环 `string(runes)` 量长；如果
以后要把上限放宽到 2000 个字符，应作为独立行为变更评估并补数据库集成验证。

**相关位置**：`internal/service/job_scheduler.go`、`internal/service/job_scheduler_test.go`

---

## 2026-08-28 头像文件生命周期跟随数据库提交结果

**当前边界**：头像更新在数据库事务中用行锁读取旧地址并写入新地址。数据库没有提交时
回收本次新上传文件；提交后保留新文件并删除真正被替换的旧文件。删除只接受严格属于配置
`URLPrefix/avatar/` 且最终位于 `upload.path/avatar` 的普通文件或最终文件软链接；外部 URL、
其他业务目录、目录上跳、目录本身和中间目录软链接一律不删。文件清理失败只记内部告警，
不反转已经确定的数据库结果。

**为什么**：Java 参考版会在头像数据库更新成功后删除旧文件，旧 Go 漏了这一步，反复换
头像会让磁盘只增不减。简单地“更新后删会话里的旧地址”又经不起同一用户并发请求：两个
请求可能读到同一个旧值并遗留中间头像；行锁让后一请求读取前一请求的新值，从而形成完整
文件链。上传先于数据库发生，因此失败路径还必须主动回收刚写入的新文件。

**禁止回退**：不得在数据库提交前删除旧头像，不得按数据库路径直接 `os.Remove`，不得
删除外部或其他业务资源；不得因清理失败谎称数据库更新已回滚。修改头像流程时必须保留
提交/未提交文件归属测试、顺序与并发接口测试、Redis 会话断言和目录逃逸测试。

**相关位置**：`internal/handler/profile.go`、`internal/service/profile.go`、
`internal/repository/user.go`、`pkg/upload/cleanup.go`、`test/profile_avatar_test.go`

---

## 2026-08-28 运行时审计除并发一致性第六批外全部收口

> ⚠️ **状态导航：范围已被后续 2026-08-28「参数缓存代数与防重复提交原子化」决议缩小。**
> P2-11、P2-12 已完成，当前只剩 P2-10 业务唯一性竞争。

**当前边界**：本轮修复 `RUNTIME_AUDIT.md` 原 P1-1～P1-4、P2-1～P2-9、P2-13、
P3-1～P3-5；按明确决定暂不处理原 P2-10（业务唯一性竞争）、P2-11（参数缓存旧值
回写）和 P2-12（防重复提交非原子），三项继续留在当前待修清单。

**受控差异**：

- 完整编辑或导入更新用户后，停用账号撤销会话，正常账号批量刷新会话；
- 角色授权、关联对象写入、角色/用户辅助详情和树接口统一校验目标对象的数据范围；
- 关联 ID 必须为正数、去重后最多 200 个、实体必须存在，越权或脏 ID 不再落关系表；
- 普通用户只能读取已发布公告，具有 `system:notice:query` 的管理用户仍可读取草稿；
- 菜单名称和路径拒绝 XSS 输入，读取历史菜单时对树标签、路由标题和危险路径字符做安全编码；
- Gin 只信任显式配置的代理，multipart 总请求超过上限返回已登记的 HTTP 413；
- 登录在查用户前执行 2～20 / 5～20 长度校验，并复用契约键
  `sys.login.blackIPList` 支持精确、后缀通配、地址段和 CIDR 匹配；
- 请求获得总 deadline，MySQL DSN 注入连接、读、写超时；非法启动配置直接拒绝启动；
- 不存在记录的用户状态/重置密码、参数和岗位更新不再假成功。

**纯内部加固**：导出查询最多读取 `MaxExportRows+1`；Excel 按行迭代且所有非空行都计入
5000 行上限，成功/失败详情各最多展示 100 条；批量删除和授权改用集合查询；操作日志和
手工任务进入独立有界执行器；忽略 context 的定时任务在 deadline 后释放调度状态并记录
失败（底层 goroutine 无法强杀，任务仍必须遵守 context 契约）；多文件上传中途失败会回滚
本请求已经保存的文件。

**为什么**：这些问题要么允许旧权限、越权关联或草稿泄露，要么让攻击者绕过限流和资源
上限，要么在数据库/日志变慢时无限堆积内存和 goroutine。Java 中存在同源缺陷的不复制；
Java 已有的登录黑名单和 multipart 总上限补齐。所有改动保持前端源码、请求路径/方法/字段、
响应字段结构、生产数据库结构/索引和现有 Redis 前缀不变。

**禁止回退**：不得恢复逐 ID 查库、无上限导入导出、每请求裸起 goroutine、默认信任全部
代理、只靠 `MaxMultipartMemory` 冒充总大小限制，或让更新用户后的旧会话继续携带旧权限。
调整上限、错误语义或任务过载策略前必须更新本决议和定向测试。第六批三项没有被本条暗中
解决，后续不得把它们误标为已完成。

**相关位置**：`internal/service/relations.go`、`internal/repository/relations.go`、
`internal/middleware/multipartlimit.go`、`internal/middleware/timeout.go`、`pkg/asyncx/pool.go`、
`pkg/excelx/import.go`、`internal/handler/common.go`。

---

## 2026-08-28 参数缓存代数与防重复提交原子化

**当前边界**：参数缓存仍使用既有 `sys_config:` 前缀，新增一个不会暴露给缓存监控的内部
代数 key `sys_config:__ruoyi_go_revision__`。缓存未命中时先读取代数，再查数据库；Lua 仅在
代数未变化时回填，并为缓存设置 30 分钟 TTL。参数新增、修改、改名、删除和缓存清理统一
用 Lua 先推进代数，再删除相关数据 key。升级前没有 TTL 的缓存首次命中时自动补 TTL。

防重复提交仍使用 `repeat_submit:` 前缀和原有指纹，改为单段 Lua 原子执行“读取最近指纹、
相同则拦截、不同则覆盖并续期”。32 个相同并发判定的定向测试只允许 1 个通过。Redis 故障
仍然放行；不同请求参数仍覆盖最近记录；接口路径、字段、正常响应和错误文案不变。

**为什么**：原参数缓存可能在更新删除缓存之后，把更新前读到的数据库值永久写回；只加
TTL 会缩短错误时间但不能消除竞争。代数使旧快照失去写入资格，TTL 继续覆盖数据库提交后
进程在失效前崩溃等无法形成分布式事务的窗口。原防重复提交把 GET 和 SET 分成两条命令，
相同并发请求能同时读到空值并全部通过；Lua 将判断和记录变成 Redis 内的一个原子步骤。

**受控差异**：P2-12 修复后，相同指纹的真正并发请求从“可能多个成功”收紧为“只允许一个
成功”，这是安全语义修复。Java 同源竞态不作为复制理由。P2-11 不改变业务可见契约。

**禁止回退**：不得把参数缓存恢复为永久值，不得在数据库回源后绕过代数直接 SET；不得
把防重复 Lua 拆回 GET + SET，也不得因 Redis 故障把防护升级为业务硬依赖。内部参数缓存
代数不能出现在缓存列表，也不能被单 key 清理接口删除。

**相关位置**：`pkg/redisx/configcache.go`、`pkg/redisx/repeatsubmit.go`、
`internal/service/config.go`、`internal/service/cache.go`、`internal/middleware/repeatsubmit.go`、
`test/redis_consistency_test.go`。

---

## 2026-08-29 连接池取 20、分页部门改为受限 JOIN，鉴权 Lua 候选不采纳

**当前边界**：MySQL `maxIdleConns` 默认值和部署配置从 10 调为 20，`maxOpenConns` 仍为
50。用户分页 COUNT 继续只查 `sys_user`，分页取数在带 LIMIT 的查询中 LEFT JOIN
`sys_dept`，只加载 Java 列表契约需要的 `dept_id/dept_name/leader`；导出继续使用独立批量
查询补完整部门对象。`/health` 同时暴露 MySQL 与 Redis 客户端池统计，供压测按阶段做差值。

**为什么**：2C2G 同机 A/B 每组 3 轮、每轮 15 秒。混合 2500 RPS 下，空闲连接 20 相对
10 将 P95/P99 从 10.778/24.851ms 降至 8.169/17.584ms，数据库等待从
1020 次/8324ms 降为 0，空闲连接关闭从 2059 降为 118；增加到 30 没有继续改善。分页部门
JOIN 相对批量补查将 1400 RPS 用户列表 P95/P99 从 5.633/13.181ms 降至
4.251/9.347ms，服务 CPU 从 68.81% 降至 58.00%。所有固定速率轮次均为 0 错误、0 丢弃。

**否决的候选**：会话 JSON 和用户 generation 不能普通 MGET，因为第二个 key 依赖 JSON
中的 `userId`。单次 Redis Lua + `cjson.decode` 虽减少命令和池等待，但 3000 RPS getInfo
的 P95/P99 从 3.201/7.637ms 回退到 3.511/8.283ms，因此恢复两次 GET。不能仅凭“网络往返
更少”宣称更快；要重新采用 Lua，必须拿出相同固定速率模型下稳定改善尾延迟的新证据。

**禁止回退**：不得把部门 JOIN 放回 COUNT 或无分页基础查询，不得恢复 DISTINCT；不得仅凭
单接口 0.1ms 级噪声把空闲连接降回 10，也不得未经 A/B 扩大 `maxOpenConns`。调整连接池、
鉴权 Redis 读取或分页部门加载时，必须同时核对 P95/P99、服务与压测端 CPU、MySQL/Redis
池等待、错误和丢弃，不能只看 QPS。

**相关位置**：`configs/application.yml`、`internal/config/config.go`、
`internal/repository/user.go`、`pkg/redisx/redisx.go`、`internal/service/health.go`、
`internal/config/config_test.go`、`internal/repository/user_test.go`、
`test/observability_test.go`、`test/session_test.go`、`test/user_test.go`、`docs/PERF.md`。

---

## 2026-08-30 权限传播、入口顺序与退出排空

**当前边界**：菜单的 `perms` 或 `status` 变化后，查询该菜单关联的角色并刷新其在线用户；
加载或写回最终权限失败时继续沿用安全撤销策略。普通 JSON/form 和 multipart 总大小上限
保持不变，但受保护接口先鉴权、匿名登录/注册先按 IP 限流，之后才完整读取或解析请求体；
不存在的路径不读取请求体。`GET /system/config/configKey/{key}` 保持路径和响应不变，参数
管理员可读任意键，用户管理员只可读取 `sys.user.initPassword`，普通登录用户不再按猜测
键名读取配置值。不存在或已删除账号与真实账号密码错误都执行相同 cost 的 bcrypt。

操作日志池和手工任务池支持停止接收、排空和带 context 等待。退出顺序固定为 HTTP 停止
接入、排空两个异步池、停止 scheduler、最后由既有 defer 关闭 Redis/MySQL。关闭开始后
`Submit` 必须返回 false，不能向已关闭 channel 写入。

会话续期和权限刷新继续比较 generation/revision，但 Go 先生成完整的新会话 JSON，Lua
只校验当前版本后原样 SET。禁止 Lua 对会话执行 `cjson.decode -> cjson.encode`：Redis Lua
会把空数组重编码成 `{}`，角色被撤掉最后一个权限或菜单停用时会生成无法反序列化的会话。
续期发现 revision 冲突时重新读取最新会话后再写，旧权限快照没有写回资格。

**为什么**：Java 菜单修改不会刷新会话，活跃用户可长期保留已经撤销的按钮权限；全局
预读让无 token、被限流和 404 请求在被拒绝前消耗内存或临时磁盘；动态参数键接口会暴露
初始密码并为未来敏感配置留下枚举入口；统一错误文案不能消除 bcrypt 时序差异。原有界
池只解决过载，没有解决正常重启时已接收任务丢失。空数组问题由菜单停用的第二次刷新
集成测试实际复现，不是推断。

**受控差异**：参数按键读取和菜单权限即时生效比 Java 更严格；请求/响应形状、正常成功
值、数据库结构和 Redis 前缀不变。无权限读取按既有普通业务 HTTP 200 + body code 403；
请求体超限仍是已登记的 HTTP 413。

**禁止回退**：不得让菜单权限变化只写数据库；不得把完整请求体读取放回鉴权或匿名限流
之前；不得恢复“任意登录用户按键读取参数”；不得为不存在账号跳过 bcrypt；不得只关闭
channel 而不等待已接收任务。会话 JSON 不得经 Lua 解码后重编码，相关修改必须保留菜单
权限改名、菜单停用、空权限数组、续期冲突、无 token/404 请求体零读取、最小参数权限和
异步池排空/超时测试。

**相关位置**：`internal/service/menu.go`、`internal/repository/menu.go`、
`internal/service/token.go`、`internal/router/router.go`、`internal/middleware/auth.go`、
`internal/service/login.go`、`pkg/asyncx/pool.go`、`cmd/server/main.go`、
`test/menu_test.go`、`test/body_limit_test.go`、`test/config_test.go`、
`pkg/asyncx/pool_test.go`。

---

## 2026-08-30 P1 收口：安全变更先撤会话、树结构防环、菜单按用户刷新、兼容 xls

> 本条取代上一条中“按菜单关联角色逐个刷新”的实现描述；菜单权限即时生效和会话 JSON
> revision/CAS 约束继续有效。

**当前边界**：停用、删除、管理员重置密码、个人修改密码以及导入停用账号，都必须在数据库
写入前先撤销目标用户会话。Redis 撤销失败则拒绝数据库变更；批量撤销不得在首个错误后停止，
必须尝试全部用户并汇总错误。数据库随后失败时允许用户被额外登出，这是安全优先的受控差异。

部门修改必须拒绝把自己挂到任意后代下面；移到虚拟根节点 `parent_id=0` 时将自身
`ancestors` 重置为 `0`，并在同一数据库事务中重算全部后代路径。菜单没有 ancestors，更新前
一次性读取完整父子关系并在内存向上追溯，禁止在循环中查库，也拒绝形成新环。

菜单 `perms/status` 变化时，不再按角色重复扫描全站会话。数据库一次查询得到去重用户 ID，
按新版用户会话反向索引批量 MGET 并刷新；Redis 读取错误向上返回。数据库提交后的刷新使用
保留 trace 值但不继承请求取消信号的 5 秒 context，避免客户端断开后中止已经提交的权限传播。

用户导入同时接受 `.xlsx` 和 `.xls`。`.xlsx` 继续由 Excelize 解析，显式限制总解压 128 MB、
单工作表 XML 32 MB；`.xls` 按 OLE 文件头识别，使用 Apache-2.0 的 `extrame/xls` 只读解析器，
原文件最多 20 MB。两种格式共用同一套表头映射、字段转换、5000 行上限和错误汇总规则。

**为什么**：数据库先成功、Redis 后失败会让停用/删除/改密后的旧会话继续存在；只挡
`parent_id == id` 不能阻止父节点挂到孙节点形成环；按角色刷新会重复处理多角色用户，且请求
取消可能让已提交的菜单变更只刷新一半；Vue 和 Java 明确接受 `.xls`，Go 只读 `.xlsx` 属于
冻结契约缺口。Java 同样存在部分树结构缺陷，不作为复制理由。

**禁止回退**：不得恢复“数据库成功后才撤销安全会话”；不得只检查父节点等于自己；不得把
菜单传播改回角色循环或静默跳过 MGET 错误；不得只根据扩展名判断工作簿格式；不得移除 Excel
解压/文件上限。修改这些流程时必须保留数据库失败前会话已撤销、部门根移动与三级防环、菜单
权限改名/停用、`.xls` 解析和 `.xlsx` 原有转换测试。

**相关位置**：`internal/service/user.go`、`internal/service/profile.go`、
`internal/service/user_import.go`、`internal/service/dept.go`、`internal/service/menu.go`、
`internal/service/token.go`、`internal/repository/menu.go`、`pkg/excelx/import.go`、
`test/user_test.go`、`test/dept_test.go`、`test/menu_test.go`、`pkg/excelx/excelx_test.go`。

---

## 2026-08-30 P2 收口第二批：任务真实生命周期、字典代数、公告已读校验、下载真实路径

**当前边界**：定时任务 context 超时后仍会及时记录失败并让外层调度回调返回，但配置为
禁止并发的任务在任务函数真正退出前一直保留 `running` 标记。调度器用独立 WaitGroup 跟踪
真实任务函数，停止时在总关闭 context 内同时等待 cron 回调和仍在运行的任务函数。

字典缓存沿用既有 `sys_dict:` 前缀，新增不暴露给缓存监控的内部代数 key
`sys_dict:__ruoyi_go_revision__`。缓存未命中时先读取代数，再查数据库；Lua 只在代数未变化且
数据 key 不存在时回填。字典新增、修改、改名、删除和手工缓存清理统一先推进代数再删除缓存。
内部代数不得出现在 key 列表，也不能通过缓存详情读取或被单 key 删除。

单条和批量公告已读只接受当前存在且状态正常的公告。校验和写入在同一事务中完成，并对公告
行使用 `FOR UPDATE`，避免公告在校验后、写入前被并发删除或切回草稿。批量请求只要包含一个
不存在或未发布的 ID 就整体拒绝，不允许部分写入。接口路径、参数位置和正常响应形状不变。

下载先做既有词法路径检查，再解析存储根目录和目标文件的真实路径；只有真实目标仍位于真实
根目录内才返回，并直接使用解析后的路径打开文件。最终文件软链接和中间目录软链接都不能把
下载带出上传目录。托管文件清理继续使用词法安全拼接及自身的逐级链接防护。

**为什么**：Go 无法强行终止忽略 context 的 goroutine，超时返回不等于任务已经结束；提前
释放禁止并发标记会让下一轮与旧任务重叠。字典原来的“查库后普通 SET”允许并发更新删除缓存
后回填旧值。公告已读原来可以预置不存在 ID 或给草稿写孤儿记录。路径字符串位于上传目录内
不代表实际文件也在目录内，软链接可以改变真实归属。

**受控差异**：Java 同样不能强停不响应中断的任务，也没有完整解决这些竞态；Go 在禁止并发、
公告已读和下载边界上更严格。公告无效 ID 从静默成功收紧为既有业务错误响应，这是数据完整性
和安全修复。没有修改数据库结构、索引、HTTP 成功契约或 Redis Key 前缀。

**禁止回退**：不得在 context 超时时立即释放禁止并发标记；不得把字典回填改回普通 SET 或
允许缓存监控操作内部代数；不得把公告校验和写入拆成无锁的两段操作；不得仅凭 Abs/Rel 或
字符串前缀判断下载路径。相关修改必须保留超时后不重叠、字典旧快照拒绝、公告非法批次零写入
以及最终/中间软链接越界测试。

**相关位置**：`internal/service/job_scheduler.go`、`internal/service/dict.go`、
`internal/service/cache.go`、`internal/service/notice.go`、`internal/repository/notice.go`、
`pkg/redisx/dictcache.go`、`pkg/upload/download.go`、`internal/handler/common.go`、
`internal/service/job_scheduler_test.go`、`test/redis_consistency_test.go`、
`test/notice_test.go`、`pkg/upload/download_test.go`。

---

## 2026-08-31 单实例唯一性收口与运行时边界加固

**当前边界**：生产明确为单台服务器、单个 Go 进程。用户、角色、岗位、参数、部门和菜单
各有独立的进程内领域锁，锁覆盖业务唯一性检查到数据库写入；注册、用户导入和个人资料修改
复用用户领域锁。没有修改生产数据库结构、约束或索引。六类并发新增回归测试要求恰好一个
请求成功。

密码长度统一按 Unicode 字符数计算；用户状态只接受 `0/1`，登录仅接受正常状态 `0`；普通
账号不能通过新增、修改或授权接口获得保留角色 ID 1。上传层把扩展名/大小错误标为可公开
错误，文件系统和路径细节只进内部日志。JWT 解析只接受 HS512，分页 OFFSET 在整数溢出时
饱和，缓存列表达到 500 个业务 key 后立即停止后续 SCAN。

启动配置在数据库、Redis、调度器等副作用之前校验端口范围、Gin mode、Redis DB、日志等级
和 CORS 来源。`allowedOrigins: ["*"]` 表示无凭证公开跨域，响应使用 `*` 且不发送
`Allow-Credentials`；通配符不能与具体来源混用。显式来源仍精确回显并允许 credentials。
监听失败和信号退出都经过 HTTP、异步池、调度器的统一关闭流程。导出限流器在未初始化时
使用容量 1 的安全默认值；任务重复注册和源码中非法限流常量仍然 panic，因为它们属于应在
开发/测试期立即暴露的编程错误，不是生产可变配置。

**为什么**：Java 和原 Go 都把 `COUNT` 与写入分开，并发下可能产生重复业务标识；数据库
冻结时，明确的单进程部署允许用最小改动消除本进程内竞争。其余问题会导致非 ASCII 密码
入口行为不一致、内部路径泄露、异常状态账号登录、保留角色提权、整数回绕、JWT 算法降级、
启动后残留后台组件、无效 Redis 全库扫描和危险 CORS 组合，均不应以 Java 同源缺陷为理由
保留。

**多实例门槛**：进程内锁不是分布式锁。只要计划启动第二个 Go 进程、使用多副本或滚动
发布，业务唯一性 P2 必须重新打开；上线前先改成数据库唯一约束或可靠跨进程锁，并重跑并发
写入与故障恢复测试。定时任务也必须增加分布式抢占。不得把本条的单实例结论外推到多实例。

**禁止回退**：不得让唯一性检查和写入脱离同一领域锁；不得恢复未知用户状态可登录、普通
账号可提交角色 1、任意 HMAC 可解析、上传内部错误直出、CORS 通配符带 credentials 或缓存
列表“停止收集但继续扫描”。不得因为正常路径测试通过而删除监听失败关闭测试和配置边界测试。

**相关位置**：`internal/service/write_locks.go`、`internal/service/user.go`、
`internal/service/register.go`、`internal/service/user_import.go`、`internal/service/profile.go`、
`internal/service/role.go`、`internal/service/post.go`、`internal/service/config.go`、
`internal/service/dept.go`、`internal/service/menu.go`、`internal/config/config.go`、
`internal/middleware/cors.go`、`cmd/server/main.go`、`pkg/jwtx/jwtx.go`、`pkg/page/page.go`、
`pkg/redisx/redisx.go`、`pkg/upload/upload.go`、`test/uniqueness_concurrency_test.go`。

---
