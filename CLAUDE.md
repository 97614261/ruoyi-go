# ruoyi-go

## pod-studio 分支补充边界

本工作区是从 `main@39bc60f` 建立的 POD 创意工作流项目，入口说明见 [README.md](./README.md)，
实施顺序和完成标准见 [docs/POD_DEVELOPMENT_PLAN.md](./docs/POD_DEVELOPMENT_PLAN.md)。
下文描述继承的 RuoYi 基线，不能把历史能力和测试当作 POD 功能已经完成。

- 既有系统接口、系统表和稳定标识符继续受冻结层保护。POD 新接口、新表及新菜单属于独立的新业务域，
  只在本项目专用数据库中迁移，不修改 ERP、门户或已有生产库。
- 本分支新增 `studio/` 创作端，普通用户仅使用创作端。用户已授权下一轮复用现存 `web/` 开发 POD 管理后台，
  不另建 `admin/`。新方案第 2 步已接通管理入口、权限、概览与访问审计，后台开发端口为 5178。
  新八步计划见 [平台设计](./docs/POD_PLATFORM_DESIGN.md)，权限和迁移见其配套文档。只读参考前端位于
  `D:\WorkSpace\go-space\RuoYi-Vue3-master`，Java 参考位于 `D:\WorkSpace\go-space\RuoYi-Vue-master`。
  下面沿用的相对路径来自原仓库布局，在此 worktree 中应按这两个实际路径读取。
- 新增 POD 业务没有现成 Java 对应功能，按本分支设计和测试验收；修改继承模块时仍核对 Java/Vue 契约。
- 启动 POD 功能前必须具备独立配置、数据库和 Redis 隔离；不得直接沿用继承配置连接现有业务库。
- 本分支不自动获得 multi-tenant 或 ERP 分支的能力。个人工作空间隔离仅针对新增 POD 资源。
- 对外服务管理设计已确认：供应商/模型版本、用户权益、正式点数、managed 执行及官方模板，
  当前 008—012 已在副本验证并安装本地，真实 schema 为 001—012；第 5 步为 managed 报价/执行/正式点数，第 6、7 步为任务用量、资源授权及官方模板；不覆盖已安装 SQL。
  供应商成本、用户正式点数、原模拟额度分别记账；新 managed 不能由 `!=qwen` 判断成模拟，不能把旧任务转成新计费。
  模型/价格快照和服务端报价由后端校验，未知调用不重发；平台总预算仍跨供应商合计。
  管理凭据/初始密码不走通用请求缓存或正文日志；密钥加密，主密钥与 JWT 分开且不入库。
  后台私有内容读取必须具体权限加资源 grant，即使 root 也不能只凭 `*:*:*` 绕过；不能冒充 owner 调创作接口。
- 管理基础见 [管理基础报告](./docs/POD_ADMIN_FOUNDATION_REPORT_20261113.md)。管理端使用 tab 内独立 `pod-admin.session`，
  受保护导航实时校验 `/pod-admin/bootstrap`，退出清理动态路由与会话；登录只可记住账号，不存密码。
  管理 API 入口和具体功能双重授权；只读运营不授予 system 通用权限。访问审计同步写入，失败时拒绝管理入口。
  审计只序列化有类型白名单，新增安全快照字段必须同步 DTO 与泄漏测试；不把完整请求/响应交给 OperLog。
  管理写接口使用 actor+requestId 幂等、安全审计及 action 结果同事务。private grant 限 operator/owner/单个资源及最多 24 小时；成功读取和缺授权拒绝都有审计，审计失败不返回正文。
  Redis pipeline 推进权限版本使用 EVAL：Script.Run 的 NOSCRIPT 回退不适用于延迟执行的 pipeline，禁止整批重放 INCR。
- 第 3 步规则与证据见 [模型目录报告](./docs/POD_MODEL_CATALOG_REPORT_20261113.md)。模型能力/成本/点数版本不可变，供应商编码/协议/地域/endpoint 固定；换计费主体建新供应商。
  主密钥文件独立于 JWT，AES 与幂等 HMAC 分离；主密钥缺失只读，旧 keyId 和 fingerprintKey 必须保留并单独受限备份。
  配置检查只做本地验证，不证明真实 Key 有效。目录只接新 managed 执行，不改变旧 qwen/simulation；不得把替身执行成功当成真实多模型调用成功。
- 2026-11-18 多厂商扩展见 [本轮报告](./docs/POD_MULTI_PROVIDER_REPORT_20261118.md)。`openai-chat-v1` 按固定官方根区分
  DeepSeek、智谱、硅基流动、Kimi 和 OpenAI；图片支持千问、智谱、硅基及 DALL-E 3 的独立协议与能力校验。
  `openai-compatible-v1` 只允许管理员新增独立供应商，绑定公网 HTTPS Base URL、地域、凭据、模型 ID、协议及不可变价格版本；
  创作者不能传 URL。DNS 校验和连接固定公网 IP，不用系统代理，不跟随重定向，不向图片 CDN 转发密钥。
  必安 `https://bian.bbroot.com/v1` 的已确认无签名 `/images/` 原图可用该调用绑定的 API Key 在同源下载；带签名或其他域名仍不带 Key，不使用网页登录会话。
  图片支持 URL/Base64。Base64/自动模式在调用前检查私有回执目录可写；回图校验后二进制先落盘，MySQL 只记小回执和归档引用，无需 64 MiB 单包。
  URL-only 渠道仍不接受违反约定的 Base64；`storageRoot/receipts` 里的未归档回执必须随素材一起备份，归档成功后才可删除恢复文件。
  自定义图片尺寸、质量和结果格式、文字 Chat/Responses 协议属于版本快照；未知结果不重发，确认回执先持久化再归档。
  官方 GPT Image 的按 Token 图片计费仍未接入，不得伪装成 DALL-E 或按张零价。2026-11-14 边界为历史快照，见 [旧报告](./docs/POD_MODEL_MAINTENANCE_REPORT_20261114.md)。
  复用原版本、意图、回执、成本预算及正式点数路径；供应商超时/未知不重发。缺 Key 或未核实定价保持停用草稿，不填虚构零价版本。
  当前累计估算预算按用户授权为 10 元，不代表供应商充值或开通付费。11-14 的全图片停用为历史快照；
  11-15 新增四个百炼文字型号及已有 Pro 单图均真实通过，图片默认值为 Pro，文字默认值保留原 Flash，见 [型号接入报告](./docs/POD_MODEL_ADDITIONS_20261115.md)。
  百炼托管 DeepSeek 使用百炼凭据；Omni 3.8 当前仅文字，使用 reasoning_effort=none；图片翻译保持无版本停用草稿，不得冒充生图能力。
- 配置与创作易用性见 [2026-11-14 报告](./docs/POD_CREATION_USABILITY_REPORT_20261114.md)：后台成本填元，以 BigInt 精确转原 micros；空白不是免费。
  报价型号/张数取持久化快照，官方入门模板安装离线且幂等，复制不执行；待确认调用优先核对原状态，归档失败不重新生图。
- 2026-11-15 试用收口见 [本轮报告](./docs/POD_BETA_REPORT_20261115.md) 与 [运营说明](./docs/POD_BETA_OPERATIONS.md)：入门引导依据本人模型/正式点数，缺图片模型明示。
  刷新、切回、网络恢复只 GET 原任务；节点结果须核对项目/节点/模式/编号，失权清除旧内容且不能因此重发。doctor 只读且不调用供应商；无密钥 Linux 交付包仅代表构建通过，不能写成目标部署通过。
- 第 4 步见 [创作者权益报告](./docs/POD_CREATOR_POLICY_REPORT_20261113.md)。资格在 HTTP、写事务、领取和新 Submit 前检查；到期只读，停用拒绝私有内容读写，已提交调用仍核对/归档。正式点数初始零，不转换原测试余额；人工 grant/deduct 有原因、revision、requestId、流水及同事务审计，扣减不得动 held。模型范围已接第 5 步 managed，不能宣称已限制旧固定模型调用。
  每日按已接受份数计数，幂等不重计，取消/失败不返还；存储在写盘前预留原图+缩略图字节，计入回收站及派生图。未知写入/提交保留 held，不按 TTL 自动释放。测试精确删文件也必须同步自己的存储账户，不能只删 DB 文件行制造账实不符。
  新账号账号名 3—20 位 ASCII，密码 8—20 字，兼容既有登录；只授专用 pod_creator 角色，不接受客户端角色列表。创作端失败页不能把所有 403 当成可自行恢复资格。
- 模型密钥和实际付费试跑单独配置，参考站点账号不等于本项目 API 凭据或付费授权。
- 本地初始化、启动和 POD 专项测试以 [docs/POD_LOCAL.md](./docs/POD_LOCAL.md) 为准；
  不运行默认连接继承业务库的历史 `test/` 集成套件。当前图契约与接口边界见
  [docs/POD_FOUNDATION.md](./docs/POD_FOUNDATION.md)，阶段证据见
  [docs/POD_STEP1_REPORT.md](./docs/POD_STEP1_REPORT.md)。
- 第二阶段项目与画布的接口、配置字段、版本冲突及测试证据见
  [docs/POD_STEP2_REPORT.md](./docs/POD_STEP2_REPORT.md)。复用 001 的项目表，未改动已安装 SQL；
  个人空间访问权不能因 init 请求而恢复，所有项目写入必须在事务内复核成员并比较 revision。
- 第三阶段素材契约与证据见 [docs/POD_STEP3_REPORT.md](./docs/POD_STEP3_REPORT.md)。
  素材不得走公共 /profile，鉴权下载不得把 Token 放进 URL；项目图与素材引用必须同事务保存，
  删除项目后继续保留引用，引用中的素材不得删除。第三阶段安装 001—003，已安装迁移不可回写。
- 第四阶段将当前 schema 扩展为 001—004，见 [docs/POD_STEP4_REPORT.md](./docs/POD_STEP4_REPORT.md)。
  执行仅在显式 simulation 模式开放文本/图片输入与测试文本，不调用真实模型；运行快照、素材保留引用、
  预占与 outbox 同事务提交。重复投递必须由数据库租约和 fencing 拦截，待确认不得盲目重新 Submit。
  账本事件必须幂等，取消先确认已领取调用再结算。Asynq 至少需要 Redis 4，当前 Windows 开发专用实例在 6387。
- 第五阶段当前 schema 为 001—005，见 [docs/POD_STEP5_REPORT.md](./docs/POD_STEP5_REPORT.md) 和
  [docs/POD_QWEN.md](./docs/POD_QWEN.md)。千问模式须显式启用、设置累计估算预算并在 Worker 服务端读取密钥，
  密钥也支持读取忽略提交的本地 `pod.qwen.apiKey`，非空时优先于环境变量；不得进入前端、DTO 或日志。
  旧请求不得自动升级成付费请求；运行固定模式和模型。真实调用意图先持久化，未知结果不重发；
  已确认响应先入库再归档，归档按调用和图片序号幂等。模拟测试额度与估算人民币费用分开，供应商账单为准。
  独立就绪分支先执行，多个待确认节点按最近检查时间轮转；不得让永久未知调用阻塞其他分支。
  2026-11-12 本地已切换 qwen；按用户追加试用额度要求，累计估算预算由 1 元提高到 5 元，并发 1；短文字、节点内润色、单参考图、五种比例与两张生成已有真实样本，
  见 [边界报告](./docs/POD_BOUNDARY_ACCEPTANCE_20261112.md)。2026-11-13 实际登录制作/批量和备份恢复已补，
  见 [交付报告](./docs/POD_DELIVERY_REPORT_20261113.md)。真实 3/4 张、多参考成功样本受供应商免费额度耗尽且仅免费调用阻塞，账单未核对。
- 节点内润色与生成见 [节点交互记录](./docs/POD_NODE_INTERACTIONS_20261112.md)。`polish/branch` 派生运行快照，
  不回写项目，不额外绕过队列调用模型；润色不能当作普通历史上游，替换原文必须核对来源仍未变化。
  分支上游重新执行可能计费。工作流预设和提示词仅插入内容，不自动运行；旧图与进阶文本节点继续兼容。
- 图片处理、CPU AI 与 T 恤预览见 [图片制作报告](./docs/POD_IMAGE_WORK_REPORT_20261112.md)。复用现有运行、
  归档序号映射与素材引用，当前 schema 仍为 001—005；原图保留，派生素材保留 parentAssetId。
  `source=mockup` 和 `remark=pod-kind:mockup` 参与服务端来源判断，处理或重新选入不能洗成印花；不得随意改写这些标记。
  普通插值和真实 CPU 模型必须明确区分，AI 失败不得静默降级；本机服务只接受回环图片字节，不接私有 URL、密钥或命令。
  印花与展示图分别导出，二维示意、有效 DPI 提示及小样本通过均不代表生产商印刷验收通过。
- 个人模板在 001—006 阶段实现，当时升级至 001—007，后续已追加 008 管理基础及 009 模型目录，见 [个人模板报告](./docs/POD_TEMPLATE_REPORT_20261112.md)。保存仅从本人已保存项目读取并核对 revision，
  历史版本不可变；使用固定版本复制独立项目，模板更新不传播。只允许替换文字/图片输入槽位，不接收任意处理节点配置。
  创建、新版本、复制、使用必须保留 requestId 与请求摘要；提交不明时重试原操作，不能换键重复创建。
  模板版本、素材引用与动作记录同事务提交，并与素材删除共用工作空间锁。历史版本和回收站仍保留引用，
  不得为释放素材删引用或自动清空回收站。个人模板不开放共享，不带运行结果、账本或凭据，不自动执行模型。

- 图片交互见 [图片交互报告](./docs/POD_IMAGE_INTERACTION_REPORT_20261112.md)，当时沿用 001—006、无新 HTTP 接口或配置字段，当时由批量升级至 001—007，后续已追加 008 管理基础及 009 模型目录。
  裁剪先旋转，两端分别向下取整；印花几何与 Go 一致。未完成手势只做本地预览，松手一次修改、一次撤销。
  对比只能使用结果的实际 parentAssetId，不能用当前画布输入替代；私有原图继续鉴权并释放对象 URL。
  按账号/空间/项目记住运行编号，刷新只读恢复，不能自动提交或恢复模型调用；来源版本与当前参数必须区分。
  旧响应不得覆盖新选择，失效权限清除旧记录；组件验收不代替完整登录链路与真机验收。

- 创作者用量见 [用量报告](./docs/POD_USAGE_REPORT_20261112.md)。GET /pod/usage 只读本人有效空间，
  按所有 pod_provider_call 记录汇总，不得改成按当前节点 output 相加；恢复会覆盖节点输出但不能漏掉历史费用。
  金额以 micros 十进制字符串传输，前端 BigInt 格式化；generatedImages 仅为已确认模型图片数，不能混入本地/模拟输出。
  预算是全实例累计上限，不返回全局已用/余额或他人用量；未知结果保留预占，人工结算仅完成设计，没有写接口。
  执行预览必须区分 workflow/node/branch/polish；若更改模型、预占或价格，需同步 execution-impact.ts、提示与测试。
  提交未知时重试原版本/原 requestId，不能以新草稿的预览冒充原请求；用量本身无迁移，当时由批量扩至 001—007，后续已追加 008 管理基础及 009 模型目录。

- 有界批量当时 schema 为 001—007，后续已追加 008 管理基础及 009 模型目录，见 [批量报告](./docs/POD_BATCH_REPORT_20261113.md)。
  每批 1—10 个明确输入，每人 20、实例 200 个待派发项；固定模板版本与绑定图，不生成笛卡尔积。
  创建摘要与 requestId 幂等，未知提交重试原意图；派发事务同时写项目、run、outbox 和关联，沿用执行门/空间锁。
  预览不预留预算；逐节点领取仍校验累计预算和并发。停止只取消待派发项，已派发运行单独取消/恢复。
  批次状态只表示派发进度，不能将 dispatched 解释为全部成功。待派发素材保留引用，不因停止解除历史引用。
  ZIP 本人鉴权、成功节点归档图片、排除输入图，逐图校验 SHA256；100 图/100 MiB/30 秒/同时两个，先生成完整临时包再返回。
- 第 6、7 步见 [任务与官方模板报告](./docs/POD_OPERATIONS_CATALOG_REPORT_20261113.md)。任务/用量查询最多 31 天，列表不含项目名、图、输出、供应商回执或存储键；管理取消只请求原状态机处理，未知调用预占不动，管理停止只释放未派发份额。
  官方模板版本不可覆盖，发布指针和草稿分离；使用固定已发布 versionId，计划与存储预留先提交，再校验复制文件，最后事务创建本人项目/素材/引用。中断重试同请求和文件键，下架不撤销已受理计划或个人副本。
  平台素材根为 StorageRoot/catalog，私人文件接口只能读原平面根。PutVerified 使用不覆盖目标的原子硬链接发布，需要支持硬链接的本地文件系统；不能回退成覆盖式 rename。备份同时包含两类素材及中断复制文件，副本核对 SHA、版本引用、复制归属和预留。
- 本地完整备份与副本恢复见 [交付报告](./docs/POD_DELIVERY_REPORT_20261113.md) 和 [本地说明](./docs/POD_LOCAL.md)。
  bundle 必须停 API/Worker 并核对队列归属；SQL、素材、配置和账号配套，敏感备份不入 Git。
  restore-check 只接受可信本地 bundle，新建副本库/素材目录，不覆盖源库、不连接 Redis 或启动 Worker。
  当前仅本地恢复检查，不能用来宣称 Linux/生产部署或供应商账单验收通过。
  明确 HTTP 403 + AllocationQuota.FreeTierOnly 映射安全本地提示，不回传原文、不自动开付费或重试；未知调用继续保留预占。

用 Go 重做 RuoYi 后端。**目标是超越 Java 版**，同时冻结已有消费者依赖的契约面。

## 三层边界（先读这个，再读别的）

**① 冻结层 —— 已有消费者依赖的契约不动**

| | 冻结的内容 |
|---|---|
| 前端 | `../RuoYi-Vue3-master` 源码一行不改 |
| 请求 | 路径、HTTP 方法、Content-Type、参数位置、名字、类型，以及前端依赖的必填 / 可选和默认语义 |
| 响应 | 字段名、是否出现、层级（平铺 vs `data`）、类型、`null` / 空值语义；下载的 Content-Type、Content-Disposition 和二进制格式 |
| HTTP 状态 | 普通 RuoYi 业务响应保持 200；只保留已登记的 CORS 预检 204、请求体超限 413、健康检查失败 503 |
| 数据库 | 生产基线的表、列、类型、可空性、默认值、字符集 / collation、约束和索引；运行期业务数据不属于结构冻结 |
| 契约数据 | 被 Go、Java 参考版、Vue、部署脚本或测试按名字 / 固定 ID 引用的稳定标识符；当前包括 `sys_config.config_key`、`sys_dict_type.dict_type`、`sys_menu.perms`、超级管理员用户 / 角色 ID |
| Redis | `pkg/redisx/keys.go` 定义的 Key 前缀；部署清理、会话迁移和测试清理按这些前缀定位 |

任何"这样设计更合理"的改动，只要破坏这张表里的既有依赖，就是错的。确需改变时先修改决议、
调用方和验证口径，不能借"超越 Java"绕过冻结层。

契约数据冻结的是**标识符**，不是运行期配置值。例如
`sys.account.chrtype`、`sys.account.initPasswordModify`、`sys.account.passwordValidateDays`、
`sys.account.captchaEnabled`、`sys.account.registerUser`、`sys.user.initPassword`、
`sys.login.blackIPList` 这些
`config_key` 不得改名或删行，但对应 `config_value` 可以按运维需要调整。菜单名称、状态可以变，
被路由权限引用的 `sys_menu.perms` 不能无迁移改名。新增被代码或前端按名字引用的标识符时，
自动纳入这一层，不以本表是否逐项列出为限。`sys_user.user_id = 1` 和
`sys_role.role_id = 1` 分别被 `AdminUserID`、`AdminRoleID` 固定引用，同样不得删除、换 ID
或移作其他实体。

Redis 前缀并非永远不能迁移；但改名必须先登记决议，并同步代码、部署清理、兼容迁移和测试清理，
不得作为普通内部重构直接修改。
`login_user_permission_version:<userId>` 是持久权限版本，不设 TTL；权限写入、鉴权懒刷新和
停机迁移都依赖它，不能只清理会话而遗漏该前缀。

**② 超越层 —— 内部保障应该做得比 Java 好**

不改变冻结契约和正常业务语义的内部实现：

- 安全内部机制：密码与 Token 不落日志、原子验证码 / 错误计数、注入防护；
- 正确性：并发控制、事务边界、缓存一致性、批量查询；
- 资源：数据库和 Redis 超时、连接池、内存控制、异步背压；
- 运维：日志、追踪、指标和不改变既有探针契约的实现优化。

「Java 也是这么写的」**不是免死金牌**。Java 的缺陷照抄过来不叫对齐，叫失职。
如果加固会改变返回值、行集、顺序或成功 / 失败语义，则还必须经过第三层审查。

**③ 受控差异层 —— 可观察语义改变必须登记**

请求和响应形状保持冻结，但返回值、行集、行序、错误文案或授权结果与 Java 不同，例如：
逻辑删除过滤、更具体的错误文案、唯一兜底排序、权限收紧、旧会话失效和富文本净化。

允许差异必须同时满足：

1. 有明确的安全或正确性收益，不是个人风格偏好；
2. 不破坏 Vue、下载工具、运维探针、脚本及其他已知消费者；
3. 在 `DECISIONS.md` 登记原因、边界和禁止回退项；
4. 有定向测试，并在 Java / Go 对拍结果中可识别。

「按主键过滤 `del_flag`」「错误文案更具体」「分页补唯一兜底列」都属于这一层，
不是前端不可见的内部实现。

**Vue 是用户界面兼容性的首要基准，但不是唯一正确性基准。** API 约定、Java 参考实现、
现有脚本和运维探针，以及认证、授权、数据权限等安全不变量共同决定行为是否正确。
参考实现在 `../RuoYi-Vue-master`；行为有疑问时先看 Java 怎么做，再判断它做得对不对。
Java 是参考，不是标准。

契约细则见 @docs/CONVENTIONS.md ，历史决议见 @docs/DECISIONS.md ，部署切换见
[docs/DEPLOYMENT.md](./docs/DEPLOYMENT.md)，压测方法见 [docs/PERF.md](./docs/PERF.md)。

## 技术栈

| 用途 | 选型 |
|---|---|
| Web | `gin-gonic/gin` |
| ORM | `gorm.io/gorm` + `gorm.io/driver/mysql` |
| 缓存 | `redis/go-redis/v9` |
| JWT | `golang-jwt/jwt/v5` |
| 配置 | `spf13/viper` |
| 日志 | `log/slog`（标准库） |
| 参数校验 | `go-playground/validator`（gin 内置） |
| 定时任务 | `robfig/cron/v3` |
| Excel | `xuri/excelize/v2` |
| 密码 | `golang.org/x/crypto/bcrypt` |

不要引入其他 Web 框架或 ORM。需要加新依赖时先说明理由再加。

## 分层与依赖方向

```
configs/             application.yml（本地覆盖用 application.local.yml，已 gitignore）
cmd/server/          入口、装配、优雅关闭
cmd/perfseed/        压测数据生成器（独立工具，见 docs/PERF.md）
internal/
  config/    [L0]    配置加载
  model/     [L1]    GORM 实体、请求/响应 DTO
  job/       [L1]    定时任务注册表（不依赖 DB，需要 DB 的任务由 service 注册）
  datascope/ [L2]    数据权限条件构造，返回 GORM Scope
  repository/[L2]    数据访问，只有这一层能持有 *gorm.DB
  service/   [L3]    业务逻辑、事务边界
  handler/   [L4]    HTTP 处理器
  middleware/[L4]    Gin 中间件
  router/    [L5]    路由注册、权限标识挂载
pkg/
  response/          统一响应封装（AjaxResult / TableDataInfo 语义）
  page/              分页与排序参数解析（含 orderByColumn 白名单）
  errs/              业务错误类型 BizError
  types/             types.Time 等基础类型
  jwtx/              JWT 签发与校验
  logx/              traceId 注入与 slog Handler 包装
  cronx/             Quartz 表达式到 robfig/cron 的适配（星期编号差 1，见 DECISIONS）
  redisx/            Redis 客户端与 key 常量
  excelx/            Excel 导出（struct tag 驱动，对齐 @Excel）
  validate/          自定义校验规则（notblank / xss / dicttype）
docs/
```

**只能从高层引低层，禁止反向引用，禁止跨层跳跃。** handler 不准直接引 repository，必须经 service。

同层依赖只允许一条：**handler → middleware**（取当前会话、挂权限中间件），不得反向。

> 注：Go 包名不能以数字开头，所以层级用上面的 `[Ln]` 标注表达，不像其他项目那样用 `00-` 前缀做目录名。改动前先确认没有破坏这个方向。

## 硬约束（不可违反）

1. **响应必须走 `pkg/response`**，禁止在 handler 里手写 `c.JSON` 拼裸 map
2. **所有列表接口必须分页**，`pageSize` 硬上限 100，超出则截断而不是报错
3. **所有 DB / Redis / HTTP 调用必须带 `context.Context` 和超时**，禁止 `context.TODO()` 进主干代码
4. **只有 repository 层能获取、保存或直接操作 `*gorm.DB`**。`internal/datascope`
   可以构造 GORM Scope；service 可以组合和传递 `func(*gorm.DB) *gorm.DB` 类型的
   Scope，但不得获取数据库连接、调用 GORM 查询方法或直接执行 SQL
5. **禁止用 panic 控制流程**，错误一律用 error 返回；middleware 统一 recover 并返回 500
6. **循环里禁止查库**，遇到 N+1 用批量查询或 `Preload`
7. **密码只用 bcrypt**，禁止出现在任何日志、响应、错误信息里；Cookie、Token 同理
8. **时间统一 `Asia/Shanghai`**，JSON 序列化格式固定 `2006-01-02 15:04:05`
9. **JSON 字段名必须是 camelCase**（`userName` / `deptId` / `createTime`），跟 Java bean 一致。Go struct tag 必须显式写，不能靠默认
10. **新建可编辑业务表必须带** `create_by` `create_time` `update_by` `update_time` `remark`，对齐 RuoYi 的 `BaseEntity`。011 四张执行事实表按专门决议例外：owner/源请求/不可变版本与点数事件提供追溯，不能伪造可编辑审计字段；此例外不外推普通 CRUD

## 新增接口前必须回答的七个问题

写代码之前先在回复里逐条回答，答不上来的先问：

1. 分页了吗？`pageSize` 有上限吗？
2. 查询条件涉及的列有索引吗？会不会全表扫？
3. 循环里有查询吗？（N+1）
4. 外部调用带 context 和超时了吗？
5. 权限标识是什么（形如 `system:user:list`）？需不需要数据权限过滤？
6. 错误路径返回的是统一 code 吗？敏感信息会不会漏进 msg 或日志？
7. 响应的 JSON 字段名跟前端期望一致吗？（camelCase，且平铺规则见 CONVENTIONS）

## 常用命令

```bash
go build ./...            # 编译
go vet ./...              # 静态检查
gofmt -l .                # 列出未格式化文件
go test ./...             # 测试
go run ./cmd/server       # 本地启动
```

提交前 `gofmt` + `go vet` 必须干净。

## 工作方式

- 改动前先读相关的 Java 实现，别凭记忆猜 RuoYi 的行为
- **性能和能力上的判断必须有数字，不能只靠原理推断。** 这条是拿三次教训换来的：

  | 推断 | 实测 |
  |---|---|
  | 「`find_in_set` 走不了索引，是明确的性能问题」 | 只占 3.6%，真凶是旁边的 DISTINCT 和 JOIN |
  | 「Java 能后台配任意 bean，Go 做不到，能力有差距」 | Java 白名单锁死在一个包，同样要改代码 |
  | 「StreamWriter 流式写，瓶颈在数据切片不在工作簿」 | 切片只占 16%，反了 |

  三次的共同点：推理链条本身没错，**错在没验证前提**。
  写下"因为 X 所以 Y"之前，先问一句 X 有没有量过。
- 优先小步修改，不要推倒重写已经跑通的模块
- 模块化 CRUD 一律先用代码生成器产出，不要手写复制；生成器产出 Go 四层代码、自动路由、
  Vue 页面/API 和菜单 SQL。模板改动必须同步渲染测试并在 DECISIONS 说明
- 做完一个模块，先补 `test/` 下的接口测试并全绿，再说完成；前端点一遍是补充验证不是主证据
- 跑测试：`.\scripts\test.ps1 [用例名正则]`，结果在 `test/results/summary.log`
- 任何影响契约、分层、依赖的决定，追加一条到 @docs/DECISIONS.md

## 第 5 步 managed 执行边界

见 [执行报告](./docs/POD_MANAGED_EXECUTION_REPORT_20261113.md)。服务端报价有效 10 分钟、每人最多 20 个有效未消费报价；报价不预占。提交后锁定输入/模型/价格，run 或全批同事务预占正式点数。调用意图、成本预占和点数份额先落库；确认回执与结算幂等，未知保留两类预占且不重发。超界记录实际成本、封停模型，用户最多扣原预占。

2026-11-17 按用户要求简化 managed 交互：显式点击生成/润色/执行/恢复或开始批量后自动核价、提交，取消额外点数确认弹窗与勾选。画布节点操作仅本次点击自动开始；已有结果或未知提交的恢复不自动创建/重试运行。报价等待期间输入变化或组件关闭不得提交；报价失败/过期不得绕过。未知提交继续复用原 requestId/quoteId 和内容，不重新报价。规则与证据见 [执行简化报告](./docs/POD_EXECUTION_SIMPLIFICATION_REPORT_20261117.md)。旧 qwen 模式的费用确认不受本次改动影响。

切换必须 API/Worker 同步重启。不同模式的未提交步骤停止，只核对/归档已提交结果；旧 requestId 原内容只读返回原资源，不转新模式。所有 managed run 领取前必须有 owner/空间一致的执行快照，缺失即拒绝。恢复仅失败步骤，显式新报价，沿用原版本/原价。批次全额持有，派发转移份额，停止只释放未派发部分。新执行不能读取旧 YAML Key，凭据版本来自模型目录。
