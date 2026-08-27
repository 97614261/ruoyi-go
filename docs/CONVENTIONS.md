# 接口与代码规范

只写**稳定的规则**。带日期的历史决议放 [DECISIONS.md](./DECISIONS.md)。

所有契约均已对照 `../RuoYi-Vue-master` 源码核实，行为存疑时以 Java 代码为准。

---

## 一、响应格式

RuoYi 的 `AjaxResult` 继承自 `HashMap`，所以**额外字段是平铺在顶层的，不在 `data` 里**。这是最容易写错的地方。

### 1. 普通响应（对应 `AjaxResult`）

```json
{ "code": 200, "msg": "操作成功", "data": {} }
```

- `data` 为 nil 时**整个键不出现**，不是 `"data": null`
- 失败：`{ "code": 500, "msg": "具体错误" }`

### 2. 分页响应（对应 `TableDataInfo`）

```json
{ "code": 200, "msg": "查询成功", "total": 100, "rows": [] }
```

`total` 和 `rows` **平铺在顶层，没有 `data` 包裹**。

### 3. 平铺扩展字段（全量清单）

很多接口在 `code`/`msg` 之外**直接挂顶层字段**。下表是全仓库扫描 `ajax.put(...)` 得到的完整清单，实现时必须逐个比对：

| 接口 | `data` | 平铺字段 |
|---|---|---|
| `POST /login` | 无 | `token` |
| `GET /getInfo` | 无 | `user`、`roles`、`permissions`、`pwdChrtype`、`isDefaultModifyPwd`、`isPasswordExpired` |
| `GET /getRouters` | 路由树 | — |
| `GET /captchaImage` | 无 | `captchaEnabled`、`uuid`、`img` |
| `GET /system/role/deptTree/{roleId}` | 无 | `checkedKeys`、`depts` |
| `GET /system/menu/roleMenuTreeselect/{roleId}` | 无 | `checkedKeys`、`menus` |
| `GET /system/user/authRole/{userId}` | 无 | `user`、`roles` |
| `POST /system/user/profile/avatar` | 无 | `imgUrl` |
| `POST /common/upload` | 无 | `url`、`fileName`、`newFileName`、`originalFilename` |
| `POST /common/uploads` | 无 | `urls`、`fileNames`、`newFileNames`、`originalFilenames` |
| **`GET /system/user/{userId}`** | **有**（SysUser） | `postIds`、`roleIds`、`roles`、`posts` |
| **`GET /system/user/profile`** | **有**（SysUser） | `roleGroup`、`postGroup` |
| **`GET /system/notice/listTop`** | **有**（公告列表） | `unreadCount` |

**最后三行是混合形态**：既有 `data`，又有平铺的兄弟字段。这类最容易写错，实现时格外注意。

几个特例：

- `captchaEnabled` 为 false 时，`uuid` 和 `img` 都不返回
- `GET /system/user/` （不带 userId，用于"新增用户"弹窗）只返回 `roles` 和 `posts`，没有 `data`、`postIds`、`roleIds`
- 树选择类接口的节点结构是 `{id, label, disabled, children}`，`children` 为空时**该键不出现**（不是空数组）

### 4. Go 侧写法

平铺字段一律用 `response.New(...).Put(...)`，**禁止为了"结构清晰"包进 data**：

```go
// 对 GET /system/role/deptTree/{roleId}
response.New(response.CodeSuccess, response.MsgSuccess).
    Put("checkedKeys", checkedKeys).
    Put("depts", deptTree).
    JSON(c)

// 混合形态：data 和平铺字段并存
response.New(response.CodeSuccess, response.MsgSuccess).
    Put("data", user).          // 注意 data 也是 Put 进去的
    Put("postIds", postIDs).
    Put("roleIds", roleIDs).
    JSON(c)
```

判断依据只有一条：**打开 Java 版对应的 Controller 看它 `put` 了什么**，不要凭直觉设计。

### 5. 文件下载接口（导出/模板）

前端 `download()` 的行为决定了三条硬约束：

**① 查询条件在请求体里，不是 query string**

`download()` 用 `POST` + `Content-Type: application/x-www-form-urlencoded`。
必须用 `c.ShouldBind`（按 Content-Type 自动选表单绑定），
用 `ShouldBindQuery` 会拿到空条件、**静默导出全表**。

**② 错误响应必须用 `response.FailDownload`**

前端靠 `blobValidate()` 区分文件和错误，而它是**严格相等**比较：

```js
return data.type !== 'application/json'
```

gin 的 `c.JSON` 写的是 `application/json; charset=utf-8`，比较不相等 →
前端当成文件 → `saveAs` 存下一个内容是 JSON 的 `.xlsx`。
用户拿到打不开的文件，且**看不到任何错误提示**。
（Spring 返回的是不带 charset 的 `application/json`，所以 Java 版没这个坑。）

handler 里对应用 `failDownload(c, err)` 而不是 `fail(c, err)`。

**③ 文件必须先在内存里构建完再写响应**

边生成边写的话，中途出错时响应头已经发出，没法再改成 JSON 错误。
`excelx.WriteResponse` 保证出错时一个字节都不写。

**④ 有上限就要报错，绝不静默截断**

`service.MaxExportRows = 100000`、`service.MaxConcurrentExports = 2`。

超限一律报错。**截断后照常返回文件是最坏的做法** —— 用户拿到一份看起来正常、
实际少了数据的表格，而且不会发现。宁可让他导不出来，也不能给错的数据。

提示语必须说清三件事：查出来多少条、上限多少、下一步怎么办。
只说"导出失败"等于让用户去猜。

**⑤ 上限由内存决定，不是由 xlsx 格式决定**

⚠️ **这一节的结论被实测推翻过一次，改之前先看数字。**

`pkg/excelx` 的基准（10 列，贴近 SysUser）：

| 行数 | 堆峰值 | 分配总量 | 分配次数 | 耗时 |
|---|---|---|---|---|
| 1 万 | ~9 MB | 50 MB | 98 万 | 80 ms |
| 10 万 | ~90 MB | 321 MB | 980 万 | 796 ms |

完全线性：约 **900 字节/行常驻**、**每行 98 次分配**。

- xlsx 格式上限是 1048575 行 → 外推 **约 940 MB 堆峰值**，
  而整个服务压测时常驻才 65 MB。一次导出内存翻十几倍，两三个人同时点就 OOM，
  正好抵消掉换 Go 省下来的内存。所以上限取 **10 万行 ≈ 90 MB**。
- 只限行数不限并发，行数上限就是摆设（10 人各导 10 万行 = 900 MB），
  所以还要 `middleware.ExportLimit`。

**曾经写在这里的错误结论**：「StreamWriter 只保留少量行 → 瓶颈在数据切片而非工作簿」。
实测数据切片只占堆峰值的 **16%**，另外 84% 是导出过程本身。
StreamWriter 确实在流式写，它只是 churn 很凶，堆峰值仍随行数线性增长。
**改成分批查库只能省下那 16%，是白费力气。**

重新验证：`go test ./pkg/excelx/ -bench=Export -benchmem -run=^$`
以及 `go test ./pkg/excelx/ -run=ReportExportMemory -v`。

**⑥ 导出用流式写入**

`excelx` 内部走 excelize 的 `StreamWriter`，对齐 Java 版的 `SXSSFWorkbook`。
代价是只能顺序写、不能回头改单元格。
注意它降低的是**峰值增长的斜率**，不是把内存变成常数（见上一条的实测）。

### 6. Java 响应契约转换层（`internal/handler/java_contract.go`）

有一批接口的响应**不是直接序列化 Go 模型**，而是先过一层逐字段 map 转换，
复刻 Java 那边的三类可观察差异：

| 差异 | 来源 |
|---|---|
| 某些字段固定为 `null` | Java 的 MyBatis `selectXxxVo` 根本没 select 那一列，Jackson 就输出 `null`；Go 从数据库把整行读回来了，直接序列化会输出真实值 |
| `children` / `roles` 固定为 `[]` | Java 实体里是 `new ArrayList<>()`，永远不是 `null` |
| `params: {"@type":"java.util.HashMap"}` | 登录会话和字典缓存是 Fastjson 反序列化出来的，空 HashMap 带类型标记 |

#### 受影响的接口（改这些模型时必须同步检查转换）

| 转换函数 | 用在哪 | 抹成 null / 固定值的字段 |
|---|---|---|
| `contractRole` / `contractRoles` | 角色列表、角色下拉、用户表单里的候选角色、**角色详情** | `createBy`、`updateBy`、`updateTime` |
| `contractPost` / `contractPosts` | 岗位列表、岗位下拉、用户表单里的候选岗位、**岗位详情** | `updateBy`、`updateTime` |
| `contractDept` / `contractDeptList` | 部门列表、排除子部门列表 | `parentName`、`updateBy`、`updateTime`、`remark`；`children` = `[]` |
| **`contractDeptDetail`** | **部门详情（选列与列表不同，见下）** | `createBy`、`createTime`、`delFlag`、`updateBy`、`updateTime`、`remark`；`children` = `[]`；**`parentName` 有值时输出真实值、无父级时输出 `null`** |
| `contractMenu` / `contractMenuList` | 菜单列表、**菜单详情** | `parentName`、`createBy`、`updateBy`、`updateTime`、`remark`；`children` = `[]`；`perms` 为 nil 时输出 `""` |
| `contractDictType` / `contractDictTypes` | 字典类型列表、字典类型下拉、**字典类型详情** | `updateBy`、`updateTime` |
| `contractDictDatum` / `contractDictData` | 字典数据列表、**字典数据详情** | `updateBy`、`updateTime` |
| `contractCachedDictData` | **按类型查字典**（走 Redis 缓存） | 同上，另加 `params` 类型标记；`cssClass` 空串输出 `null` |
| `contractUserList` | 用户列表 | `pwdUpdateDate`、`updateBy`、`updateTime`；`roles` = `[]`；删掉 `userType` |
| `contractUser(user, true)` | 用户详情 | 删掉 `userType`；`roleId` = `null`；填充 `roles` 和 `dept` |
| `contractLoginUser` | **`/getInfo` 的 `user`** | 在 `contractUser` 基础上加 `params` 标记，空 `avatar` / `updateBy` 输出 `null` |
| `contractEmbeddedRole` | 用户对象里内嵌的角色 | `SysUserMapper.RoleResult` 只填 6 个业务字段，其余抹掉 |
| `contractUserDept` | 用户对象里内嵌的部门 | 非 full 时只保留 `deptId`/`deptName`/`leader` |
| `contractNotices` | 公告管理列表 | 无字段改写，仅统一出口 |

#### 详情接口容易被漏掉

这层转换最初只铺了列表和下拉，**6 个详情接口全漏了** ——
岗位、字典类型、字典数据、角色、菜单、部门。表现是新增后 `updateBy` 是 `""`、
修改后是 `"admin"`，而 Java 恒为 `null`。

不带转换的详情只剩两个，因为它们的 `selectVo` 是**全选**的：

| 接口 | 为什么不用转换 |
|---|---|
| `GET /system/notice/{noticeId}` | `selectNoticeVo` 选了全部列，含 `update_by` / `update_time` |
| `GET /system/config/{configId}` | `selectConfigVo` 同上 |

**不要"顺手"把这两个也抹掉** —— 它们的 `updateBy` 该有值就得有值，
测试里有反向断言压着（`TestNoticeAndConfigDetailKeepValues`）。

⚠️ **部门是唯一一处「详情和列表选列不同」的接口。**
Java 的 `selectDeptById` 没有 `include selectDeptVo`，而是单独写了一条 SQL：
比列表**少了** `create_by` / `create_time` / `del_flag`，
**多了** `parent_name`（子查询取父部门名）。两个方向都有差异，
所以它有独立的 `contractDeptDetail`，不能复用 `contractDept`。

#### 五条硬约束

1. **给上表这些实体加字段、或改 repository 的 select 列时，必须同步检查
   `java_contract.go`。** 漏改的表现是响应里多出一个 Java 没有的字段，
   或者本该为 `null` 的字段冒出真实值。`TestContractBaseMapsMatchModelJSON` 会拦住
   模型新增字段却漏改基础 map，但判断 Java 的选列和 `null` 语义仍要靠定点断言与双端对拍。
2. **判断某个字段该不该抹成 `null`，去打开 Java 的 mapper xml 看它 select 了哪些列**，
   不要凭直觉，也不要照抄实体定义 —— 实体有那个字段不代表查询填充了它。
3. **只在响应层转换。** 数据库模型保持适合写入和校验的 Go 类型（指针、`types.Time` 等），
   不要为了对齐响应去改模型。
4. **改任何响应字段前，先 `grep` 测试里对该字段的断言。**
   `assertField(t, detail, "updateBy", ...)` 这类断言散落在各个测试文件里，
   只盯着 handler 和新写的测试就会漏。给岗位详情挂转换那次就撞了 ——
   `post_test.go` 里压着一条 `updateBy == "admin"`，那是照着改之前
   Go 自己的输出写的，把不对齐固化成了"期望"。
5. **被转换抹掉的字段，如果背后有真实行为，要落到数据库层去验。**
   `sys_post.update_by` 现在不通过岗位接口暴露，但「修改时要记录操作人」这件事还在 ——
   用 `assertColumn(t, "sys_post", "update_by", "post_id", id, "admin")` 直接查库。
   这是**唯一**该绕开接口去断言的场景，别拿它当常规手段。

#### 唯一可靠的验证手段是双端对拍

见下方 `cmd/contractcheck`。这层转换是靠人工比对 Java 源码写出来的，
**没有对拍就没有保障**。

### 7. 双端对拍工具 `cmd/contractcheck`

真起 Java 和 Go 两个服务，对同一批请求逐字段比较响应。

```powershell
# 1. 两端必须用不同的 Redis DB
#    Go 用 DB 0，Java 用 DB 1 —— 两边的 value 序列化格式不同
#    （Java 走 Fastjson 带 @type），共用一个 DB 会互相读坏对方的
#    会话、验证码和配置缓存。

# 2. 起 Java（端口 8081）
java -Xmx512m -jar ..\RuoYi-Vue-master\ruoyi-admin\target\ruoyi-admin.jar `
     --server.port=8081 `
     --spring.data.redis.database=1

# 3. 起 Go（端口 8080，用配置里的 Redis DB 0）
go run ./cmd/server

# 4. 对拍（手动启动双服务时）
go run ./cmd/contractcheck                       # 36 个核心 GET（23 个固定路径 + 13 个动态路径）
go run ./cmd/contractcheck -file-probes          # 加 4 项上传 / Excel 探针
go run ./cmd/contractcheck -write-probes         # 加 7 项非持久化错误场景
go run ./cmd/contractcheck -crud-probes          # 加 10 组、60 项一次性 CRUD
go run ./cmd/contractcheck -permission-probes    # 加 18 项数据/功能权限
go run ./cmd/contractcheck -validation-probes    # 加 16 项字段校验和 JSON 类型

# 5. 推荐：一键启动、对拍、停止和清理
.\scripts\contractcheck.ps1 -All -AllowDifferences

# 6. 重新生成完整路由/权限/探针覆盖清单
go run ./cmd/routeaudit -java-root ..\RuoYi-Vue-master
```

期望输出：

```text
SUMMARY matched=36 different=0 total=36 percent=100.00%
```

有差异时进程返回非零，可以直接进 CI。

默认 36 个探针由 **23 个固定路径 + 13 个动态路径**组成。动态部分覆盖
**11 个详情接口和 2 个角色树接口**；工具先读取两端列表，自动找出双方共有的 ID，
再请求同一条记录，禁止把种子库里的 `1`、`2`、`100`、`103` 写死到探针里。
这依赖两端连接**同一个 MySQL**，Redis 则必须继续使用不同 DB。

**判定口径**：比较的是规范化后的 JSON，不是只看 `code=200`。
字段缺失、`null` 与空串、数字与字符串的类型差异、数组顺序变化都会判失败。
验证码图片、UUID、token、登录 IP/时间这类动态字段已登记为忽略。

`-crud-probes` 覆盖用户、角色、部门、菜单、岗位、参数、字典类型、字典数据、公告、
定时任务共 **10 组、60 项**，每组固定比较新增、新增后详情、修改、修改后详情、删除、
删除后详情。测试值使用 `zz_contract_` 前缀；部门父 ID 和字典类型从当前库动态解析。
两端共用 MySQL 时，唯一字段必须使用不同值；工具只忽略两端必然不同的动态 ID、
唯一测试值和时间字段。

`-permission-probes` 创建一次性部门、角色和用户，比较五种数据范围（全部、自定义、
本部门、本部门及以下、仅本人）下的用户/角色/部门列表，共 15 项；再比较无功能权限
访问三个列表，共 **18 项**。角色菜单权限从当前菜单动态读取。每次切换 `dataScope`
后必须重新登录两端，不能复用 Redis 中缓存旧权限的会话。

`-validation-probes` 共 **16 项**，同时输出两种摘要：`VALIDATION_PROBE_SUMMARY`
是响应字段、类型和值的严格比较；`VALIDATION_SEMANTIC_SUMMARY` 只判断非法输入是否被
双方拒绝。错误文案不同会导致严格口径不一致，但不应误报为校验语义放行。任何一端
意外写入的数据必须按本次唯一值定位并回收。

正常结束、发现差异和初始化失败都必须清理本次创建的业务记录、验证码和登录会话，
禁止使用 `FLUSHDB` 或前缀全删。`scripts/contractcheck.ps1` 会检查端口，从 Go DSN
读取当前 MySQL 连接（不输出密码），给 Java 临时注入同库连接，隔离两端 Redis DB，
构建临时可执行文件，并在 `finally` 中停止精确进程和删除临时文件。
文件探针运行时，两端上传根目录也必须指向该次运行目录，随运行目录一起删除，不能污染
Go 或 Java 的默认 `uploadPath`。
`-AllowDifferences` 仅用于记录已知差异；要求完全一致的 CI 不应传该开关。

`cmd/routeaudit` 静态提取 Go 路由和 Java Controller，统一尾斜杠与路径参数名，比较
HTTP 方法、结构化路径和权限标识，并标注上述探针覆盖。代码生成器、Swagger 内存演示
接口和 dev/test Profile 必须列在报告排除项中，不能混入业务接口对齐率。

### 测试和 Race Detector

```powershell
.\scripts\test.ps1 -Unit
.\scripts\test.ps1 -Unit -Race -CCompiler D:\path\to\gcc.exe
```

普通结果写入 `test/results/summary.log` 和 `latest.log`；Race 结果单独写入
`summary-race.log` 和 `latest-race.log`，不能互相覆盖。Windows 上 `-Race`
会启用 CGO，并把 `-CCompiler` 所在目录临时加入 `PATH`，因为 GCC 还要调用同目录的
`as.exe`。脚本退出时必须恢复 `CGO_ENABLED`、`CC` 和 `PATH`。

接口测试只清理自己能精确归属的 Redis key：登录后从 JWT 记录会话 key，
验证码响应记录 UUID，防重复提交按同一算法记录 key；密码错误计数只扫描
`pwd_err_cnt:zz_test_*`，限流只清理 httptest 固定 IP `192.0.2.1` 的三个键。
删除后必须逐项 `EXISTS` 验证，清理失败要让整个测试进程失败。

**换库或换环境后**：详情样本 ID 会从两端列表自动解析，无需修改探针常量；
如果新库缺少某类可选记录（例如没有根部门或子部门），应补足测试数据，
或重新审查该探针的查询路径和语义前提，**不要改业务代码去迎合旧数据**。

### 状态码

| code | 含义 |
|---|---|
| 200 | 成功 |
| 401 | 未认证 / token 失效 |
| 403 | 已认证但无权限 |
| 500 | 业务或系统错误 |

**HTTP 状态码一律 200**，错误通过 body 里的 `code` 表达（RuoYi 前端就是这么判断的）。

---

## 二、认证与会话

### JWT 的真实设计

**JWT 里不存用户数据**，只有两个 claim：

```
claims = { "login_user_key": "<uuid>", "sub": "<username>" }
```

签名算法 HS512。`Constants.java` 里虽然定义了 `JWT_USERID`、`JWT_AVATAR`、`JWT_CREATED`、`JWT_AUTHORITIES`，但 `TokenService.createToken()` 从未使用，是遗留常量——**不要照着常量表往 token 里加字段**。

推论：**除了 uuid 和 username，任何用户信息都必须去 Redis 取**，禁止写出"从 JWT 里读 userId"这种逻辑。

真实会话存在 Redis：`login_tokens:<uuid>` → 序列化的 LoginUser（含 user、roles、permissions、过期时间、登录 IP 等）。

**必须照做**，否则"在线用户""强制退出""权限变更即时生效"这些功能全部失效。

- 请求头：`Authorization: Bearer <token>`
- 默认有效期 30 分钟；剩余不足 20 分钟时自动续期（刷新 Redis TTL，不换 token）
- 登出即删除 Redis key

### 密码

bcrypt，直接沿用 `sys_user.password` 里现有的 hash，**Java 版生成的密码 Go 能直接校验**。默认账号 `admin / admin123`。

### 匿名放行路径

`/login`、`/register`、`/captchaImage`，以及静态资源 `/profile/**`。其余一律鉴权。

---

## 三、Redis key 命名

必须与 Java 端完全一致（对照 `CacheConstants.java`），否则两边不能共存：

| 前缀 | 用途 |
|---|---|
| `login_tokens:` | 登录会话 |
| `captcha_codes:` | 验证码，TTL 2 分钟 |
| `sys_config:` | 参数缓存 |
| `sys_dict:` | 字典缓存 |
| `repeat_submit:` | 防重复提交 |
| `rate_limit:` | 限流 |
| `pwd_err_cnt:` | 密码错误次数，5 次锁 10 分钟 |

统一定义在 `pkg/redisx`，禁止在业务代码里裸写字符串前缀。

### ⚠️ key 名一致 ≠ 数据能互通

Java 的 `RedisTemplate` 配了 `FastJson2JsonRedisSerializer`，写入时带 `@type`，
**连 String 也会被 JSON 化**（验证码 `1234` 存进去是 `"1234"`，带引号）。
Go 侧写的是自己的 JSON。

所以 key 名对齐只保证**两边不会互相踩键**，不代表能读懂对方的数据。
**做不到"Java 和 Go 同时在线、会话互通"** —— 灰度要按用户或按域名整体切，不能混流。
详见 [DECISIONS.md](./DECISIONS.md) 2026-08-23 那条。

---

## 四、分页

前端传参（query）：

| 参数 | 说明 |
|---|---|
| `pageNum` | 页码，从 1 开始，缺省 1 |
| `pageSize` | 每页条数，缺省 10，**硬上限 100** |
| `orderByColumn` | 排序字段，**必须用白名单校验**，禁止拼接 |
| `isAsc` | `asc` / `desc` |

`orderByColumn` 直连 SQL 是注入面，每个接口必须显式声明允许排序的列。

### 分页必须行序确定：`pg.Stable(默认排序, 唯一兜底列)`

```go
// ✓ 两个参数都要显式写，即使相同
err := db.Order(pg.Stable("post_id", "post_id")).
    Offset(pg.Offset()).Limit(pg.PageSize).Find(&list).Error

// ✓ 默认排序可以是多列、可以带方向；兜底列必须是单列且唯一
pg.Stable("r.role_sort, r.role_id", "r.role_id")
pg.Stable("info_id DESC", "info_id")

// ✗ 前端没传排序时不加 ORDER BY
if pg.OrderBy != "" { db = db.Order(pg.OrderBy) }

// ✗ 只按业务列排，并列值时行序未定义
orderBy := "role_sort"
```

| 参数 | 含义 | 约束 |
|---|---|---|
| `fallback` | 前端没传 `orderByColumn` 时用的完整排序 | 可多列、可带方向 |
| `tiebreaker` | 追加在用户指定排序后面的兜底列 | **单列、唯一、不带方向** |

行为：没传排序 → `fallback`；传了 → `<用户指定的>, <tiebreaker>`。

**不要试图从 `fallback` 里解析出兜底列。** 这里踩过坑：
原先只收一个参数、用 `strings.Fields(fallback)[0]` 推断，
遇到多列 fallback `"r.role_sort, r.role_id"` 切出来是 `"r.role_sort,"`（带逗号），
拼出 `ORDER BY r.role_sort asc, r.role_sort,` 直接 SQL 语法错误、接口 500。
调用方本来就知道主键叫什么。

**`tiebreaker` 传空串不会被容忍** —— 没有"为空就不追加"的分支。
留那个口子等于给"绕过稳定排序"开一条不报错的路；真忘了传，SQL 会立刻报错。

不接受前端排序的分页查询不走 `Stable`，直接在 SQL 里写死确定性排序，
判断标准是**最终排序里必须含唯一列**：

- 公告已读用户 `ORDER BY r.read_time DESC, u.user_id` ——
  `read_time` 精度到秒会并列，必须补主键
- 角色授权用户 `ORDER BY u.user_id` ——
  单列即唯一，已经确定，不需要再追加

**为什么两种情况都要管**：`LIMIT + OFFSET` 在「没有 ORDER BY」和「排序键有并列值」
两种情况下，行顺序都是未定义的 —— 翻页可能**重复某行或漏掉某行**。
症状是"某条记录在列表里怎么都找不到，翻回上一页又出现了"，
没有任何报错，极难复现。

`dict_sort`、`role_sort`、`post_sort` 这类列大量并列（新建时默认都是同一个值），
是最容易踩到的。

**与 Java 的差异是有意的**：Java 的 mapper 多数只写单列排序甚至完全不排序。
加主键兜底会让双端对拍在有并列值时报差异 —— 那是预期的，
**不要靠去掉兜底列来消差异**。行序不属于接口契约，前端依赖的是字段和结构。

> 判断 Java 到底有没有排序，要看 **service 转调到哪条 SQL**，不能只看 mapper 里
> 有没有同名 select。例如 `selectRoleAll()` 转调的是 `selectRoleList`，
> 那条才带 `order by r.role_sort`。曾经据此误删过排序，而且双端对拍没发现 ——
> 因为种子数据里 `role_id` 和 `role_sort` 恰好同序，样本把差异盖住了。

---

## 五、权限

### 功能权限

标识形如 `system:user:list` / `system:user:add`，在路由注册时挂载，不写在 handler 里。超级管理员（`user_id = 1`）拥有 `*:*:*`，跳过全部校验。

### 数据权限

`sys_role.data_scope` 取值：

| 值 | 含义 |
|---|---|
| 1 | 全部数据 |
| 2 | 自定义（查 `sys_role_dept`） |
| 3 | 本部门 |
| 4 | 本部门及以下 |
| 5 | 仅本人 |

多角色取**并集**（Java 端用 `OR` 拼接）。实现放 `pkg/datascope`，返回 GORM Scope，禁止字符串拼 SQL。

> Java 版的"本部门及以下"用 `find_in_set(deptId, ancestors)`，走不了索引。Go 版可以改成 `ancestors LIKE '0,100,%'` 前缀匹配，**但过滤结果必须与 Java 版完全一致**。

---

## 六、命名映射

同一个字段在三个地方有三种写法，必须显式声明，不能靠框架默认转换：

| 位置 | 写法 | 例 |
|---|---|---|
| 数据库列 | snake_case | `user_name` |
| Go 字段 | PascalCase | `UserName` |
| JSON | camelCase | `userName` |

```go
type SysUser struct {
    UserID   int64  `gorm:"column:user_id;primaryKey" json:"userId"`
    UserName string `gorm:"column:user_name" json:"userName"`
    DeptID   *int64 `gorm:"column:dept_id"   json:"deptId"`
}
```

- 缩写全大写：`ID` / `URL` / `IP`（Go 侧），JSON 侧仍是 `userId` / `avatarUrl`
- 可空列用指针或 `sql.Null*`，不要用零值冒充 NULL
- 状态类字段在 RuoYi 里是**字符串**不是数字：`status` = `"0"`（正常）/ `"1"`（停用），`del_flag` 同理。不要擅自改成 int 或 bool

---

## 七、参数校验

### 零值陷阱（必读）

Go 的零值语义会在**两个地方**咬人，两处都是静默失败，不报错但行为不对：

**1. `binding:"required"` 会拒掉合法的零值**

`required` 判定 int 的 `0`、bool 的 `false`、string 的 `""` 都是"未填写"。所以：

```go
PostSort int `binding:"required"`   // ✗ postSort=0 会被拒，而 0 是合法排序值
PostSort int `binding:"min=0"`      // ✓
```

**规则：`required` 只用在字符串和指针字段上。数值和布尔字段一律用 `min` / `max` / `oneof` 表达约束**，确实需要区分"没传"和"传了零值"时，把字段声明成指针。

### 数值必填：Java 的 `@NotNull` 只能用指针翻译

`@NotNull` 在 Integer 上是**必填**，不传就拒。Go 侧只写 `min=0` 是翻译不过来的：

| 写法 | 不传 | 传 0 | 传 -1 |
|---|---|---|---|
| Java `Integer` + `@NotNull` | **拒** | 过 | 过 |
| Go `int` + `min=0` | **过**（当成 0） | 过 | 拒 |
| Go `*int` + `required,min=0` | **拒** | 过 | 拒 |

`int` + `min=0` 在"不传"这一格上**比 Java 更宽** —— 会放进 Java 版拒绝的请求，
正是下面"与 Java 校验规则的偏离"里明令禁止的方向。

所以所有排序字段都用指针：

```go
OrderNum *int `binding:"required,min=0"`   // ✓ 不传拒、0 过、负数拒
OrderNum int  `binding:"min=0"`            // ✗ 不传时静默当成 0
```

入库前用 `model.IntValue(p)` 解引用，不要在业务代码里散落 `if p != nil`。

> 这个漏洞是接口契约审计发现的：只登记了"负数比 Java 严"，
> 漏了"不传比 Java 宽"。**登记偏离时两个方向都要看。**

注意 RuoYi 的 `status` / `delFlag` 是**字符串** `"0"`，用 `required` 没问题；但如果哪天建模成 int，`status=0` 就会被拒。

**2. `GORM Updates(struct)` 会跳过零值**

同一个根因。用 `Updates` 传结构体时，值为零的字段不会进 UPDATE 语句，导致「把排序改成 0」「把状态改回正常」静默失效。

```go
db.Model(&m).Where(...).Updates(post)                          // ✗
db.Model(&m).Where(...).Select("post_sort", "status").Updates(post)  // ✓ 显式列出可改字段
```

### 查询条件必须是独立结构体

**不要用实体结构体接列表接口的查询参数。**

Java 版 list 和 add/edit 复用同一个 `SysPost`，靠 `@Validated` 只加在 add/edit 方法上来区分。
Go 的 binding tag 写在结构体字段上，无法按接口区分——用实体接 `ShouldBindQuery`，
`required` 会立刻触发，导致不带参数就查不了列表。

```go
type SysPost struct {                       // 实体：带 binding，供 add/edit
    PostName string `binding:"required,max=50"`
}
type PostQuery struct {                     // 查询：只有 form 标签，无 binding
    PostName string `form:"postName"`
}
```

### 禁止用 oneof 硬编码字典取值

**凡是取值来自 `sys_dict_data` 的字段，一律不准写 `oneof=...`。**

字典是后台可编辑的内置功能。管理员往 `sys_normal_disable` 加一个"2 冻结"，
前端的 radio/select 是 `v-for="dict in sys_normal_disable"` 渲染的，会立刻多出这个选项；
而后端的 `oneof=0 1` 会把这个完全合法的提交拒掉，且报错信息指向"参数不合法"，
排查时根本想不到是校验写死了。

涉及的字段包括但不限于：`status`、`delFlag`、`noticeType`、`sex`、`visible`、
以及所有业务模块自定义的字典字段。这类字段只写 `required`。

确实需要校验取值合法性时，在 service 层拿 `GetDictDataByType` 的结果比对，
不要写进 binding tag —— 那是编译期常量，字典是运行期数据，两者生命周期不同。

### 注解映射表

写新模块时照此逐字段翻译，**不要凭印象**：

| Java 注解 | Go binding | 说明 |
|---|---|---|
| `@NotBlank` | `notblank` | **不是 `required`**，见下方陷阱 |
| `@NotNull`（Integer） | `*int` + `required` | **必须用指针**，见下方"数值必填"一节 |
| `@Size(min=0, max=N)` | `max=N` | `min=0` 是空约束。两边都按字符数算，中文均计 1 |
| `@Email` | `omitempty,email` | 内置规则已被覆盖成"空值合法"；`omitempty` 仍不可省，见下方陷阱 |
| `@Pattern(regexp=...)` | 自定义 tag | Go validator 无通用正则 tag，在 `pkg/validate` 注册 |
| `@Xss`（RuoYi 自定义） | `xss` | 已在 `pkg/validate` 实现，禁止 HTML 标签 |
| 无注解 | 无 tag | 不要自作主张加 |

### 两个必踩的陷阱

**1. `required` 比 `@NotBlank` 宽**

Go 的 `required` 对字符串只判断 `""`，`"   "` 能通过；Java 的 `@NotBlank` 会拒。
这属于"比 Java 更宽"，是被禁止的。所以注册了 `notblank`：

```go
PostName string `binding:"notblank,max=50"`   // ✓
PostName string `binding:"required,max=50"`   // ✗ "   " 会存进去
```

**2. 指针字段有两个独立的失败点，必须同时挡住**

以 `Email *string binding:"...email..."` 为例，前端有两种"没填邮箱"的表现：

| 前端行为 | Go 里的值 | 会怎么失败 | 靠什么挡 |
|---|---|---|---|
| 请求体不含 `email` | nil 指针 | validator 对 nil 指针**直接判失败，不调用规则函数** | `omitempty` |
| 传 `"email": ""` | 非 nil 指针 → `""` | `hasValue` 对非 nil 指针返回 true，`omitempty` **不跳过**，落到 `email` 规则上判失败 | 覆盖内置 `email` |

所以两者缺一不可：

```go
Email *string `binding:"omitempty,email,max=50"`   // ✓ 两个点都挡住
Email *string `binding:"email,max=50"`             // ✗ 字段不传时报错
Email *string `binding:"omitempty,email"`          // 若没覆盖 email 规则，传 "" 时报错
```

内置 `email` 已在 `pkg/validate` 覆盖为"空值合法"（对齐 Jakarta 的 `@Email`）。

**推广到所有可选的指针字段：`binding` 一律以 `omitempty` 开头。**
`max` / `min` 这类规则对空串本身是通过的，所以只有 `email`、`url`、`uuid`
这种"对空值判失败"的规则才需要额外覆盖。

### Java 校验全量清单

| 实体 | 字段 | Java | Go binding |
|---|---|---|---|
| SysPost | postCode | `@NotBlank` `@Size(64)` | `notblank,max=64` |
| | postName | `@NotBlank` `@Size(50)` | `notblank,max=50` |
| | postSort | `@NotNull` | `min=0` |
| | status | 无 | `required`（见偏离登记） |
| SysDept | deptName | `@NotBlank` `@Size(30)` | `notblank,max=30` |
| | orderNum | `@NotNull` | `min=0` |
| | phone | `@Size(11)` | `omitempty,max=11` |
| | email | `@Email` `@Size(50)` | `omitempty,email,max=50` |
| SysUser | userName | `@Xss` `@NotBlank` `@Size(30)` | `notblank,xss,max=30` |
| | nickName | `@Xss` `@Size(30)` | `omitempty,xss,max=30`（**无 NotBlank**） |
| | email | `@Email` `@Size(50)` | `omitempty,email,max=50` |
| | phonenumber | `@Size(11)` | `omitempty,max=11` |
| SysRole | roleName | `@NotBlank` `@Size(30)` | `notblank,max=30` |
| | roleKey | `@NotBlank` `@Size(100)` | `notblank,max=100` |
| | roleSort | `@NotNull` | `min=0` |
| SysMenu | menuName | `@NotBlank` `@Size(50)` | `notblank,max=50` |
| | orderNum | `@NotNull` | `min=0` |
| | path | `@Size(200)` | `omitempty,max=200` |
| | component | `@Size(200)` | `omitempty,max=200` |
| SysDictType | dictName | `@NotBlank` `@Size(100)` | `notblank,max=100` |
| | dictType | `@NotBlank` `@Size(100)` `@Pattern` | `notblank,max=100,dicttype` |
| SysDictData | dictLabel | `@NotBlank` `@Size(100)` | `notblank,max=100` |
| | dictValue | `@NotBlank` `@Size(100)` | `notblank,max=100` |
| | dictType | `@NotBlank` `@Size(100)` | `notblank,max=100` |
| | cssClass | `@Size(100)` | `omitempty,max=100` |
| SysConfig | configName | `@NotBlank` `@Size(100)` | `notblank,max=100` |
| | configKey | `@NotBlank` `@Size(100)` | `notblank,max=100` |
| | configValue | `@NotBlank` `@Size(500)` | `notblank,max=500` |
| SysNotice | noticeTitle | `@Xss` `@NotBlank` `@Size(50)` | `notblank,xss,max=50` |
| | noticeType | 无 | `required`（见偏离登记） |
| | remark | 无 | `omitempty,max=255`（**不是 500**） |
| SysJob | jobName | `@NotBlank` `@Size(64)` | `notblank,max=64` |
| | invokeTarget | `@NotBlank` `@Size(500)` | `notblank,max=500` |
| | cronExpression | `@NotBlank` `@Size(255)` | `notblank,max=255` |

注意 `SysUser.nickName` **没有** `@NotBlank`，只有 `@Xss` 和 `@Size` —— 昵称允许为空。
不要"顺手"给它加必填。

### 与 Java 校验规则的偏离

允许比 Java 更严，但必须满足**两个**条件，且要在此登记：

1. 前端正常操作走不到（含后台改配置、改字典这类内置操作）
2. 约束本身不随运行期数据变化

| 字段 | Java | Go | 理由 |
|---|---|---|---|
| `SysPost.postSort` | `@NotNull`，允许负数 | `*int` + `required,min=0` | `required` 对齐 `@NotNull`；`min=0` 是加严，前端表单本就是 `:min="0"`，负数排序无业务含义 |
| `SysDept.orderNum` / `SysMenu.orderNum` / `SysRole.roleSort` | 同上 | 同上 | 同上 |
| `SysDictData.dictSort` | **无 `@NotNull`**（裸 `Long`） | `*int` + `required,min=0` | 这个是纯加严：排序列存 NULL 会让列表顺序不可预期，而前端总会带值 |
| `SysPost.status` | 无校验 | `required` | `sys_post.status` 是 `char(1) not null` 且**无默认值**，不传会 SQL 报错。（`sys_dept.status` 有 `default '0'`，所以那边不加，这个不一致是有依据的） |
| `SysPost.remark` | 无校验 | `omitempty,max=500` | 对齐列长，见下 |
| `SysDept.leader` | 无校验 | `omitempty,max=20` | 对齐列长，见下 |
| `SysRole.status` | 无校验 | `required` | `sys_role.status` 是 `char(1) not null` 且无默认值 |
| `SysRole.remark` | 无校验 | `omitempty,max=500` | 对齐列长 |
| `SysUser.password` | 无校验 | `omitempty,min=5,max=20` | 前端表单规则就是 5–20；空密码会生成一个"空串的哈希"，账号等于被锁死 |
| `SysUser.remark` | 无校验 | `omitempty,max=500` | 对齐列长 |
| `SysNotice.noticeType` | 无校验 | `required` | `sys_notice.notice_type` 是 `char(1) not null` 且无默认值 |
| `SysMenu.menuType` | 无校验 | `required` | 空值会产出一条既不是目录也不是菜单的记录，`getRouters` 直接乱掉。前端 radio 是硬编码的 M/C/F，不是字典 |
| `SysMenu` / `SysDictData` 的其余字符串字段 | 多数无 `@Size` | `omitempty,max=列长` | 见下方"补齐列长校验" |

⚠️ **`remark` 的列长不是所有表都一样**：`sys_notice.remark` 是 `varchar(255)`，
其它表是 `varchar(500)`。照抄 500 会让超长备注在 SQL 层报错。

### 补齐 Java 漏掉的列长校验

Java 有若干字段没标 `@Size`，但数据库列有长度限制（`remark varchar(500)`、`leader varchar(20)`）。
超长时 Java 也会失败——只是失败在 SQL 层，报的是驱动异常而不是可读提示。

**所以给这类字段补 `max=列长` 不算"比 Java 更严"**：两边都拒绝，只是我们的错误信息能看懂。
新增模块时，**所有字符串字段的 `max` 一律对齐数据库列长**，Java 没标也要标。

反过来**不允许比 Java 更宽**——那会放进 Java 版会拒绝的脏数据，两版并行验证时行为对不上。

### 校验失败的提示

统一走 `handler.bindMessage(err)`，会带出首个失败字段名和规则，例如
`参数 PostSort 不合法（规则：不小于 0）`。

**不要回 "参数错误" 这种无信息量的提示** —— 前端看不出哪个字段有问题，排查要靠猜。
但也不要回显用户输入的值，避免把敏感内容原样打回去。

## 八、错误处理

- 业务错误用哨兵 error 或自定义类型，在 handler 边界翻译成 `code` + `msg`
- **`msg` 是给用户看的**，不要把 SQL 错误、堆栈、内部路径塞进去
- 详细错误进日志，用 `slog` 带上 `traceId`、`userId`、`path`
- 第三方/上游错误在边界处归一化成自己的错误枚举，业务层只认自己的枚举

---

## 九、建表

新建业务表必须包含（对齐 `BaseEntity`）：

```sql
create_by    varchar(64)  default '',
create_time  datetime,
update_by    varchar(64)  default '',
update_time  datetime,
remark       varchar(500) default null
```

- 主键统一 `bigint auto_increment`
- 逻辑删除用 `del_flag char(1) default '0'`，`'2'` 表示已删除（RuoYi 的约定）
- 需要数据权限的表必须有 `dept_id`，需要"仅本人"的必须有 `user_id`
- 所有查询条件列必须建索引，日志类大表额外考虑按时间分区或定期归档
