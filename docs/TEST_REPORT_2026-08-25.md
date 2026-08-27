# RuoYi Go Java 契约对拍与测试加固报告

日期：2026-08-25

范围：当前 `ry-vue` MySQL、Redis DB 0/1；Java 版本为当前
`../RuoYi-Vue-master`。排除 `/tool/gen/**` 代码生成器、Swagger 内存演示接口和
dev/test Profile 接口。

本报告是本次 1～4 计划的实跑快照。规则维护在 `docs/CONVENTIONS.md`，完整路由现状维护在
`docs/API_ROUTE_COVERAGE_2026-08-25.md`。本轮只新增或修改测试、审计、脚本和文档，
发现的业务差异未擅自修改。

## 一、最终结论

- Go/Java 非生成器业务路由按“HTTP 方法 + 结构化路径”匹配 **130/130**；权限标识匹配
  **130/130**。Go 额外提供 `GET /health`，不是 Java 缺口。
- 自动双端探针已覆盖匹配路由 **73/130（56.15%）**。该数字表示路由有自动探针，
  不表示每条路由的所有参数、分支和副作用均已覆盖。
- 核心 GET **36/36**、文件 **4/4**、五种数据范围及无功能权限 **18/18** 严格一致。
- CRUD 为 **47/60（78.33%）**，共 13 项明确差异；没有用忽略字段掩盖。
- 字段校验严格响应为 **0/16**，原因主要是错误文案不同；按“双方是否都拒绝非法输入”
  的语义口径为 **15/16（93.75%）**。唯一真实放行差异是 Java 接受空任务名。
- 普通完整回归和 Race Detector 均为 **625 通过、0 失败、0 跳过**；`go vet`、
  `gofmt -l`、`git diff --check` 全部通过。

不能笼统宣称“所有接口 100% 对齐 Java”。可以准确表述为：**非生成器业务路由和权限标识
已静态对齐；当前自动覆盖的核心读取、文件和数据权限场景严格对齐；CRUD、错误响应和一项
字段校验仍有已登记差异。**

## 二、计划 1：数据权限双端对拍

探针动态创建 3 个部门、4 个角色和 4 个用户，不写死种子 ID；角色的菜单权限从当前菜单
动态读取。每次切换角色 `dataScope` 后重新登录 Java/Go，防止 Redis 会话继续使用旧权限。

| 数据范围 | 用户列表 | 角色列表 | 部门列表 |
|---|---:|---:|---:|
| 全部数据 `1` | 一致 | 一致 | 一致 |
| 自定义 `2` | 一致 | 一致 | 一致 |
| 本部门 `3` | 一致 | 一致 | 一致 |
| 本部门及以下 `4` | 一致 | 一致 | 一致 |
| 仅本人 `5` | 一致 | 一致 | 一致 |
| 无对应功能权限 | 拒绝行为一致 | 拒绝行为一致 | 拒绝行为一致 |

```text
PERMISSION_PROBE_SUMMARY matched=18 different=0 total=18 percent=100.00%
```

结论：本次覆盖的五种数据范围、用户/角色/部门三个数据权限入口，以及三项功能权限拒绝行为，
在当前库上均与 Java 严格一致。

## 三、计划 2：CRUD、字段校验和数据库副作用

### 1. CRUD

CRUD 由原 4 个模块扩展到 10 个模块，每个模块固定执行新增、新增后详情、修改、修改后详情、
删除、删除后详情，共 60 项。部门父 ID、字典类型和所有记录 ID 均从当前库动态解析。

| 模块 | 结果 | 差异 |
|---|---:|---|
| 用户 | 5/6 | 删除后详情 |
| 角色 | 5/6 | 删除后详情 |
| 部门 | 4/6 | 新增详情空串/`null`；删除后详情 |
| 菜单 | 5/6 | 删除后详情 |
| 岗位 | 5/6 | 删除后详情 |
| 参数 | 5/6 | 删除后详情 |
| 字典类型 | 5/6 | 删除后详情 |
| 字典数据 | 4/6 | 新增详情空串/`null`；删除后详情 |
| 公告 | 4/6 | Java 修改时不更新 `remark`；删除后详情 |
| 定时任务 | 5/6 | 删除后详情 |
| **合计** | **47/60（78.33%）** | **13 项** |

13 项差异分为三类：

1. 10 个模块的删除后详情均不同。Go 明确返回“对象不存在”和 `code=500`；Java 返回
   `code=200`，数据通常为 `null`，用户/角色等逻辑删除对象还可能被详情查询返回。
2. 部门新增后详情中，Go 的 `phone/email` 为 `""`，Java 为 `null`；字典数据新增后详情中，
   Go 的 `cssClass` 为 `""`，Java 为 `null`。
3. 公告修改后，Go 更新 `remark`，Java mapper 不更新该列。

本轮保留这些差异。删除后明确报不存在和公告正常更新备注更易理解；空串/`null` 则属于响应
契约差异。如果后续要求逐字段复制 Java，再单独评审是否修改业务行为。

```text
CRUD_PROBE_SUMMARY matched=47 different=13 total=60 percent=78.33%
```

### 2. 字段校验与 JSON 类型

16 项覆盖用户、角色、部门、菜单、岗位、参数、字典类型、字典数据、公告和定时任务的必填、
格式与 JSON 数值类型。

```text
VALIDATION_SEMANTIC_SUMMARY rejectedByBoth=15 different=1 total=16 percent=93.75%
VALIDATION_PROBE_SUMMARY matched=0 different=16 total=16 percent=0.00%
```

- 15 项双方都拒绝，严格响应不一致仅因为错误文案不同；Go 的文案统一包含字段和规则，Java
  使用 Bean Validation 或 Jackson 原始文案。
- 1 项是真实语义差异：任务名为空时 Go 拒绝，Java 成功新增。探针为调用目标生成唯一值，
  若任一端意外写入会立即定位并删除。
- 首次探针曾用非唯一调用目标，Java 留下一条空名称任务。本轮已用 `jobId=1029`、空名称、
  调用目标、创建人和创建时间窗五重条件精确确认并删除，复查为 0。

### 3. 错误场景和文件副作用

| 探针组 | 结果 | 说明 |
|---|---:|---|
| 文件 | **4/4（100%）** | 单/多文件上传、用户导入模板、空模板导入语义一致 |
| 错误场景 | **1/7（14.29%）** | 注册关闭严格一致；其余 6 项见下 |

6 项错误场景差异中，单文件、多文件、用户导入和头像上传缺文件时双方都失败，但错误文案不同；
清理不存在的缓存名/键时 Go 返回“不支持清理”，Java 返回成功。这些探针不忽略 `msg`，所以
严格口径如实记为差异。

## 四、计划 3：完整路由、权限和探针覆盖清单

`cmd/routeaudit` 静态解析 Go 路由注册和 Java Controller 注解，统一尾斜杠别名及路径参数名，
并比较权限标识。Java `/logout` 由 Spring Security 注册，审计器按真实运行路由补入。

| 指标 | 结果 |
|---|---:|
| Go 路由 | 131 |
| Java 业务路由 | 130 |
| 方法 + 结构化路径匹配 | 130/130 |
| 权限标识匹配 | 130/130 |
| 权限标识差异 | 0 |
| 自动双端探针覆盖 | 73/130（56.15%） |

Go 唯一多出的路由是 `GET /health`。完整逐路由清单、权限标识、探针证据和排除项见
`docs/API_ROUTE_COVERAGE_2026-08-25.md`。

## 五、计划 4：一键双服务脚本

新增 `scripts/contractcheck.ps1`，支持：

```powershell
.\scripts\contractcheck.ps1 -All -AllowDifferences
.\scripts\contractcheck.ps1 -PermissionProbes
.\scripts\contractcheck.ps1 -CrudProbes -ValidationProbes -AllowDifferences
```

脚本会检查 8080/8081 端口，从 Go 当前 DSN 读取 MySQL 连接信息并临时注入 Java 进程，
隔离 Java/Go Redis DB，构建临时服务和检查器，生成路由覆盖清单，启动真实双服务并执行探针。
文件探针会把 Java/Go 上传根目录指向本次运行目录。无论成功、差异或异常退出，`finally`
都停止精确进程并递归删除经过路径校验的本次运行目录，连同临时二进制和上传文件一起清理；
不会输出数据库密码，不会使用 `FLUSHDB` 或模糊前缀批量删除共享数据。

最终一键实跑日志：`test/results/contractcheck-20260825-124336.log`。
字段校验单项复跑日志：`test/results/contractcheck-20260825-124231.log`。
上传目录隔离与自动清理复跑日志：`test/results/contractcheck-20260825-130349.log`。

## 六、本地完整验证

| 检查 | 结果 |
|---|---:|
| 普通完整回归 | 625 通过，0 失败，0 跳过；16.1 秒 |
| Race Detector 完整回归 | 625 通过，0 失败，0 跳过；91.52 秒 |
| `go test ./cmd/contractcheck ./cmd/routeaudit` | 通过 |
| `go vet ./...` | 通过，无输出 |
| `gofmt -l internal/handler test pkg cmd` | 通过，无输出 |
| `git diff --check` | 通过，无输出 |
| PowerShell 脚本解析与上传隔离复跑 | 通过；4/4 文件探针一致，默认上传目录和临时运行目录均无残留 |

```powershell
.\scripts\test.ps1 -Unit
.\scripts\test.ps1 -Unit -Race `
  -CCompiler "D:\WorkSpace\CodeX\tools\w64devkit-2.9.1\w64devkit\bin\gcc.exe"
go vet ./...
gofmt -l internal/handler test pkg cmd
git diff --check
```

普通和 Race 完整日志分别为 `test/results/latest.log`、`test/results/latest-race.log`；摘要为
`test/results/summary.log`、`test/results/summary-race.log`。

## 七、环境与测试数据清理

最终运行后执行了独立核验：

| 检查 | 结果 |
|---|---:|
| 端口 8080/8081 监听 | 0 / 0 |
| Redis DB 0 测试相关键 | 0 |
| Redis DB 1 测试相关键 | 0 |
| MySQL 活动 `zcs_*` / `zz_contract_*` / `zzv_*` 用户、角色、部门及十类 CRUD 数据 | 0 |
| 空名称校验任务残留 | 0 |
| 本轮文件探针上传残留 | 0 |

Redis 分别扫描了 `login_tokens:*`、`captcha_codes:*`、`pwd_err_cnt:zz_test*`、
`rate_limit:*`、`repeat_submit:*`。MySQL 核验覆盖用户、角色、部门、菜单、岗位、参数、
字典类型、字典数据、公告和定时任务。逻辑删除表只统计活动记录；探针使用正常业务删除语义。
本轮首次一键实跑写入默认上传目录的 6 个文件已按运行时间、大小和内容哈希精确确认并删除；
脚本随后改为使用每次运行的隔离上传目录，避免再次产生该类残留。

## 八、本轮修改记录

| 文件 | 修改 |
|---|---|
| `cmd/contractcheck/permission.go` | 新增五种数据范围和无功能权限共 18 项真实双端探针，动态建数并清理 |
| `cmd/contractcheck/crud.go` | CRUD 扩展到 10 个模块、60 项；动态解析部门父 ID和字典类型 |
| `cmd/contractcheck/validation.go` | 新增 16 项字段/类型探针、严格与语义双摘要、意外写入自动回收 |
| `cmd/contractcheck/main.go` | 增加 `-permission-probes`、`-validation-probes` 并汇总各探针组 |
| `cmd/contractcheck/main_test.go` | 增加探针数量、权限 ID 集合和动态行为保护测试 |
| `cmd/routeaudit/main.go` | 新增 Go/Java 路由、权限和探针证据静态审计及 Markdown 生成 |
| `cmd/routeaudit/main_test.go` | 增加 Java/Go 路由解析、尾斜杠别名测试 |
| `scripts/contractcheck.ps1` | 新增一键启动双服务、运行对拍、隔离上传目录、停止和完整清理脚本 |
| `docs/API_ROUTE_COVERAGE_2026-08-25.md` | 生成 130 条匹配业务路由的完整覆盖清单 |
| `docs/CONVENTIONS.md` | 写入全部探针、清理约束、一键脚本和路由审计运行规则 |
| `docs/TEST_REPORT_2026-08-25.md` | 记录本次 1～4 的实跑结果、差异、验证和清理证据 |

`docs/TEST_REPORT_2026-08-24.md` 保留为历史快照，不用本次数字覆写。
