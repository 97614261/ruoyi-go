# POD 平台权限与接口清单

日期：2026-11-13。第 2—7 步管理基础、模型目录、创作者、managed 执行、任务用量/授权及官方模板接口均已注册。实际范围见本页及 [任务与官方模板报告](./POD_OPERATIONS_CATALOG_REPORT_20261113.md)；未提供人工释放未知费用或收费重试的管理接口。
总设计见 [POD_PLATFORM_DESIGN.md](./POD_PLATFORM_DESIGN.md)，数据见 [POD_PLATFORM_DATA.md](./POD_PLATFORM_DATA.md)。

已实现的辅助只读接口：`GET /pod-admin/creator-limits` 需 `podAdmin:creator:list`，返回实例可设置上限及 writesReady；`GET /pod-admin/creator-model-options` 需 `podAdmin:creator:grant`，只返回模型身份/类型/启停，无成本/凭据，按 name 分页。

## 1. 角色和授权

第 2 步实现 `GET /pod-admin/bootstrap`、`GET /pod-admin/overview`、`GET /pod-admin/audits`、`GET /pod-admin/audits/:id`。
第 3 步实现表内 providers、models（含 versions/check-config）、model-defaults 和 actions 的全部目录接口，见 [模型目录报告](./POD_MODEL_CATALOG_REPORT_20261113.md)。
第 4 步已注册 creators 的 list/get/create/policy/models、points/point-events/point-adjustments，以及 creator-limits、creator-model-options；本人 /pod/eligibility、/pod/points 已注册，见 [权益报告](./POD_CREATOR_POLICY_REPORT_20261113.md)。
管理任务/用量、private grant/私有读取和官方模板接口已注册。管理写入及模板复制不触发模型调用。
overview 当前为最多 31 天的运行/批次/调用成本汇总及当前空间、项目、素材、文件存储计数；creators 字段暂统计启用的个人空间，与 creator policy 状态分开，不作为有效权益人数。
bootstrap 当前返回 stage=7、schemaVersion=012，overview/audits/models/creators/tasks/templates/usage/privateContent 均按具体权限返回；privateContent=true 仅表示具备读取功能权限，每次仍须匹配具体资源 grant；catalogWrites 只表示独立主密钥就绪，实际写入仍检查具体权限。无管理权限的访问与成功进入后台同步写入 `admin.access` 审计；查询不采集正文。
审计现支持 `admin.access` 及 provider.create/update/credential/status、model.create/update/status/version、defaults.update、creator.create/policy/models、points.adjust、task.cancel、batch.stop、private.grant/revoke/read、official.create/update/version/publish/unpublish/asset；结果支持 success/denied/failure。仅开放已实现动作。

| 能力 | 平台管理员 | 受限运营默认值 | 普通创作者 |
|---|---|---|---|
| 登录管理端/读概览 | 允许 | 显式赋予后允许 | 禁止 |
| 开通/停用创作资格、配置用户权益 | 允许 | 默认禁止，可单独授权 | 仅看自己的资格 |
| 供应商密钥、模型及默认值变更 | 允许 | 默认禁止 | 只能选自己被授权的模型 |
| 发放/扣减正式点数 | 允许，必须原因和流水 | 默认禁止，可单独授权 | 仅看本人点数/明细 |
| 全平台任务元信息与成本统计 | 允许 | 默认只读 | 仅本人任务/点数 |
| 取消运行、停止待派发项 | 允许 | 默认禁止，可单独授权 | 本人按现有规则操作 |
| 收费恢复或新模型调用 | 用创作者身份走报价执行 | 不提供代用户收费按钮 | 本人明确确认 |
| 官方模板编辑/发布 | 允许 | 默认禁止，可单独授权 | 读取已发布内容并复制使用 |
| 私有提示词、原图和结果 | 必须额外有效 grant | 权限 + grant，默认无 | 本人资源 |
| 凭据明文、供应商原始回执 | 不通过管理 DTO 返回 | 禁止 | 禁止 |

复用系统 RBAC。创作者继续有 pod:studio:access；管理端入口新增 podAdmin:access。
管理业务权限使用 podAdmin 前缀，不能因拥有 pod:studio:access 放行后台。
受限运营不授予 system:user/role/menu/config 的泛管理权限，创作者管理走专用接口，不能透传任意角色 ID。
HTTP 层检查会话和具体权限，service 检查资源范围、实时资格、模型范围和私有 grant；repository 事务内再次验证。
管理员通配权限只解决功能权限，不能跳过资源 grant 或伪装为资源 owner。

## 2. 通用契约

- 普通接口复用 `pkg/response`：HTTP 200 + `code/msg`；详情在 data，分页统一顶层 rows/total；没有数据 rows=[]。
- 401 未登录、403 功能或资格不允许、404 无资源或无权见、409 版本/幂等/报价冲突、400 参数错误，均为业务 code。
- 请求体超限等已有入口 HTTP 状态保持原约定；二进制接口失败在写文件头前返回统一 JSON。
- pageNum/pageSize，默认 1/10，最多 100；白名单筛选/排序，末尾唯一 ID；大导出不做无界同步返回。
- 所有 ID、revision、version、micros、点数和大字节计数用十进制字符串，业务计数为有限整数。
- 写操作 body 含 requestId（规范 UUID）、可变资源 revision、必要 reason；同键同内容返回原结果，同键异内容 409。
- requestId 在发送前持久化安全原意图；结果不明先查原操作，不换键重复发放或扣点。
- 列表/元信息 5 秒，普通写 10 秒，受控 ZIP 30 秒；所有 I/O 继承取消。外部调用由 Worker 使用模型超时，不放在管理 HTTP 事务中。
- 新敏感/私有响应 `Cache-Control: no-store`；Token 不进入查询参数；密钥、初始密码不经过通用缓存请求或通用正文操作日志。

## 3. 管理端接口

所有 `/pod-admin` 路由要求登录 + podAdmin:access，下表权限为额外要求；并不要求管理员拥有个人创作空间。

| 接口 | 权限 | 请求/结果要点 |
|---|---|---|
| `GET /pod-admin/bootstrap` | podAdmin:access | 安全管理身份、允许功能、schema/功能版本；无私有内容 |
| `GET /pod-admin/overview` | podAdmin:overview:query | 有限日期范围统计、任务/成本/存储与服务就绪；不返回路径或连接资料 |
| `GET /pod-admin/creators`、`GET .../:userId` | podAdmin:creator:list/query | 账号安全字段、资格/权益和统计；不返回密码哈希 |
| `POST /pod-admin/creators` | podAdmin:creator:add | username、nickName、write-only 初始密码、明确权益；事务创建账号/指定创作者角色/空间/零点数账户；不回传密码 |
| `PUT /pod-admin/creators/:userId/policy` | podAdmin:creator:edit | revision、status、validUntil、资源限制、reason；不能授予其他系统角色 |
| `PUT /pod-admin/creators/:userId/models` | podAdmin:creator:grant | revision、明确 modelIds 集合、reason；空集合表示无模型权限，不表示全开放 |
| `GET /pod-admin/creators/:userId/points`、`GET .../point-events` | podAdmin:points:query | 正式账户和分页流水 |
| `POST /pod-admin/creators/:userId/point-adjustments` | podAdmin:points:adjust | requestId、accountRevision、kind=grant/deduct、amount、reason；无 setBalance |
| `GET /pod-admin/providers`、`GET .../:id` | podAdmin:provider:list/query | 安全元信息、credentialConfigured、当前凭据版本及配置检查状态 |
| `POST /pod-admin/providers`、`PUT .../:id` | podAdmin:provider:edit | 有类型的协议、地域、官方 endpoint；无任意 Header/payload |
| `PUT /pod-admin/providers/:id/credential` | podAdmin:provider:credential | requestId、revision、明确 sourceType 和 write-only secret/envName；新增凭据版本，绝不回显正文 |
| `POST /pod-admin/providers/:id/status` | podAdmin:provider:edit | revision、enabled/disabled、reason；不删除历史调用 |
| `GET /pod-admin/models`、`GET .../:id`、`GET .../:id/versions` | podAdmin:model:list/query | 目录/分页不可变版本，成本规则仅管理端可读 |
| `POST /pod-admin/models`、`PUT .../:id` | podAdmin:model:edit | 目录元信息及 revision，不能覆盖已发布版本 |
| `POST /pod-admin/models/:id/versions` | podAdmin:model:edit | requestId、revision、官方 ID、能力、成本和点数规则；完整校验后新增版本并切当前指针 |
| `POST /pod-admin/models/:id/status` | podAdmin:model:edit | revision、enabled/disabled、reason；阻止新的外部调用 |
| `POST /pod-admin/models/:id/check-config` | podAdmin:model:check | 仅验证规格、适配器、凭据来源存在及 endpoint；不调用生成、不证明 Key 有效 |
| `GET /pod-admin/model-defaults`、`PUT ...` | podAdmin:model:query/edit | 平台 revision、textModelId/imageModelId；已停用/无有效版本不能设默认 |
| `GET /pod-admin/runs`、`GET .../:id`、`GET .../:id/nodes` | podAdmin:task:list/query | 有限时间、用户/模型/状态筛选；元信息和安全失败原因，去除提示词/graph/output 私有正文 |
| `GET /pod-admin/batches`、`GET .../:id` | podAdmin:task:list/query | 分页批次，详情最多 10 项；派发状态和运行状态分开 |
| `POST /pod-admin/runs/:id/cancel`、`POST /pod-admin/batches/:id/stop` | podAdmin:task:cancel | requestId、reason；复用原状态机，不直接改终态或清除预占 |
| `GET /pod-admin/usage` | podAdmin:usage:query | 有限范围、按用户/模型成本与点数分别聚合，包含恢复尝试 |
| `GET /pod-admin/audits`、`GET .../:id` | podAdmin:audit:list/query | 安全审计字段，无 secrets/原始请求正文 |
| `GET /pod-admin/actions/:requestId` | 对应原操作权限 | 仅当前 actor 的原操作安全结果；无权者不能据 requestId 探查他人 |
| `POST /pod-admin/private-grants`、`POST .../:id/revoke` | podAdmin:privacy:grant | 指定 operator、owner、run/asset、reason（保存为 purpose）、expiresAt、<=24h；不提供全用户通配授权 |
| `GET /pod-admin/private/runs/:id`、`GET /pod-admin/private/assets/:id/content`、`GET .../download` | podAdmin:private:read + 有效 grant | service 检查 grant，即使 root；读取前同步审计，文件继续鉴权，不发公共 URL |

普通运营默认只授予 access、overview、creator 元信息、task 元信息和 usage 只读权限。
当前目录写成功与业务数据、安全审计和幂等 action 结果同事务；服务校验拒绝写固定脱敏失败事件，不把请求 body 交给通用 OperLog。
无效 Content-Type、坏 JSON/未知字段或入口请求体超限在解析阶段拒绝，不保证生成业务变更审计。
目录 DTO 不返回 secret、密文、nonce、envName、keyId 或请求指纹；请求不进入通用正文缓存。
真实模型试跑用专用创作者账号走正常报价、点数、预算和 Worker，不提供绕过费用控制的管理员裸 Submit。

### 点数调整例

```json
{
  "requestId": "54d19c7d-8128-4f67-97ba-400f759633ea",
  "accountRevision": "3",
  "kind": "grant",
  "amount": "1000",
  "reason": "首次试用额度"
}
```

1000 只是协议示例，不自动发放，也没有与人民币的固定兑换含义。

### 凭据提交特别约束

web 的凭据/初始密码使用专用请求封装，禁 sessionStorage/localStorage 草稿、重复提交正文缓存、
响应回显、错误对象打印和通用正文日志；提交后清空内存输入。
重试只持久保存 requestId、providerId、expected revision 等安全元信息；查原 action 确认完成后再允许下一次变更。
幂等摘要中的秘密使用专用域隔离 HMAC 指纹，主密钥服务端持有，不能用明文或直接 SHA256 存密码/Key 指纹。
同 requestId 但不同秘密必须 409，不因排除了 secret 而错误返回成功。
浏览器接口无权读取任意服务端环境变量，只能引用部署配置允许的凭据变量名称。

## 4. 创作端新增与扩展

全部是登录 + pod:studio:access；/eligibility、/points 允许本人看到资格受限原因，其他资源仍查 owner/成员/状态。

| 接口 | 用途与边界 |
|---|---|
| `GET /pod/eligibility` | 本人资格、到期时间、有效限制和正式账户摘要，不接受切换 userId |
| `GET /pod/models?kind=text|image`、`GET /pod/models/:id` | 仅本人可用目录、能力和点数规则；不带凭据、endpoint 或平台成本规则 |
| `GET /pod/points`（账户和分页流水） | 正式点数、预占和分页流水；不混原 /pod/credits |
| `POST /pod/execution-quotes` | kind=run/batch/retry，加原执行描述；服务器确定模型版本/点数；不提交模型调用 |
| `GET /pod/execution-quotes/:quoteId` | 仅本人报价，返回有效期、状态、可见点数和模型选择；不泄漏平台全局余额 |
| 已有 `POST /pod/runs` | managed 新增 quoteId；原 requestId、projectId、revision、mode、目标/baseRun 保留，必须匹配报价源 |
| 已有 `POST /pod/runs/:id/retry` | managed 恢复新增 quoteId；只预占新增调用，已确认失败且用户显式确认，未知不重发 |
| 已有 `POST /pod/batches` | managed 新增 quoteId，逐份输入和固定模板版本继续匹配摘要；同事务持有全批点数 |
| 已有 `POST /pod/batches/preview` | 保留旧模式兼容；managed 用 execution-quotes 取得可确认报价，不能用本地硬编码预估替代 |
| 已有项目/模板保存 | textGenerate/imageGenerate config 新增 modelId；textInput 新增 polishModelId，供节点内润色；只保存选择，不在保存时执行 |

模型目录和管理配置都可能变，不能只靠前端列表缓存鉴权。
普通新任务报价若模型当前版本变化须重报价；恢复报价沿用原 run 模型/定价版本，不因默认或当前版本变化自动换价，仍复核启停及用户授权。
已有运行详情保留 reserved/spent 的历史含义；正式点数新增 billing 对象，不能把旧测试字段改单位。
managed 的 testOnly=false，本地节点在 managed 流程内也不是 simulation；所有 !=qwen 的旧判断必须逐处修正。
graph schemaVersion 仍为 1，本阶段增补字段白名单；后续旧客户端不理解新字段时不允许静默保存丢失选择。
新 API/Worker 升级必须停写同步，不能承诺带 modelId 的新草稿可以由旧二进制继续编辑。

报价请求示例：

```json
{
  "kind": "run",
  "run": {
    "executionMode": "managed",
    "projectId": "1896",
    "revision": "4",
    "mode": "workflow",
    "requestId": "bb03f608-e249-4ea3-8d09-ab4d9c7af9cb"
  }
}
```

报价响应 data 包含 quoteId、kind、state、expiresAt（带时区 ISO 时间）、points（十进制字符串）、lines、resourceType/resourceId；
lines 含 itemNo、nodeId、modelId/modelVersionId、name、points，以及 kind（text/image）、officialModelId、images（图片张数，文字为 0）。
新增展示字段全部读取该报价保存的模型选择快照，不查当前目录替换。selection/sourceHash/平台成本只在服务器保存，不回传；
提交传原执行描述和 quoteId，收费确认只针对该描述。模型/价格/资格变更或草稿变更使旧报价不可新消费。
HTTP 重试原已成功提交时先依据 requestId 查结果，同内容返回原 run，即使报价后来过期或已切换模式。
报价 10 分钟有效、每人最多 20 个有效未消费报价；报价阶段不预占。run/batch/retry 明确分型，不能混传。
run 和 batch 详情新增 billing={scheme:"managedPointsV1",held,spent,released}，数值均为十进制字符串；run 另带 executionMode。
恢复请求包含 requestId 和 quoteId；原失败节点固定原版本、原价，已成功或未知节点不能重复收费。

## 5. 官方模板接口

| 接口 | 权限/行为 |
|---|---|
| `GET /pod-admin/catalog/templates`、`GET .../:id`、`GET .../:id/versions` | podAdmin:template:list/query，分页管理版本 |
| `POST /pod-admin/catalog/templates`、`PUT .../:id`、`POST .../:id/versions` | podAdmin:template:edit，revision/幂等保存草稿，版本不可覆盖 |
| `POST .../:id/publish`、`POST .../:id/unpublish` | podAdmin:template:publish，发布指定版本/下架，审计必需 |
| `GET /pod-admin/catalog/assets`、`POST ...` | podAdmin:template:edit，平台素材分页/上传，要求授权说明和独立存储边界 |
| `GET /pod/catalog/templates`、`GET .../:id` | 创作者读取已发布安全版本，不返回未发布图、后台字段或私人引用 |
| `POST /pod/catalog/templates/:id/use` | requestId、固定 version、name、bindings；占用用户存储、幂等复制并创建本人项目，不自动执行 |
| `GET /pod/catalog/use-actions/:requestId` | 本人安全复制进度/项目结果，允许不明提交续作 |

公开发布是产品可见性，不代表供应商 Key、平台存储目录或所有用户素材公开；平台素材读取也使用有界、受控入口。

## 6. 新接口七项检查

| 必答项 | 约束 |
|---|---|
| 分页/上限 | 所有列表默认 10、最多 100；模型有限选择也分页；批次最多 10；图最大 100 节点 |
| 索引 | DATA 文档列出 owner/time/ID、状态、provider/model 和幂等键；实现时验证 EXPLAIN，不凭推断改旧索引 |
| N+1 | 模型/授权/版本/任务/成本均集合查询；已知有界内存循环不逐条查询数据库 |
| 超时 | 请求 context + 5/10/30 秒及 Worker 适配器上限；外部 I/O 不在数据库事务内 |
| 权限 | 管理入口+具体操作+数据范围；创作入口+owner+实时权益；私有内容另查 grant |
| 错误/秘密 | 统一响应和安全错误；凭据无缓存/正文日志，DTO 不直接序列化实体或回执 |
| JSON | camelCase、字符串 IDs/金额/点数；详情 data、列表 rows/total，旧接口单位不改 |


## 2026-11-14 配置提示与失败指导补充

管理模型列表和详情新增 `providerCredentialConfigured`（bool）、`providerAdapterKind`、`providerEndpoint`。
只返回已有管理权限允许查看的凭据存在性、协议和官方根地址，不返回 Key、密文或环境变量值。
创作端 `/pod/models` 继续不含这些管理字段、成本规则或凭据。`providerId` 筛选沿用既有接口。

后台输入成本使用人民币元字符串，最多六位小数；在浏览器以 BigInt 精确转为原 `costRule` 的 CNY micros 十进制字符串。
接口单位和历史不可变版本不变。点数仍是非负整数字符串，不自动与人民币兑换；未填写价格不等于免费。

已确认 HTTP 拒绝的本地错误码：401 `providerCredentialInvalid`，403 `providerAccessDenied`，404 `providerModelUnavailable`，
429 `providerRateLimited`，400/413/422 `providerInputRejected`。已知仅免费额度耗尽继续用 `providerFreeQuotaExhausted`。
这些为安全本地提示，不透传供应商响应正文。5xx、传输异常和不明调用继续保留预占、核对原调用，不自动重试。
`archivePending` 继续恢复原结果归档，不重新调用图片模型；以上没有增加人工结算或无条件恢复入口。
