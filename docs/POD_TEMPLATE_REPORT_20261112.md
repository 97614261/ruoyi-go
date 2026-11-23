# 个人模板实现与验收

日期：2026-11-12。对应后续计划第 1 项；本报告是本次验收快照，现行接口和规则分别维护于 POD_FOUNDATION 与 CLAUDE。

## 结论

个人模板的代码、006 本地迁移和自动化验收已完成。本人可以保存流程、增加版本、选择固定版本换输入并创建独立项目，
也可以修改名称/说明、复制、移入回收站和恢复。内置预设与个人模板分开显示。

完整 POD 集成通过，包含真实本机 CPU 图片模型；新增模板项目的本地处理与鉴权下载链路通过。
真实登录后的整条浏览器链路、模板驱动的付费模型生成、备份恢复与生产部署仍待单独验收。
本轮没有调用千问，未新增模型费用。

## 怎么使用

1. 登录创作端，打开项目，在画布顶部点“保存为模板”。提交前会先保存画布；保存失败或版本冲突时不提交模板。
2. 选择“新建个人模板”，或“为已有模板保存新版本”，填写名称和说明后保存。
3. 顶部导航点“模板”进入“我的模板”；画布左侧“我的模板”也可打开模板库。
4. 点“使用模板”，选固定版本，填写新项目名称，替换需要的文字或图片，点“创建项目并打开”。
5. 在新画布继续编辑，需要生成时再执行。创建项目不会自动运行或收费；修改模板也不会改变旧项目。
6. 列表中的修改按钮改名称/说明，复制按钮创建独立模板；详情中的“复制此版本”复制选中的历史版本。
7. “移入回收站”可以恢复。历史版本和回收站仍保留图片引用，不释放磁盘，也没有永久清空功能。

响应丢失时显示“重试原提交/原创建”，模板列表提供“重试原操作”；沿用原输入及 requestId 核对结果，
不能换请求标识重新创建。待确认记录保存在当前标签页的 sessionStorage，按账号/空间区分，不含 Token；
它包含原输入，不是加密备份，跨浏览器或标签页关闭后的恢复不在本轮范围。

## 代码与数据

| 部分 | 位置与边界 |
|---|---|
| 四层实现 | `internal/{model,repository,service,handler}/pod_template.go`，handler 经 service 调用 repository |
| 路由 | `internal/router/router.go`，复用 `pod:studio:access` 和本人有效个人空间检查 |
| 素材保护 | `pod_asset.go` 中计数与删除校验增加全部模板历史引用，写入共用空间锁 |
| 创作端 | `Templates.vue`、`TemplateSaveDialog.vue`、`TemplateUseDialog.vue`、`TemplateMetadataDialog.vue`、`TemplateGraph.vue` |
| 提交恢复 | `studio/src/template-requests.ts`；先保存原意图，再 POST，超时/5xx 不丢请求编号 |
| 新迁移 | `sql/pod/migrations/006_templates.sql`；只新增 POD 四表，不改已安装 001—005 |
| 测试 | `test/pod/template_test.go`、`studio/src/templates.component.test.ts` |

CRUD 骨架先经现有生成器真实渲染，再补所有权、不可变版本、事务与独立 DTO；九份输出保留在忽略提交的
`.local/generated/template/` 供本机审阅。临时生成测试和临时浏览器验收页已移除，未修改用户提供的 `web/`。

四张新表：

| 表 | 作用 |
|---|---|
| `pod_template` | 名称、说明、当前版本、乐观锁 revision、回收站、workspace 归属 |
| `pod_template_version` | 固定版本图快照，模板+版本联合主键，历史不可变 |
| `pod_template_asset_ref` | 各历史版本的图片引用，node_id 大小写敏感，回收站继续保留 |
| `pod_template_action` | 空间内 requestId、请求摘要与原结果定位；使用记录包含来源版本与项目 |

保存仅接受已保存的 `projectId/projectRevision`，不接受客户端另传任意图。
服务端读取项目并校验图，写事务内重新核对来源 revision；引用、版本与动作记录同事务。
使用版本时仅允许替换 textInput/imageInput 槽位，拒绝重复槽位、错类型、处理节点篡改与跨空间图片。
新项目复制版本快照，`remark=pod-template:<id>:<version>`，不复制运行、历史结果、费用或凭据。

## 接口

普通响应保持 HTTP 200 + 统一业务码；ID、revision、version 使用字符串。

| 方法与路径 | 请求/用途 |
|---|---|
| `GET /pod/templates` | `pageNum/pageSize/deleted`，上限 100，列表无整图 |
| `POST /pod/templates` | `requestId/projectId/projectRevision/name/description`，创建 v1 |
| `GET /pod/templates/:id` | 元信息与当前版本详情 |
| `PUT /pod/templates/:id` | `revision/name/description`，只改元信息 |
| `DELETE /pod/templates/:id`、`POST .../restore` | `revision`，软删除/恢复 |
| `GET /pod/templates/:id/versions` | 分页版本摘要，无整图 |
| `POST /pod/templates/:id/versions` | 保存字段另加模板 `revision`，追加不可变版本 |
| `GET /pod/templates/:id/versions/:version` | 图、输入槽位和终端输出位置 |
| `POST /pod/templates/:id/copy` | `requestId/version/name`，所选版本复制为新模板 v1 |
| `POST /pod/templates/:id/use` | `requestId/version/name/bindings`，创建独立项目 |

创建、新版本、复制、使用通过规范 UUID + 请求摘要去重。同键变参数、陈旧 revision 返回 409；
缺失或跨空间模板返回 404；非法输入返回 400。列表按更新时间+唯一 ID 倒序，版本按唯一版本号倒序。
保存/复制/使用不会调用运行接口或模型。

## 本地迁移与备份

迁移前确认无活动运行，停止本项目 API/Worker/前端/图片服务；生成 SQL 并保留私有素材、配置及账号副本。

- SQL：`.local/backups/pod_studio_dev-20261112-175635-3329838540.sql`，170510 字节。
- 配套副本：`.local/backups/template-before-006-20261112-175636/`。
- 安装：`go run ./cmd/podsetup -mode migrate -config configs/pod.local.yml`。
- 结果：本地 `pod_studio_dev` 的 001—006 ready；未重置已有账号或项目。

备份文件已生成，尚未恢复到独立副本，不能据此宣称恢复验收通过。新环境按 POD_LOCAL 停写、备份和 migrate，
不要重新 init 已存在的库；启动检查会拒绝缺少 006 的环境。上述备份包含敏感配置，不提交 Git。

## 测试证据

本地 Windows、MySQL 5.7、POD 专用 Redis 6387/DB15。集成测试期间 API/Worker 停止，显式使用
`.local/image-work-integration.yml` 的 simulation 模式与替身供应商；真实 CPU 测试连接回环 18112，未连接付费模型。
没有运行默认指向继承业务库的根 `test/` 套件。

| 检查 | 本次结果 |
|---|---|
| Go 回归 | `go test ./internal/... ./pkg/... ./cmd/... ./sql/pod` 通过 |
| 模板专项 | 7 组子场景通过，包含实际接口、并发与私有 PNG 下载 |
| 完整 POD 集成 | `go test ./test/pod -count=1 -json` 通过，20.84 秒；85 个通过事件包含父级/子测试，不是 85 个独立业务用例 |
| Worker 子进程 | 正常子进程场景已跑；顶层 `TestPODWorkerProcessHelper` 按设计跳过，仅由子进程设置环境后执行 |
| 前端 | 9 个测试文件、66 项通过，其中新增模板组件与提交恢复 6 项 |
| 静态与编译 | `go vet ./...`、`go build ./...`、前端类型检查及生产构建通过 |
| 格式 | 新增 Go 文件 gofmt 检查通过，`git diff --check` 通过 |
| 浏览器布局 | 使用真实组件+模拟数据，1280×720 桌面及 390×844 窄屏检查，版本切换、换文字、创建反馈与保存新版本交互通过；没有真实登录 |

前端回归曾发现新增模板按钮挤占原测试的“保存”按钮位置，已恢复原保存按钮优先顺序，全部回归通过。
生产构建仍有单个 JS 包约 541 kB、超过 500 kB 的体积提示，不影响本轮构建；不宣称零警告。

模板专项覆盖：本人快照和跨账号拒绝、未知 DTO 字段、重复创建/使用与同键变参、错槽位/跨空间图片、
旧版本不可变及旧项目不跟随、来源变更后的原请求重放、改名/复制/删除/恢复、历史引用删除保护、
pageSize 100 与同时间唯一排序、并发相同 requestId 只创建一次、撤销本轮测试成员后拒绝。
还覆盖模板项目执行本地图片处理/导出、PNG 尺寸/下载头、另一账号下载拒绝。fixture 按本轮精确空间和 ID 清理。

复现自动化时先停止 API/Worker，保持 CPU 图片服务单独就绪，再执行：

```powershell
. .\scripts\pod-env.ps1
go test ./internal/... ./pkg/... ./cmd/... ./sql/pod
$env:POD_TEST_CONFIG = Join-Path $PWD '.local/image-work-integration.yml'
$env:POD_IMAGE_AI_TEST_URL = 'http://127.0.0.1:18112'
go test ./test/pod -count=1 -json
go vet ./...
go build ./...
Push-Location studio
pnpm test
pnpm build
Pop-Location
```

上述本地测试配置不随 Git 分发；新机器先按 POD_LOCAL 初始化并建立明确的隔离测试配置。
原始集成输出在 `.local/template-20261112/integration.jsonl`。未设置 CPU URL 时跳过模型场景不能算真实 CPU 验收。

## 服务与费用

验收后已恢复脚本管理的 API 18110、创作端 5177、Worker 和 CPU 图片服务 18112，数据库/Redis 健康，Worker 心跳存在。
只读核对：预算 500 分、并发均 1、12 条真实调用、累计估算 900385 微元（0.900385 元）、预占 0、活动运行 0。
这些数值是本次快照；模型最终费用仍以供应商账单为准。

## 后续

下一项为图片交互优化：鼠标框选裁剪、原图/处理结果对比、T 恤居中与复位。本轮未开始该项。
完整登录浏览器闭环、模板付费生成样本、备份恢复、Linux 部署仍按后续交付验收单独完成；共享与模板市场不在当前范围。
