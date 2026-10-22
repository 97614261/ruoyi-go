# ERP 第一期设计：多店铺订单履约基础版

状态：2026-10-21步骤1已完成基线/设计，步骤2已完成独立本地库建表、显式迁移与恢复演练，见[ERP_PHASE1_SCHEMA.md](./ERP_PHASE1_SCHEMA.md)。
后续已实施步骤3—7：店铺/范围见[ERP_PHASE1_SHOPS.md](./ERP_PHASE1_SHOPS.md)，商品配对见[ERP_PHASE1_ITEMS.md](./ERP_PHASE1_ITEMS.md)，待审核订单/导入见[ERP_PHASE1_ORDERS.md](./ERP_PHASE1_ORDERS.md)，事务履约见[ERP_PHASE1_FULFILLMENT.md](./ERP_PHASE1_FULFILLMENT.md)，页面工作台见[ERP_PHASE1_WORKSPACE.md](./ERP_PHASE1_WORKSPACE.md)。审核预占、取消、手工运单/本地发货、工作台/批量操作已实现；步骤8的本地验收、配套备份恢复和剩余发布门槛见[ERP_PHASE1_ACCEPTANCE.md](./ERP_PHASE1_ACCEPTANCE.md)。新菜单不自动为已有企业开通。
当前底座用法见[ERP_FOUNDATION.md](./ERP_FOUNDATION.md)，库存规则见[ERP_CONTROLS.md](./ERP_CONTROLS.md)，本轮证据见[ERP_PHASE1_BASELINE.md](./ERP_PHASE1_BASELINE.md)。
2026-10-22已完成[本地GUI补验](./ERP_LOCAL_ACCEPTANCE_2026-10-22.md)：员工范围/旧会话撤权、导入结果订单链接、ERP弹窗四视口布局与回归。此前阶段8快照保留历史未测项，现状以补验记录为准；用户明确不做远程，生产部署/容量与回退门槛另行授权，不外推通过。

## 1. 目标与范围

经营模式固定为：企业自己备货、自有仓库、自行发货。企业可以有多个店铺；第一期每个订单只支持一个仓库、一个包裹。
业务闭环为：店铺订单录入/导入 → SKU配对 → 审核预占 → 包裹与手工运单 → 确认出库。

| 能力 | 第一期范围 | 不包含 |
|---|---|---|
| 渠道 | 服务端预置只读目录及能力标识，手工/文件适配器 | 真实平台授权、自动拉单、回传、刊登、凭据保存 |
| 店铺 | 企业内店铺资料、渠道/站点、启停、员工店铺范围 | 跨企业共享店铺、代登录 |
| 商品 | 复用内部SKU；店铺商品变种与配对 | 独立SPU重构、组合商品、赠品、单位换算 |
| 订单 | 手工/模板导入、待审核编辑、审核、取消、详情 | 拆单、合单、部分发货、自动审单、售后流程 |
| 履约 | 一个包裹、物流方式/手工运单、本地发货记录 | 面单申请、物流轨迹、平台已发货承诺 |
| 任务 | 有限文件导入、进度、逐订单结果、显式重试 | 通用工作流引擎、分布式任务、多实例 |
| 财务/采购 | 可记录原始订单金额/币种，已有手工入库可补货 | 采购订单、利润、结算、收付款核销 |

渠道目录不是SaaS平台租户管理，也不是每个企业都能编辑的全局表。首期用服务端登记目录，不引入tenant_id=0的业务数据例外。
渠道名称不代表已接入：目录必须明确`supportsApi=false`、可用来源MANUAL/IMPORT；不提供空的“授权成功”按钮。

## 2. 代码核对与复用边界

| 现有位置 | 复用能力 | 后续需要补的边界 |
|---|---|---|
| [internal/model/sku.go](../internal/model/sku.go)、[internal/service/sku.go](../internal/service/sku.go) | 企业SKU、校验、导出 | 不把内部SKU当店铺商品；不修改现有单位冻结规则 |
| [internal/middleware/erp.go](../internal/middleware/erp.go)、[internal/repository/erp_access.go](../internal/repository/erp_access.go) | 客户身份、实时仓库范围、员工行锁 | 步骤3已增加实时店铺范围；后续订单仍须接入，同租户不能替代店铺权限 |
| [internal/repository/erp.go](../internal/repository/erp.go) | 销售单、预占、释放、出库、同事务流水 | 现有函数各自开启事务，不能连续调用它们或HTTP来假装订单整体原子提交 |
| [internal/repository/erp_corrections.go](../internal/repository/erp_corrections.go) | 草稿、来源纠错、盘点保护 | 新订单关联单据的旧入口必须增加领域所有权检查 |
| [internal/router/erp_routes.go](../internal/router/erp_routes.go) | 既有`/erp/sales`等入口 | 保留独立手工销售单；新商业订单用`/erp/order`，不能改名覆盖旧接口 |
| [internal/service/gen_render.go](../internal/service/gen_render.go) | 普通CRUD生成 | 店铺/店铺商品先生成，再补规则；订单状态机/导入/授权不是纯CRUD |

现有SALE单据是库存凭据，不是电商订单；不把客户地址、运单、外部订单状态等都塞进erp_document。
不改原Java/main库、参考Vue、既有Redis前缀；仅使用独立多租户本地库。步骤3—7已实施，步骤8的本地回归/恢复及后续GUI补验已执行，不外推生产或未覆盖的真机/跨浏览器组合。

## 3. 逻辑数据模型（步骤2已建表；步骤3—6已实现店铺、配对、订单/导入及事务履约）

所有新增业务表含无业务默认值的`tenant_id BIGINT NOT NULL`及五个审计字段，关联以企业复合键约束。
主表有持久version；外部订单号、外部商品/变种ID、运单号均为字符串，不强转整数、不丢前导零。

| 预案实体 | 关键字段与约束 |
|---|---|
| erp_shop | shop_id/code/name、channel_code、site_code、status、currency、version；企业内shop_code唯一，渠道/站点不能在已引用后随意更换 |
| erp_shop_access | 企业+user_id+shop_id唯一；所有者不存隐含“全部”行 |
| erp_shop_policy | 企业+user_id唯一、version；显式[]撤回，缺失/null拒绝 |
| erp_shop_item | 店铺的一个可售变种，item_id/shop_id/shop_item_code、外部ID/标题/状态/version；企业+店铺+shop_item_code唯一，外部ID如存在也按店铺约束 |
| erp_shop_sku_mapping | 店铺变种→一个内部sku_id、活动状态、version；企业+shop_item_id唯一；创建/取消配对均检查归属和引用 |
| erp_order | order_id/no/shop_id、source_type/source_order_no、create_request_key/hash、status/version、收件快照、币种/原始金额、warehouse_id、stock_document_id；企业+店铺+来源订单号唯一，库存凭据关联唯一 |
| erp_order_line | order_id/source_line_no、shop_item_id、原始店铺SKU/名称/数量；审核时另保存配对ID/version、内部SKU及单位快照；企业+订单+来源行号唯一 |
| erp_order_event | 不可变动作记录、前后版本、动作request_key/hash、操作者/时间；与成功状态变化同事务，幂等键作用域为企业+订单+动作+request_key |
| erp_package | package_id/no、order_id、warehouse_id、status/version、carrier_code/tracking_no/发货时间；一期企业+order_id唯一 |
| erp_package_line | package_id/order_line_id/sku_id/quantity及快照；包裹行与订单/企业的约束须保证不能关联到别的订单 |
| erp_import_job | job_id、固定shop_id、操作者、来源文件指纹/私有存储引用、任务状态/version、行数/结果数量；企业+操作者+创建request_key唯一 |
| erp_import_result | job_id、源行/订单定位、输入摘要、结果码、order_id；仅有界且脱敏的错误摘要，不保存原始收件信息到错误日志 |

店铺一个变种只配一个内部SKU；不同店铺/不同变种可以配到同一个内部SKU。
取消配对是版本化停用，不物理删除历史引用；重新配对推进版本。已审核行保留当时的配对版本及SKU快照，不按当前配对反查解释历史。
同订单多行映射到同一内部SKU时，保留各原始行，库存凭据按内部SKU汇总且再校验每SKU总数1—十亿，不能直接把重复SKU交给现有库存接口。录入阶段单行可至ERPMaxStock（一万亿）不代表审核能绕过现有单据数量限制，超限审核明确拒绝且整体回滚。
金额计划用`DECIMAL(20,4)`及十进制字符串JSON，不用float；未知金额为null，不冒充0，有金额必须明确币种。第一期不推导利润。
内部主键JSON沿用现有接口数值约定，不在本步统一改写既有ID类型；外部标识始终为字符串。
具体列长、可空性、唯一键、索引、外键以003_order_fulfillment.sql为准；恢复副本的实际约束验收见结构说明。阶段6已补业务数量、状态、权限与事务回归，不能以FK验收替代这些测试。

## 4. 状态和事务

### 订单

| 当前状态 | 动作 | 成功后的状态/副作用 |
|---|---|---|
| 不存在 | 创建/导入 | PENDING_REVIEW；不预占、不扣库存 |
| PENDING_REVIEW | 编辑 | 仍待审核，version递增；原创建幂等指纹保留 |
| PENDING_REVIEW | 审核 | READY_TO_SHIP；原子创建SALE并预占、固定配对/收件快照、创建包裹和成功事件 |
| PENDING_REVIEW | 取消 | CANCELLED；无库存变化 |
| READY_TO_SHIP | 取消 | CANCELLED；原子取消库存销售凭据、释放预占、取消包裹并记事件 |
| READY_TO_SHIP | 包裹发货 | SHIPPED；原子出库、更新包裹/订单、记流水和事件 |
| SHIPPED/CANCELLED | 编辑、重新审核或相反终态动作 | 拒绝；第一期不提供已发货撤回或售后入口 |

包裹只用READY_TO_SHIP、SHIPPED、CANCELLED；库存SALE仍用现有DRAFT/RESERVED/SHIPPED/CANCELLED，不混用三种状态。
缺货/未配对/地址问题是待审核订单的阻断原因，不伪造新的已审核状态。发货表示本地出库，不等于妥投、平台回传或平台结算。
店铺停用阻止新建、导入、编辑、审核、填写运单和发货；允许有权限的人取消历史未发货单以释放预占，仍可按权限查询。停用不批量删除/取消旧订单。

### 并发、幂等和旧入口

1. 已有行写路径锁顺序：当前员工 → 店铺 → 订单 → 包裹（若有）→ 库存单据 → 仓库 → 排序SKU/余额。保留现有库存内部相对次序；审核新建单据/包裹由父订单锁保护尚未提交的新行，商品/配对共用店铺父锁。不得反向加锁。
2. 授权替换仍锁目标员工，与该员工写入串行。企业状态/身份沿用现有认证及生命周期规则，不承诺强杀已进入事务的请求。
3. 审核/取消/发货复用提取后的**repository事务内步骤**，由一个事务统一编排；service不获取连接，不开启独立事务后拼接结果。
4. 数据/版本/状态/权限在写锁内再检查。不同操作的并发只能有一个合法结果；任何一步失败整体回滚。详情中的头/行/包裹使用一致快照读取。
5. 动作要求version及requestKey；相同键、相同内容返回原结果，不产生二次流水；同键不同内容拒绝。即使重放也先检查当前权限，不能用旧成功结果绕过撤权。
6. 新订单托管的库存单据不得经`/erp/sales`旧入口独立编辑、确认、取消、出库或创建重放。已实现旧详情/列表/流水及关联SR的店铺范围，独立历史手工单不强制要求店铺。当前旧库存凭据没有导出路由；将来增加必须继承同一范围，不能声称本轮验证了不存在的接口。
7. 既有库存纠错不等于订单售后：关联手工纠错需权限、来源和原因，不改写已发货事实。不得借当前退货接口提前宣称完成售后流程。
8. 订单动作传订单version；包裹动作传包裹version。物流保存不变库存，但同时推进订单/包裹版本，防止旧取消弹窗覆盖新物流；返回version始终是订单版本，packageVersion是包裹版本。重放返回原动作结果，不保证是当前最新版本。

## 5. 权限模型

- 基础检查为企业开通范围∩员工功能权限∩服务端企业身份。平台管理员不能代入客户。
- 当前企业所有者拥有本企业全部店铺，员工默认无范围；授予/撤回需`erp:shop:grant`且实时所有者校验。
- 店铺范围读取/替换沿用仓库授权的version、显式集合、原子成功审计；不按角色名、客户端tenantId/userId或前端列表决定权限。
- 订单/店铺商品/包裹列表、详情、导出和任务结果须检查所属店铺；审核/发货还检查发货仓库。第一期不引入“无店铺权限但能读完整订单”的仓库员工特例。
- 关联库存凭据不能成为店铺权限旁路。撤权后同一旧会话的下一次请求拒绝，异步导入在每个订单写入前重新检查企业、操作者和店铺范围。
- 新菜单只登记定义，不为现有企业/员工自动开通。是否向demo等追加权限，必须另行确认。

## 6. 接口契约（阶段3—7均已注册；实际字段/重试/文件限制见各阶段报告）

新增接口使用现有response，列表`rows/total`顶层、详情`data`，JSON camelCase。普通业务错误沿用HTTP200中的code，不改既有契约。
全部列表分页上限100，排序白名单加唯一兜底；导出独立限额，不借导出开放无上限查询。

| 接口组 | 请求/动作预案 | 权限 |
|---|---|---|
| 渠道 | GET `/erp/channel/list`，返回只读目录/能力，分页 | erp:channel:list |
| 店铺 | GET `/erp/shop/list`、GET `/:shopId`、POST基础路径、PUT `/:shopId`（带version）；不提供物理删除 | erp:shop:list/query/add/edit |
| 店铺范围 | GET/PUT `/erp/shop/access/:userId`；PUT需version、显式shopIds | erp:shop:grant + 当前所有者 |
| 店铺商品 | GET `/erp/shopItem/list`、GET `/:itemId`、POST基础路径、PUT `/:itemId`；停用代替删除 | erp:shopItem:list/query/add/edit |
| 配对 | PUT/DELETE `/erp/shopItem/:itemId/mapping`；需version，DELETE的版本放查询参数 | erp:shopItem:map |
| 订单 | GET `/erp/order/list`、GET `/:orderId`、POST基础路径、PUT `/:orderId`；创建含shopId/requestKey/来源单号/行/收件信息 | erp:order:list/query/add/edit |
| 订单动作 | PUT `/erp/order/:orderId/review`、`/cancel`；均有version/requestKey，审核带warehouseId，取消带reason | erp:order:review/cancel |
| 工作台 | GET `/erp/order/workbench`按shopId/sourceOrderNo聚合固定状态，忽略当前状态页签，不含收件资料 | erp:order:list + 实时店铺范围 |
| 审核检查 | GET `/erp/order/:orderId/readiness?warehouseId=...`，最多100行，只读、不预占 | erp:order:review + 实时店铺/所选仓库；已审核还查原仓库 |
| 包裹 | GET `/erp/package/list`、GET `/:packageId`；PUT `/:packageId/logistics`填运单、PUT `/:packageId/ship`确认发货 | erp:package:list/query/edit/ship |
| 导入 | GET `/erp/order/importTemplate`；POST `/erp/order/import`为multipart，显式shopId/requestKey/file，返回jobId而不是导入成功 | erp:order:import |
| 导入任务 | GET `/erp/importJob/list`、GET `/:jobId`、GET `/:jobId/results`分页；POST `/:jobId/retry`带version/requestKey | erp:importJob:list/query；重试还需erp:order:import |
| 私有原文件 | GET `/erp/importJob/:jobId/file`，创建者/当前所有者、实时店铺范围、24小时内 | erp:importJob:query |
| 订单导出 | POST `/erp/order/export`按店铺范围和既有导出限额 | erp:order:export |

静态路由必须在参数路由前登记。tenantId、审计字段、库存凭据ID及订单/包裹状态不能由客户端控制。
店铺资料维护例外接受status=0/1以启停，创建强制version=1，更新用路径shopId及当前version；请求DTO不包含企业/主键/审计字段。
步骤3目录编码为MANUAL/AMAZON/SHOPEE/LAZADA/TIKTOK/ALIEXPRESS，仅分类标签；sources列出一期规划的MANUAL/IMPORT，不代表本阶段已开放订单/导入。站点仅作1—32位ASCII归类，币种仅校验3位字母并归一大写，不提供平台站点真实性、币种支持或换汇保证。
包裹详情的订单信息至少要求订单query权限，不借package:query暴露完整收件资料；任务结果只向有任务query权限且仍有店铺范围的创建者/当前所有者开放。
订单/导入新路由须专门审查OperLog及异常日志的请求体/响应捕获，不把收件人、地址、电话直接落入通用操作日志；这是步骤5的实现门槛，不代表现有通用脱敏已自动覆盖这些新字段。
其余员工只看到其功能权限允许的最小数据，包裹列表默认脱敏；敏感字段不进入日志、任务错误、浏览器持久防重缓存。
动作使用JSON，PUT编辑必须带version；模板下载、普通JSON和文件接口分别遵守原有响应/文件安全规则。
步骤4商品编辑/配对/解绑均提交商品version；mappingVersion单独显示历史版本，不是请求令牌。商品shopId不可转移，有配对历史或任意订单引用后识别信息不可改。非SHIPPED/CANCELLED订单已引用配对时拒绝重配/解绑；未配对待审核行不因此被阻止首次配对。后续订单审核/取消/发货必须先锁同一店铺，继续保存审核时的mappingVersion和SKU/单位快照，不能用当前配对反写历史。

### 新接口七项预审答案

1. 列表/结果分页100，详情行上限100，导出用既有限额；渠道虽小仍分页。
2. 按企业+店铺+来源单号唯一；列表按企业/店铺/状态/主键、任务按企业/创建者/状态/主键规划索引；包含式名称搜索不宣称命中范围索引，DDL阶段实际EXPLAIN。
3. 头/明细/配对/SKU批量读取；不逐行查库。导入允许按订单分别事务，不把事务单元误称N+1优化已完成。
4. 所有DB/Redis调用继承context和超时；导入进入有界执行器、每订单独立超时，非裸goroutine；本期无外部渠道HTTP调用。
5. 权限见表，同时做企业/店铺/仓库过滤；平台不是业务豁免。
6. 统一业务错误与traceId；不返回SQL、路径、凭据和收件隐私，日志只保留必要定位字段。
7. 显式camelCase，沿用response形状；原始金额JSON字符串/null，外部ID字符串，数量正整数，枚举由服务端控制。

## 7. 文件导入和失败重试

- 一期仅固定模板XLSX，最多5000非空数据行，每订单1—100行；单文件最大10MiB且仍取服务器/企业上限较小值。复用安全解析和有界解压，不接受宏/公式作为业务值。
- 一次任务固定一个店铺。按来源订单号分组，重复来源行号、同单收件/币种冲突、缺字段等导致整个订单组失败，不创建半个订单。
- 外部编号须为文本；Excel已经截断/转科学计数的编号不猜测恢复。未知店铺商品可在待审核订单保留原始行及未配对原因，不能自动创建内部SKU。
- 任务状态为PENDING/RUNNING/SUCCEEDED/PARTIAL_FAILED/FAILED/INTERRUPTED；统计新建、同内容已存在、失败的不同结果，不把“跳过”算新建。
- 数据库唯一键和原创建内容指纹控制去重；重复输入同内容得到已有订单，同来源不同内容明确冲突，不覆盖待审核修改或已审核数据。
- 重试只处理失败/中断部分，带版本和幂等键，最多一个执行者。服务重启后的遗留RUNNING标记中断并允许显式重试，不声称内存队列保证重启续跑。
- 每个订单写入前重查实时权限。权限撤回或企业停用后停止后续处理，保留已提交记录；不把已提交订单伪称整批回滚。
- 原文件包含收件隐私，放专用私有导入存储，使用业务授权下载和有限保留策略，不能直接走当前企业级`/profile`附件读取。磁盘/企业额度、失败清理和留存回收方案须在步骤2/5明确并测试，不能绕过现有配额。
- 任务列表/错误摘要不暴露原始整行；失败明细需店铺及任务权限，不提供公开文件链接，不回显服务器存储路径。

## 8. 页面和后续步骤验收

页面按工作台、店铺与渠道、商品配对、订单、包裹、任务记录组织；仓储库存复用现有页面。
首页只聚合当前权限范围内的待审核/未配对/缺货/待发货/失败任务，不能全企业计数后只隐藏列表。
未提交时保留编辑内容，提交中禁止重复动作；异步弹窗沿用busy覆盖确认框、旧请求失效规则。
阶段7当前实现状态页签计数、审核快照、订单/包裹/商品配对互跳与导入结果订单链接。计数在当前订单范围SQL后聚合，失败显示未知；没有选仓库时不显示全局缺货数字，未配对/缺货在所选仓库审核检查里显示。没有实现全局未配对订单/缺货订单/失败任务数量大屏，导入失败处理复用任务页，不把入口称为聚合计数。
批量动作每批1—20条、前端串行调用原单笔接口，逐行保存请求键/版本/结果，不是服务端整批事务。成功项不再发送，未确认项用原请求显式重试，不用一个成功提示掩盖部分失败；关闭刷新后重新选批次才使用新参数。只读检查不预占，正式提交再次校验；通过检查不保证稍后的审核成功。

后续落地顺序：步骤2 DDL/显式升级 → 步骤3店铺/权限 → 步骤4商品配对 → 步骤5订单/导入 → 步骤6事务履约 → 步骤7页面收口 → 步骤8完整验收。
每步先定向测试再继续；步骤1—8的实际范围分别见对应记录，步骤8区分本地恢复通过和未认证的生产/GUI门槛。阶段5编辑需version及requestKey，来源单号/店铺不可变；导出是无收件信息的订单摘要，不是面单。导入一次初次执行加最多5次显式重试，保留原操作者，原文件24小时到期后需重传。阶段7页面收口不等于恢复验收，后者证据见阶段8，不能用接口回归冒充全量GUI测试。

验收必须覆盖：两企业同名数据、多店铺范围、旧会话撤权、旧库存入口旁路、SKU聚合、重复订单/动作、并发取消与发货、库存不足/停用、失败整体回滚、导入中断/重试/权限变化、隐私与导出。
新普通CRUD须有生成来源，领域事务不能由生成模板冒充；不自动给真实企业开通、不重置原账号密码、不清库/FLUSH。
