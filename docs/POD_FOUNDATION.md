# POD 工程、数据与接口边界

更新：2026-11-13，同步至有界批量和 schema 001—007。项目契约见 [POD_STEP2_REPORT.md](./POD_STEP2_REPORT.md)，素材契约见 [POD_STEP3_REPORT.md](./POD_STEP3_REPORT.md)，执行与历史引用见 [POD_STEP4_REPORT.md](./POD_STEP4_REPORT.md)，图片能力见 [图片制作报告](./POD_IMAGE_WORK_REPORT_20261112.md)，模板见 [个人模板报告](./POD_TEMPLATE_REPORT_20261112.md)，批量见 [批量报告](./POD_BATCH_REPORT_20261113.md)。规划接口单独标明，不代表可以调用。

## 1. 复用与新增

| 能力 | 位置 | 当前边界 |
|---|---|---|
| 账号与验证码 | `internal/service/login.go`、`captcha.go`、token 与权限中间件 | 复用既有登录、Redis 会话与权限刷新；验证码保持开启 |
| 统一响应与错误 | `pkg/response`、`pkg/errs` | 普通业务响应沿用 HTTP 200 + `code/msg`；错误隐藏内部细节 |
| 上传校验 | `pkg/upload`、既有 common handler | 仅复用实现经验；既有 `/profile` 是公共资源，不能承载私有 POD 素材 |
| 生命周期 | `cmd/server/main.go` | 复用超时、信号与停机排空；新增 POD 启动保护 |
| 代码生成 | `internal/service/gen*` | 项目模块已从现有生成器产出骨架，再补归属、版本和独立 DTO；其他 CRUD 继续遵守该流程 |
| 图校验 | `internal/workflow` | 纯计算，无数据库、网络或执行器 |
| 创作端 | `studio/` | 登录、项目与回收站、可编辑画布、七类配置、私有素材与节点目录；管理后台本轮不开发 |

新增业务仍遵守 handler → service → repository；只有 repository 持有数据库连接。
项目、素材与任务等资源访问必须同时具备功能权限和工作空间访问权。当前只实现个人空间，不支持团队邀请。

## 2. 当前接口与授权

| 接口 | 参数/响应 | 授权 | 是否新增 |
|---|---|---|---|
| `GET /captchaImage` | 原契约 `captchaEnabled/img/uuid` | 公开 | 否 |
| `POST /login` | `username/password/code/uuid`；顶层 `token` | 账号、密码与验证码 | 否 |
| `POST /logout` | 原会话退出契约 | 登录会话 | 否 |
| `GET /pod/bootstrap` | `code/msg/data`，空间、身份、节点规格与能力开关 | 登录 + `pod:studio:access` + 有效个人空间成员关系 | 是 |
| `POST /pod/workspace` | 显式初始化个人空间，已有有效空间则返回 | 登录 + `pod:studio:access`；不能恢复已停用空间或撤销的成员 | 是，步骤 2 |
| `/pod/projects` 及其详情/恢复路由 | 项目分页、创建、读取、保存、软删除和恢复 | 登录 + `pod:studio:access` + 本人有效空间 | 是，步骤 2，方法与 body 见阶段报告 |
| `/pod/assets` 及其详情/内容/下载/恢复路由 | 素材分页、上传、鉴权二进制、软删除与恢复 | 登录 + `pod:studio:access` + 本人有效空间，引用中的素材不允许删除 | 是，步骤 3，方法与 body 见阶段报告 |
| `/pod/runs` 及详情/取消/恢复，`GET /pod/credits` | 固定图快照、分页历史、单节点/整条测试与账本 | 相同创作权限及私有空间；写入/领取时再次复核 | 是，步骤 4，详见阶段报告 |
| `/pod/templates` 及详情/版本/复制/使用/恢复路由 | 个人模板分页、保存项目快照、新版本、固定版本输入替换并创建项目 | 登录 + `pod:studio:access` + 本人有效空间，历史素材保留引用 | 是，步骤 7，详见模板报告 |
| `GET /health` | 原健康检查契约 | 本地探针 | 否 |
| `GET /pod/usage` | 本人累计调用统计与分页运行明细，见下文 | 登录 + `pod:studio:access` + 本人有效空间和成员；不支持切换 userId/workspaceId | 是，后续第 4 项；无迁移 |
| `GET /pod/batches`、`POST /pod/batches/preview`、`POST /pod/batches` | 分页批次、逐项校验预览、幂等创建 | 登录 + `pod:studio:access` + 本人有效空间和成员 | 是，后续第 3 项；007 |
| `GET /pod/batches/:id`、`POST /pod/batches/:id/stop`、`GET /pod/batches/:id/download` | 每项实际状态、停止待派发项、成功归档 ZIP | 相同权限，不能跨账号查批次或下载 | 是，后续第 3 项；007 |

创作端开发代理将 `/api` 去前缀后转发给 `127.0.0.1:18110`，后端没有 `/api/pod/bootstrap` 路由。

`bootstrap.data` 包含：`schemaVersion`、`userId`、`userName`、`nickName`、
`workspace:{workspaceId,name}`、`nodeTypes`、`capabilities`。
`userId` 和 `workspaceId` 是十进制字符串，避免 JavaScript bigint 精度损失。
能力 `projectEditing/assets=true`；显式 simulation/qwen 模式下 `execution=true`。两种模式均开放 textInput/imageInput/textGenerate/imageProcess/productPreview/export，qwen 另开放 imageGenerate；simulation 的 textGenerate 是测试输出。缺省或 disabled 模式不接受执行提交。
`execution` 返回有效模式、并发、等待上限、初始测试单位和 `imageAIEnabled`，qwen 模式另返回 textModel/imageModel；不返回供应商凭据或内部 `imageAIURL`。
新增 `limits:{maxNodes,maxEdges,maxGraphBytes}`，供前端与服务端按同一上限校验。
`assetLimits` 返回上传字节、像素、最长边与节点选图数量上限。

角色边界：

| 角色 | 创作空间 | 系统管理接口 | 其他用户空间 |
|---|---|---|---|
| `pod_creator` | 仅本人有效空间 | 无权限 | 无权限 |
| `pod_creator_b` | 仅本人有效空间 | 无权限 | 无权限 |
| `admin` | 默认未开通空间，返回业务码 403；可显式初始化自己的空间 | 沿用管理员权限 | 不绕过 POD 成员检查 |
| 未来平台运营 | 尚未创建 | 后续按接口单独授权 | 不自动获得私有素材权限 |

个人空间查询同时校验 `owner_user_id`、空间状态和成员状态，GET 不自动创建空间。
客户端提交 `workspaceId` 查询参数不能选择其他空间。成员或空间停用后，下次查询立即拒绝。

新增接口前的七项核对：

| 问题 | bootstrap 的答案 |
|---|---|
| 分页 | 不是业务列表；返回单个空间和代码内固定的七类规格，不无限枚举库数据 |
| 索引 | `pod_workspace.owner_user_id` 唯一索引；成员联合主键及用户索引 |
| N+1 | 一个带成员 JOIN 的查询；节点规格来自内存定义 |
| 超时 | HTTP 请求 context 传递到 repository，使用既有 DB 超时机制 |
| 权限 | `pod:studio:access` + owner 与有效成员检查 |
| 错误 | `pkg/response`；401/403 为业务码，内部错误不透传 SQL、密码或连接串 |
| JSON | 显式 camelCase；数据位于 `data`，IDs 为字符串 |

项目、素材、运行、测试额度、个人模板和有界批量已实现，具体请求结构见各阶段报告。共享模板和市场接口尚未注册。

### 有界批量契约

创建/预览 body 为 `requestId/templateId/version/name/executionMode/items`，每项为 `name/bindings`；
binding 沿用模板的 `nodeId/text/assetIds`，不能改任意处理节点。每批 1—10 项，逐项输入不做笛卡尔积。
预览返回 `items/textCalls/imageCalls/images/reservationMicros/testCredits`，不创建运行或预留预算。
模板版本和绑定图固定，创建的每项独立保存。IDs、version、金额均为十进制字符串。

创建以本人空间 + requestId 去重；相同摘要返回原批次，不同内容返回 409。
每人 20、实例 200 项待派发；派发沿用原运行的队列、并发与预算，不保证全批均有预算。
批次 `active/dispatched/stopped` 仅表示派发状态；每项连接实际 run 状态。
停止只取消 waiting，已派发运行仍由原运行详情取消/恢复，未知调用不自动重发。
列表稳定分页最多 100；详情最多 10 项；服务超时沿用读 5 秒、创建 10 秒、ZIP 30 秒。
ZIP 本人鉴权，含成功节点本地归档图片、排除图片输入，按 `item-NN/print|mockup/asset-ID.ext` 分类；
100 图、100 MiB 原始字节、同时两个打包，逐图 SHA256 校验，不返回私有回执或存储键。

### 创作者用量契约

`GET /pod/usage?pageNum=1&pageSize=10` 返回统一 `code/msg/data`，响应禁止缓存。
`data` 含 `summary/rows/total/budgetScope/budgetLimitMicros`，budgetScope 固定为 instanceCumulative，
上限来自当前 qwen 配置；模拟或禁用时为 "0"（不表示可以免费使用真实模型）。不返回全局已用或剩余金额。
`summary` 为本人当前空间全历史，含 runCount/failedRuns/pendingRuns，以及
chargedMicros/reservedMicros/providerCalls/pendingCalls/generatedImages。
`rows` 为稳定分页的逐次运行，含相同调用统计和 runId/projectId/projectName/state/attempt/testOnly/createTime。
IDs 与金额用十进制字符串，计数用整数；页大小最多 100，无数据 rows 为 []，越界页仍返回全历史汇总。

累计费用来源为所有持久调用记录，不受节点当前 output 或后续恢复覆盖影响；
reservedMicros 是未结清的预占，不等于确定扣费。pendingCalls 包含正在执行但尚未确认的调用。
generatedImages 仅为已确认调用记录的图片数，包含归档未完成图片，不代表可下载张数，不含本地处理或模拟输出。
失败/待确认运行数按当前状态统计，不是历史失败尝试次数。完整原始响应、供应商 URL、请求标识和他人统计不进入 DTO。
所有查询在只读一致快照事务内完成，有 5 秒服务超时，分页统计批量聚合；复用已有空间/运行索引，无新 SQL 迁移。

人工核对只完成权限、审计与幂等要求设计，没有改费、释放未知预占或重发未知调用的写接口。
详见 [用量设计](./POD_USAGE_DESIGN.md) 与 [验收报告](./POD_USAGE_REPORT_20261112.md)。

### 图片交互与运行恢复

不新增接口、配置字段或迁移，继续使用现有项目保存、素材原图和运行详情 GET。
裁剪以旋转后图片为坐标系，保存既有比例字段，像素宽高由两端分别向下取整相减。
T 恤预览沿用 1000 × 1200 画布与 `[325,300,675,850]` 印花区域，居中与复位只更新现有印花字段。
框选和印花拖动只在结束时一次修改画布；取消或换图不能保存未完成参数。

对比只沿素材的实际 parentAssetId 读直接输入，两张原图各自鉴权；不公开原图地址或凭据。
项目按账号/空间/项目在当前标签页保留所选运行编号，刷新只读恢复，不是执行授权。
非终态轮询不得产生新任务；运行面板与编辑器避免重复轮询，旧响应与失效权限不得恢复旧图。
状态注明来源快照版本，不能把旧结果表示成修改后参数已经成功执行。

### 个人模板契约

`GET/POST /pod/templates`；`GET/PUT/DELETE /pod/templates/:id`；`POST .../restore`；
`GET/POST .../versions`；`GET .../versions/:version`；`POST .../copy`；`POST .../use`。
分页 `pageSize` 上限 100，模板按更新时间和唯一 ID 倒序，版本按唯一版本号倒序；列表不带整图。
所有 ID、revision、version 为十进制字符串，详情返回 graph/slots/outputs；槽位为 textInput/imageInput，输出位置为无下游的终端节点。

保存 body 为 `requestId/projectId/projectRevision/name/description`，追加版本另带模板 `revision`；
服务端只读取已保存的来源项目，并在写事务内重新核对来源 revision。元信息修改为 `name/description/revision`，
软删除及恢复为 `revision`。复制 body 为 `requestId/version/name`；使用为 `requestId/version/name/bindings`，
每个 binding 仅包含 `nodeId` 和对应 `text` 或 `assetIds`，不能借此修改处理节点。

版本图不可变；使用生成独立项目，保存来源标记与动作记录，不带运行结果和费用。新增版本不影响已创建项目。
创建、新版本、复制和使用用规范 UUID requestId + 请求摘要去重；同键不同参数返回业务码 409。
陈旧 revision 返回 409，缺失或跨空间资源返回 404，非法输入返回 400，均保持 HTTP 200 + 统一响应。
历史版本和回收站继续保留素材引用，写入、引用同步与幂等记录同事务完成；保存/复制/使用不自动执行模型。

## 3. 图草稿契约 v1

```json
{
  "schemaVersion": 1,
  "nodes": [
    {"id":"brief","type":"textInput","version":1,"position":{"x":80,"y":100},"config":{}},
    {"id":"art","type":"imageGenerate","version":1,"position":{"x":400,"y":100},"config":{}}
  ],
  "edges": [
    {"id":"brief_art","source":"brief","sourcePort":"text","target":"art","targetPort":"text"}
  ]
}
```

这是可保存的结构示例，可在 `PUT /pod/projects/:id` 中作为 graph 提交，并附 revision；不是执行请求。
步骤 2 已按类型限制 config，步骤 3 放开 `imageInput.assetIds` 至最多 8 个不同的规范 ID 字符串，
项目保存事务校验素材归属并同步引用。提交运行时另行校验必填输入、执行模式、模型、AI 服务配置与导出格式，
不能把草稿结构和字段合法当作可执行。兼容旧草稿的 WebP 导出配置可保存，执行前要求改为 PNG/JPEG。

| 节点类型 | 输入端口 | 输出端口 |
|---|---|---|
| `textInput` | 无 | `text` |
| `imageInput` | 无 | `images` |
| `textGenerate` | 必填 `text`，可选 `images` | `text` |
| `imageGenerate` | 必填 `text`，可选 `images` | `images` |
| `imageProcess` | 必填 `images` | `images` |
| `productPreview` | 必填 `images` | `images` |
| `export` | 必填 `images` | `files` |

端口键与值类型当前一致，为 `text/images/files`，不另设单图类型；单图是只有一个元素的图片集合。
图内部集合合并、隐式批量展开和选图策略尚未实现，不允许靠多连线产生笛卡尔积；有界批次是图外的明确逐份绑定。

草稿校验规则：

- `schemaVersion=1`，已知类型的节点 `version=1`；未知类型或版本拒绝。
- `nodes/edges` 必须是数组，可以为空；默认最多 100 节点、300 连线、整图 1 MiB，可在 POD 配置中下调数量、调整整图大小。
- 节点与边 ID 各自唯一，匹配 `[A-Za-z0-9_-]{1,64}`；位置为有限数值，绝对值不超过 1,000,000。
- 每个 config 为对象且不超过 64 KiB；不包含模型密钥、登录凭据或本地文件路径。
- 端口必须存在且类型一致，一个目标输入端口最多一个上游；允许输出连接多个下游。
- 拒绝自环、缺失节点、重复边标识与有向环；草稿允许暂缺必填连线。

纯图校验不访问数据库，素材归属由项目写事务验证。未来执行时还须重新验证素材、完整性、模型能力和额度，不能只信任曾成功保存的图。

运行状态暂定义为 `queued/running/succeeded/failed/cancelRequested/cancelled/awaitingReconciliation`。
第四阶段落地运行表、节点级状态/阻断/恢复、取消与测试执行器；第五阶段增加千问适配与调用记录。
本机已为 qwen；短文字、单参考图、五种比例和两张生成的真实证据见 POD_BOUNDARY_ACCEPTANCE_20261112，未覆盖组合不能据此视为已验收。
第五阶段沿用 `/pod/runs`，真实提交必须显式指定 `executionMode: "qwen"` 且不接受 testScenario。
详情中的 `nodes[].output.usage` 提供模型、返回标识、Token/图片数与字符串 `estimatedCostMicros`（百万分之一元）；
内部 submission key、原始调用记录和签名地址不返回前端。模式和模型随运行固定，真实与模拟的单节点上游不能互用。

### 节点内操作

`POST /pod/runs` 的 mode 支持 `workflow/node/polish/branch`，响应 mode 使用相同值。
两种新模式仍要求本人有效空间、保存图 revision、nodeId、requestId 和显式 executionMode。

| mode | 执行范围 | baseRunId |
|---|---|---|
| workflow | 保存图的全部可执行节点 | 不接受 |
| node | 单个目标，按需复用同版本/同模式的成功上游 | 可选；不能来自 polish |
| polish | 仅非空文本输入，派生原文到文本生成的快照 | 不接受 |
| branch | 目标及全部祖先；排除无关分支和下游 | 不接受 |

`polish/branch` 只派生运行快照，不改项目图或 revision；润色的生成节点 ID 等于原目标 ID。
润色结果由用户在创作端确认后作为普通项目编辑保存。分支上游重新执行，可能产生额外模型费用。
原有幂等键、outbox、租约、预算和结果保留规则继续生效，未增加 SQL 迁移。
具体保护与验证证据见 [节点交互记录](./POD_NODE_INTERACTIONS_20261112.md)。

### 本地图片处理与输出

继续使用现有运行/素材路由、权限、快照、租约和恢复机制。每节点接收 1—8 张图片，按输入顺序处理并输出对应的素材 ID；不展开额外付费调用。

| 节点 | 配置 | 执行约束 |
|---|---|---|
| `imageProcess` | `operation` 为 resize/crop/removeBackground/upscale；`rotate` 为 0/90/180/270；尺寸 `width/height`；归一化裁剪 `cropX/cropY/cropWidth/cropHeight` | 先旋转再裁剪/等比缩放；裁剪起点加宽高不得越界；输出最长边 8192、最多 2400 万像素，另受素材字节上限约束 |
| 去背景/放大 | `method: basic/ai`；`scale: 2/4`；纯色去背景用 `background/tolerance` | basic 为边缘连通颜色移除或 Catmull-Rom 插值；ai 为本机 CPU 模型，未配置则拒绝，不静默降级 |
| `productPreview` | `product: tshirt`、`side: front/back`、`color`、`printX/printY/printScale` | 1000×1200 原创二维示意；位置 0—1；大小限制在印花区域，旧草稿 scale 最高 1.5 兼容读取，渲染限制为 1 |
| `export` | `mode: print/mockup/image`、`format: png/jpeg`、`fileName`、`printWidthMM/printHeightMM` | PNG 保留 alpha，JPEG 铺白底；毫米范围 1—2000；按像素与毫米计算有效 DPI，低于 150 提示，不能据此承诺生产商验收 |

`nodes[].output.images` 按输出顺序返回 `assetId/width/height/kind/format/effectiveDpi/warning`，
全部使用 camelCase。素材 ID 继续使用字符串；有效 DPI 不是修改图片标签，也不会凭空增加细节。

派生素材保留 `parentAssetId`，来源 `source` 分别为 `processed/mockup/export`；
`remark` 中的 `pod-kind:artwork` 或 `pod-kind:mockup` 作为来源分类标记。不得随意改写这些标记。
授权后的数据库来源才是判断依据，不能相信客户端 output 元数据。
商品展示图经重新选入 imageInput、尺寸处理或普通导出仍保持 mockup 分类，禁止当作印花原图或再次套 T 恤。
商品展示导出要求 mockup 来源，印花导出拒绝 mockup 来源。

复用 `pod_provider_asset` 的步骤键+图片序号映射及运行素材引用，归档中断后复用已完成图片，不增加表或修改 001—005。
输入与已有归档元数据分别批量授权查询，文件逐张读取与处理，不在 service 内自行查库。
`pod.execution.imageAIURL` 只接受字面回环根地址 `http://127.0.0.1:端口`，不返回前端；
请求只传图片字节，禁代理/重定向，单进程 AI 并发 1、请求边界 120 秒。服务安装与分发边界见 [本机 AI](../scripts/image-ai/README.md)。

## 4. 已安装的数据结构

迁移位置：`sql/pod/migrations/001_foundation.sql`、`002_assets.sql`、`003_asset_node_identity.sql`、`004_execution.sql`、`005_qwen.sql`、`006_templates.sql`、`007_batches.sql`。
独立系统基线副本位于 `sql/pod/baseline/`，许可证随附。
现有系统表结构不改，POD 新表均带 `create_by/create_time/update_by/update_time/remark`。

| 表 | 主要内容 | 索引/约束 |
|---|---|---|
| `pod_schema_version` | 版本、SQL checksum、实例 ID、安装状态 | `version` 主键 |
| `pod_workspace` | 空间名称、owner、启停 | `workspace_id` 主键，owner 唯一 |
| `pod_workspace_member` | 成员、角色、启停 | `(workspace_id,user_id)` 主键；`(user_id,status,workspace_id)` 索引 |
| `pod_project` | 名称、图 JSON、schema、revision、逻辑删除 | 主键；`(workspace_id,del_flag,update_time,project_id)` 索引 |
| `pod_asset` | 原图归属、名称、类型、大小、尺寸、版本、删除状态 | 主键；空间/状态/创建时间/ID 索引 |
| `pod_asset_file` | 原图/缩略图文件键、类型、大小、尺寸、SHA256 | 素材+variant 主键，文件键唯一 |
| `pod_project_asset_ref` | 项目、节点与素材引用 | 项目+节点+素材主键（node_id 使用 ascii_bin），素材引用索引 |
| `pod_run`、`pod_node_run` | 固定快照、运行与节点状态、输入输出、租约 | 空间+请求键唯一、历史分页、运行+大小写敏感节点主键、状态/租约及用户/供应商索引 |
| `pod_run_asset_ref`、`pod_outbox` | 历史素材保留与可靠唤醒 | 运行+素材主键、素材索引、active+投递时间+运行索引 |
| `pod_credit_account`、`pod_credit_event` | 整数测试账户及预占/结算/释放 | 空间主键、空间+事件键唯一、空间/运行流水索引 |
| `pod_retry_request`、`pod_execution_gate` | 恢复请求去重与短事务门闩 | 空间+恢复键主键、单行门闩主键 |
| `pod_provider_call` | 模型调用意图、私有结果记录、估算预占和费用 | submission key 主键、运行/节点及状态索引 |
| `pod_provider_asset` | 供应商调用或本地图片步骤的第几张输出对应哪个私有素材 | submission key+图片序号主键、素材唯一 |
| `pod_template` | 本人模板名称、说明、当前版本、revision 和回收站 | 主键；空间/状态/更新时间/ID 索引 |
| `pod_template_version` | 不可变的版本图快照 | 模板+版本主键；空间/模板/版本索引 |
| `pod_template_asset_ref` | 每个历史版本保留的图片引用 | 模板+版本+大小写敏感节点+素材主键；素材与空间索引 |
| `pod_template_action` | 幂等请求摘要和创建结果、使用来源 | 空间+requestId 主键；模板/版本及项目索引 |
| `pod_batch` | 固定模板版本、执行模式、幂等摘要和预览快照 | 空间+requestId 唯一；空间/创建时间/ID 索引 |
| `pod_batch_item` | 逐项绑定图、派发状态、独立项目与运行编号 | 批次+序号主键；状态/轮转时间、空间/等待状态、运行索引 |
| `pod_batch_asset_ref` | 历史批次输入素材保留引用 | 批次+素材主键；素材/空间索引 |

项目接口已在步骤 2 实现；保存、重命名、软删除与恢复统一增加 revision，陈旧版本拒绝写入。
删除状态为 `del_flag='2'`，内容保留；尚无永久清理接口。
当前没有数据库外键；后续写入必须在 repository 事务内验证资源关系，不能依赖表结构自动保证授权。

初始化只接受新库；已有独立 POD 环境通过本地 migrate 安装缺失的 002/003/004/005/006/007，不复用 init。
DDL 非事务操作，失败保留 installing 和已完成部分，不自动删除重建或盲目重试。
001 checksum 覆盖规范化换行后的系统基线与 001 SQL，002/003/004/005/006/007 分别校验自身 SQL；启动验证七个版本、实例与 ready 状态，
这不是逐表比对物理结构的 schema drift 检测。后续变更必须新增迁移和升级方案，不修改已安装的 001 来绕过检查。

## 5. 环境保护

默认启动读取 `configs/pod.local.yml`，先 `ValidatePOD` 再连接数据库。
MySQL 库名必须符合 `pod_studio_*` 并与显式声明一致，运行 DSN 禁止 `multiStatements`；
Redis 使用声明的独立非 0 逻辑库，Lua 原子声明 `pod:instance`，其他实例或没有标记的非空库拒绝。
JWT 为独立随机密钥，私有素材目录不能与 `/profile` 公共目录相同或互相包含。

本机步骤 4 已为 POD 切至独立 Redis 6387、DB 15 并验证归属和持久化重启，启用执行时检查 Redis 4+。这不代表生产 DB 权限、Redis 网络安全或路径符号链接防护已经验收。
当前本地 API 和 Vite 只监听 `127.0.0.1`；私有素材读取已实现，生产账号、网络、TLS 仍需部署阶段落实。
