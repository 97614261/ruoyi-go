# POD 平台数据设计

日期：2026-11-13。008—012 已在本地安装，当前实际 schema 为 001—012。012 实际范围与恢复见 [任务与官方模板报告](./POD_OPERATIONS_CATALOG_REPORT_20261113.md)。011 恢复与账本证据见 [执行报告](./POD_MANAGED_EXECUTION_REPORT_20261113.md)。
010 的范围与恢复证据见 [创作者权益报告](./POD_CREATOR_POLICY_REPORT_20261113.md)。
009 的范围与恢复证据见 [模型目录报告](./POD_MODEL_CATALOG_REPORT_20261113.md)。
008 的实际范围和证据见 [管理基础报告](./POD_ADMIN_FOUNDATION_REPORT_20261113.md)。
总设计见 [POD_PLATFORM_DESIGN.md](./POD_PLATFORM_DESIGN.md)，升级顺序见 [POD_PLATFORM_MIGRATION.md](./POD_PLATFORM_MIGRATION.md)。

## 通用约束

- 普通可编辑业务表包含五个 BaseEntity 审计字段。011 四张执行事实表按专门决议例外：报价/快照/调用有 create_time，绑定通过原 run/不可变版本追溯；owner、请求、点数事件和凭据版本构成执行证据，不提供通用编辑入口。不得据此省略普通 CRUD 审计。
- 主键使用 BIGINT 或明确复合键；外部 IDs、revision、版本、micros、点数和大字节计数以十进制字符串传输。
- 不可变版本/流水禁止普通 UPDATE 和 DELETE；更正通过新版本或冲正流水，唯一键由数据库兜底。
- request_id 使用规范 UUID，ASCII 二进制比较；node_id 沿用大小写敏感语义；有限枚举及 JSON 按有类型规格校验。
- 逻辑关系在 repository 事务验证，沿用现有无物理外键方案。禁止 GORM AutoMigrate 或直接改旧 SQL。
- 金额/点数采用 BIGINT 非负整数，所有乘加、上取整和字节求和检查溢出；MySQL 5.7 不依赖 CHECK 约束保证正确性。
- 所有新写操作有 revision 或幂等键；列表有唯一兜底排序、pageSize<=100。下表省略通用审计字段。

## 008：管理入口、平台状态与审计

| 表 | 关键字段 | 主键/唯一与查询索引 |
|---|---|---|
| `pod_platform_state` | state_id=1、revision、managed_enabled=false、default_text_model_id、default_image_model_id、legacy_run_max_id、legacy_batch_max_id、cutover_time | 单行主键；默认模型为可空，切换字段只允许专用切换流程修改 |
| `pod_admin_action` | action_id、actor_user_id、request_id、request_hash、action_type、resource_type/id、state、result_json | PK action_id；UNIQUE(actor_user_id,request_id)；(resource_type,resource_id,action_id) |
| `pod_admin_audit` | audit_id、actor_user_id、owner_user_id、action_type、resource_type/id、reason、before_json、after_json、trace_id、outcome | PK audit_id；(actor_user_id,create_time,audit_id)、(owner_user_id,create_time,audit_id)、(resource_type,resource_id,audit_id) |
| `pod_private_access_grant` | grant_id、operator_user_id、owner_user_id、resource_type=run/asset、resource_id、purpose、expires_at、revoked、revision | PK grant_id；(operator_user_id,resource_type,resource_id,expires_at,grant_id)；(owner_user_id,grant_id) |

sys_menu/sys_role/sys_role_menu 只增加新的 POD 菜单、角色与授权数据，不修改这些系统表结构或旧稳定标识。
菜单不使用猜测的固定空闲 ID；工具解析已存在项并分配，重复安装可精确识别，不能扩大旧角色权限。
admin_action 是管理幂等记录，不保存密钥/初始密码；涉及秘密的内容摘要用服务端域隔离 HMAC 指纹参与比较，
不保存秘密本身或其直接 SHA256。凭据变更同时核对 revision；同键不同秘密仍返回 409，指纹不进入 DTO/审计。
admin_audit 只存有类型的安全字段白名单，不能直接序列化整个请求、响应或数据库实体。

## 009：供应商与模型目录

已实现。当前仅支持 qwen-v1、北京地域及服务端白名单模型；目录与默认值不改变旧 qwen/simulation 执行。
供应商编码、协议、地域、endpoint 在创建后固定，换计费主体或地域新建供应商；新模型版本固定对应快照。

| 表 | 关键字段 | 主键/唯一与查询索引 |
|---|---|---|
| `pod_provider` | provider_id、provider_code、name、adapter_kind、region、endpoint、status、revision、current_credential_version_id | PK；UNIQUE(provider_code)；(status,provider_id) |
| `pod_provider_credential_version` | credential_version_id、provider_id、version、source_type=encrypted/env、key_id、nonce、ciphertext、env_name、state | PK；UNIQUE(provider_id,version)；(provider_id,state,credential_version_id) |
| `pod_model` | model_id、model_code、provider_id、kind=text/image、name、current_version_id、status、visible、sort、revision | PK；UNIQUE(model_code)；(provider_id,status,model_id)、(kind,visible,status,sort,model_id) |
| `pod_model_version` | model_version_id、model_id、version、official_model_id、adapter_kind/version、endpoint/region快照、capabilities_json、cost_rule_json、point_rule_json、timeout_seconds、spec_hash | PK；UNIQUE(model_id,version)；(model_id,model_version_id) |

新模型版本同时固定能力、成本和点数规则，避免三套独立版本自由组合；任一规则变更都新增版本。
model 和 provider 的当前状态是领取时的安全开关，不会覆盖版本快照。
相同官方模型可有多个地域/供应商配置，但不能把重复目录记录当成两个不同模型的适配验收。
env 凭据只存变量名，encrypted 只存密文；两种来源互斥。更换计费账号应建新 provider，而不是轮换凭据偷换归属。

成本规则 JSON v1：currency=CNY、unit=micros、文字按每百万输入/输出 Token 的整数单价、
图片按合法规格的每张整数单价。点数规则 JSON v1 使用每百万 Token 点数及每张点数。
计费阶梯、缓存 Token、思考 Token、异步查单等规格未被适配器支持时不得仅靠 JSON 开放。
capabilities_json 限制 64 KiB；服务端验证范围，不接受自定义 URL/Header/请求体模板。

## 010：用户权益、点数和资源

| 表 | 关键字段 | 主键/唯一与查询索引 |
|---|---|---|
| `pod_creator_policy` | user_id、workspace_id、status、valid_until、max_concurrent_nodes、max_pending_runs、max_daily_submissions、max_storage_bytes、revision、last_dispatched_at | PK user_id；UNIQUE(workspace_id)；(status,valid_until,user_id)、(last_dispatched_at,user_id) |
| `pod_user_model_grant` | user_id、model_id、enabled、revision | PK(user_id,model_id)；(model_id,enabled,user_id) |
| `pod_points_account` | user_id、available、held、spent、granted_total、deducted_total、revision | PK user_id；全部计数非负 |
| `pod_points_event` | event_id、user_id、workspace_id、run_id、batch_id、submission_key、event_key、kind、amount、available_after、held_after、spent_after、reason、actor_user_id | PK；UNIQUE(user_id,event_key)；(user_id,event_id)、(run_id,event_id)、(batch_id,event_id)、(submission_key,event_id) |
| `pod_user_daily_usage` | user_id、business_date、accepted_submissions | PK(user_id,business_date)；业务日期 Shanghai，不能依赖午夜重置任务 |
| `pod_storage_account` | user_id、used_bytes、held_bytes、revision | PK user_id；计入所有 POD 文件含回收站和缩略图 |
| `pod_storage_reservation` | reservation_id、user_id、operation_key、reserved_bytes、committed_bytes、state、lease_until、result_asset_ids_json | PK；UNIQUE(user_id,operation_key)；(state,lease_until,reservation_id)、(user_id,reservation_id) |

points 账户恒等式：`available + held + spent = granted_total - deducted_total`；deduct 只能从 available 扣。
grant/deduct 流水保存绝对正 amount 和明确 kind，不使用直接设置余额；同键不同内容 409。
流水 kind 为 grant/deduct/hold/settle/release/transfer，transfer 表示批次持有额转给项，不改变账户总 held。
部分结算、失败恢复、批次停止均按唯一事件键发生一次。无付费调用的本地运行不生成虚假模型扣点。

storage 的 operation_key 覆盖上传请求或 submissionKey+imageIndex+variant；提交结果不明先查原记录。
租约过期进入检查，不直接释放。磁盘遗留文件与 DB 引用要按精确键核对，不能用日龄批量清理。
历史 used_bytes 从 pod_asset_file 按 owner 集合聚合初始化，额外磁盘孤立文件单独列报告；不吞掉或自动删文件。

## 011：报价、执行快照与正式结算

| 表 | 关键字段 | 主键/唯一与查询索引 |
|---|---|---|
| `pod_execution_quote` | quote_id、user_id、workspace_id、kind=run/batch/retry、source_hash、selection_json、policy_revision、catalog_hash、points_upper_bound、cost_upper_micros、expires_at、state、consumed_resource_type/id | PK；(user_id,state,expires_at,quote_id)；UNIQUE(user_id,kind,consumed_request_id)，未消费时 NULL；报价无点数预留 |
| `pod_execution_snapshot` | scope_kind=run/batch、scope_id、workspace_id、user_id、quote_id、billing_scheme=managedPointsV1、policy_revision、selection_json、hold_points、unallocated_points | PK(scope_kind,scope_id)；(user_id,scope_kind,scope_id)、(quote_id) |
| `pod_run_model_binding` | run_id、node_id、workspace_id、user_id、model_id、model_version_id、provider_id、selection_json（包含不可变版本、adapter、spec_hash、预占、费率、能力及 endpoint/region 快照） | PK(run_id,node_id)；(provider_id,run_id,node_id)、(model_id,run_id,node_id)、(user_id,run_id,node_id) |
| `pod_call_billing` | submission_key、run_id、node_id、user_id、workspace_id、model_version_id、credential_version_id、held_points、charged_points、released_points、state、usage_json | PK submission_key；(run_id,node_id,submission_key)、(user_id,state,submission_key)、(model_version_id,submission_key) |

quote_id 为服务端生成 UUID；source_hash 覆盖项目 revision、模式、目标、baseRun、绑定输入、显式模型选择和恢复来源等语义。
报价模型/费率以服务器为准，不接受客户端自报价格或点数；提交时相同源和报价版本才能消费一次。
有效期使用 DB 时间；未消费报价最多 20/人，不自动清理过期报价；后续清理需按精确 ID 分页，不能清未决调用或账本。
快照保存 policy_revision，不声称保存完整 policy_json；权益变更原因及前后字段由管理审计追溯，领取与 Submit 前仍检查当前权益。

批次 snapshot 固定全部模型映射和输入摘要。run 派发时复制所对应映射，不能重新读取平台默认或当前价格。
batch 的 hold_points 为全批份额，unallocated_points 随转移/停止递减；每个 run 各有已分配份额，
在 gate/空间/点数账户锁内完成，防止派发与停止重复释放。
润色使用派生节点绑定；node/baseRun 复用需比较模型版本、来源图和执行模式，不能仅比较项目 revision。

call_billing 与 pod_provider_call 同 submissionKey，并与正式点数份额、成本预占同事务建立。
实际供应商成本仍以 pod_provider_call.charged_micros 为统计源，不再另外造一份可分叉的成本余额。
新 managed 结算允许记录超过预占的真实确认用量，产生超界告警；用户扣点不超过已预占份额，禁止静默丢回执。
不改变旧 qwen 已记账数据；旧结果读取和归档继续使用原调用/素材映射。

## 012：官方模板与平台素材

| 表 | 关键字段 | 主键/唯一与查询索引 |
|---|---|---|
| `pod_catalog_template` | template_id、code、name、description、status、current_version_id、published_version_id、revision、sort | PK；UNIQUE(code)；(status,sort,template_id) |
| `pod_catalog_template_version` | version_id、template_id、version、graph_json、spec_hash、published_at（槽位与输出由图派生） | PK；UNIQUE(template_id,version) |
| `pod_catalog_asset` | asset_id、name、media_type、byte_size、width、height、license_note、status、revision | PK；(status,create_time,asset_id) |
| `pod_catalog_asset_file` | asset_id、variant、storage_key、byte_size、sha256、media_type | PK(asset_id,variant)；UNIQUE(storage_key) |
| `pod_catalog_template_asset_ref` | version_id、node_id、asset_id | PK(version_id,node_id,asset_id)；(asset_id,version_id) |
| `pod_catalog_use_action` | action_id、user_id、workspace_id、request_id、request_hash、version_id、state、copy_map_json、graph_json、name、project_id、storage_operation_key | PK；UNIQUE(user_id,request_id)；(state,action_id)、(version_id,action_id) |

平台素材根派生为 StorageRoot/catalog，私人根保持平面文件键；平台素材不进入个人 pod_asset 的 owner 查询，也不借公共 /profile 暴露私人内容。
平台上传时有 license_note，发布前检查缺失素材/不支持模型和图结构；已发布版本不可覆盖。
修改保存新草稿版本；下架不删除历史版本或用户副本。使用时平台 assetId 到私人 assetId 显式映射，
幂等复制文件/元数据后再提交项目和引用，失败保留 use_action 以续作，不自动执行模型。

## 锁顺序和事务

涉及执行/授予/撤权/预算/默认模型的写入统一遵守：

`execution_gate → platform_state → workspace（按 ID）→ creator_policy → provider/model（按 ID）→ resource（batch/run）→ points/storage account（按 userId）→ item/call/event`

纯用户资源写入仍可从 workspace 起锁，但不得在持有 workspace 后再补取 gate；需要 gate 的完整调用从上层事务入口取得。
创建、批次派发、取消、停止、恢复、外部意图、结算分别是短事务，网络和图片复制/推理不持有 DB 锁。
资格/模型停用和真正 Submit 前的意图事务共用上述锁，保证变更后未提交调用不能穿透。
对已 submitted 的核对分支允许绕过新资格限制完成结算，仍必须验证租约、原 run、原 owner 和 submissionKey。

执行查询按集合批量加载模型/授权/版本，不能按每节点逐条查库。
统计使用时间范围+用户/模型条件和稳定分页；跨表按 run/submissionKey 聚合，不能按当前 output 重复或漏计恢复费用。
跨全库成本查询需要在实现阶段基于 EXPLAIN/实际数据再决定是否新增索引，不能以推测改生产基线。
