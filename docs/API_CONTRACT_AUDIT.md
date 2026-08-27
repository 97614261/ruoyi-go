# RuoYi Go / Java 接口契约复核报告

> ## 📌 这是一份「某次复核的快照」，不是现状文档
>
> 它记录的是**做这次复核那一刻**的判断和数字。项目一直在改，
> 所以里面的结论、文件数、测试数**天然会过期**，不要拿它当准绳。
>
> | 你想知道 | 去看 |
> |---|---|
> | 现在的规则是什么 | `docs/CONVENTIONS.md` |
> | 当初为什么这么定 | `docs/DECISIONS.md` |
> | 现在有多少测试 | 跑 `.\scripts\test.ps1 -Unit`，看 `test/results/summary.log` |
> | 现在改了哪些文件 | `git status` / `git diff --stat` |
>
> **这份文档已经被整篇覆写过三次**，每次都留下过期段落误导下一个人
> （例如 2.2 的角色排序结论是错的，照着改会引入 bug）。
> 所以规矩是：
>
> - **结论一旦被推翻，在原文就地标注，不要删除** —— 删了就看不出踩过什么坑
> - **不要在这里维护"当前有多少文件/多少测试"这类会变的数字**，
>   那种数字属于命令输出，不属于文档
> - 复核出的**规则**要落进 CONVENTIONS，**取舍理由**要落进 DECISIONS，
>   这里只留过程和证据

复核日期：2026-08-24

对比对象：

- Go：当前工作区 `ruoyi-go`
- Java：`RuoYi-Vue-master`，版本 3.9.2
- 前端调用基准：`RuoYi-Vue3-master`
- 数据：两端共用当前 MySQL `ry-vue`
- Redis：Go 使用 DB 0，Java 使用 DB 1

明确排除：

- `/tool/gen/**` 代码生成器，按当前决定暂不核对、不计入缺口。
- `/test/user/**` Swagger 示例接口。
- Swagger UI、Druid 页面等非业务接口。

## 一、最终结论

| 维度 | 结果 | 判断 |
|---|---:|---|
| HTTP 方法、路径、权限静态清单 | 128/128，100% | 已对齐 |
| Go 完整自动化回归 | 复核当时 559 通过、0 失败（**这个数字会变，看 `test/results/summary.log`**） | 全部通过 |
| 25 个核心 GET 真实双端对拍 | 25/25，100% | 严格对齐 |
| 上传、模板、空模板导入 | 4/4，100% | 契约对齐 |
| 额外验证/错误场景 | 1/7 文案和状态严格一致 | 存在 6 项有意差异 |

结论分两层：

1. 非代码生成器业务接口的路由、方法、权限，以及本次 25 个核心 GET 的字段存在性、类型、值和数组顺序，已经对齐 Java。
2. 仍不能把整个项目表述为“所有行为 100% 复制 Java”。Go 保留了更严格的上传错误提示、缓存白名单、限流、分页和导出限制等增强，这些行为和 Java 不完全相同。

如果只统计本次自动双端样本：

- 核心 GET：25/25，100%。
- 文件与 Excel 场景：4/4，100%。上传路径、UUID 等动态值被规范化后比较响应结构；XLSX 验证的是合法性和业务语义，不要求二进制逐字节相同。
- 将 7 个额外错误场景也按响应逐字段严格计入：总计 30/36，83.33%。6 个差异均列在本文第五节，不能拿 83.33% 代替全部 128 个接口的总体覆盖率。

## 二、本轮已完成的修正

### 2.1 响应字段、类型和空值语义

新增响应层契约转换，不改变适合数据库写入和参数校验的 Go 模型，专门复刻 Java 的 MyBatis 选列、Jackson 空值和 Redis/Fastjson 缓存对象语义。

已覆盖：

- `/getInfo` 登录用户、部门、内嵌角色和缓存 `params`。
- 用户列表、新增/编辑表单数据。
- 角色列表和下拉选项。
- 岗位列表和下拉选项。
- 部门列表和排除子部门列表。
- 菜单列表。
- 字典类型、字典数据列表和缓存字典下拉。
- 公告管理列表和顶栏公告。

修正内容包括：

- Java 查询未选择的字段按 `null` 返回，不再错误地返回 Go 零值或数据库完整值。
- Java 必定输出的 `children`、`roles` 等集合按空数组返回，不再返回 `null` 或省略。
- 用户列表的 `avatar`、内嵌角色字段、部门字段范围与 Java 一致。
- Java 缓存对象中的 `params: {"@type":"java.util.HashMap"}` 得到保留。
- 公告列表返回完整 `noticeContent`，并始终输出 `isRead`。
- 顶栏公告不查询正文大字段，但和 Java 一样输出 `noticeContent: null` 等属性。

### 2.2 查询顺序

> ⚠️ **这一节的结论在 2026-08-24 被推翻了，下面这段是当时的记录，不要照着改代码。**
> 现行规则见 `docs/CONVENTIONS.md` 第四节「分页必须行序确定」和
> `docs/DECISIONS.md` 2026-08-24「分页必须有唯一兜底列」。
>
> 推翻的理由：
>
> - **角色排序那条是错的。** `SysRoleServiceImpl.java:112` 的 `selectRoleAll()`
>   转调 `selectRoleList`，而那条 mapper 有 `order by r.role_sort` ——
>   Java 是排序的，去掉排序反而制造了不一致。当时双端对拍没发现，
>   是因为种子数据里 `role_id` 和 `role_sort` 恰好同序。
> - **岗位那条方向也不对。** Java 的 `SysPostMapper.xml` 确实没有 `order by`，
>   但 `LIMIT + OFFSET` 不带排序时行序未定义，翻页会重复/漏行。
>   行序不属于接口契约，复刻这个等于把缺陷搬过来。
>
> 现状：13 处分页全部保证行序确定 —— 11 处走 `pg.Stable`，
> 2 处直写：公告已读用户按 `read_time DESC, user_id` 两级排序，
> 角色授权用户按唯一主键 `user_id` 排序。
> 这会让双端对拍在排序键有并列值时报差异，**那是预期的，不要靠去掉兜底列来消差异**。

～～以下为 2026-08-24 早些时候的原始记录～～

岗位全部列表、角色全部列表和字典类型全部列表不再擅自增加 Java SQL 中不存在的排序。当前数据库快照下，两端列表顺序已严格一致。

需要注意：Java 某些 SQL 本身没有 `ORDER BY`，数据库执行计划变化时仍可能产生不稳定顺序。若后期希望稳定排序，应同时修改两端契约，不能只改 Go。

### 2.3 Excel 空模板

只有表头、没有数据行的 XLSX 现在被识别为“合法但没有业务数据”，由业务层返回：

```text
导入用户数据不能为空！
```

不再误报“解析 Excel 失败，请确认文件格式”。已增加接口回归，并与 Java 真实上传结果对拍一致。

### 2.4 测试数据污染

接口测试现在会在启动前和退出后物理清理精确 `zz_test_` 前缀数据，并验证以下表无残留：

- 用户、角色、岗位、部门、菜单及其关联表。
- 参数、字典类型、字典数据、公告。
- 定时任务、任务日志、登录日志、操作日志。

分页清理由“只处理第一页”改成循环至空，测试结束后的残留断言也已经加入。本轮完整回归结束时断言通过，即当前库 `zz_test_*` 残留为 0。

### 2.5 双端对拍工具

已固化 `cmd/contractcheck`，支持：

- Java/Go 分别登录。
- 强制检查 Redis DB 隔离。
- 25 个核心 GET 自动比较。
- JSON 字段存在性、类型、值、数组长度和顺序比较。
- 忽略验证码图片、UUID、Token、登录 IP/时间等登记过的动态字段。
- 可选的错误场景探针 `-write-probes`。
- 可选的上传、模板和空模板导入探针 `-file-probes`。
- 差异时非零退出，适合后续纳入 CI。

### 2.6 具体修改文件清单

本轮没有修改 Java 和 Vue3 参考项目，也没有修改或实现代码生成器。Go 项目的实际代码改动如下。

| 文件 | 具体改动 | 解决的问题 |
|---|---|---|
| `internal/handler/java_contract.go` | 新增用户、角色、岗位、部门、菜单、字典和公告的 Java 响应契约转换；区分数据库查询对象与 Redis/Fastjson 缓存对象 | 统一字段是否存在、`null`/空串、空数组、内嵌对象选列和缓存 `params` |
| `internal/handler/login.go` | `/getInfo` 改用登录用户契约转换 | 对齐登录用户、部门、内嵌角色、头像和 `params` |
| `internal/handler/user.go` | 用户列表、用户详情、新增/编辑表单中的用户、角色、岗位统一走契约转换 | 对齐用户列表和表单响应字段、类型、空值及集合 |
| `internal/handler/role.go` | 角色列表和角色下拉使用角色契约转换 | 对齐 Java `selectRoleVo` 未选择字段的 `null` 语义 |
| `internal/handler/post.go` | 岗位列表和岗位下拉使用岗位契约转换 | 对齐更新者、更新时间等字段 |
| `internal/handler/dept.go` | 部门列表和排除子部门列表使用部门契约转换 | 补齐 `children` 空数组，并对齐 `parentName`、更新字段和备注 |
| `internal/handler/menu.go` | 菜单列表使用菜单契约转换 | 对齐 Java 菜单查询的实际选列和 `children` |
| `internal/handler/dict.go` | 区分普通字典查询与 Redis 缓存字典响应 | 对齐字典空值以及缓存对象的 Fastjson `params` 类型标记 |
| `internal/handler/notice.go` | 公告列表使用契约转换；`markReadAll` 空 `ids` 改为成功但不修改数据 | 对齐公告字段及 Java 的空数组边界行为 |
| `internal/model/sys_notice.go` | `SysNotice` 增加始终输出的 `isRead`；顶栏 DTO 增加值为 `null` 的正文、更新和备注字段 | 对齐 Java `SysNotice` 的序列化字段，同时避免顶栏查询读取 longblob |
| `internal/repository/notice.go` | 管理列表查询完整正文并转换为字符；删除不再使用的“查询全部未读 ID”逻辑 | 对齐 Java 公告管理列表和空 `markReadAll` 行为 |
| `internal/service/notice.go` | 批量已读在 `noticeIDs` 为空时直接返回成功 | 防止 Go 把“空 ID”错误解释成“全部已读” |
| ~~`internal/repository/post.go`~~ | ~~只有请求明确带排序字段时才排序~~ | ⚠️ **已推翻**，见 2.2 的批注。现为 `pg.Stable("post_id", "post_id")` |
| ~~`internal/repository/role.go`~~ | ~~全部角色查询移除额外的 `role_sort` 排序~~ | ⚠️ **已推翻，当时的判断是错的**。Java `selectRoleAll()` 转调 `selectRoleList`，那条有 `order by r.role_sort`。现为 `ORDER BY role_sort, role_id` |
| `internal/repository/dict.go` | 全部字典类型查询移除额外的 `dict_id` 排序 | 对齐 Java `selectDictTypeAll`（这条成立，Java 侧确实没有排序） |
| `internal/router/router.go` | 直接注册 `GET /system/user/`；导入模板改为仅登录；公告已读用户列表增加 `system:notice:list` 权限 | 对齐 Java 路径映射和 `@PreAuthorize` 注解 |
| `pkg/excelx/import.go` | 只有表头的工作簿返回空数据而不是解析错误 | 由业务层返回与 Java 一致的“导入用户数据不能为空！” |
| `cmd/contractcheck/main.go` | 新增双端登录、25 个 GET、错误场景、上传和 Excel 对拍工具 | 把人工核对固化为可重复执行、差异时可失败的自动检查 |

测试代码改动如下。

| 文件 | 具体改动 |
|---|---|
| `test/main_test.go` | 测试前后物理清理精确 `zz_test_` 前缀；分页循环清理；清理后逐表断言残留为 0 |
| `test/helper_test.go` | 增加 multipart 文件请求助手 |
| `test/permission_test.go` | 新增未登录、无权限、管理员三身份矩阵；覆盖 97 条受保护路由 |
| `test/user_test.go` | 增加空模板导入、导入模板仅需登录、`GET /system/user/` 尾斜杠回归 |
| `test/notice_test.go` | 增加顶栏 `null` 字段、空 `markReadAll`、已读用户列表权限回归 |
| `test/dept_test.go` | 增加部门批量排序成功及非法参数回归 |
| `test/menu_test.go` | 增加菜单批量排序成功及非法参数回归 |
| `test/role_test.go` | 增加角色批量授权和批量取消授权回归 |

> **不再在这里记"改了多少个文件"。** 之前写的 24 个已跟踪文件很快就过期了
> （后续又改了排序、`pg.Stable`、`pkg/page` 单测等）。
> 这类数字属于命令输出，查现状用：
>
> ```powershell
> git status --short          # 改了哪些
> git diff --stat             # 改了多少行
> ```

## 三、25 个核心 GET 对拍结果

运行环境：Java `8081`、Go `8080`；两端使用同一 MySQL，Redis 分别使用 DB 1 和 DB 0。

| 接口组 | 数量 | 结果 |
|---|---:|---:|
| 验证码、登录信息、动态路由 | 3 | 3/3 |
| 用户与部门树 | 3 | 3/3 |
| 角色 | 3 | 3/3 |
| 岗位 | 2 | 2/2 |
| 部门 | 1 | 1/1 |
| 菜单 | 3 | 3/3 |
| 参数配置 | 2 | 2/2 |
| 字典类型与字典数据 | 4 | 4/4 |
| 通知公告 | 2 | 2/2 |
| 定时任务与任务日志 | 2 | 2/2 |
| **合计** | **25** | **25/25，100%** |

实际汇总输出：

```text
SUMMARY matched=25 different=0 total=25 percent=100.00%
```

比较的是规范化后的 JSON 契约，不是只比较 `code=200`。除明确登记的动态字段外，字段缺失、`null`/空串、数字/字符串类型差异、数组位置变化都会判定失败。

## 四、文件和 Excel 对拍

| 场景 | 结果 |
|---|---|
| `POST /common/upload` 单文件成功 | 字段、类型和固定值一致；动态文件路径规范化 |
| `POST /common/uploads` 多文件成功 | 字段、类型、文件名拼接语义一致；动态路径规范化 |
| `POST /system/user/importTemplate` | 两端均返回合法 XLSX |
| 各自生成的空模板重新导入 | 两端均返回“导入用户数据不能为空！” |

实际汇总输出：

```text
FILE_PROBE_SUMMARY matched=4 different=0 total=4 percent=100.00%
```

头像成功上传此前已实测响应结构一致，管理员原头像已恢复。本轮没有再次改变管理员头像。

## 五、仍未严格复制 Java 的行为

### 5.1 缺文件时的错误文案

两端都会拒绝请求，但 Go 主动返回稳定、可读的业务错误；Java 当前版本暴露了运行时异常文本。

| 场景 | Go | Java 3.9.2 |
|---|---|---|
| 单文件上传缺文件 | `请选择要上传的文件` | `Cannot invoke ... because "file" is null` |
| 多文件上传缺文件 | `请选择要上传的文件` | `Cannot invoke ... because "files" is null` |
| 用户导入缺文件 | `请选择要导入的文件` | `Cannot invoke ... because "file" is null` |
| 头像上传缺文件 | `请选择要上传的头像` | `Current request is not a multipart request` |

这里保留 Go 行为。复制 Java 文案会把 JDK/Spring 的内部异常暴露给前端，并且会随运行时版本变化，Go 的处理更适合作为稳定接口契约。

### 5.2 缓存安全边界

| 场景 | Go | Java |
|---|---|---|
| 清理不存在/非白名单缓存名 | `code=500`，`不支持清理该缓存` | `code=200` |
| 清理不存在/非白名单缓存键 | `code=500`，`不支持清理该缓存` | `code=200` |
| 清全部缓存 | 只清可管理白名单前缀 | 删除当前 Redis DB 全部 key |

这里也保留 Go 行为。它可以防止拥有缓存监控权限的调用方删除登录会话或同 Redis 中的其它业务数据。代价是不能声称缓存接口错误语义与 Java 完全一致。

### 5.3 其它主动加严项

- `dictSort` 在 Go 中必填；部分排序值拒绝负数。
- `pageSize` 最大 100。
- 登录、注册、验证码有限流。
- 新增类接口有防重复提交。
- 导出最多 100000 行，最多 2 个并发导出。
- 上传大小、扩展名和多文件数量有白名单/上限。
- 部门、菜单批量排序对空值、数量不匹配和非数字返回明确错误。
- 部分逻辑删除查询比 Java 更严格。
- Go 额外提供 `/health`。

这些差异大多提升安全性和故障可诊断性，但严格意义上仍属于与 Java 不同的行为。

## 六、字段校验和基础类型判断

基础映射已核对：

- Java `Long` 对应 Go `int64`，可空 ID 使用指针。
- Java `Integer` 的可空/必填语义在 Go 中使用 `*int` 或专用 DTO 表达。
- Java `String` 对应 Go `string`，数据库可空且需要区分空串的字段使用 `*string` 或响应转换。
- ID 数组对应 `[]int64`。
- 日期统一通过项目时间类型输出 `yyyy-MM-dd HH:mm:ss`。
- JSON Body、query、path、表单和 multipart 的主要绑定位置与 Java/Vue3 调用一致。
- JSON 字段名保持前端要求的 camelCase。

路由和权限方面已确认：

- Java Controller 的 128 个非生成器业务映射均有 Go 对应项。
- 97 条受保护路由均在无权限身份矩阵中验证为 `403`。
- `GET /system/user/`、公告已读列表权限、用户导入模板权限、空 `markReadAll` 等历史差异均已修正并有回归。

仍应把第五节的主动加严项视为校验差异，而不是遗漏。

## 七、测试与质量检查

本轮最终执行（`pass=559` 是**复核当时**的数字，后续还在增加，
现状看 `test/results/summary.log`）：

```text
go test ./... -count=1
TEST_SUMMARY pass=559 fail=0 skip=0 exit=0

go build ./...
go vet ./...
gofmt -l .
git diff --check
```

其中：

- 完整接口回归使用真实 MySQL 与 Redis。
- 测试退出时物理残留断言通过。
- 双端 GET 对拍为 25/25。
- 文件/Excel 对拍为 4/4。
- 对拍完成后 Java/Go 服务已停止，`8080`、`8081` 无监听。
- 本轮临时上传目录和对拍配置已删除。

## 八、Redis、时区和后期换库

### 8.1 Redis 必须隔离

Java 和 Go 的 Redis value 序列化格式不同。双服务并行运行时不能直接共用同一个 Redis DB，否则配置缓存、验证码、会话等值可能被另一端误读。

当前约定：

- Go：Redis DB 0。
- Java：Redis DB 1。

### 8.2 时区

当前清理后的基础任务数据上，`/monitor/job/list` 和 `/monitor/jobLog/list` 已严格匹配。后期换库仍应同时固定：

- OS 时区。
- Go MySQL DSN 的 `loc`。
- Java JDBC `serverTimezone`。
- Jackson 时区。
- Cron 计算时区。

### 8.3 后期换库执行方式

测试已支持通过 `RUOYI_TEST_CONFIG` 指向新配置。换库后建议按顺序执行：

1. 导入同版本 RuoYi 基础表结构和种子数据。
2. 给 Go/Java 配置独立 Redis DB。
3. 执行 `go test ./... -count=1`。
4. 启动 Java 8081、Go 8080。
5. 给两端配置可丢弃的上传目录后，执行 `go run ./cmd/contractcheck -file-probes -write-probes=false`。
6. 检查新库基础角色、部门、字典和任务数据是否与当前快照不同；若不同，更新对拍样本 ID，不要修改业务代码去迎合旧数据。

## 九、最终评价

- 以 Java 兼容为标准：路由/权限和核心成功响应现在已经达到本次范围内的 100%。
- 以安全性和可维护性为标准：Go 在缺文件错误、缓存隔离、限流、导出和上传限制方面更好。
- 最准确的项目表述是：**非代码生成器接口的静态契约已对齐，核心 GET 与文件/Excel 成功契约已完成真实双端对拍；少量安全和错误处理行为有意优于 Java，因此不是逐错误、逐副作用的完全克隆。**
