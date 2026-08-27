# 运行时审计（并发 / 会话 / 调度）

2026-08-26。前几轮审计集中在**接口契约和分页**（见
[API_CONTRACT_AUDIT.md](./API_CONTRACT_AUDIT.md)、[DECISIONS.md](./DECISIONS.md)），
会话、事务、并发这条线基本没碰过。这份是补上那一块。

范围：`internal/service` 全量 + `internal/middleware` 关键调用面 +
`pkg/page` / `pkg/upload`，并核对用户导入结果对应的 Vue 前端渲染方式。
方法是静态读代码、调用链复核和定点验证；常规回归测试不等于并发问题已被覆盖。

**状态**：截至 2026-08-26，代码中的问题均**未修复**。本文件是复核快照；
是否修复以代码和测试为准，不用这里的状态代替回归验证。

判定口径：

- **确认**：从当前代码可以直接构造出错误路径
- **条件成立**：代码具备风险，但是否能触发取决于数据库或部署配置
- **记录项**：确有资源或维护成本，但不属于接口契约错误

---

## 结论速览

| 级别 | 问题 | 位置 |
|---|---|---|
| P1 | 密码错误锁定可绕过（非原子计数） | `service/login.go:130` |
| P1（条件成立） | 密码错误锁定可绕过（用户名大小写） | `service/login.go:56` |
| P2 | 用户导入结果存在反射型 XSS | `service/user_import.go:74` |
| P2 | 批量授权 = N 遍全量 Redis 扫描 | `service/role.go:250` |
| P2 | 在线用户列表翻页会重复/漏行 | `service/online.go:68` |
| P2 | 权限刷新之间互相覆盖，撤权被回滚 | `service/token.go:122` |
| P2 | 非 multipart 请求体没有全局大小限制 | `middleware/operlog.go:92` |
| P2 | 验证码 GET+DEL 非原子 | `service/captcha.go:74` |
| P2 | 调度器 add/remove 竞态，任务可能跑两遍 | `service/job_scheduler.go:117` |
| P2 | 操作日志脱敏可被大小写绕过，明文密码落库 | `middleware/operlog.go:36` |
| P3 | `truncateRunes` 是 O(n²) | `service/job_scheduler.go:245` |
| P4（记录项） | 头像旧文件不会清理 | `service/profile.go:142` |

**已确认没问题的**（这轮看过，不用再查）：事务边界（用户 / 角色 / 部门的多表写入
都在 `repository.Transaction` 里）、`orderByColumn` 白名单、上传路径
`upload.SafeJoin` 防目录穿越、`datascope` 的 `Sprintf` 只拼代码里的列名常量。

---

## P1-1 密码错误计数非原子，并发可完全绕过锁定

**位置**：`internal/service/login.go:57`（读）、`:130` `recordPasswordFailure`（写）

**现在的写法**是 `GET` → 内存 `+1` → `SET`：

```go
count, err := currentRetryCount(ctx, retryKey)   // GET
...
next := current + 1
redisx.C().Set(ctx, key, next, passwordLockTime) // SET
```

**问题**：三步之间没有原子性。同时打 10 个错密码请求，10 个 goroutine 全部读到
`count = 0`，全部写 `1`。**实际只记了 1 次**，`maxPasswordRetry = 5` 形同虚设。

撞库脚本本来就是并发发的，这个洞对真实攻击场景恰好完全敞开。

**为什么这条不能用"Java 也这样"糊弄过去**：`SysLoginService` 确实是同样的写法，
但项目里已经因为**一模一样的理由**做过一次加固 ——
DECISIONS 2026-08-23「限流用 Lua，不用 INCR + EXPIRE 两条命令」。
`pkg/redisx/ratelimit.go` 就是现成的参考实现，这里只是漏了。

**怎么改**：先决定窗口语义，再用一段 Lua 原子执行，不能拆成两条 Redis 命令：

- 保持当前语义：`INCR` 后**每次** `EXPIRE`，每次失败都重新计算锁定窗口
- 改成固定窗口：`INCR` 后仅在首次计数时 `EXPIRE`

当前代码每次都用 `SET(..., passwordLockTime)` 重置 TTL，属于前一种滑动窗口。
如果采用“首次才设置过期时间”，那是行为变更，不应写成“保持现有语义”。
两种方案都必须放在同一段 Lua 内，避免自增成功、设置过期时间失败后留下永不过期的 key。

**回归怎么写**：并发起 N 个错密码请求，断言最终 `pwd_err_cnt:<user>` 的值等于 N。
现有测试全是串行的，测不出来。

---

## P1-2 用户名大小写不同 → 各记一份错误计数

**位置**：`internal/service/login.go:56`

```go
retryKey := redisx.PwdErrCntKey(body.Username)   // 用的是前端传来的原文
```

而登录查库走的是 `WHERE user_name = ?`，`sys_user.user_name` 在
`ry_20260417.sql` 里**没有写 `COLLATE`**，会继承数据库 / 表的排序规则。
很多 MySQL 5.7、8.0 部署默认使用不区分大小写的 `*_ci` 排序规则，
但不能只根据 MySQL 版本断定当前库一定不区分大小写。

于是：`admin` / `Admin` / `ADMIN` / `aDmIn` 登进的是**同一个账号**，
但 Redis key 各不相同、**各自独立计 5 次**。
5 个字符的账号名有 2⁵ = 32 种大小写组合 → 锁定前有 160 次尝试机会。

**判定：条件成立，待当前库实测。** DDL 只能证明这里依赖外部默认值，
不能证明实际继承到的排序规则。
按 CLAUDE.md 的规矩，动手前先跑一遍：

```sql
SELECT COLLATION_NAME FROM information_schema.COLUMNS
 WHERE TABLE_SCHEMA='ry-vue' AND TABLE_NAME='sys_user' AND COLUMN_NAME='user_name';

SELECT COUNT(*) FROM sys_user WHERE user_name = 'ADMIN';   -- 命中 1 行即证实
```

第二条返回 1 就说明确实不区分大小写，这个洞成立；返回 0 则本条作废。

**怎么改**：查库成功后，优先使用数据库返回的规范用户名生成计数 key；
用户不存在时再使用经过明确规则规范化的输入。只做 `strings.ToLower` 虽然能处理
ASCII 大小写，却不一定与数据库 collation 对重音、全角字符等行为完全一致。
无论采用哪种规则，都应集中在一个函数里，避免查询、计数和解锁使用不同规则。

**顺带**：`test/main_test.go` 里清理 `pwd_err_cnt:zz_test_*` 的前缀匹配不受影响
（前缀本来就是小写）。

---

## P2-1 用户导入结果存在反射型 XSS

**位置**：`internal/service/user_import.go:74-84`；前端
`RuoYi-Vue3-master/src/components/ExcelImportDialog/index.vue:122`

导入服务把 Excel 中的原始 `user.UserName` 直接拼进带 `<br/>` 的结果字符串：

```go
fmt.Fprintf(&failureMsg, "<br/>%d、账号 %s 导入失败：%s", ..., user.UserName, ...)
fmt.Fprintf(&successMsg, "<br/>%d、账号 %s 导入成功", ..., user.UserName)
```

前端再用 `dangerouslyUseHTMLString: true` 显示 `response.msg`。虽然账号字段有 `xss`
校验，但恶意 HTML 正因为校验失败会进入第一条“导入失败”分支，随后原样回显；
校验本身不能保护错误消息的 HTML 上下文。管理员导入攻击者提供的 Excel 时，
其中的标签或事件属性可能在管理员浏览器中执行。

**怎么改**：不要拼 HTML。后端返回结构化的成功 / 失败明细，前端用普通文本节点和列表
渲染；如果暂时必须维持 Java 版字符串契约，至少对所有来自 Excel 和错误对象的动态内容
做 HTML 转义，只保留服务端自己写入的 `<br/>`。需要覆盖“非法账号进入失败消息”的回归用例。

---

## P2-2 批量授权 / 取消授权 = N 遍全量 Redis 扫描

**位置**：`internal/service/role.go:250`

```go
func refreshUsers(ctx context.Context, userIDs []int64) error {
	for _, userID := range userIDs {
		if err := RefreshOnlineUserByID(ctx, userID); err != nil {
			return err
		}
	}
	return nil
}
```

`RefreshOnlineUserByID` 内部是 `scanLoginUsers` —— **扫描整个 `login_tokens:*`
空间**，只为找出属于某一个用户的会话。放进循环就是 N 遍全量扫描。

给 100 个用户批量授角色：100 遍全量 SCAN + 每人一次 `SelectUserByID` +
一次 `GetMenuPermission`。在线会话上千时，一次批量授权能把 Redis 压很久。

违反的是 CLAUDE.md 硬约束 6「循环里禁止查库」—— 只是这次查的是 Redis 不是 MySQL，
上一轮审计按 SQL 找 N+1，正好漏在这里。

**怎么改**：加 `refreshOnlineUsersByIDs(ctx, ids []int64)`，
把 ids 装进 `map[int64]struct{}`，**一次 SCAN** 里判断命中，
per-user 的用户和权限做快照缓存。

隔壁的 `RefreshOnlineUsersByRole`（`token.go:122`）已经是这个结构了 ——
`snapshots map[int64]permissionSnapshot` 那段照抄即可，
把 `hasRole(...)` 换成 `ids` 的集合判断。

**顺带一个设计问题（不急）**：`RefreshOnlineUserByID` 被
`profile.go` 的改密码、改资料、**改头像**都调用了。改个头像就全量扫一遍 Redis
成本偏高，但刷新并非没有价值：它能让当前会话立即拿到新头像和资料。
Java 版的 `updateUser` 不刷新在线会话，只有角色变更才刷；Go 版是额外保证了
会话资料及时一致。要保留这项能力，应考虑建立
`login_tokens_by_user:<userId>` 的反向索引；那是另一个决定，先记在这。

---

## P2-3 在线用户列表内存分页行序不稳定

**位置**：`internal/service/online.go:68`

```go
sort.Slice(all, func(i, j int) bool {
	return all[i].LoginTime.Std().After(all[j].LoginTime.Std())
})
```

两个条件叠在一起：

1. `LoginTime` 存在并列的可能，比较函数没有唯一兜底
2. `scanLoginUsers` 的 SCAN 输入顺序本身不稳定

后面紧跟着 `all[start:end]` 做内存分页 —— 于是翻页可能**重复某行或漏掉某行**。

这和 DECISIONS 2026-08-24「分页必须有唯一兜底列」是**同一个 bug**，
只是那轮只扫了 SQL 分页（13 处），内存分页这条路没进视野。

**注意 `LoginTime` 每次续期都会被重置**（`RefreshToken` 里
`loginUser.LoginTime = now.UnixMilli()`，与 Java 的
`TokenService.refreshToken` 一致）—— 也就是说列表里活跃用户的登录时间会
不断跳到"刚刚"，**并列的概率比看上去高得多**：一批人同时在线操作，
毫秒级撞上很常见。

**怎么改**：在比较函数里补唯一的 `TokenID` 兜底：

```go
sort.Slice(all, func(i, j int) bool {
	ti, tj := all[i].LoginTime.Std(), all[j].LoginTime.Std()
	if !ti.Equal(tj) {
		return ti.After(tj)
	}
	return all[i].TokenID < all[j].TokenID
})
```

有唯一兜底后比较关系已经是全序，普通 `sort.Slice` 足够；`sort.SliceStable`
只能保留本次输入中相等元素的相对顺序，无法修复每次都可能变化的 SCAN 输入顺序，
因此它不是必要条件。

---

## P2-4 权限刷新之间互相覆盖，撤权被回滚

**根因**：`login_tokens:<uuid>` 的每一次写入都是**读出整个 `LoginUser` → 改几个字段
→ 整体 `SET` 回去**（`token.go:53` `RefreshToken`）。这是典型的 read-modify-write，
而全项目有三条路径都在这么干，彼此没有任何协调。

### 主路径：权限刷新之间互相覆盖（窗口最宽）

**位置**：`internal/service/token.go:122` `RefreshOnlineUsersByRole`、
`:165` `RefreshOnlineUserByID`

```
SCAN 一批 100 个 key -> MGET 取回 -> 逐个 SelectUserByID + GetMenuPermission -> 逐个 SET
                                     ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
                                     中间隔着 N 次数据库往返
```

MGET 拿到的是**批次开始那一刻**的快照，但 `SET` 发生在 N 次查库之后。
两个管理员并发改不同角色时：

1. 管理员 A 改角色甲 → MGET 拿到用户 X 的会话快照
2. 管理员 B 改角色乙 → 全流程跑完，把 X 的新权限写进 Redis
3. 管理员 A 查完库，拿**步骤 1 的陈旧快照**为底覆盖写回 → B 的撤权被抹掉

窗口是**毫秒到秒级**（取决于批次内的用户数和查库耗时），不是需要碰运气的微秒竞态。
`refreshUsers` 的循环（见 P2-2）会把这个窗口重复 N 遍，两个问题是叠加的。

### 次要路径：鉴权续期覆盖刚写入的权限

**位置**：`internal/middleware/auth.go:33-45`

`GetLoginUser` 读出完整会话，紧接着 `VerifyToken` → `RefreshToken` 整体写回。
如果撤权的 `SET` 恰好落在这两步之间，旧权限会被重新保存一整个会话周期。

**但这条窗口是微秒级**，两行代码之间没有 `c.Next()`、没有查库、没有 IO。
它是真实存在的路径，不该作为本条的主要论据。

**顺带纠正一个容易算错的地方**：`expireTime: 30m` / `refreshWindow: 20m` 意味着
续期后 `remaining` 回到 30 分钟，要再过 10 分钟才会低于 20 分钟阈值 ——
**每个会话约每 10 分钟才整体重写一次，不是每个请求都写**。
按"每请求一次写"去估 Redis 写放大会高估一个数量级。

### 为什么测不出来

三条路径发出的 Redis 命令单独看全都合法，Race Detector 也发现不了 ——
它检测的是进程内共享内存的数据竞争，而这里的共享状态在 Redis 里。

**怎么改**（按代价从低到高）：

1. **先修 P2-2**。批量刷新改成一次 SCAN 后，`refreshUsers` 那 N 遍重复窗口直接消失，
   剩下的暴露面小得多。
2. **续期不再整体覆盖**。`VerifyToken` 的语义只是"延长有效期"，
   用 `EXPIRE` 单独续 TTL 即可，不需要把整个对象写回去。
   代价是 `ExpireTime` 字段和 Redis TTL 会脱钩，得决定以哪个为准（建议以 TTL 为准）。
3. **权限刷新加乐观并发控制**。会话里加修订号，用 Lua 做「比较修订号后再写」，
   发现变了就重新加载再算一遍。这条最彻底但改动最大，等前两条做完再评估还需不需要。

**回归怎么写**：固定「MGET 快照 → 另一路写入新权限 → 第一路 SET」的顺序
（用可注入的钩子或直接在 service 层拆出可测函数），断言最终权限是后写入的那一份。
纯并发压测复现不了，必须把顺序钉死。

---

## P2-5 非 multipart 请求体没有全局大小限制

**位置**：`internal/middleware/repeatsubmit.go:109`、`internal/middleware/operlog.go:92`

两个中间件都会对非 multipart 请求执行无上限 `io.ReadAll`，然后把完整内容重新放回
`Request.Body`。`http.Server.ReadTimeout` 只限制读取时间，`gin.Engine.MaxMultipartMemory`
只控制 multipart 解析的内存阈值，都不能限制普通 JSON / form 请求的总字节数。
拥有任一相关写接口权限的账号可以提交超大请求体，造成单请求多份内存副本；并发提交时
可能触发明显的 GC 压力甚至进程 OOM。

**怎么改**：在所有读取请求体的中间件之前统一使用 `http.MaxBytesReader`，超限明确返回
HTTP 413。普通 JSON 与文件上传应分别配置上限；`OperLog` 只需要保留截断后的日志内容，
但不能用截断读取后再把残缺请求体交给 handler。需要补单请求超限和并发大请求测试。

---

## P2-6 调度器 add / remove 竞态，任务可能跑两遍

**位置**：`internal/service/job_scheduler.go:117-127`、`:143` `reschedule`

```go
entryID, err := s.cron.AddFunc(translated, func() { execute(&snapshot) })
...
s.mu.Lock()
s.entries[target.JobID] = entryID     // 注册在锁外，登记在锁内
s.mu.Unlock()
```

`reschedule` 是 `remove` + `add` 两步，整体不原子。两个人同时改同一个任务时：

- 两次 `AddFunc` 都成功 → cron 里有**两个** entry
- `s.entries[jobID]` 只留得下后一个 → 前一个**永远删不掉**
- 表现：这个任务从此每次触发跑两遍，改状态为暂停也停不掉其中一个

要清掉只能重启进程，而且现象是"任务莫名其妙跑了两次"，
排查时基本不会往并发上想。

**怎么改**：用独立的调度变更锁把同一任务的 `add` / `remove` / `reschedule`
串行化，并让 `AddFunc` / `Remove` 与 `entries` 更新处于同一个临界区。
最好不要复用 `beginRun` / `endRun` 的运行状态锁，避免把“修改调度”和“任务执行互斥”
耦合到一起。测试需要并发修改同一个 jobID 后检查 cron 中只有一个 entry，
不能只依赖 Race Detector：这是业务状态竞态，底层对象各自线程安全时不会报告 data race。

---

## P2-7 验证码 GET + DEL 非原子

**位置**：`internal/service/captcha.go:74-76`

```go
answer, err := redisx.C().Get(ctx, key).Result()
redisx.C().Del(ctx, key)
```

注释写的是"取到就删，防止同一个验证码被重复使用"，意图对；
但两条命令之间，另一个请求可以读到同一个值 —— **同一个验证码能用两次**。

攻击者仍需知道正确答案，但同一验证码可被两个并发登录请求消费；它又与密码失败计数
竞态叠加，削弱的是登录入口的组合防护，因此按 P2 处理。

**怎么改**：Redis 6.2 起有 `GETDEL`，go-redis v9 对应
`redisx.C().GetDel(ctx, key)`。如果要兼容 Redis 6.0 或更低版本，使用 Lua 原子取值并删除。
删除错误不能继续忽略。

**注意**：`cmd/contractcheck` 和 `test/` 都会从 Redis 直接读验证码答案
（`redisx.CaptchaKey(uuid)`）来构造登录请求，那是在**调用登录之前**读的，
不受影响。

---

## P2-8 操作日志脱敏可被大小写绕过，明文密码落库

**位置**：`internal/middleware/operlog.go:36-37`

```go
sensitiveJSON  = regexp.MustCompile(`"(password|oldPassword|newPassword|confirmPassword)"\s*:\s*"[^"]*"`)
sensitiveQuery = regexp.MustCompile(`(?i)\b(password|oldPassword|newPassword|confirmPassword)=[^&\s]*`)
```

**注意两行的差别：查询串那条有 `(?i)`，JSON 这条没有。**

而 **Go 的 `encoding/json` 反序列化时字段名匹配是不区分大小写的**
（优先精确匹配，匹配不上再退回大小写不敏感匹配）。所以 `{"Password":"secret"}`：

- 能正常绑定到 `Password` 字段，请求**成功执行**、业务照常完成
- 完全绕过 `sensitiveJSON`，明文原样写进 `sys_oper_log.oper_param`

**这不是失败路径的问题，是成功路径的问题。** 受影响的三条路由都挂了 `OperLog`
且密码走 JSON body（`internal/router/router.go:297-311`）：

| 路由 | 字段 |
|---|---|
| `PUT /system/user/profile/updatePwd` | `oldPassword` / `newPassword` |
| `PUT /system/user/resetPwd` | `password` |
| `POST /system/user` | `password` |

`sys_oper_log` 在前端可查询、可导出 —— 直接违反 CLAUDE.md 硬约束 7
「密码禁止出现在任何日志里」。**因此定级 P2，不是 P3。**

**其余几条绕过路径**（同一个根因，一并修）：

- 非字符串值：`{"password":123456}` 不匹配 `"[^"]*"`，不脱敏
- 值里带转义引号：`{"password":"a\"b"}` 只被替换掉前半段
- 查询串把字段名百分号编码：`%70assword=...`，正则在解码前就跑了
- 将来新增密钥类字段但忘了同步正则名单

**怎么改**：不要在原始文本上跑正则。

- JSON：`json.Unmarshal` 成 `any` 后递归遍历，键名 `strings.EqualFold` 比对敏感清单，
  命中就整个值替换掉（不管它是字符串、数字还是对象），再 `Marshal` 回去
- 查询串：`url.ParseQuery` 解码后按键替换，再重新编码
- 两者都解析失败时走保守策略 —— 宁可整段记成 `[无法解析，已省略]`，也不要原样落库

敏感键清单收在一个变量里集中维护。测试要覆盖：大小写变体、数字值、转义引号、
嵌套对象、数组、百分号编码，以及"解析失败时不落原文"。

---

## P3 `truncateRunes` 是 O(n²)

**位置**：`internal/service/job_scheduler.go:245`

```go
runes := []rune(s)
for len(string(runes)) > max {
	runes = runes[:len(runes)-1]
}
```

每轮循环都 `string(runes)` **重新分配整个字符串**再量长度。
输入 1 MB 的错误信息（比如任务把 HTTP 响应体包进了 error），
循环要跑约 100 万轮、每轮分配上百 KB —— 接近卡死，而且是在
“任务已经失败了、正要写失败原因”的路径上。

当前内置任务未必会产生 1 MB 错误文本，因此这是明确的复杂度缺陷，
不是已经观察到的线上故障；P3 保持不变。

**怎么改**：正向遍历累计字节数，超过 `max` 就在那里切，一次完成。

**另外**：注释说 `sys_job_log.exception_info` 是 `varchar(2000)`，
但 `len(s)` 量的是**字节**、varchar(2000) 是 **2000 字符**。
按字节截偏保守，不会写不进去，但注释和实现的口径对不上，应一并说明。

---

## P4 头像旧文件永不删除（记录，不改）

**位置**：`internal/service/profile.go:142` `UpdateAvatar`

只更新 `sys_user.avatar` 的路径，磁盘上的旧头像文件留在原地。
用户反复换头像，`uploadPath` 只增不减。

**与 Java 版行为一致**（`SysProfileController.avatar` 同样不删旧文件），
所以不算契约偏离。真要清理，方向是加一个定时任务扫孤儿文件 ——
在 `UpdateAvatar` 里直接删有风险：路径来自数据库，删之前得确认它确实落在
`uploadPath` 之下（`upload.SafeJoin` 那套），而且改头像失败回滚时旧文件已经没了。

先记在这，不动。

---

## 修改建议的优先级

1. **立即处理**：P1-1、P2-8 —— 两条都是明文密码在成功路径上外泄，
   直接违反硬约束 7；P1-2 先实测当前库的 collation，成立后并入这一批
2. **高优先级**：P2-1、P2-4、P2-6、P2-7 涉及安全、权限或并发正确性；P2-2、P2-3、P2-5
   涉及可用性和稳定性
3. **正常排期**：P3
4. **暂不改**：P4，只记录并考虑后续孤儿文件清理任务

**有一处顺序依赖**：P2-4 的暴露面被 P2-2 放大了 N 倍（`refreshUsers` 把宽窗口
重复 N 遍）。**先修 P2-2 再评估 P2-4 要做到哪一步** —— 很可能修完前者，
后者只需要做"续期改成只 `EXPIRE`"这一档，用不着上乐观锁。

改完需要补的验证：

- 并发错密码的回归用例（现有测试全串行，测不出 P1-1）
- 用户名不同大小写共用计数，以及与当前库 collation 一致的集成测试
- `{"Password":"..."}`（首字母大写）能成功绑定、且 `sys_oper_log.oper_param`
  里查不到明文；再覆盖数字值、转义引号、嵌套、URL 编码
- 恶意账号即使校验失败，导入结果也只能显示文本，不能形成 HTML 节点
- 批量授权后断言 Redis SCAN 次数不随 userIDs 数量线性增长
- 固定「MGET 快照 → 另一路写入新权限 → 第一路 SET」的顺序，断言最终权限是后写入的那份
- 相同 `LoginTime` 且 SCAN 输入顺序变化时，分页结果仍保持一致
- 并发重排同一个任务后，cron 中只能存在一个对应 entry
- 同一验证码并发消费只能有一个请求取得值
- 普通请求体超限返回 413
- `truncateRunes` 使用表格测试覆盖 ASCII、多字节字符、边界值和长输入
- 最后运行 `.\scripts\test.ps1` 和 `.\scripts\test.ps1 -Race`。Race Detector 只能补充发现
  数据竞争，不能替代会话覆盖和调度器状态竞态的定向行为测试 ——
  P2-4 和 P2-6 的共享状态分别在 Redis 和 cron 内部，都不是进程内的 data race

现有完整回归和 Race Detector 即使全部通过，也不能反证上述问题不存在：
当前用例没有制造 Redis 命令交错、同时间排序、超大请求体或危险 HTML 回显等触发条件。
