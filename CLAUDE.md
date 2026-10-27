# ruoyi-go
2026-10-27按用户选择完成本地基础ERP收口，Linux实机验收排除。正式可靠性入口默认预览/显式写新企业，补采购→出库→应收应付完整页面链，修复财务远程账户选择器的重复查询循环；不改变正式后端或SQL/索引/既有企业授权。结果、代码检查、交付哈希及边界见[收口报告](./docs/ERP_CLOSEOUT_2026-10-27.md)，复用入口见[可靠性指南](./docs/ERP_RELIABILITY.md)。本地验收不承诺永无缺陷；本轮结束，不自动新增功能或提交推送。

2026-10-26可靠性五项已本地完成：30分钟混合业务与六个事务/重启案例、独立账实核对、自然资源回落及边界GC诊断、完整双轮和新库/附件恢复，见[本轮报告](./docs/ERP_RELIABILITY_REVIEW_2026-10-26.md)。应用源码保持前轮哈希，未改表/索引/私有配置或既有企业授权，原8081进程未停动；仅更新证据和文档。Windows/Linux运行包带源码与文件清单，Linux仅交叉构建未实机验收；不自动提交推送，不外推生产容量。
2026-10-26订单读取后续：仅深偏移按实测门槛使用单条SQL选ID再取摘要；offset<10000保留原SQL与OFFSET。统计、授权集合候选未采用：前者存在历史排序规则口径且兼容后收益不足，后者使所有者变慢。工作台/概览源码已恢复本轮初始哈希，保留异常状态和范围回归。最终连续对拍、回归/审计/恢复及局限见[读取优化复核](./docs/ERP_ORDER_READ_OPTIMIZATION_2026-10-26.md)。下方“深分页/统计仍另行评估”为前轮快照；本轮不改DDL/索引/池，不自动提交推送。
2026-10-26后续：建单查重P2已通过单条UNION读取现有唯一索引优化，见[复核](./docs/ERP_ORDER_CREATE_OPTIMIZATION_2026-10-26.md)。同轮同夹具旧/新100次批次约3→88—356单/s，16并发P95约6.3s→60ms；额外3000单约8.08s全部成功，短批次不外推生产容量。保留同来源/内容换键重放、当前授权、事务/指纹和审计；无DDL/索引/池改动。12组集合比较、旧新专项、完整两轮54顶层/225前端及配套恢复通过；123新企业停用/范围0，58旧表摘要不变。下面“尚未改查重”属于前轮快照，原报告已加后续指针；深分页/统计仍另行评估。当前本地运行优化二进制，未提交推送。
2026-10-26第4项本地规模/负载测试完成，有待优化性能问题，见[方法](./docs/ERP_CAPACITY.md)和[报告](./docs/ERP_CAPACITY_REVIEW_2026-10-26.md)。5万单8并发混合读约68—71QPS/P95约0.36s，同店铺建单约4单/s且16并发P95约4.49s；执行计划显示查重OR只用租户索引前缀，尚未改业务查询。新增默认预览只读工具/有界测试器；正式13档14998次成功、151真实新增/同键一次、导出/短持续/本轮恢复通过。11新企业停用、范围0，58张旧表摘要不变。批量数据只作读取夹具，不当真实建单吞吐；早期CPU保护、工具校验错误和未完成阶段保留排除。开发机不是2C2G，不外推在线人数/生产或2小时稳定性。本轮无生产业务/SQL/前端改动，未提交推送。
2026-10-26第3项权限矩阵本地完成：独立ERP路由登记、三类入口拒绝及真实导入文件/结果/成功重试/导出的旧token撤权补验，见[矩阵](./docs/ERP_PERMISSION_MATRIX.md)与[本轮报告](./docs/ERP_PERMISSION_REVIEW_2026-10-26.md)。完整两轮54顶层、225前端、构建及本轮配套恢复通过；117新测试企业停用、范围0，58张旧业务/授权/范围表摘要不变。只加测试/文档，无生产Go、SQL或前端业务改动；完整路由检查不读取权限闭包字面量，不外推所有权限组合、浏览器、容量或生产认证。不动既有企业/密码/原库/远程，未提交推送。

2026-10-26跨模块第1、2项已完成：已有ERP模块的离页释放/旧finally归属、旧确认与校验后写入、原键/原内容重试已统一补强。具体能力按接口区分：持久请求键可重放，旧CAS/唯一编码冲突须查询，不能虚构原结果缓存。见[规则](./docs/ERP_ASYNC_OPERATIONS.md)和[本轮报告](./docs/ERP_ASYNC_REVIEW_2026-10-26.md)。仅改自有web与文档，不改后端/SQL/既有权限或密码，不写真实业务，不把前轮接口/恢复当本轮重跑，未提交推送。

2026-10-26履约离页保护已补：订单主页面、单笔审核/取消、包裹与批量履约在失活/卸载后作废旧确认和迟到结果，逐行循环停止未发送的后续单据；旧finally不得解除新操作的提交锁。已发请求仍可能成功，回页先核对，不宣称撤销服务器事务。使用说明/验证见[生命周期复核](./docs/ERP_FULFILLMENT_LIFECYCLE_REVIEW_2026-10-26.md)。本轮仅自有web和文档，不改后端/SQL、旧账号权限或远程，不建测试企业，不重复宣称前轮接口/恢复，未提交推送。

2026-10-26拣货打印行为已修复：每次仍重读权限/清单，只有完整新旧清单一致才保留内存未提交扫描；资料变化/拒绝清理，离开或同ID的新读取作废旧打印。仅改前端脚本、测试和文档，见[打印补验](./docs/ERP_PICKING_PRINT_REVIEW_2026-10-26.md)。本轮不改后端/SQL、不建测试企业，不将前轮接口/恢复证据当本轮重跑；没有本轮提交/推送，实体打印仍未实测。

2026-10-26扫码履约联动补验：保留原手工完整链路，新增正常/丢成功回包的扫码完整链路，接原包裹退货、应收/收款与物流记录；另补两入口同包裹竞争和双路径重试只扣一次。专项及完整两轮52顶层、前端150项、构建通过，见[联动报告](./docs/ERP_PICKING_LINK_REVIEW_2026-10-26.md)。仅测试/文档改动，不新模块、权限或SQL；生产Go源码构建哈希与既有运行文件一致。本轮资源/恢复与最终回执以该报告为准，不把旧GUI、硬件或容量当本轮重验。无本轮Git提交/推送，不动原库/远程/既有密码或权限。

2026-10-26当前导航：用户明确批准固化后，前245个已验文件本地提交为`a71b0e6`，没有推送；下文历史“未提交/不自动提交”不再描述这批文件的现状。本轮拣货与打包五项已本地完成，源码尚未提交，见[使用](./docs/ERP_PICKING.md)与[验收](./docs/ERP_PICKING_ACCEPTANCE.md)。9600—9603只登记定义；PACK_CHECK沿原事件、CHECKED_SHIP沿原库存事务，无新表/索引/012。旧手工SHIP独立权限保留，新仓库员模板采用复核路径，旧角色不自动变更。条码/规格或包裹版本变化使复核失效，原请求重试绑定员工/内容且重查当前双范围；查询日志和通用防重缓存不保存扫描正文。完整两轮48顶层、最终150前端、真实员工四视口与配套恢复通过；107新企业停用/范围0，58张旧业务授权范围表摘要不变。原库、远程、既有密码/权限不动，不外推实体打印、扫码硬件、Linux、容量或生产认证。

2026-10-25新企业基础启用五项已完成本地实现及测试：所有者专属9500/erp:setup:list与16项只读检查、五类角色模板预览/精确预填、已有配置入口和从零指南。见[ERP_GETTING_STARTED](./docs/ERP_GETTING_STARTED.md)、[验收](./docs/ERP_ENABLEMENT_ACCEPTANCE.md)及发布清单。模板不自动保存/分配/授权，父子联动关闭；个人企业不强制建员工，期初库存沿单据，不新表/索引/012。新隔离14306/16380/18082空环境实际安装与首笔业务/恢复通过；完整两轮46顶层、最终146前端、GUI/撤权和旧58表摘要通过，105新测试企业停用、范围0、验证码恢复。不动既有企业/旧密码/原库/远程，不自动提交推送；本地辅助工具不冒充公开生产安装器。

2026-10-25经营概览五步本地完成：六个只读汇总GET、菜单9400/erp:overview:list、六区域紧凑页面及原列表筛选跳转。沿原业务权限与实时仓库/店铺范围，按上海自然日、库存记录、毛收货和逐单/分币种余额统计；缺模块、错误和0分开。完整两轮/142项前端、真实员工四视口/撤权、58张旧业务授权范围表摘要和82表/两附件根/十一守卫恢复通过，见[ERP_OVERVIEW_ACCEPTANCE](./docs/ERP_OVERVIEW_ACCEPTANCE.md)，使用见ERP_OVERVIEW。本轮无表/索引/012迁移，菜单仅定义；测试企业全部停用、范围清零、验证码恢复，不自动给旧企业开通，不动原库/远程/旧密码，不自动提交推送。小夹具EXPLAIN不代表容量认证；此前同步审批阻塞已解除。

2026-10-25物流基础五步本地已完成：独立011四表、企业公司/配送方式、包裹目录选择与名称快照、发货记录/导出，保留原手工入口。全套两轮、135项前端、真实员工GUI/撤权及82表/两附件根恢复、十一组结构校验通过，见[ERP_LOGISTICS_ACCEPTANCE](./docs/ERP_LOGISTICS_ACCEPTANCE.md)，使用见ERP_LOGISTICS。Gin新旧包裹路由通配符统一packageId并补注册测试。新菜单仅定义，旧企业需显式开通；测试清理完毕，不改原库/旧密码/远程，不自动提交推送。本地通过不代表承运商接入、容量或生产认证。

已提交但客户端丢失成功回包的付款、收款、发货及双向退货已补本地验证，见[ERP_UNCERTAIN_OUTCOME_ACCEPTANCE](./docs/ERP_UNCERTAIN_OUTCOME_ACCEPTANCE.md)。新`TestERPBusinessFlowLostResponses`与普通链路共用步骤，先独立读证实已提交，再用原requestKey/version重试并要求existing=true；不在业务代码加故障开关。仅客户端传输层模拟，不外推到提交前超时取消、进程崩溃或网络分区。

同一商品贯穿010建档、采购/退货、店铺订单/发货/退货及应付应收核销的本地真实HTTP链路已补验，见[ERP_BUSINESS_FLOW_ACCEPTANCE](./docs/ERP_BUSINESS_FLOW_ACCEPTANCE.md)。`TestERPBusinessFlow`纳入默认完整集成回归；本轮只补测试/证据，未改业务逻辑、前端或SQL。更广的权限/并发/故障仍由各模块套件验证，不能以此单仓单SKU场景代替。

2026-10-25商品后联动复核已收口，见[ERP_LINK_REVIEW_2026-10-25](./docs/ERP_LINK_REVIEW_2026-10-25.md)：通用下载不能删除catalog/upload/avatar附件，delete=true仅当前上传者的企业download临时文件，完整GET后先精确删登记再清文件；分段/中断/304保留。首页及现状文档已纠正商品范围和基础财务说明。完整两轮、旧授权/商品摘要审计及清理后配套恢复通过，不新增SQL、不改既有企业、不自动提交推送；浏览器/容量/生产边界见报告。

2026-10-25商品基础1—5本地已完成：独立010五表、SPU/多SKU/分类/图片/条码尺寸、原SKU引用保护、整批新增导入及导出；用户已明确允许本地010安装。最终两轮、127项前端、真实页面/撤权、测试企业清理及78表/两附件根恢复通过，详见[ERP_CATALOG_ACCEPTANCE](./docs/ERP_CATALOG_ACCEPTANCE.md)，使用见[ERP_CATALOG](./docs/ERP_CATALOG.md)。浏览器自动化下载事件捕获未计通过，模板/导出实际HTTP与页面导入已验。旧企业未自动加权，001—009/原库/远程/旧密码未动，无本轮Git提交推送；不是全浏览器、容量或生产认证。此前安装拒绝与待确认记录为历史，不能当作当前仍未安装。

2026-10-24销售财务基础1—5本地已完成：独立009六表、复用008账户、已发货且明确金额订单显式应收、原退货人工抵减、分次/多单收款核销、整笔记账冲销、店铺对账及导出。真实员工页面/撤权、最终两轮及配套恢复已核验，见[ERP_SALES_FINANCE_ACCEPTANCE](./docs/ERP_SALES_FINANCE_ACCEPTANCE.md)，使用见ERP_SALES_FINANCE。既有001—008/原库/远程/密码不动，不给旧企业加权，不自动Git提交推送；业务记账不代表银行、退款、完整会计或生产认证。

2026-10-24采购财务1—5已实现：独立008七表、内部账户、收货应付、退货抵减、同供应商同币种分次/多单核销、整笔记账冲销及范围内对账/导出。源库安装在用户继续授权后显式执行，不自动补历史账或给既有企业开通。使用见[ERP_FINANCE](./docs/ERP_FINANCE.md)，最终验证证据与限制见[ERP_FINANCE_ACCEPTANCE](./docs/ERP_FINANCE_ACCEPTANCE.md)。不是银行转账、供应商退款、销售结算、总账或生产认证；仍限本地，源码未自动提交/推送。

## 当前多租户分支的已授权边界

2026-10-23用户批准仓库基础1—5：资料CAS/地址负责人、库存追溯/导出、预警、两阶段整单调拨、只读余额/流水核对。独立007五表，不改001—006，不自动给既有企业加权；仅本轮新企业测试，结束撤权/恢复验证码/停用。入口见[ERP_WAREHOUSE](./docs/ERP_WAREHOUSE.md)，设计或旧二进制不冒充最终验收。保留前轮006改动，本轮未提交/推送。

当前完成导航（2026-10-23）：采购退货已本地提交4c82ba7，下方“005源码尚未提交”为此前快照，不再描述Git现状。商业订单退货006的本地五项已完成，最终完整两轮/95项前端、仅退货员工GUI/撤权、55表与公开/私有附件配套恢复通过，见[ERP_SALES_RETURN_ACCEPTANCE](./docs/ERP_SALES_RETURN_ACCEPTANCE.md)。新模块未提交/推送，不代表退款、财务、远程、Linux、容量或生产认证。

2026-10-23用户批准本地回归的持续夹具授权：以后可仅在当轮新建专用测试企业内建员工/角色、分配测试店铺及仓库；GUI可临时关闭该企业验证码。结束撤权、恢复验证码、停用保留。不得扩展到既有企业、真实账号、旧密码、原库、远程或生产；不得重开旧测试企业代替当轮新建。当前商业订单退货1—5已获确认，设计/进度见[ERP_SALES_RETURN_DESIGN](./docs/ERP_SALES_RETURN_DESIGN.md)，不能用设计状态冒充验收完成。

`multi-tenant` / `erp` 分支采用独立多租户数据库、Redis 命名空间、上传目录与项目内 `web/` 前端。
以下 Java 单租户冻结规则仍约束 `main`、原数据库和参考前端；本分支新增契约见
[docs/MULTITENANCY.md](./docs/MULTITENANCY.md) 与 2026-10-19 决议。
历史 user_id=1 / role_id=1 不再用于鉴权。租户管理属于平台生命周期编排，不是可直接生成的普通 CRUD；
ERP 等普通业务 CRUD 继续使用生成器。缺失租户上下文必须拒绝，平台身份不能免除业务数据隔离。
本分支启动先只读校验多租户结构，禁止启动时自动迁移。现有新库的配额表由 tenantconsole 显式创建；
账号/附件配额必须按“企业行 → 配额行”顺序锁定，同一事务检查并登记，不能只在前端或service里计数。
企业管理写入必须校验持久版本；归档只保留数据并冻结访问，恢复为停用，不恢复旧会话。
新平台专属接口使用 `/platform/` 路径与 `platform:*` 权限；兼容的旧平台工具同样必须经平台入口检查。
不得把本分支直接连到原单租户库启动。尚未提交的多租户改动会随工作区移动，切分支前先提交或暂存。

2026-10-21用户已要求ERP一期步骤1—8：基线/设计、备份及显式结构升级、渠道/店铺/员工范围、商品SKU配对、待审核订单/私有模板导入、事务履约、页面工作台及本地验收收口，见
[docs/ERP_PHASE1_DESIGN.md](./docs/ERP_PHASE1_DESIGN.md)及[核对记录](./docs/ERP_PHASE1_BASELINE.md)。
步骤2的本地DDL及恢复演练见[docs/ERP_PHASE1_SCHEMA.md](./docs/ERP_PHASE1_SCHEMA.md)，步骤3见[docs/ERP_PHASE1_SHOPS.md](./docs/ERP_PHASE1_SHOPS.md)，步骤4见[docs/ERP_PHASE1_ITEMS.md](./docs/ERP_PHASE1_ITEMS.md)，步骤5见[docs/ERP_PHASE1_ORDERS.md](./docs/ERP_PHASE1_ORDERS.md)，步骤6见[docs/ERP_PHASE1_FULFILLMENT.md](./docs/ERP_PHASE1_FULFILLMENT.md)，步骤7见[docs/ERP_PHASE1_WORKSPACE.md](./docs/ERP_PHASE1_WORKSPACE.md)，步骤8见[docs/ERP_PHASE1_ACCEPTANCE.md](./docs/ERP_PHASE1_ACCEPTANCE.md)。本地回归及公开/私有附件配套恢复已验；不代表全部浏览器/手机真机、Linux实际部署、生产迁移或容量认证。
新菜单仅注册定义，不给已有企业/员工自动开通；本地前后端为测试启动，其他环境不动。此前基础收口的验证与生产边界继续有效。erpphase1默认只读，写入必须显式确认独立库及已核验备份报告。
2026-10-22用户明确“不要远程，先本地”。本轮本地补验已收口：员工范围/旧会话撤权、导入结果跳订单、ERP弹窗四视口与回归，见[docs/ERP_LOCAL_ACCEPTANCE_2026-10-22.md](./docs/ERP_LOCAL_ACCEPTANCE_2026-10-22.md)。只修改自有web的ERP布局/测试和文档，不再连接远程，不改生产/原库或既有账号密码；Linux部署、容量、生产升级/回退和异地备份按用户选择延后，不计成本轮失败或伪装通过。
一期拟做店铺、商品配对及订单履约；供应商、采购订单、真实渠道API和财务仍不在一期实施范围。

2026-10-22采购步骤2—6的本地范围已完成：供应商/采购草稿、精确金额/快照/动作、分批收货同事务库存、托管IN/PR旁路及三页。完整两轮erp-20261022T182232388Z-df62f004六步与79项前端、所有者GUI/四视口及配套恢复通过；获用户新目标明确授权后，仅收货员工GUI也完成，见[最终验收第10节](./docs/ERP_PURCHASING_ACCEPTANCE.md#10-仅收货员工实际页面补验与最终收口)。新企业10860的receiver仅有receive/历史与GUI-W，实际收4件，采购/供应商私有API拒绝；旧会话撤仓库后提交/原请求重试/重读明细拒绝，两列表为空，库存仍4/预占0。范围已撤回、验证码恢复true、企业停用保留，旧企业未重开、demo未加权。重放仍须当前范围，receive不隐含价格query，失败锁原键/版本/数量。无原库/远程/旧密码/已应用001—004改动、无Git提交推送；本地通过不代表生产、容量或完整ERP。
采购后续1—5的本地范围已收口：基线71个采购收货文件按清单核验后本地提交ed524c3，005退货源码尚未提交，均未推送。独立两表/托管PR/原批退货版本保留原采购收货事实，见[采购退货](./docs/ERP_PURCHASE_RETURN.md)。完整两轮及85项前端通过后，用户2026-10-23明确批准短编码purchase-return-ui-20261023：新企业10918的returner仅退货三权限及GUI-W，真实退2后毛10/退2/净8、实物8/预占3；旧会话撤仓库后提交/重试/重读拒绝，列表为空，已注销、恢复验证码true、撤范围、停用保留。四视口及含GUI记录的新53表配套恢复通过，见[验收第7节](./docs/ERP_PURCHASE_RETURN_ACCEPTANCE.md#7-2026-10-23-授权后的真实员工gui及最终收口)。不动demo/原库/远程/旧密码或001—004，不当作完整ERP/生产容量认证。

erp分支开发入口见[docs/ERP_FOUNDATION.md](./docs/ERP_FOUNDATION.md)，交付边界见
[docs/ERP_RELEASE.md](./docs/ERP_RELEASE.md)。验证优先使用`scripts/verify-erp.ps1`：默认无数据库接口测试，
只有显式`-Integration`才执行本地多租户回归并写入专用验收数据。不要误用下方旧单租户`test.ps1`作为本分支默认入口。

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
10. **新建业务表必须带** `create_by` `create_time` `update_by` `update_time` `remark`，对齐 RuoYi 的 `BaseEntity`

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
  （仅旧单租户入口；erp分支用`.\scripts\verify-erp.ps1`，真实本地回归须显式`-Integration`）
- 任何影响契约、分层、依赖的决定，追加一条到 @docs/DECISIONS.md
