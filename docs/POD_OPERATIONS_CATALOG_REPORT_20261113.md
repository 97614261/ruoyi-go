# POD 第 6、7 步：任务运营与官方模板

日期：2026-11-13。此报告记录本轮实现与证据；当前规则以 CLAUDE、平台接口/数据文档为准。

## 已实现

| 范围 | 行为和入口 |
|---|---|
| 任务中心 | 后台 POD 管理 → 任务中心，运行/批量/私有授权三个 Tab。按日期、用户、模型、状态过滤，分页上限 100，唯一 ID 排序兜底 |
| 详情和操作 | 节点元信息分页；取消运行只请求原执行状态机；停止批次只取消未派发项，不释放已派发或未知调用份额 |
| 用量统计 | 按用户和模型汇总所有供应商调用尝试，日期最多 31 天；供应商估算 micros 与正式点数分别统计。调用点数预占不包含还未分配给调用的运行/批次份额 |
| 私有访问 | 功能权限 + operator/owner/单个 run 或 asset 的限时 grant；最多 24 小时，root 同样检查；成功读取及缺授权拒绝均审计，审计失败拒绝返回正文 |
| 官方模板管理 | 草稿创建、说明/排序编辑、追加不可变版本、发布指定版本、下架；保存和发布本身不执行模型 |
| 平台素材 | 独立 catalog 根、图片检查/缩略图、使用权说明、鉴权预览；私人素材不能用 owner 别名直接变平台素材 |
| 创作端 | 工作流模板 → 官方模板，只显示已发布版本。预览、换文字/个人图片，复制为独立私有项目；复制不扣生成点数，图片计入个人存储 |
| 可靠复制 | 原请求/固定版本幂等；空间锁下记录完整文件计划和存储预留，事务外按 SHA 复制，最终事务创建素材、项目、引用并结算；中断用原请求继续，不新建第二份 |

后台元信息不返回项目名、图、节点输入输出、存储键、租约、供应商回执或凭据。私有内容对话框关闭即清除内容和对象 URL，不存浏览器草稿。
管理写入仅在 tab 内保存安全操作编号与目标/版本；原因、图正文和文件需重填原内容，服务端摘要拦截内容变化。
创作者复制的原意图按本人/空间保存在会话内，以便响应丢失时沿用原输入和 requestId。

## 数据与恢复

012 已先在 011 独立恢复副本演练，再安装本地 `pod_studio_dev`。新增六张 `pod_catalog_*` 表，均含五个审计字段；未回写 001—011 或修改继承系统结构。
模板图保存规范化摘要，输入槽位与输出从图派生；发布指针与草稿指针分离。复制文件键在 I/O 前落库，平台 ID 与私人 ID 显式映射。
平台根为 `StorageRoot/catalog`，私人接口只读原平面根。文件用完整临时文件加不覆盖式原子硬链接发布；同键仅接受相同字节，需要支持硬链接的本地文件系统。
备份 allowlist 只接受两种平面路径，拒绝目录穿越、额外目录和符号链接。合计上限 10000 文件/2 GiB，单文件 256 MiB，恢复不展开任意嵌套路径。

变更前 011 包：`.local/backups/bundle-20261113-185552-2187495993`，演练日志 `.local/official-upgrade-rehearsal.log`。
最终含页面验收数据的 012 包为 `.local/backups/bundle-20261113-210951-2320440266`（121 个校验文件），生成日志 `.local/official-page-final-bundle.log`。
独立恢复报告为 `.local/pod_studio_restore_20261113_211022_f48fd8d7-348482379/restore-report.json`，日志 `.local/official-page-final-restore.log`。
恢复核对 102 个私人文件、14 个平台文件和 2 个已落盘的中断复制文件 SHA256；官方目录含 8 个模板、15 个版本、15 次复制动作（14 次 completed、1 次 copying）。
版本/引用、复制归属及点数/存储守恒校验通过；中断计划尚未复制的文件允许缺失，已存在文件必须与持久计划一致。
历史一次失败测试留下的 `copying` 计划及 held 存储作为恢复证据保留，其专用账号已停用；没有删账或按 TTL 放款来制造通过。
主密钥仍独立保管，不进入普通 bundle。副本保持 `sourceUntouched=true`、`queueConnected=false`、`workerStarted=false`。

## 自动化证据

| 检查 | 证据 |
|---|---|
| POD 完整回归 | `.local/operations-official-regression-final.log`，包括原项目/素材/执行/模型/权益/点数/managed/模板/批量；最终版本通过，用时 19.334 秒 |
| 新专项 | `test/pod/operations_official_test.go`：草稿隔离、并发单结果、中断复制续作、个人替换/跨用户拒绝、版本不变/下架重放、元信息防漏、grant/撤销/实际到期/错误 owner、失败审计拒绝正文、取消幂等、日期与用量实账聚合及恢复不变量 |
| 管理执行安全 | `test/pod/managed_test.go`：管理停止后只有未派发份额释放，管理取消未知调用不释放预占、不重新 Submit；只读运营新增接口与写入拒绝见 admin_test.go |
| Go 单元/静态检查 | `.local/official-unit-closeout.log`、`.local/official-vet-final.log`；含不覆盖并发写盘、catalog 路径/损坏恢复、复制归属/中断文件核验和服务前置鉴权 |
| 创作端 | `.local/official-studio-tests.log`：18 个文件、120 项通过；新组件验证明确确认才复制、不自动执行、版本固定、响应丢失原请求与 copying 保留 |
| 管理端 | `.local/official-admin-tests.log`：17 项通过；授权原目标/到期时间重放且剔除原因/私有正文，大整数和资源边界 |
| 构建 | `.local/official-studio-build.log`、`.local/official-admin-build.log`；已有大 chunk 提示保留，未掩盖构建错误 |

可复跑（先停服务，测试配置必须为专用 simulation）：

```powershell
. ./scripts/pod-env.ps1
$env:POD_TEST_CONFIG=(Resolve-Path .local/image-work-integration.yml).Path
go test ./test/pod -count=1 -v
go test ./internal/service ./internal/repository ./internal/handler ./internal/workflow/... ./cmd/podsetup ./pkg/storage ./pkg/modelcatalog/... ./pkg/executor/...
go vet ./internal/service ./internal/repository ./internal/handler ./internal/router ./internal/workflow/... ./cmd/podsetup ./pkg/storage ./test/pod
pnpm --dir studio test
pnpm --dir studio run build
node --test web/test/*.test.mjs
pnpm --dir web run build:prod
```

## 页面与边界

实际登录管理端与创作端完成以下页面检查，使用原 `admin` / `pod_creator`，未改账号权限或密码，验证码保持开启：

- 管理端任务中心：运行任务、批量派发、私有访问授权三个 Tab；任务详情显示节点元信息。用量页显示按用户/模型汇总的调用、成本与点数。
- 官方模板：平台图片鉴权预览；创建 `local-ui-20261113`（模板 ID 8）、保存无模型的文字输入版本 ID 15，再发布此固定版本，列表显示“已发布”。截图 `.local/official-admin-page.png`。
- 创作端：模板 → 官方模板，只看到本次已发布条目；预览默认文字，修改项目名和文字，点击“复制并打开画布”。项目 4380 为独立私人副本，画布显示替换后的文字及版本 v1，未点击运行或润色。

取消/停止、授权/撤销、实际到期、审计故障、并发及中断续作主要由接口专项验证；本次页面没有新增私有访问 grant，也没有把所有分支宣称为页面逐一操作通过。
最终按当前代码重编译启动，schema 检查为 001—012，Worker 为 managed；启动日志 `.local/official-page-final-start.log`。创作端 5177、管理端 5178、API 18110。
本轮未导入真实 API Key、调用真实模型、增加 500 分累计预算或导入支付；离线供应商替身不能当作真实账单。
完整回归中真实队列崩溃恢复和 CPU AI 实体样本仍按专门环境开关跳过；worker helper 是子进程入口，直接运行跳过属于预期。历史证据保留，不宣称本轮重新测过。
Linux/TLS、手机真机、生产容量、千问多图/多参考的成功样本和供应商账单核对仍待验收。
后续第 8 步先做平台全链路收口与目标环境验收；支付、模板市场、团队协作、更多协议/供应商适配属于另行规划。
