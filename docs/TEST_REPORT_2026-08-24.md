# RuoYi Go 测试加固与 Java 对拍报告

日期：2026-08-24  
范围：当前 `ry-vue` MySQL、Redis DB 0/1；排除 `/tool/gen/**` 代码生成器。  
原则：本轮只修改测试、对拍工具和文档；新发现的业务契约差异没有直接修改业务代码。

## 一、完成的加固

### 1. Redis 测试数据精确清理

- 登录后从 JWT 解析并登记本轮的 `login_tokens:<uuid>`。
- 每次获取验证码时登记响应 UUID 对应的 `captcha_codes:<uuid>`。
- 按生产中间件相同算法登记测试请求可能创建的 `repeat_submit:*`。
- 密码错误计数只清理 `pwd_err_cnt:zz_test_*`。
- 限流只清理 httptest 固定 IP `192.0.2.1` 对应的 captcha/login/register 三个键。
- 删除后逐项执行 `EXISTS` 验证，任何清理失败都会让 `TestMain` 返回失败。
- `cmd/contractcheck` 会精确清理 Go、Java 两端本轮创建的会话和验证码；登出没有删除会话时会兜底删除并把运行标记为失败。

没有使用 `FLUSHDB`、全前缀删除或其它可能踢掉真实用户的清理方式。

### 2. Race Detector

`scripts/test.ps1` 新增：

```powershell
.\scripts\test.ps1 -Unit -Race -CCompiler D:\path\to\gcc.exe
```

Windows 下脚本会临时启用 CGO，把 GCC 所在目录加入 `PATH`，结束后恢复
`CGO_ENABLED`、`CC` 和 `PATH`。普通测试与 Race 结果分别写入：

- `test/results/summary.log`、`latest.log`
- `test/results/summary-race.log`、`latest-race.log`

本轮使用 w64devkit 2.9.1 / GCC 16.2.0；下载文件执行前与 GitHub 官方摘要
`9208c19755cd4964b7915b9afcf02c66d493a4c870c4b3e83f6c538d9c1237a5`
核对一致。

### 3. Java/Go CRUD 对拍

`cmd/contractcheck` 新增 `-crud-probes`，使用一次性 `zz_contract_` 数据覆盖四组接口：

| 模块 | 场景 |
|---|---|
| 岗位 | 新增、详情、修改、修改后详情、删除、删除后详情 |
| 参数 | 新增、详情、修改、修改后详情、删除、删除后详情 |
| 字典类型 | 新增、详情、修改、修改后详情、删除、删除后详情 |
| 公告 | 新增、详情、修改、修改后详情、删除、删除后详情 |

两端共用 MySQL 时使用不同唯一值，比较前只规范化动态 ID、测试唯一值和时间；
字段是否存在、`null`/空串、类型和其它业务值仍会参与严格比较。

### 4. Excel 测试

新增 `pkg/excelx/excelx_test.go`，覆盖：

- 列排序、导入/导出专用列、数字和强制文本。
- converter、suffix、default、时间格式和指针字段。
- nil 指针行、空模板、空行。
- 重排列头、多余列、逐行错误收集。
- 非法文件、缺少工作表、错误目标类型、不支持字段类型。
- HTTP 下载响应头和“生成失败时不写半个响应”。

`pkg/excelx` 语句覆盖率从约 **37.9%** 提升到 **88.3%**。

## 二、最终测试结果

| 检查 | 结果 |
|---|---:|
| 普通完整回归 | 607 通过，0 失败，0 跳过；11.23 秒 |
| Race Detector 完整回归 | 607 通过，0 失败，0 跳过；80.87 秒 |
| `pkg/excelx` 覆盖率 | 88.3% |
| `go build ./...` | 通过 |
| `go vet ./...` | 通过 |
| `gofmt -l .` | 通过，无输出 |
| `git diff --check` | 通过，无输出 |
| 模块完整性 | 之前的 `go mod verify` 已通过；本轮未修改 `go.mod` / `go.sum` |

普通回归和 Race 回归结束后，Redis DB 0、DB 1 的以下测试键均为 0：

- `login_tokens:*`
- `captcha_codes:*`
- `pwd_err_cnt:zz_test*`
- `rate_limit:*`
- `repeat_submit:*`

MySQL 的 `zz_test_*` 数据仍由 `TestMain` 物理清理并逐表断言；两轮完整回归均通过。

## 三、Java/Go 对拍结果

| 对拍组 | 严格一致 |
|---|---:|
| 25 个核心 GET | 25/25，100% |
| 文件和 Excel | 4/4，100% |
| 已登记错误场景 | 1/7，14.29% |
| 新增 CRUD 场景 | 15/24，62.50% |
| 全部自动探针 | 45/60，75.00% |

原有错误场景的 6 个差异仍是主动保留项：Go 的缺文件错误更友好，缓存白名单比 Java 安全。

CRUD 对拍新发现 9 个严格差异：

1. 岗位新增后详情：Go `updateBy=""`，Java `updateBy=null`。
2. 岗位修改后详情：Go `updateBy="admin"`，Java `updateBy=null`。
3. 字典类型新增后详情：Go `updateBy=""`，Java `updateBy=null`。
4. 字典类型修改后详情：Go `updateBy="admin"`，Java `updateBy=null`。
5. 岗位、参数、字典类型、公告删除后再查：Go 返回 `code=500` 和明确的“不存在”；Java 返回 `code=200`、`data=null`，共 4 项。
6. 公告修改 `remark`：Go 会更新备注；Java 3.9.2 的 `updateNotice` SQL 没有更新 `remark`，仍返回旧备注。

判断：`updateBy` 是纯响应契约差异；删除后明确报不存在和公告备注可修改这两类行为，
Go 更合理，但严格意义上不等于 Java。是否为了兼容改回 Java，需要单独做产品取舍，
本轮没有擅自修改。

## 四、环境与残留

- Java、Go 临时服务已停止，8080/8081 无监听。
- 对拍产生的 Go/Java 登录会话和验证码均已清理。
- 一次性 CRUD 数据已通过删除接口删除，并完成删除后详情验证。
- 对拍原始输出：`test/results/contractcheck.log`。
- 代码生成器仍未纳入本轮范围。

## 五、本轮修改文件

| 文件 | 修改 |
|---|---|
| `test/main_test.go` | 精确登记、删除并验证 Redis 测试键；清理失败影响退出码 |
| `test/helper_test.go` | 请求完成后登记验证码和防重复提交 key |
| `scripts/test.ps1` | 增加 Race/编译器参数、环境恢复、独立日志和 `cmd/...` 单测 |
| `cmd/contractcheck/main.go` | 可靠退出码、JWT 会话定位、两端登出和 Redis 精确清理 |
| `cmd/contractcheck/crud.go` | 新增四组、24 项一次性 CRUD 对拍 |
| `cmd/contractcheck/main_test.go` | 增加 JWT、忽略字段和动态值规范化单测 |
| `pkg/excelx/excelx_test.go` | 增加导入、导出、模板和 HTTP 响应测试 |
| `docs/CONVENTIONS.md` | 记录 CRUD 对拍、Race 和 Redis 清理约束 |
| `docs/DECISIONS.md` | 记录精确清理而不是全库删除的决策 |

