# 接口与代码规范

> 物流011采用独立四表，原001—010及手工包裹契约不改。设置共享档案用logistics:list/query/add/edit；
> options/查重及新包裹shipping写复用package:edit，快照读用package:query；shipment:list/export另行授权并查店铺/仓库范围。
> 新选方式必须公司/方式启用，保存公司/方式名称快照；停用不改已保存包裹，不自动匹配旧手工代码。
> 待发货切回手工时同事务清当前关联，已发货不可改物流。查重只提示当前可见包裹的相同代码/运单，不返回隐藏资源线索。
> 档案持久原结果重放/CAS/最小审计；包裹选择沿用LOGISTICS动作域并区分目录/手工指纹，同事务推进订单/包裹版本而不动库存。
> 运单/正文不进入通用日志。规则详见ERP_LOGISTICS_DESIGN，完成状态只认独立验收报告。

> 多租户通用下载：企业共享附件的读取权限不包含删除权限。`/common/download` 的
> `delete=true` 仅允许当前上传者、本企业 `tenants/<tenantId>/download/` 内已登记的临时文件；
> catalog/upload/avatar 均拒绝此删除参数，使用资源下载或 `delete=false` 读取。
> 只有完整GET成功写回且请求未取消才清理；Range、HEAD、304及写入失败均保留。
> 清理必须先按企业、登记ID、storageKey、上传者精确删除登记，再删除托管临时文件；
> 登记删除失败或零行时保留物理文件。物理清理失败只记内部告警，可能遗留无登记的临时文件，
> 不能改回先删物理文件而留下被业务引用的失效登记。成功写回不等于客户端已保存。

> 销售财务009复用008内部账户，显式登记已发货订单总额；缺金额不当零，不虚构明细售价。退货抵减每次原退货一笔，人工金额/原因（明确零额可），累计不超过毛额；有未核对退货禁止新收款，后续超收不等于退款。应收/来源/对账/导出叠加当前店铺与仓库范围，收款全部核销范围可读才返回头总额/详情/重放。新菜单不自动给旧企业加权。金额字符串、原requestKey/版本/内容固定，最小无价款审计与业务同事务；不改原订单库存。启动只读，009发布后不可改写；使用/验证分别见ERP_SALES_FINANCE及独立验收。

> 采购财务008规则见[ERP_FINANCE](./ERP_FINANCE.md)：已安装001—008不可原地改写。应付按实际收货批次显式登记，价格冻结自原采购；退货抵减独立保留事实，有未登记退货时禁止新付款。付款固定供应商/币种/内部账户，核销恰好等于付款且不超过逐单待付；整笔冲销只撤本系统记账，不代表退款。应付按当前仓库范围，付款列表/详情/写入/重放须拥有全部核销仓库；财务价格权限独立于收货。事务先员工、供应商，再按动作锁来源或账户/已有付款/有序应付；业务、原结果、最小无价款审计同事务。金额使用十进制字符串；超限导出拒绝不截断，不能构建超大receiptId IN再查退货。页面失败隐藏旧价格，未知结果只允许固定原请求显式重试，不持久缓存正文；新菜单不自动授予既有企业。

> 仓库增强007独立CREATE、旧001—006冻结。资料编辑CAS且同快照读取；预警显式null清除但保留版本。调拨按员工→头→两仓ID排序→SKU/余额排序，TOUT/TIN、流水、状态、原结果和最小成功审计同事务，旧CRUD不得绕过，草稿也冻结SKU单位。任一仓侧可读此调拨不等于拥有对方库存范围；dispatch源、receive目标，重试仍查当前范围。导出限额/范围，来源另查原query/店铺范围；核对只返回合计，包括零净额孤立流水，不自动修账。详见[ERP_WAREHOUSE](./ERP_WAREHOUSE.md)。

> 多租户分支先遵循 [MULTITENANCY.md](./MULTITENANCY.md) 的新契约。
> 原库、原参考前端不改；企业编码用于登录选择，会话绑定 tenantId；数据库查询必须携带服务端租户上下文。
> 平台接口另行检查，不能由角色名称或历史固定 ID 获得平台身份。Redis 使用 ruoyi_mt: 命名空间。
> 新业务实体必须具有只由服务端赋值的 TenantID，标量查询/更新使用 repository.TenantTable。
> JOIN 仅支持已登记的简单表关联，复杂 SQL 必须显式审查；不能把隔离守卫当成任意 SQL 的安全解析器。
> 多租户 `/getInfo` 增加顶层 `identity`，用于显示当前企业、用户名、角色中文名及平台身份。
> 展示数据不是授权凭据；平台租户管理同时校验服务端平台身份和 `platform:tenant:list/add/edit/grant/resetPwd` 权限。
> 平台菜单只授予平台空间，不得为展示“超级管理员”而提升客户账号或取消租户过滤。
> 企业功能授权和管理员密码重置必须校验目标企业编码、持久版本并原子记录成功审计；密码重置还须再次校验当前平台管理员密码。
> 撤回功能必须先清理角色关联，再清理企业开通项；重新开通不能恢复普通角色已撤回权限。
> `operatorPassword` 与 password/newPassword/confirmPassword 同样脱敏；含密码的前端请求不得进入 sessionStorage 防重缓存。
> 普通企业编辑/交接同样必须提交 version。配额、归档、恢复使用独立平台权限，客户注销申请仅限当前所有者。
> 账号额度计入未逻辑删除的全部账号；附件额度计入登记元数据。检查与登记同事务，锁顺序必须是企业行 → 配额行。
> 单文件有效上限是企业上限和服务器上限的较小值；0表示企业不额外限制，不得绕过服务器限制。
> 待注销/归档不能走普通编辑启用；恢复/驳回只变成停用，另行启用且必须重新登录。
> ERP业务只在erp分支；商品/仓库CRUD从生成器扩展。库存不能由CRUD直接覆盖，必须通过单据状态机，
> 同事务写余额、不可变流水和单据状态；写入先锁当前员工，纠错再锁原单→当前单据→仓库→按SKU排序的商品与余额。
> 草稿编辑必须校验version，只允许DRAFT；原创建requestKey/hash不得重写，已过账只能用关联退货/整单冲销纠错。
> 退货确认在原单锁下复核累计可退数量；写事务用READ COMMITTED，分组聚合避免旧快照与无限明细加载。
> 盘点保存账面和库存版本快照；期间即使库存往返到原值也拒绝旧盘点，必须重新核对实盘并编辑草稿。
> ERP功能、企业、仓库权限同时检查。当前所有者拥有全部仓库，员工默认无仓库；授权版本校验、成功审计与替换同事务。
> 仓库范围由服务端身份实时查询，涵盖列表、详情、写入和已有仓库导出；平台身份不能代入客户业务。SKU是企业级共享目录。
> 自有前端授权弹窗的busy必须覆盖确认框与提交，按确认前的目标/版本/仓库快照发送；关闭重开与切换员工须使旧响应失效。
> 回归的状态变更使用专用验收企业，结束停用保留记录；不借真实示例企业改所有者或状态，不为测试重置已有密码。
> 功能测试遵守登录限流窗口，等待正常TTL而不清计数/改IP；组件状态、真实浏览器和原账号恢复证据分别记录，不能互相冒充。
> ERP基础开发复用现有生成器与实际SKU/仓库/单据示例，不为基础收口新建业务模块或任意工作流。
> 本分支默认验证用`scripts/verify-erp.ps1`，不运行旧单租户API包；真实回归须显式`-Integration`，独立日志保留未执行状态。
> 现有tenantinit/tenantconsole/erpinit仅支持其代码允许的本地独立环境，不得解除DSN保护冒充生产迁移；生产升级需单独演练并登记兼容/失败方案。
> ERP一期设计见[ERP_PHASE1_DESIGN.md](./ERP_PHASE1_DESIGN.md)；结构验收不等于业务验收。步骤3已扩展生成器店铺骨架，渠道目录只读、无平台API；店铺资料写入仅限所有者并校验version，不提供物理删除。已引用的编码/渠道/站点/币种不可改，名称/启停仍可改。
> 店铺范围同样为实时SQL，员工默认无范围。GET/PUT授权均限当前所有者；PUT必须version及显式shopIds，null/缺失拒绝，[]撤回，正整数去重校验、上限200。锁顺序为当前员工→目标员工→排序店铺；替换、版本和完整成功审计同事务，审计失败回滚。长集合分片保存审计，禁止截断。
> 店铺新菜单只登记定义，不给已有企业或角色自动授权。新建企业仍按现有默认开通规则初始化；若将来要求默认不开放新模块，另行修改开户规则，不能把本条“已有企业不变”误解为改变开户契约。
> 店铺商品是一行一个可售变种，从erp_shop_item实际生成后扩展；列表/详情/写入均检查实时店铺范围。shopId创建后不可转移，停用代替物理删除；有任何配对历史或订单引用后，shopItemCode及外部标识不可改。外部ID始终字符串，保留前导零和大小写，拒绝首尾空白/控制字符；可选外部ID的空串/null统一为SQL NULL。
> 商品编辑、配对和解绑共享商品version作为CAS令牌；mappingVersion是另一个持久历史版本，不能拿它替代请求version。配对/解绑必须在当前员工→店铺→商品→配对→SKU的顺序下重新校验；首次配对建立行，解绑只停用，重新配对复用原行并递增版本。成功审计与两个版本同事务；非终态订单已引用该配对时拒绝变更，终态历史按原行快照解释，不能回查当前配对覆盖。
> 建立配对要求店铺/商品/SKU均启用；已停用店铺或商品仍可按权限读历史、解绑，但不能新增/编辑商品或建立配对。SKU仍为企业共享目录，沿用原有单位冻结规则，不因新增配对模块放宽或另改它。新订单写入必须先锁店铺，不能绕过配对引用保护的串行边界。
> 浏览器验收可在用户明确允许后仅临时关闭专用测试企业的验证码，并在收尾恢复原值/停用该企业；不能外推到平台或真实示例企业，不能以临时配置代替正常鉴权。
> 商业订单录入/导入只创建PENDING_REVIEW，不改变库存。创建幂等指纹不随编辑改写；编辑需version/requestKey，来源单号/店铺不可变。订单头和行用一致快照读，写入按员工→店铺→订单，同事务写行/事件/脱敏成功审计；未审核行不保存配对/SKU审核快照。
> 订单/导入不用通用OperLog捕获正文/响应，不缓存收件正文到浏览器防重缓存；日志仅元信息，列表/摘要导出不含收件字段。任务/私有原文件/结果同时检查创建者或当前所有者及实时店铺范围；平台不代入。
> 导入仅固定XLSX，5000非空行/每单100行/10MiB及较小企业/服务器上限，64MiB总解压/32MiB单文件解压、10000物理数据行；拒绝公式、宏、外链和数值化编号/电话/邮编。同单任何行错误整单失败；一订单及其结果行同事务，成功部分不回滚。持久任务单执行者，排队100全局/20企业，重启PENDING/RUNNING标中断，不自动续跑；每单重查企业/账号/功能/店铺权限，重试仍使用原操作者、version及requestKey，最多5次。
> 导入原文件在upload.path的私有兄弟目录（后缀-order-import-private），不在/profile下；sys_file的erp-import:元数据只用于共享配额，不是公开资源URL。登记按企业→配额→员工→店铺→任务锁序。文件保留24小时，终态先删物理文件再同事务释放配额/标记DELETED；失败下轮重试，孤立文件按固定命名和年龄清理。备份要明确包含私有目录，不能把旧附件恢复记录外推为已覆盖。
> Redis脚本在pipeline中必须使用EVAL而非Script.Run依赖EVALSHA自动降级：错误到Exec才暴露，Run不能在那时补发NOSCRIPT回退。权限版本仍原子推进且不设TTL，不改既有Key前缀。
> 商业订单履约按当前员工→店铺→订单→已有包裹→已有库存单据→仓库→排序SKU/余额加锁，库存内原有相对锁序不变；审核新建单据/包裹时由父订单锁保护尚未提交的新行。商品/配对与履约共同先锁店铺，批量定位配对，不反向以SKU锁再取仓库。一个repository事务统一调用事务内库存步骤，不能拼接多个公开事务或HTTP。
> 审核按当前有效配对聚合内部SKU并复核每SKU总数1—十亿；订单行保留原始行、配对版本和SKU/单位快照，包裹行保存原订单行关联及SKU快照。全部库存预占、快照、单包裹、订单版本、事件和成功审计同事务；任一失败整体回滚。待审核录入的数量上限不等于可绕过审核汇总上限。
> 订单审核/取消使用订单version，包裹物流/发货使用包裹version；物流保存同时推进订单和包裹版本但不变库存。取消待审核不动库存，取消待发货同时释放预占并取消包裹；已发货不可取消。停用店铺/仓库/SKU不阻止有当前范围的人取消释放，但审核/发货仍须有效主数据，填运单须启用店铺。
> 每动作requestKey/hash与原结果同事务保存；相同键/相同输入返回原结果而非最新版本，不重复流水。同键不同输入拒绝，重放也先查当前企业身份、店铺及适用仓库范围。发货须先持久保存物流代码/文本运单号；只表示本地出库，不承诺平台回传或揽收。
> 订单托管的SALE不得经旧库存入口独立创建重放、编辑、确认、取消或出库，即使所有者也不能绕过。旧单据读取、流水、幂等重放及关联SR均叠加实时店铺和仓库范围；先查范围再返回托管提示，不能用错误文案探测越权单据。独立手工单仍只按原仓库范围；SR是库存纠错，不改写原商业订单已发货事实，也不代表售后/退款。
> 包裹列表不含运单/收件资料；详情含手工物流但不含订单收件资料。订单详情仅投影包裹ID/状态/版本，不借订单query泄露物流字段；读取收件资料必须另走订单详情及其授权。包裹动作不挂通用正文操作日志/防重缓存，HTTP日志不记其查询串，成功审计仅ID/版本/动作，取消原因和运单不落通用日志。
> 工作台状态统计必须在数据库应用与订单列表相同的实时店铺范围后聚合；统计按店铺/来源号，不被当前状态页签再次裁剪。统计失败显示未知，不显示假0；列表与计数是独立读取，不承诺同一瞬间快照。没有选仓库时不伪造全局缺货数字。
> 审核检查仅只读一致快照，复用实际审核的配对解析/数量上限；权限为order:review加实时店铺和所选仓库范围，已审核订单还检查原分配仓库，不能换选仓库探测状态。响应使用专用无收件DTO，不能返回ERPOrder；允许有审核权限但没有订单query的人读取最小SKU需求/可用/缺口。检查不锁定、预占或持久写入，正式审核始终重新鉴权和校验；阻断结果不能代替后端事务。
> 页面批量动作最多20条、逐单串行调用既有单笔接口，不冒充整批事务。确认前固定目标/版本/参数，各行requestKey保留到本批结束；成功项不重发，未确认项显式用原请求重试，不偷偷换版本/键。检查失败/跳过/未确认均逐行展示，不用一条成功提示覆盖部分结果；未确认不是失败未提交的保证。换仓库或关闭检查使旧响应失效，换批参数应关闭后刷新重选。
> 订单/包裹/配对深链接只接受可精确表示的正整数ID，仍依赖各自接口权限，不因来自本企业页面而免除授权。旧详情/收件资料在关闭、链接切换或卸载时清除并使旧响应失效；包裹链接与预期订单不一致不打开。订单详情与包裹互跳分别检查功能权限；库存/配对历史按审核快照展示，不按当前商品回填。
> 一期DDL以嵌入的003资源为单一来源，启动只读；迁移仅本地显式执行，先备份恢复核验，既有结构漂移拒绝而非自动修复，已登记DDL不得原地改写。erp_schema_migration是系统技术元数据，不是可生成的租户业务表。
> 采购004同样冻结、显式迁移/只读启动；供应商企业共享、编码不可变、停用不删除。联系人仅supplier:query，采购list/写结果不回价款，采购query才返回精确字符串报价；金额按万分位定点计算，不用float64/JS Number，不代表成本/应付。
> 采购单供应商/仓库创建后固定；确认冻结资料/数量/价格快照，草稿起保护SKU单位和历史引用。收货在一个repository事务内先锁员工→供应商→采购单→仓库→排序SKU，再插入新FK子行并复用事务内库存过账，不能拼接公开事务；累计已收/状态/version/事件/最小审计同事务。
> 分批收货仅增加实物、不改预占、不超余量，收齐完成，关闭不回滚已收；供应商停用允许已确认单收货，仓库/SKU停用仍阻止入库。幂等指纹包含当前操作者/目标/版本/规范化行，先查当前功能和仓库范围再返原结果，不以当前状态替代原状态、不因重试再次入库。
> receive作业和purchaseReceipt历史必须用无价格/联系人DTO，不能前端隐藏列；库存/采购详情分别校验自己的query。采购托管IN及其来源PR仅阻断旧创建重放/编辑/确认/取消等写入旁路，保留有范围读取。元数据兼容探测仅固定表名局部System，不能带入业务及关联子查询。
> 采购/收货页面首次写前固定ID/版本/键/白名单内容，失败锁住原请求并显式重试；关闭后先查询结果，不默默换键或升级版本。详情关闭/失活/卸载必须清空及作废晚响应，不存浏览器持久正文缓存。下拉有名称的冻结快照优先，只有ID的占位不得覆盖实际名称；新菜单只登记定义，不替已有企业/员工扩权。

只写**稳定的规则**。带日期的历史决议放 [DECISIONS.md](./DECISIONS.md)。

商业订单退货按原包裹行累计，同SKU原行不能互借余量，库存再按SKU汇总过账；头、行、托管SR、库存、流水及最小审计单事务，原订单/包裹已发货事实不回写。旧SKU级已过账SR不能猜分到原行；已有商业退货不得再通过旧入口过账同源SR，原SALE锁下互斥，保留历史读取。重放需当前功能、店铺和仓库范围；退货DTO无收件/运单/价格。独立006只显式本地升级；已应用001—005不可改。见ERP_SALES_RETURN_DESIGN。

采购退货005按原收货批次串行，version为不可变退货头计数+1，不是采购version。毛收货不回写，独立记录退货/净收货，不重开采购。锁序为员工→供应商→采购→原收货→原IN→仓库→有序SKU/余额，主数据先于新FK子行。退货/PR/余额/流水/成功审计同事务；托管来源例外只准repository私有精确证明，禁止客户端/context开关。list/query/return独立、无价格；重放须当前功能/实时仓库，失败锁原键/版本/原因/数量。005显式本地安装、先配套备份恢复，不启动迁移或给已有企业加权，见[采购退货](./ERP_PURCHASE_RETURN.md)。

原Java兼容接口以`../RuoYi-Vue-master`源码为参考，行为存疑时先核对实际实现并遵循三层边界。ERP新增领域（例如商业订单退货）另有本分支设计/契约/验收，不能声称Java参考版已有对应能力，也不借此修改原兼容接口。

---

## 一、响应格式

RuoYi 的 `AjaxResult` 继承自 `HashMap`，所以**额外字段是平铺在顶层的，不在 `data` 里**。这是最容易写错的地方。

### 1. 普通响应（对应 `AjaxResult`）

```json
{ "code": 200, "msg": "操作成功", "data": {} }
```

- `data` 为 nil 时**整个键不出现**，不是 `"data": null`
- 失败：`{ "code": 500, "msg": "具体错误" }`

### 2. 分页响应（对应 `TableDataInfo`）

```json
{ "code": 200, "msg": "查询成功", "total": 100, "rows": [] }
```

`total` 和 `rows` **平铺在顶层，没有 `data` 包裹**。

### 3. 平铺扩展字段（全量清单）

很多接口在 `code`/`msg` 之外**直接挂顶层字段**。下表是全仓库扫描 `ajax.put(...)` 得到的完整清单，实现时必须逐个比对：

| 接口 | `data` | 平铺字段 |
|---|---|---|
| `POST /login` | 无 | `token` |
| `GET /getInfo` | 无 | `user`、`roles`、`permissions`、`pwdChrtype`、`isDefaultModifyPwd`、`isPasswordExpired`，多租户分支另有 `identity` |
| `GET /getRouters` | 路由树 | — |
| `GET /captchaImage` | 无 | `captchaEnabled`、`uuid`、`img` |
| `GET /system/role/deptTree/{roleId}` | 无 | `checkedKeys`、`depts` |
| `GET /system/menu/roleMenuTreeselect/{roleId}` | 无 | `checkedKeys`、`menus` |
| `GET /system/user/authRole/{userId}` | 无 | `user`、`roles` |
| `POST /system/user/profile/avatar` | 无 | `imgUrl` |
| `POST /common/upload` | 无 | `url`、`fileName`、`newFileName`、`originalFilename` |
| `POST /common/uploads` | 无 | `urls`、`fileNames`、`newFileNames`、`originalFilenames` |
| **`GET /system/user/{userId}`** | **有**（SysUser） | `postIds`、`roleIds`、`roles`、`posts` |
| **`GET /system/user/profile`** | **有**（SysUser） | `roleGroup`、`postGroup` |
| **`GET /system/notice/listTop`** | **有**（公告列表） | `unreadCount` |

**最后三行是混合形态**：既有 `data`，又有平铺的兄弟字段。这类最容易写错，实现时格外注意。

几个特例：

- `captchaEnabled` 为 false 时，`uuid` 和 `img` 都不返回
- `GET /system/user/` （不带 userId，用于"新增用户"弹窗）只返回 `roles` 和 `posts`，没有 `data`、`postIds`、`roleIds`
- 树选择类接口的节点结构是 `{id, label, disabled, children}`，`children` 为空时**该键不出现**（不是空数组）

### 4. Go 侧写法

平铺字段一律用 `response.New(...).Put(...)`，**禁止为了"结构清晰"包进 data**：

```go
// 对 GET /system/role/deptTree/{roleId}
response.New(response.CodeSuccess, response.MsgSuccess).
    Put("checkedKeys", checkedKeys).
    Put("depts", deptTree).
    JSON(c)

// 混合形态：data 和平铺字段并存
response.New(response.CodeSuccess, response.MsgSuccess).
    Put("data", user).          // 注意 data 也是 Put 进去的
    Put("postIds", postIDs).
    Put("roleIds", roleIDs).
    JSON(c)
```

判断依据只有一条：**打开 Java 版对应的 Controller 看它 `put` 了什么**，不要凭直觉设计。

### 5. 文件下载接口（导出/模板）

前端 `download()` 的行为决定了三条硬约束：

**① 查询条件在请求体里，不是 query string**

`download()` 用 `POST` + `Content-Type: application/x-www-form-urlencoded`。
必须用 `c.ShouldBind`（按 Content-Type 自动选表单绑定），
用 `ShouldBindQuery` 会拿到空条件、**静默导出全表**。

**② 错误响应必须用 `response.FailDownload`**

前端靠 `blobValidate()` 区分文件和错误，而它是**严格相等**比较：

```js
return data.type !== 'application/json'
```

gin 的 `c.JSON` 写的是 `application/json; charset=utf-8`，比较不相等 →
前端当成文件 → `saveAs` 存下一个内容是 JSON 的 `.xlsx`。
用户拿到打不开的文件，且**看不到任何错误提示**。
（Spring 返回的是不带 charset 的 `application/json`，所以 Java 版没这个坑。）

handler 里对应用 `failDownload(c, err)` 而不是 `fail(c, err)`。

**③ 文件必须先在内存里构建完再写响应**

边生成边写的话，中途出错时响应头已经发出，没法再改成 JSON 错误。
`excelx.WriteResponse` 保证出错时一个字节都不写。

**④ 有上限就要报错，绝不静默截断**

`service.MaxExportRows = 100000`、`service.MaxConcurrentExports = 2`。

超限一律报错。**截断后照常返回文件是最坏的做法** —— 用户拿到一份看起来正常、
实际少了数据的表格，而且不会发现。宁可让他导不出来，也不能给错的数据。

提示语必须说清三件事：查出来多少条、上限多少、下一步怎么办。
只说"导出失败"等于让用户去猜。

**⑤ 上限由内存决定，不是由 xlsx 格式决定**

⚠️ **这一节的结论被实测推翻过一次，改之前先看数字。**

`pkg/excelx` 的基准（10 列，贴近 SysUser）：

| 行数 | 堆峰值 | 分配总量 | 分配次数 | 耗时 |
|---|---|---|---|---|
| 1 万 | ~9 MB | 50 MB | 98 万 | 80 ms |
| 10 万 | ~90 MB | 321 MB | 980 万 | 796 ms |

完全线性：约 **900 字节/行常驻**、**每行 98 次分配**。

- xlsx 格式上限是 1048575 行 → 外推 **约 940 MB 堆峰值**，
  而整个服务压测时常驻才 65 MB。一次导出内存翻十几倍，两三个人同时点就 OOM，
  正好抵消掉换 Go 省下来的内存。所以上限取 **10 万行 ≈ 90 MB**。
- 只限行数不限并发，行数上限就是摆设（10 人各导 10 万行 = 900 MB），
  所以还要 `middleware.ExportLimit`。

**曾经写在这里的错误结论**：「StreamWriter 只保留少量行 → 瓶颈在数据切片而非工作簿」。
实测数据切片只占堆峰值的 **16%**，另外 84% 是导出过程本身。
StreamWriter 确实在流式写，它只是 churn 很凶，堆峰值仍随行数线性增长。
**改成分批查库只能省下那 16%，是白费力气。**

重新验证：`go test ./pkg/excelx/ -bench=Export -benchmem -run=^$`
以及 `go test ./pkg/excelx/ -run=ReportExportMemory -v`。

**⑥ 导出用流式写入**

`excelx` 内部走 excelize 的 `StreamWriter`，对齐 Java 版的 `SXSSFWorkbook`。
代价是只能顺序写、不能回头改单元格。
注意它降低的是**峰值增长的斜率**，不是把内存变成常数（见上一条的实测）。

### 6. Java 响应契约转换层（`internal/handler/java_contract.go`）

有一批接口的响应**不是直接序列化 Go 模型**，而是先过一层逐字段 map 转换，
复刻 Java 那边的三类可观察差异：

| 差异 | 来源 |
|---|---|
| 某些字段固定为 `null` | Java 的 MyBatis `selectXxxVo` 根本没 select 那一列，Jackson 就输出 `null`；Go 从数据库把整行读回来了，直接序列化会输出真实值 |
| `children` / `roles` 固定为 `[]` | Java 实体里是 `new ArrayList<>()`，永远不是 `null` |
| `params: {"@type":"java.util.HashMap"}` | 登录会话和字典缓存是 Fastjson 反序列化出来的，空 HashMap 带类型标记 |

#### 受影响的接口（改这些模型时必须同步检查转换）

| 转换函数 | 用在哪 | 抹成 null / 固定值的字段 |
|---|---|---|
| `contractRole` / `contractRoles` | 角色列表、角色下拉、用户表单里的候选角色、**角色详情** | `createBy`、`updateBy`、`updateTime` |
| `contractPost` / `contractPosts` | 岗位列表、岗位下拉、用户表单里的候选岗位、**岗位详情** | `updateBy`、`updateTime` |
| `contractDept` / `contractDeptList` | 部门列表、排除子部门列表 | `parentName`、`updateBy`、`updateTime`、`remark`；`children` = `[]` |
| **`contractDeptDetail`** | **部门详情（选列与列表不同，见下）** | `createBy`、`createTime`、`delFlag`、`updateBy`、`updateTime`、`remark`；`children` = `[]`；**`parentName` 有值时输出真实值、无父级时输出 `null`** |
| `contractMenu` / `contractMenuList` | 菜单列表、**菜单详情** | `parentName`、`createBy`、`updateBy`、`updateTime`、`remark`；`children` = `[]`；`perms` 为 nil 时输出 `""` |
| `contractDictType` / `contractDictTypes` | 字典类型列表、字典类型下拉、**字典类型详情** | `updateBy`、`updateTime` |
| `contractDictDatum` / `contractDictData` | 字典数据列表、**字典数据详情** | `updateBy`、`updateTime` |
| `contractCachedDictData` | **按类型查字典**（走 Redis 缓存） | 同上，另加 `params` 类型标记；`cssClass` 空串输出 `null` |
| `contractUserList` | 用户列表 | `pwdUpdateDate`、`updateBy`、`updateTime`；`roles` = `[]`；删掉 `userType` |
| `contractUser(user, true)` | 用户详情 | 删掉 `userType`；`roleId` = `null`；填充 `roles` 和 `dept` |
| `contractLoginUser` | **`/getInfo` 的 `user`** | 在 `contractUser` 基础上加 `params` 标记，空 `avatar` / `updateBy` 输出 `null` |
| `contractEmbeddedRole` | 用户对象里内嵌的角色 | `SysUserMapper.RoleResult` 只填 6 个业务字段，其余抹掉 |
| `contractUserDept` | 用户对象里内嵌的部门 | 非 full 时只保留 `deptId`/`deptName`/`leader` |
| `contractNotices` | 公告管理列表 | 无字段改写，仅统一出口 |

#### 详情接口容易被漏掉

这层转换最初只铺了列表和下拉，**6 个详情接口全漏了** ——
岗位、字典类型、字典数据、角色、菜单、部门。表现是新增后 `updateBy` 是 `""`、
修改后是 `"admin"`，而 Java 恒为 `null`。

不带转换的详情只剩两个，因为它们的 `selectVo` 是**全选**的：

| 接口 | 为什么不用转换 |
|---|---|
| `GET /system/notice/{noticeId}` | `selectNoticeVo` 选了全部列，含 `update_by` / `update_time` |
| `GET /system/config/{configId}` | `selectConfigVo` 同上 |

**不要"顺手"把这两个也抹掉** —— 它们的 `updateBy` 该有值就得有值，
测试里有反向断言压着（`TestNoticeAndConfigDetailKeepValues`）。

⚠️ **部门是唯一一处「详情和列表选列不同」的接口。**
Java 的 `selectDeptById` 没有 `include selectDeptVo`，而是单独写了一条 SQL：
比列表**少了** `create_by` / `create_time` / `del_flag`，
**多了** `parent_name`（子查询取父部门名）。两个方向都有差异，
所以它有独立的 `contractDeptDetail`，不能复用 `contractDept`。

#### 五条硬约束

1. **给上表这些实体加字段、或改 repository 的 select 列时，必须同步检查
   `java_contract.go`。** 漏改的表现是响应里多出一个 Java 没有的字段，
   或者本该为 `null` 的字段冒出真实值。`TestContractBaseMapsMatchModelJSON` 会拦住
   模型新增字段却漏改基础 map，但判断 Java 的选列和 `null` 语义仍要靠定点断言与双端对拍。
2. **判断某个字段该不该抹成 `null`，去打开 Java 的 mapper xml 看它 select 了哪些列**，
   不要凭直觉，也不要照抄实体定义 —— 实体有那个字段不代表查询填充了它。
3. **只在响应层转换。** 数据库模型保持适合写入和校验的 Go 类型（指针、`types.Time` 等），
   不要为了对齐响应去改模型。
4. **改任何响应字段前，先 `grep` 测试里对该字段的断言。**
   `assertField(t, detail, "updateBy", ...)` 这类断言散落在各个测试文件里，
   只盯着 handler 和新写的测试就会漏。给岗位详情挂转换那次就撞了 ——
   `post_test.go` 里压着一条 `updateBy == "admin"`，那是照着改之前
   Go 自己的输出写的，把不对齐固化成了"期望"。
5. **被转换抹掉的字段，如果背后有真实行为，要落到数据库层去验。**
   `sys_post.update_by` 现在不通过岗位接口暴露，但「修改时要记录操作人」这件事还在 ——
   用 `assertColumn(t, "sys_post", "update_by", "post_id", id, "admin")` 直接查库。
   这是**唯一**该绕开接口去断言的场景，别拿它当常规手段。

#### 唯一可靠的验证手段是双端对拍

见下方 `cmd/contractcheck`。这层转换是靠人工比对 Java 源码写出来的，
**没有对拍就没有保障**。

### 7. 双端对拍工具 `cmd/contractcheck`

真起 Java 和 Go 两个服务，对同一批请求逐字段比较响应。

```powershell
# 1. 两端必须用不同的 Redis DB
#    Go 用 DB 0，Java 用 DB 1 —— 两边的 value 序列化格式不同
#    （Java 走 Fastjson 带 @type），共用一个 DB 会互相读坏对方的
#    会话、验证码和配置缓存。

# 2. 起 Java（端口 8081）
java -Xmx512m -jar ..\RuoYi-Vue-master\ruoyi-admin\target\ruoyi-admin.jar `
     --server.port=8081 `
     --spring.data.redis.database=1

# 3. 起 Go（端口 8080，用配置里的 Redis DB 0）
go run ./cmd/server

# 4. 对拍（手动启动双服务时）
go run ./cmd/contractcheck                       # 36 个核心 GET + 57 个补充路由场景
go run ./cmd/contractcheck -file-probes          # 加 4 项上传 / Excel 探针
go run ./cmd/contractcheck -write-probes         # 加 7 项非持久化错误场景
go run ./cmd/contractcheck -crud-probes          # 加 10 组、60 项一次性 CRUD
go run ./cmd/contractcheck -permission-probes    # 加 18 项数据/功能权限
go run ./cmd/contractcheck -validation-probes    # 加 16 项字段校验和 JSON 类型

# 5. 推荐：一键启动、对拍、停止和清理
.\scripts\contractcheck.ps1 -All -AllowDifferences

# 6. 重新生成完整路由/权限/探针覆盖清单
go run ./cmd/routeaudit -java-root ..\RuoYi-Vue-master
```

期望输出：

```text
SUMMARY matched=36 different=0 total=36 percent=100.00%
SUPPLEMENTAL_PROBE_SUMMARY matched=52 different=5 total=57 percent=91.23%
```

补充场景中的当前 5 项受控差异见 `DECISIONS.md`；不传 `-AllowDifferences` 时进程返回非零。
严格 CI 可以直接使用该退出码，允许既有差异的人工复核才传 `-AllowDifferences`。

默认 36 个探针由 **23 个固定路径 + 13 个动态路径**组成。动态部分覆盖
**11 个详情接口和 2 个角色树接口**；工具先读取两端列表，自动找出双方共有的 ID，
再请求同一条记录，禁止把种子库里的 `1`、`2`、`100`、`103` 写死到探针里。
这依赖两端连接**同一个 MySQL**，Redis 则必须继续使用不同 DB。

补充 57 项覆盖剩余读取、登录/登出、解锁、导出和写路由。依赖实体 ID 的读取同样先从两端
列表取共同 ID；当前库没有对应记录时才用非法 ID 执行双方错误语义。破坏性写路由只执行未登录
拒绝场景，证明路由和共同认证边界，不能据此宣称成功副作用已经对拍。

**判定口径**：比较的是规范化后的 JSON，不是只看 `code=200`。
字段缺失、`null` 与空串、数字与字符串的类型差异、数组顺序变化都会判失败。
验证码图片、UUID、token、登录 IP/时间这类动态字段已登记为忽略。

`-crud-probes` 覆盖用户、角色、部门、菜单、岗位、参数、字典类型、字典数据、公告、
定时任务共 **10 组、60 项**，每组固定比较新增、新增后详情、修改、修改后详情、删除、
删除后详情。测试值使用 `zz_contract_` 前缀；部门父 ID 和字典类型从当前库动态解析。
两端共用 MySQL 时，唯一字段必须使用不同值；工具只忽略两端必然不同的动态 ID、
唯一测试值和时间字段。

`-permission-probes` 创建一次性部门、角色和用户，比较五种数据范围（全部、自定义、
本部门、本部门及以下、仅本人）下的用户/角色/部门列表，共 15 项；再比较无功能权限
访问三个列表，共 **18 项**。角色菜单权限从当前菜单动态读取。每次切换 `dataScope`
后必须重新登录两端，不能复用 Redis 中缓存旧权限的会话。

`-validation-probes` 共 **16 项**，同时输出两种摘要：`VALIDATION_PROBE_SUMMARY`
是响应字段、类型和值的严格比较；`VALIDATION_SEMANTIC_SUMMARY` 只判断非法输入是否被
双方拒绝。错误文案不同会导致严格口径不一致，但不应误报为校验语义放行。任何一端
意外写入的数据必须按本次唯一值定位并回收。

正常结束、发现差异和初始化失败都必须清理本次创建的业务记录、验证码和登录会话，
禁止使用 `FLUSHDB` 或前缀全删。`scripts/contractcheck.ps1` 会检查端口，从 Go DSN
读取当前 MySQL 连接（不输出密码），给 Java 临时注入同库连接，隔离两端 Redis DB，
构建临时可执行文件，并在 `finally` 中停止精确进程和删除临时文件。
文件探针运行时，两端上传根目录也必须指向该次运行目录，随运行目录一起删除，不能污染
Go 或 Java 的默认 `uploadPath`。
`-AllowDifferences` 仅用于记录已知差异；要求完全一致的 CI 不应传该开关。

`cmd/routeaudit` 静态提取 Go 路由和 Java Controller，统一尾斜杠与路径参数名，比较
HTTP 方法、结构化路径和权限标识，并标注上述探针覆盖。代码生成器已经纳入路由契约；
只排除 Swagger 内存演示接口和 dev/test Profile。清单中的
“已纳入探针”只表示存在自动执行场景，实际通过与差异必须看当次 `contractcheck` 日志。

### 代码生成器

- `/tool/gen/**` 的 13 条路径、方法、参数位置、权限和响应层级以 Java/Vue3 为冻结契约；
  `createTable` 继续只允许 `admin` 角色。
- 元数据只读取当前 `DATABASE()` 的 `information_schema`；`qrtz_`、`gen_` 表不能导入。
  单次导入最多 200 张表、编辑最多 500 个字段、单次建表最多 20 张且 SQL 不超过 1MB。
- 建表入口只接受未指定数据库名的 `CREATE TABLE`，不得借代码生成器执行任意 SQL。
  DDL 不会在测试或启动时自动运行。
- 生成物是本项目可用的 Go `model/repository/service/handler/router`、Vue API/页面和菜单 SQL；
  Go 路由文件通过 `generatedRouteRegistrars` 自动登记，不要求手改中心路由。
- `crud`、`tree`、`sub` 三类模板都必须产出可运行语义：树形页面按每页 100 条稳定翻页后再组树，
  不能只取第一页；树形新增支持选择父节点和从当前行新增子节点，修改时必须排除自身子树；
  该前端过滤只用于改善操作体验，生成的 service 必须独立校验父节点存在，并拒绝把节点挂到
  自身或自身后代；这些可预期的树校验失败必须返回 `errs.BizError`，数据库故障才使用内部错误；
  repository 必须一次读取父子关系后在内存追溯，不得沿父链循环查库；删除树节点前必须批量
  检查是否存在未同时删除的直接子节点，存在时明确拒绝，不得留下孤儿节点；
  树编码和父编码只允许使用相同的整数或字符串类型；
  主子表的详情、增改删必须同时处理子表，并在同一数据库事务内完成写操作。
- 生成模块必须包含受 `MaxExportRows` 和全局并发限制保护的 Excel 导出接口、前端按钮、
  `*:export` 路由权限和菜单权限；超限时明确失败，禁止静默截断。
- 生成前必须确认树字段真实属于主表、主表主键与子表外键类型一致；生成实体的物理主键必须带
  GORM `primaryKey`，自增主键还必须带 `autoIncrement`，保证主表新增后的 ID 能回填并传给子表。
  自动递增主键由服务端清零，更新主键和必填字段不得只依赖前端校验。
- 生成实体同时作为请求对象时，不能给数值和布尔值机械添加 `binding:"required"`，因为 validator
  会把合法的 `0`/`false` 当作缺失；确需区分“未传”和“显式零值”的模块应改用指针请求 DTO
  并补业务校验。字符串必填仍生成服务端校验。更新存在 `update_by`/`update_time` 时，repository
  必须显式持久化 service 写入的审计值，不以生成配置的 `is_edit` 开关为前提。
- ZIP 和预览始终可用。自定义路径写盘默认关闭；即使显式开启，也只能写入
  `gen.outputRoot` 内，不能用 `genPath` 跳出根目录。
- 生成器只写生成物，不自动执行菜单 SQL、不自动修改生产表结构、不自动覆盖已有文件。
  Windows 覆盖已有文件时必须先做同目录可恢复备份。模板变化必须补渲染测试，并在
  `DECISIONS.md` 登记可观察差异。

### 测试和 Race Detector

```powershell
.\scripts\test.ps1 -Unit
.\scripts\test.ps1 -Unit -Race -CCompiler D:\path\to\gcc.exe
```

普通结果写入 `test/results/summary.log` 和 `latest.log`；Race 结果单独写入
`summary-race.log` 和 `latest-race.log`，不能互相覆盖。Windows 上 `-Race`
会启用 CGO，并把 `-CCompiler` 所在目录临时加入 `PATH`，因为 GCC 还要调用同目录的
`as.exe`。脚本退出时必须恢复 `CGO_ENABLED`、`CC` 和 `PATH`。

接口测试只清理自己能精确归属的 Redis key：登录后从 JWT 记录会话 key，
验证码响应记录 UUID，防重复提交按同一算法记录 key；密码错误计数只扫描
`pwd_err_cnt:zz_test_*`，限流只清理 httptest 固定 IP `192.0.2.1` 的三个键。
删除后必须逐项 `EXISTS` 验证，清理失败要让整个测试进程失败。

**换库或换环境后**：详情样本 ID 会从两端列表自动解析，无需修改探针常量；
如果新库缺少某类可选记录（例如没有根部门或子部门），应补足测试数据，
或重新审查该探针的查询路径和语义前提，**不要改业务代码去迎合旧数据**。

### 状态码

| code | 含义 |
|---|---|
| 200 | 成功 |
| 401 | 未认证 / token 失效 |
| 403 | 已认证但无权限 |
| 413 | 请求体超过入口上限 |
| 500 | 业务或系统错误 |

普通业务响应的 **HTTP 状态码一律 200**，错误通过 body 里的 `code` 表达（RuoYi 前端
就是这么判断的）。入口在请求体超过 `server.maxRequestBodyMB` 时返回真实 HTTP 413，
同时仍输出统一 `{"code":413,"msg":"..."}`；`/health` 依赖异常时返回真实 HTTP 503。

---

## 二、认证与会话

### JWT 的真实设计

**JWT 里不存用户数据**，只有两个 claim：

```
claims = { "login_user_key": "<uuid>", "sub": "<username>" }
```

签名算法 HS512。`Constants.java` 里虽然定义了 `JWT_USERID`、`JWT_AVATAR`、`JWT_CREATED`、`JWT_AUTHORITIES`，但 `TokenService.createToken()` 从未使用，是遗留常量——**不要照着常量表往 token 里加字段**。

推论：**除了 uuid 和 username，任何用户信息都必须去 Redis 取**，禁止写出"从 JWT 里读 userId"这种逻辑。

真实会话存在 Redis：`login_tokens:<uuid>` → 序列化的 LoginUser（含 user、roles、permissions、过期时间、登录 IP 等）。

**必须照做**，否则"在线用户""强制退出""权限变更即时生效"这些功能全部失效。

- 请求头：`Authorization: Bearer <token>`
- 默认有效期 30 分钟；剩余不足 20 分钟时自动续期（刷新 Redis TTL，不换 token）
- 登出即删除 Redis key
- 新会话必须同时维护 `login_user_sessions:<userId>` 反向索引、
  `login_user_generation:<userId>` 会话代数，并把当前
  `login_user_permission_version:<userId>` 写入会话 JSON
- 普通续期必须比较当前 `sessionRevision`，冲突后重新读取最新会话；只能从最新快照生成
  完整 JSON，禁止整体写回请求最初读取的旧权限快照
- 权限更新和续期必须同时比较 `sessionRevision`、会话代数和权限版本；冲突后重新读会话、
  重新查数据库，无法确认最终权限时安全撤销会话
- 会话 JSON 必须由 Go 编码后由 Lua 原样写入；禁止 `cjson.decode -> cjson.encode`，否则
  空权限数组会被 Redis Lua 改成对象。菜单 `perms/status` 变化必须刷新所有关联角色会话
- 按用户刷新权限和撤销会话只走反向索引，禁止退回 `SCAN login_tokens:*`；过期 token
  留下的索引成员要清理
- 按角色刷新权限必须先从 `sys_user_role` 查询当前成员，再按用户反向索引刷新；禁止为了找
  角色成员扫描全站 Redis 会话。索引 Pipeline 每批最多 200 个用户，会话 MGET 每批最多
  100 个；在线用户的资料、部门、角色和权限必须批量加载，禁止逐用户查库。批量读取会话
  失败必须向上传递，不能跳过当前批次后宣告成功
- 权限写入必须先在数据库写入前固定受影响用户 ID，再推进这些用户的 Redis 权限版本；成员
  查询或 Redis 版本推进失败时不得提交数据库变更。数据库提交后主动刷新会话使用保留 trace
  值、脱离客户端取消且自带 5 秒上限的 context
- 登录的数据库权限快照、权限版本读取和 Redis 会话创建必须处于同一权限临界区。鉴权发现
  会话版本落后时必须懒加载数据库权限并 CAS 写回；数据库、Redis 或写回失败时按未登录拒绝，
  不能继续使用旧权限。权限版本不设 TTL，普通重启或进程崩溃后仍是安全判断依据
- 进程内补偿队列只负责尽快回收无效会话 key，不再承担安全正确性。待处理 ID 最多 100000，
  每批 200，错误详情最多 20 条，重试按 250ms、500ms、1s、2s、5s 指数退避；日志必须汇总
  pending、重试次数和最老等待时间。服务退出必须停止该 worker
- Java 完全切换到 Go 时如存在旧会话残余，必须按 [DEPLOYMENT.md](./DEPLOYMENT.md)
  停服并精确清理四个会话/权限前缀；普通重启不得清理

### 密码

bcrypt，直接沿用 `sys_user.password` 里现有的 hash，**Java 版生成的密码 Go 能直接校验**。默认账号 `admin / admin123`。
用户不存在或已删除时也必须比较一个同 cost 的固定合法 hash，不能通过响应耗时暴露账号是否存在。

### 匿名放行路径

`/login`、`/register`、`/captchaImage`，以及静态资源 `/profile/**`。其余一律鉴权。

---

## 三、Redis key 命名

必须与 Java 端完全一致（对照 `CacheConstants.java`），否则两边不能共存：

| 前缀 | 用途 |
|---|---|
| `login_tokens:` | 登录会话 |
| `login_user_sessions:` | Go 用户到登录 UUID 的反向索引 |
| `login_user_generation:` | Go 用户会话撤销代数 |
| `login_user_permission_version:` | Go 用户持久权限版本（无 TTL） |
| `captcha_codes:` | 验证码，TTL 2 分钟 |
| `sys_config:` | 参数缓存 |
| `sys_dict:` | 字典缓存 |
| `repeat_submit:` | 防重复提交 |
| `rate_limit:` | 限流 |
| `pwd_err_cnt:` | 密码错误次数，5 次锁 10 分钟 |

统一定义在 `pkg/redisx`，禁止在业务代码里裸写字符串前缀。

### ⚠️ key 名一致 ≠ 数据能互通

Java 的 `RedisTemplate` 配了 `FastJson2JsonRedisSerializer`，写入时带 `@type`，
**连 String 也会被 JSON 化**（验证码 `1234` 存进去是 `"1234"`，带引号）。
Go 侧写的是自己的 JSON。

所以 key 名对齐只保证**两边不会互相踩键**，不代表能读懂对方的数据。
**做不到"Java 和 Go 同时在线、会话互通"** —— 灰度要按用户或按域名整体切，不能混流。
详见 [DECISIONS.md](./DECISIONS.md) 2026-08-23 那条。

---

## 四、分页

前端传参（query）：

| 参数 | 说明 |
|---|---|
| `pageNum` | 页码，从 1 开始，缺省 1 |
| `pageSize` | 每页条数，缺省 10，**硬上限 100** |
| `orderByColumn` | 排序字段，**必须用白名单校验**，禁止拼接 |
| `isAsc` | `asc` / `desc` |

`orderByColumn` 直连 SQL 是注入面，每个接口必须显式声明允许排序的列。

### 分页必须行序确定：`pg.Stable(默认排序, 唯一兜底列)`

```go
// ✓ 两个参数都要显式写，即使相同
err := db.Order(pg.Stable("post_id", "post_id")).
    Offset(pg.Offset()).Limit(pg.PageSize).Find(&list).Error

// ✓ 默认排序可以是多列、可以带方向；兜底列必须是单列且唯一
pg.Stable("r.role_sort, r.role_id", "r.role_id")
pg.Stable("info_id DESC", "info_id")

// ✗ 前端没传排序时不加 ORDER BY
if pg.OrderBy != "" { db = db.Order(pg.OrderBy) }

// ✗ 只按业务列排，并列值时行序未定义
orderBy := "role_sort"
```

| 参数 | 含义 | 约束 |
|---|---|---|
| `fallback` | 前端没传 `orderByColumn` 时用的完整排序 | 可多列、可带方向 |
| `tiebreaker` | 追加在用户指定排序后面的兜底列 | **单列、唯一、不带方向** |

行为：没传排序 → `fallback`；传了 → `<用户指定的>, <tiebreaker>`。

**不要试图从 `fallback` 里解析出兜底列。** 这里踩过坑：
原先只收一个参数、用 `strings.Fields(fallback)[0]` 推断，
遇到多列 fallback `"r.role_sort, r.role_id"` 切出来是 `"r.role_sort,"`（带逗号），
拼出 `ORDER BY r.role_sort asc, r.role_sort,` 直接 SQL 语法错误、接口 500。
调用方本来就知道主键叫什么。

**`tiebreaker` 传空串不会被容忍** —— 没有"为空就不追加"的分支。
留那个口子等于给"绕过稳定排序"开一条不报错的路；真忘了传，SQL 会立刻报错。

不接受前端排序的分页查询不走 `Stable`，直接在 SQL 里写死确定性排序，
判断标准是**最终排序里必须含唯一列**：

- 公告已读用户 `ORDER BY r.read_time DESC, u.user_id` ——
  `read_time` 精度到秒会并列，必须补主键
- 角色授权用户 `ORDER BY u.user_id` ——
  单列即唯一，已经确定，不需要再追加

**为什么两种情况都要管**：`LIMIT + OFFSET` 在「没有 ORDER BY」和「排序键有并列值」
两种情况下，行顺序都是未定义的 —— 翻页可能**重复某行或漏掉某行**。
症状是"某条记录在列表里怎么都找不到，翻回上一页又出现了"，
没有任何报错，极难复现。

`dict_sort`、`role_sort`、`post_sort` 这类列大量并列（新建时默认都是同一个值），
是最容易踩到的。

**与 Java 的差异是有意的**：Java 的 mapper 多数只写单列排序甚至完全不排序。
加主键兜底会让双端对拍在有并列值时报差异 —— 那是预期的，
**不要靠去掉兜底列来消差异**。行序不属于接口契约，前端依赖的是字段和结构。

内存分页同样必须先建立包含唯一键的全序，再执行切片；稳定排序不能替代唯一兜底。
在线用户列表固定使用 `LoginTime DESC, TokenID ASC`，避免 Redis SCAN 输入顺序变化造成
跨页重复或遗漏。5000 条保护上限按 SCAN 返回的 key 数计数，必须在 MGET、JSON 解码和
用户名/IP 过滤之前裁剪最后一批；不能按有效会话或过滤命中数计数，否则损坏 key 和零命中
查询仍会扫描全部会话。

> 判断 Java 到底有没有排序，要看 **service 转调到哪条 SQL**，不能只看 mapper 里
> 有没有同名 select。例如 `selectRoleAll()` 转调的是 `selectRoleList`，
> 那条才带 `order by r.role_sort`。曾经据此误删过排序，而且双端对拍没发现 ——
> 因为种子数据里 `role_id` 和 `role_sort` 恰好同序，样本把差异盖住了。

---

## 五、权限

### 功能权限

标识形如 `system:user:list` / `system:user:add`，在路由注册时挂载，不写在 handler 里。超级管理员（`user_id = 1`）拥有 `*:*:*`，跳过全部校验。

### 数据权限

`sys_role.data_scope` 取值：

| 值 | 含义 |
|---|---|
| 1 | 全部数据 |
| 2 | 自定义（查 `sys_role_dept`） |
| 3 | 本部门 |
| 4 | 本部门及以下 |
| 5 | 仅本人 |

多角色取**并集**（Java 端用 `OR` 拼接）。实现放 `pkg/datascope`，返回 GORM Scope，禁止字符串拼 SQL。

“本部门及以下”继续与 Java 一致使用 `find_in_set`。2026-08-22 在 10 万用户数据下实测，
改成 `ancestors` 前缀匹配只快 3.6%；去掉真正昂贵的 `DISTINCT` 和无用 JOIN 后，整条查询
约 1.5ms。没有新的实测证据不得切换实现，详见 [DECISIONS.md](./DECISIONS.md) 2026-08-22
「补四个索引（推翻 find_in_set 的判断）」决议。

---

## 六、命名映射

同一个字段在三个地方有三种写法，必须显式声明，不能靠框架默认转换：

| 位置 | 写法 | 例 |
|---|---|---|
| 数据库列 | snake_case | `user_name` |
| Go 字段 | PascalCase | `UserName` |
| JSON | camelCase | `userName` |

```go
type SysUser struct {
    UserID   int64  `gorm:"column:user_id;primaryKey" json:"userId"`
    UserName string `gorm:"column:user_name" json:"userName"`
    DeptID   *int64 `gorm:"column:dept_id"   json:"deptId"`
}
```

- 缩写全大写：`ID` / `URL` / `IP`（Go 侧），JSON 侧仍是 `userId` / `avatarUrl`
- 可空列用指针或 `sql.Null*`，不要用零值冒充 NULL
- 状态类字段在 RuoYi 里是**字符串**不是数字：`status` = `"0"`（正常）/ `"1"`（停用），`del_flag` 同理。不要擅自改成 int 或 bool

---

## 七、参数校验

### 零值陷阱（必读）

Go 的零值语义会在**两个地方**咬人，两处都是静默失败，不报错但行为不对：

**1. `binding:"required"` 会拒掉合法的零值**

`required` 判定 int 的 `0`、bool 的 `false`、string 的 `""` 都是"未填写"。所以：

```go
PostSort int `binding:"required"`   // ✗ postSort=0 会被拒，而 0 是合法排序值
PostSort int `binding:"min=0"`      // ✓
```

**规则：`required` 只用在字符串和指针字段上。数值和布尔字段一律用 `min` / `max` / `oneof` 表达约束**，确实需要区分"没传"和"传了零值"时，把字段声明成指针。

### 数值必填：Java 的 `@NotNull` 只能用指针翻译

`@NotNull` 在 Integer 上是**必填**，不传就拒。Go 侧只写 `min=0` 是翻译不过来的：

| 写法 | 不传 | 传 0 | 传 -1 |
|---|---|---|---|
| Java `Integer` + `@NotNull` | **拒** | 过 | 过 |
| Go `int` + `min=0` | **过**（当成 0） | 过 | 拒 |
| Go `*int` + `required,min=0` | **拒** | 过 | 拒 |

`int` + `min=0` 在"不传"这一格上**比 Java 更宽** —— 会放进 Java 版拒绝的请求，
正是下面"与 Java 校验规则的偏离"里明令禁止的方向。

所以所有排序字段都用指针：

```go
OrderNum *int `binding:"required,min=0"`   // ✓ 不传拒、0 过、负数拒
OrderNum int  `binding:"min=0"`            // ✗ 不传时静默当成 0
```

入库前用 `model.IntValue(p)` 解引用，不要在业务代码里散落 `if p != nil`。

> 这个漏洞是接口契约审计发现的：只登记了"负数比 Java 严"，
> 漏了"不传比 Java 宽"。**登记偏离时两个方向都要看。**

注意 RuoYi 的 `status` / `delFlag` 是**字符串** `"0"`，用 `required` 没问题；但如果哪天建模成 int，`status=0` 就会被拒。

**2. `GORM Updates(struct)` 会跳过零值**

同一个根因。用 `Updates` 传结构体时，值为零的字段不会进 UPDATE 语句，导致「把排序改成 0」「把状态改回正常」静默失效。

```go
db.Model(&m).Where(...).Updates(post)                          // ✗
db.Model(&m).Where(...).Select("post_sort", "status").Updates(post)  // ✓ 显式列出可改字段
```

### 查询条件必须是独立结构体

**不要用实体结构体接列表接口的查询参数。**

Java 版 list 和 add/edit 复用同一个 `SysPost`，靠 `@Validated` 只加在 add/edit 方法上来区分。
Go 的 binding tag 写在结构体字段上，无法按接口区分——用实体接 `ShouldBindQuery`，
`required` 会立刻触发，导致不带参数就查不了列表。

```go
type SysPost struct {                       // 实体：带 binding，供 add/edit
    PostName string `binding:"required,max=50"`
}
type PostQuery struct {                     // 查询：只有 form 标签，无 binding
    PostName string `form:"postName"`
}
```

### 禁止用 oneof 硬编码字典取值

**凡是取值来自 `sys_dict_data` 的字段，一律不准写 `oneof=...`。**

字典是后台可编辑的内置功能。管理员往 `sys_normal_disable` 加一个"2 冻结"，
前端的 radio/select 是 `v-for="dict in sys_normal_disable"` 渲染的，会立刻多出这个选项；
而后端的 `oneof=0 1` 会把这个完全合法的提交拒掉，且报错信息指向"参数不合法"，
排查时根本想不到是校验写死了。

涉及的字段包括但不限于：`status`、`delFlag`、`noticeType`、`sex`、`visible`、
以及所有业务模块自定义的字典字段。这类字段只写 `required`。

确实需要校验取值合法性时，在 service 层拿 `GetDictDataByType` 的结果比对，
不要写进 binding tag —— 那是编译期常量，字典是运行期数据，两者生命周期不同。

### 注解映射表

写新模块时照此逐字段翻译，**不要凭印象**：

| Java 注解 | Go binding | 说明 |
|---|---|---|
| `@NotBlank` | `notblank` | **不是 `required`**，见下方陷阱 |
| `@NotNull`（Integer） | `*int` + `required` | **必须用指针**，见下方"数值必填"一节 |
| `@Size(min=0, max=N)` | `max=N` | `min=0` 是空约束。两边都按字符数算，中文均计 1 |
| `@Email` | `omitempty,email` | 内置规则已被覆盖成"空值合法"；`omitempty` 仍不可省，见下方陷阱 |
| `@Pattern(regexp=...)` | 自定义 tag | Go validator 无通用正则 tag，在 `pkg/validate` 注册 |
| `@Xss`（RuoYi 自定义） | `xss` | 已在 `pkg/validate` 实现，禁止 HTML 标签 |
| 无注解 | 无 tag | 不要自作主张加 |

### 两个必踩的陷阱

**1. `required` 比 `@NotBlank` 宽**

Go 的 `required` 对字符串只判断 `""`，`"   "` 能通过；Java 的 `@NotBlank` 会拒。
这属于"比 Java 更宽"，是被禁止的。所以注册了 `notblank`：

```go
PostName string `binding:"notblank,max=50"`   // ✓
PostName string `binding:"required,max=50"`   // ✗ "   " 会存进去
```

**2. 指针字段有两个独立的失败点，必须同时挡住**

以 `Email *string binding:"...email..."` 为例，前端有两种"没填邮箱"的表现：

| 前端行为 | Go 里的值 | 会怎么失败 | 靠什么挡 |
|---|---|---|---|
| 请求体不含 `email` | nil 指针 | validator 对 nil 指针**直接判失败，不调用规则函数** | `omitempty` |
| 传 `"email": ""` | 非 nil 指针 → `""` | `hasValue` 对非 nil 指针返回 true，`omitempty` **不跳过**，落到 `email` 规则上判失败 | 覆盖内置 `email` |

所以两者缺一不可：

```go
Email *string `binding:"omitempty,email,max=50"`   // ✓ 两个点都挡住
Email *string `binding:"email,max=50"`             // ✗ 字段不传时报错
Email *string `binding:"omitempty,email"`          // 若没覆盖 email 规则，传 "" 时报错
```

内置 `email` 已在 `pkg/validate` 覆盖为"空值合法"（对齐 Jakarta 的 `@Email`）。

**推广到所有可选的指针字段：`binding` 一律以 `omitempty` 开头。**
`max` / `min` 这类规则对空串本身是通过的，所以只有 `email`、`url`、`uuid`
这种"对空值判失败"的规则才需要额外覆盖。

### Java 校验全量清单

| 实体 | 字段 | Java | Go binding |
|---|---|---|---|
| SysPost | postCode | `@NotBlank` `@Size(64)` | `notblank,max=64` |
| | postName | `@NotBlank` `@Size(50)` | `notblank,max=50` |
| | postSort | `@NotNull` | `min=0` |
| | status | 无 | `required`（见偏离登记） |
| SysDept | deptName | `@NotBlank` `@Size(30)` | `notblank,max=30` |
| | orderNum | `@NotNull` | `min=0` |
| | phone | `@Size(11)` | `omitempty,max=11` |
| | email | `@Email` `@Size(50)` | `omitempty,email,max=50` |
| SysUser | userName | `@Xss` `@NotBlank` `@Size(30)` | `notblank,xss,max=30` |
| | nickName | `@Xss` `@Size(30)` | `omitempty,xss,max=30`（**无 NotBlank**） |
| | email | `@Email` `@Size(50)` | `omitempty,email,max=50` |
| | phonenumber | `@Size(11)` | `omitempty,max=11` |
| SysRole | roleName | `@NotBlank` `@Size(30)` | `notblank,max=30` |
| | roleKey | `@NotBlank` `@Size(100)` | `notblank,max=100` |
| | roleSort | `@NotNull` | `min=0` |
| SysMenu | menuName | `@NotBlank` `@Size(50)` | `notblank,max=50` |
| | orderNum | `@NotNull` | `min=0` |
| | path | `@Size(200)` | `omitempty,max=200` |
| | component | `@Size(200)` | `omitempty,max=200` |
| SysDictType | dictName | `@NotBlank` `@Size(100)` | `notblank,max=100` |
| | dictType | `@NotBlank` `@Size(100)` `@Pattern` | `notblank,max=100,dicttype` |
| SysDictData | dictLabel | `@NotBlank` `@Size(100)` | `notblank,max=100` |
| | dictValue | `@NotBlank` `@Size(100)` | `notblank,max=100` |
| | dictType | `@NotBlank` `@Size(100)` | `notblank,max=100` |
| | cssClass | `@Size(100)` | `omitempty,max=100` |
| SysConfig | configName | `@NotBlank` `@Size(100)` | `notblank,max=100` |
| | configKey | `@NotBlank` `@Size(100)` | `notblank,max=100` |
| | configValue | `@NotBlank` `@Size(500)` | `notblank,max=500` |
| SysNotice | noticeTitle | `@Xss` `@NotBlank` `@Size(50)` | `notblank,xss,max=50` |
| | noticeType | 无 | `required`（见偏离登记） |
| | remark | 无 | `omitempty,max=255`（**不是 500**） |
| SysJob | jobName | `@NotBlank` `@Size(64)` | `notblank,max=64` |
| | invokeTarget | `@NotBlank` `@Size(500)` | `notblank,max=500` |
| | cronExpression | `@NotBlank` `@Size(255)` | `notblank,max=255` |

注意 `SysUser.nickName` **没有** `@NotBlank`，只有 `@Xss` 和 `@Size` —— 昵称允许为空。
不要"顺手"给它加必填。

### 与 Java 校验规则的偏离

允许比 Java 更严，但必须满足**两个**条件，且要在此登记：

1. 前端正常操作走不到（含后台改配置、改字典这类内置操作）
2. 约束本身不随运行期数据变化

| 字段 | Java | Go | 理由 |
|---|---|---|---|
| `SysPost.postSort` | `@NotNull`，允许负数 | `*int` + `required,min=0` | `required` 对齐 `@NotNull`；`min=0` 是加严，前端表单本就是 `:min="0"`，负数排序无业务含义 |
| `SysDept.orderNum` / `SysMenu.orderNum` / `SysRole.roleSort` | 同上 | 同上 | 同上 |
| `SysDictData.dictSort` | **无 `@NotNull`**（裸 `Long`） | `*int` + `required,min=0` | 这个是纯加严：排序列存 NULL 会让列表顺序不可预期，而前端总会带值 |
| `SysPost.status` | 无校验 | `required` | `sys_post.status` 是 `char(1) not null` 且**无默认值**，不传会 SQL 报错。（`sys_dept.status` 有 `default '0'`，所以那边不加，这个不一致是有依据的） |
| `SysPost.remark` | 无校验 | `omitempty,max=500` | 对齐列长，见下 |
| `SysDept.leader` | 无校验 | `omitempty,max=20` | 对齐列长，见下 |
| `SysRole.status` | 无校验 | `required` | `sys_role.status` 是 `char(1) not null` 且无默认值 |
| `SysRole.remark` | 无校验 | `omitempty,max=500` | 对齐列长 |
| `SysUser.password` | 无校验 | `omitempty,min=5,max=20` | 前端表单规则就是 5–20；空密码会生成一个"空串的哈希"，账号等于被锁死 |
| `SysUser.remark` | 无校验 | `omitempty,max=500` | 对齐列长 |
| `SysNotice.noticeType` | 无校验 | `required` | `sys_notice.notice_type` 是 `char(1) not null` 且无默认值 |
| `SysMenu.menuType` | 无校验 | `required` | 空值会产出一条既不是目录也不是菜单的记录，`getRouters` 直接乱掉。前端 radio 是硬编码的 M/C/F，不是字典 |
| `SysMenu` / `SysDictData` 的其余字符串字段 | 多数无 `@Size` | `omitempty,max=列长` | 见下方"补齐列长校验" |

⚠️ **`remark` 的列长不是所有表都一样**：`sys_notice.remark` 是 `varchar(255)`，
其它表是 `varchar(500)`。照抄 500 会让超长备注在 SQL 层报错。

### 补齐 Java 漏掉的列长校验

Java 有若干字段没标 `@Size`，但数据库列有长度限制（`remark varchar(500)`、`leader varchar(20)`）。
超长时 Java 也会失败——只是失败在 SQL 层，报的是驱动异常而不是可读提示。

**所以给这类字段补 `max=列长` 不算"比 Java 更严"**：两边都拒绝，只是我们的错误信息能看懂。
新增模块时，**所有字符串字段的 `max` 一律对齐数据库列长**，Java 没标也要标。

反过来**不允许比 Java 更宽**——那会放进 Java 版会拒绝的脏数据，两版并行验证时行为对不上。

### 校验失败的提示

统一走 `handler.bindMessage(err)`，会带出首个失败字段名和规则，例如
`参数 PostSort 不合法（规则：不小于 0）`。

**不要回 "参数错误" 这种无信息量的提示** —— 前端看不出哪个字段有问题，排查要靠猜。
但也不要回显用户输入的值，避免把敏感内容原样打回去。

## 八、错误处理

- 业务错误用哨兵 error 或自定义类型，在 handler 边界翻译成 `code` + `msg`
- **`msg` 是给用户看的**，不要把 SQL 错误、堆栈、内部路径塞进去
- 详细错误进日志，用 `slog` 带上 `traceId`、`userId`、`path`
- 第三方/上游错误在边界处归一化成自己的错误枚举，业务层只认自己的枚举

---

## 九、建表

新建业务表必须包含（对齐 `BaseEntity`）：

```sql
create_by    varchar(64)  default '',
create_time  datetime,
update_by    varchar(64)  default '',
update_time  datetime,
remark       varchar(500) default null
```

- 主键统一 `bigint auto_increment`
- 逻辑删除用 `del_flag char(1) default '0'`，`'2'` 表示已删除（RuoYi 的约定）
- 需要数据权限的表必须有 `dept_id`，需要"仅本人"的必须有 `user_id`
- 所有查询条件列必须建索引，日志类大表额外考虑按时间分区或定期归档

---

## 十、本地上传文件生命周期

- 文件已经落盘、但数据库引用写入失败时，必须回收本次新文件
- 替换资源时只能在数据库提交成功后删除旧文件；并发替换需要锁定或等价机制，确保每次
  删除的是自己真正替换掉的地址
- 数据库存储的资源地址是不可信输入，删除必须同时校验 URL 前缀、业务子目录和最终磁盘
  归属；禁止把地址直接拼接后交给 `os.Remove`
- 外部 URL、其他业务目录、目录本身和中间目录软链接不属于可清理对象，应安全跳过或拒绝
- 文件系统和数据库不能做原子事务；清理失败记录内部告警并交给后续巡检，不能谎称已经
  提交的数据库更新被回滚

头像使用 `upload.RemoveManagedFile(upload.path, URLPrefix, "avatar", resource)`；新增其他
本地资源类型时传各自固定子目录，不能为了复用放宽为整个 `upload.path` 任意删除。

---

## 十一、运行时资源与关联写入

- 当前部署边界是单台服务器、单个 Go 进程、单实例。用户、角色、岗位、参数、字典、部门、
  菜单的写入必须持有各自的进程内领域锁；锁不仅覆盖唯一性检查，也覆盖关联存在性检查、
  删除前引用检查和随后的数据库写入。自助注册、用户导入和个人资料修改复用用户锁。
  多领域操作统一按“用户 → 角色 → 岗位 → 部门”和“角色 → 菜单”的顺序加锁，禁止反向
  获取。不得在没有替换机制时启动第二个 Go 进程；改为多实例前必须先换成数据库约束或
  可靠跨进程锁，并重新跑并发写入测试。
- 定时任务的新增、修改、状态切换、删除和立即执行必须共用 job 领域锁，锁覆盖旧值读取、
  数据库写入和 scheduler 更新。新增时 `jobId`、`status` 是服务端控制字段：无条件清空 ID、
  强制暂停；启用只能走带 `monitor:job:changeStatus` 权限的接口。缺省 `misfirePolicy` 按 Java
  字段初始值使用 `0`，不能误用数据库默认值 `3` 推断 Java 行为。修改和状态切换必须校验
  `status/concurrent` 只能为 `0/1`、`misfirePolicy` 只能为 `0/1/2/3`。
- 被代码、Java 基线或 Vue 按名称引用的配置键、字典类型和菜单权限是契约标识符。对应写
  接口必须拒绝改名或删除；七个代码配置键必须保持内置标记。布尔开关、密码策略和初始密码
  等有明确消费格式的配置值必须在写入和运行时读取两端校验，非法历史值要显式报错，不能
  通过空值、解析默认值或 `false` 静默降级。新增静态引用时必须同步
  `internal/service/testdata/contract_identifiers.json` 和定向测试；用
  `go test ./internal/service -run Contract -count=1` 独立核对，普通测试不得自动覆写基线。
- Gin 的 `ClientIP()` 只能信任 `server.trustedProxies` 显式列出的 IP/CIDR；直连部署保持
  空列表，经过本机 Nginx 时通常只配置 `127.0.0.1` / `::1`，不能填全网段图省事。
- `server.maxRequestBodyMB` 管普通 JSON/form；`upload.maxSizeMB` 管单文件；
  `upload.maxRequestSizeMB` 管 multipart 整个请求，且必须不小于单文件上限。总请求超限
  使用已登记的 HTTP 413。完整读取/解析必须位于受保护接口鉴权之后、匿名接口 IP 限流
  之后；未认证和 404 请求不得为大小检查预读完整 body。
- 每个请求必须继承 `server.requestTimeout`；MySQL 同时配置 connect/read/write timeout。
  HTTP Server 的 Read/WriteTimeout 不能代替数据库和请求 context deadline。
- MySQL DSN 无论用户字符串如何填写，都必须强制 `parseTime=true` 且时区使用
  `types.Location`；否则连接检测可成功，但第一次扫描业务时间列才失败。`jwt.header` 当前
  只能是实际实现消费的 `Authorization`，`captcha.type` 只能是 `math` 或 `char`，不支持的
  配置必须在启动副作用前失败。
- 路径批量 ID 和 JSON 关联 ID 都必须为正数、去重并限制为 200 个；写关系表前一次性验证
  实体存在和数据范围，禁止在循环里逐 ID 查库。
- 导出 Repository 只取 `MaxExportRows+1` 判断超限，绝不静默截断；导入的 5000 行限制
  统计所有非空行，解析失败行同样计数，返回详情数量和单条长度都必须有上限。
- 异步工作必须进入有界执行器。操作日志队列满时同步降级，不能静默丢审计；手工任务队列
  满时返回可读的繁忙错误。注册任务必须响应 `ctx.Done()`，调度器 deadline 只能释放自身
  状态，Go 不能安全强杀一个忽略 context 的 goroutine。服务退出时必须先停止 HTTP，
  再停止接收并排空操作日志/手工任务池并停止权限补偿 worker，最后停止 scheduler 和关闭依赖。
- 动态参数键接口必须最小授权：`system:config:query` 可读任意键；用户管理相关权限只能读
  `sys.user.initPassword`。新增前端消费者必须显式登记键和权限，不能恢复“登录即可读取”。
- 批量上传属于一个文件副作用单元：任一文件失败时，只回滚本请求此前创建且通过托管目录
  校验的文件，不触碰历史文件或目录外资源。
- 参数缓存回源前必须读取 `sys_config:` 内部代数，回填时用 Lua 校验代数；新增、修改、
  改名、删除和清空缓存必须先推进代数再删数据 key。缓存 TTL 固定为 30 分钟，不能恢复
  永久缓存或绕过 `redisx.InvalidateConfigCache` 直接删除参数缓存。
- 防重复提交必须用单段 Redis Lua 原子完成“比较最近指纹、覆盖并续期”。相同指纹在窗口内
  只能放行一个并发请求；不同指纹仍覆盖最近记录，Redis 故障仍按既有降级策略放行。
- 操作日志只旁路捕获 JSON 响应，内存缓冲不得超过 `json_result` 的 2000 字节上限；截断后
  必须保持合法 UTF-8，不能让多字节字符边界造成日志插入失败。

## ERP商品010聚合边界（2026-10-24）

- SPU/分类/档案/图片/动作独立010；库存、订单、采购和店铺配对继续引用原erp_sku。旧SKU不自动归组，初次明确关联保留ID，归组后不能移除/转组；SKU停用保留历史，商品/分类编码建立后不变。SPU名称更新同步当前SKU名称，不改历史单据快照。
- 已有库存、库存单据、订单、采购、店铺配对或调拨引用时，SKU编码/规格/单位禁止变更。旧SKU编辑/删除接口拒绝已归组SKU，聚合编辑以商品version防覆盖。不要把缺引用检查或旧接口绕过当兼容行为恢复。
- 商品主数据是企业共享，功能权限与仓库/店铺范围独立；平台不能代入企业。新增/编辑/导入商品权限包含聚合内部SKU写入，image仅控制新图片上传；图片沿用企业媒体访问权限，不是商品级私密文件。
- 写入员工→商品（聚合编辑）→分类→SKU升序；不反锁仓库/订单。读引用标志用一组批量EXISTS，子查询显式tenant_id。旧接口SKU锁与聚合共用；归组写提交后旧接口必须看到档案，不得依赖进程互斥替代数据库锁。
- 全写入携带requestKey，同事务记录动作、原结果与最小审计；同键跨员工/不同内容拒绝，当前HTTP权限先于重放。CAS失败不改主数据。每商品1—50个SKU、最多5图，正整数克/毫米或null，条码文本保留前导零；空条码存NULL。
- 商品图片专用上传仅PNG/JPEG/GIF/WebP、8MB、2000万像素，检查扩展名、实际类型与尺寸后登记原配额；仅引用同企业catalog路径附件。移除图片关联不删除物理附件，失败/关闭后可能留未关联附件，仍计入配额。
- 导入只新增、单工作表严格模板、100行/10MB、解压32MB/XML8MB；拒绝公式及数值型编码/条码，整批回滚，不覆盖旧商品。导出按SKU展开并受MaxExportRows上限约束。页面正文仅内存重试，关闭/失活/卸载清空旧数据并作废迟到响应。
- 商品页面区分明确业务拒绝与网络结果未知。旧request封装的明确业务拒绝字符串error应局部清空表单/列表/原请求并作废迟到响应；网络错误保留不可变请求供重试。详情及待确认状态下SKU编码/规格/单位必须显式只读，不能以disabled=false覆盖父表单状态。

## ERP物流011边界（2026-10-25）

- 公司/方式为企业共享档案，编码和方式所属公司固定；写入使用持久version、requestKey、动作原结果和同事务最小审计。快照与动作不暴露通用CRUD，不补历史手工别名。
- 新选方式须公司、方式都启用；同一已保存方式只改运单沿原快照，即使已停用。换选须重新检查启用；切回手工同事务清关联。已发货不可改物流，原手工发货仍查当前仓库/店铺/SKU，不把主数据停用当作旧包裹禁止发货。
- 包裹选择锁序员工→店铺→订单→包裹→公司→方式。关联、包裹字段、订单/包裹版本、原结果事件及审计同事务；不改变库存。公司/方式写锁序不得反向获取包裹/订单。
- 原包裹GET形状保留；新shipping读取带packageVersion、available、snapshot，页面只组合相同版本。全无011时保留旧手工能力；部分结构、孤立版本或漂移必须拒绝。001—010和已登记011不可原地改写。
- Gin同一路径前缀的通配符统一使用已有packageId，即/erp/package/:packageId/shipping；新增注册器必须与既有订单/包裹注册器共同验收，不能仅靠处理器测试代替启动注册。
- options仅提供启用方式的最小标签，复用package:edit；重复运单检查叠加实时范围，只返回布尔提醒，不泄露隐藏包裹、不作为全局唯一约束。设置权限不能代替包裹操作权限。
- 发货记录仅SHIPPED且当前店铺/仓库可见；不含收件/地址/电话/金额。导出与列表同过滤和统一限额，失败精确application/json，成功由XLSX输出器覆盖类型；旧下载接口不随之改契约。
- 新物流JSON明确业务拒绝为HTTP200/code400，DB/提交异常保留500未知结果。旧手工接口不改；页面未知结果冻结原内容/版本/键、只显式原请求重试。关闭/失活/撤权清旧数据和迟到响应。
- 运单与正文不写通用成功审计、URL请求日志或浏览器防重持久缓存。菜单9200—9203/9300—9301只注册定义，不自动给旧企业或员工开通。

## ERP经营概览边界（2026-10-25）

- 概览9400/erp:overview:list仅入口权限；六个GET必须同时检查各自原业务list权限、客户身份和实时店铺/仓库范围。平台不能代入客户数据，详情/导出/对账权限不能由概览隐含授予。菜单只登记定义，不给旧企业自动加权。
- 每区域独立只读一致读取并继承请求context；元数据不存在、部分结构/DB失败、合法0分别处理。不按仓库/店铺/结果行循环查询，不加统计缓存或后台补事实。011可整体未安装而003已存在，以011新表识别可用性；启动结构守卫不放宽。
- 发货为上海自然日、含今天近7日、半开区间的包裹数；原秒精度列表跳转结束为前一秒。库存统计仓库+SKU记录，不混加单位；低库存沿既有阈值，007未装为null。采购待收基于毛收货且排除关单，退货不重开。
- 财务逐单正余额与超额分别汇总、按币种分行、精确字符串；已登记账之外不猜抵减或收入，禁止跨单互抵/跨币种总额。币种行使用data.rows/total分页、最大100；固定数量DTO不靠分页列表求总数。
- 原列表深链接只接收白名单状态/时间/币种：新增HAS_STOCK/HAS_RESERVED和awaitingReceipt复用概览条件。修改统计口径必须同步原列表/跳转与真实HTTP测试。页面失活/退出/明确拒绝后清空旧值并作废迟到响应，错误不能伪装0。

## ERP企业启用边界（2026-10-25）

- 9500/erp:setup:list仅授予当前企业所有者读取固定检查项，员工/平台均拒绝；各项还需原业务权限。先查当前所有者，再读元数据并固定聚合，子查询/关联显式tenant_id；同一只读快照，无按结果行查询。不可把缺功能、未安装、DB错误、无记录或已具记录混淆。
- 检查只描述存在性，不作为可写门槛或完成认证。个人企业不强制员工/角色/范围，物流目录可选；期初库存走原单据，不直接改余额，不制造假账来“变绿”。当前已实现与日期验收快照分开维护，历史不应继续覆盖现状。
- 五类角色模板为固定目录，只有者与system:role:add可读；只含ERP显式权限，不含通配符/平台/系统管理/资源授权/启用检查。以本企业已开通菜单求交集，缺权限明示，原POST仍重查。业务管理员不是tenant_admin或平台管理员。
- 模板只预填新增角色，不覆盖旧角色、不保存、不分配员工或仓库/店铺范围。按菜单ID精确选择并关闭父子联动，不能因父目录连带扩权；异步预填防覆盖/关闭后迟到，预填期间不允许保存。模板更新不自动改历史角色。
- 初始化/升级工具本地限制和备份门槛不放松。新环境验收须全新独立MySQL/Redis/文件根，不能删除现有库冒充空库；临时服务结束停止。私有日志/账号/备份不进Git，不把本机辅助脚本当公开生产能力。

## ERP拣货与扫码复核边界（2026-10-26）

- 9600—9603/erp:picking:list/query/check/ship要求客户工作区、企业开通、功能授权及当前仓库/店铺双范围；定义不自动给已有企业加权。新仓库员模板采用复核路径，旧角色不自动改，原package:ship独立保留，不能声称全系统强制扫码。
- 清单单包裹最多100原行，同SKU汇总且不混单位，固定批量读取。商品/数量/单位来自履约快照，扫描码/规格取当前资料；精确字符串保留前导零，无条码用原SKU编码，包裹内不同SKU编码冲突必须阻断。010全无可退化，部分损坏必须报错。
- PACK_CHECK沿既有不可变事件记录最小结果及SHA256指纹，现有remark字段内小于500字节，不保存完整扫描正文、不改库存。包裹版本或当前扫描资料变化后复核失效；不能把读取时的checked当作后续写许可。
- CHECKED_SHIP在原发货事务内验证有效复核；锁序员工→店铺→订单→包裹→原库存凭据（发货）→仓库→SKU升序→商品档案，条码/档案写入与复核串行。订单/包裹/实物/预占/流水/事件同事务，不能新增扣库存旁路。
- 新动作原键重试绑定员工与内容，重试也须当前功能与双范围。未知结果冻结原版本/键/正文，仅显式重试；明确业务拒绝code400，DB/提交异常保持未知code500。旧手工契约不变；页面退出/失活/切包裹后作废迟到响应。
- 拣货路径响应no-store，查询日志省略查询正文，不挂保存扫描正文的通用成功审计；两项写请求显式repeatSubmit:false，避免通用防重持久缓存正文或阻断领域原键重试。明细/打印不含收件、运单、金额，打印前重读授权，不在浏览器持久保存扫描草稿。
- 扫描计数是操作员提交的数据一致性检查，不是实物或硬件认证。实体出纸、真实扫码硬件、容量和生产上线分别验；不得用本地键盘/四视口结果代替。改此流程同步菜单/契约权限/模板/接口/前后端测试与使用说明。
- 打印每次重读权限及清单；只有完整新旧清单一致才可恢复内存中的未提交扫描/输入，不得仅比包裹版本或ID。资料变化清空并提示，拒绝清空且不打印；离开、切换或同ID新读取均作废旧打印。在最后一次DOM刷新之后也须检查读取代次、页面活跃状态与当前权限，不能让迟到打印作用于另一页面。

## ERP履约页面离页保护（2026-10-26）

- 订单主页、包裹、单笔/批量履约必须同时处理卸载和KeepAlive失活；离开清旧表单、私有收件资料、文件选择、批次结果和内存待确认内容，作废所有旧读取/操作。重新进入读取当前可见事实，不能恢复旧编辑草稿或继续旧批次。
- 确认框、异步表单校验、运单预检等返回后，调用下一写接口前必须重查所属页面/表单生命周期。响应成功/失败后也须检查；只有仍属于当前操作才更新结果、弹本地成功提示或通知父页面刷新。
- 逐单写入和逐条预检在每次调用前及成功/失败返回后均检查所属批次，失活立即停止未发送的后续行。批量不是持久后台任务；离页不再隐式续跑。
- finally只可释放自己所属操作的busy状态。操作序号/生命周期与预检条件序号分开，避免旧回包解除新操作的锁，或把仓库预检失效误当成变更已固定的提交参数；关闭动画的旧回调也不能清理新弹窗。
- 离页只能阻止尚未发送的操作，不保证已发送请求未提交，不把取消HTTP或忽略回包当作回滚。回页重新核对状态/库存/导入任务；同批内的未知结果仍沿原键/版本/内容显式重试，成功项不重发，不持久保存敏感请求正文。
- 改这些分支时补受控Promise回归，覆盖离页后确认、当前请求成功和失败、下一行阻止，以及重新激活后旧finally/旧响应不能影响新表单。组件调度测试与真实浏览器、服务器故障、数据库事务测试分别记录，不互相代替。

## ERP跨模块锁归属与结果未确认（2026-10-26）

- 失活/卸载既要作废旧请求，也要释放旧页面自己的busy/保存锁；旧finally必须核对所属操作/视图，不能解除重新激活后的新锁。隐藏页的路由监听不得继续读取或提交。列表/详情/私有文件在迟到返回后也检查归属。
- 校验、确认及预检返回后，调用写接口前重新检查归属；目标ID与动作在确认前固定。关闭回调先检查是否已重新打开，不能清新窗口。原详情加载锁不得在重构时遗漏。
- 结果未确认时固定完整原目标、版本、内容及请求键；导入固定原File/店铺/键。重试不能重新读取已经改变的表单、所选范围或重新生成键。敏感正文只留内存，关闭后先查询，不自动换键重做。
- 持久请求键/原结果、CAS/唯一编码、无键状态动作、文件上传是不同能力。前端冻结内容不创造服务端幂等；CAS冲突/编码重复不能证明上一请求没成功，须查询确认。不得为统一UI而改冻结结构或伪造requestKey字段。
- 明确拒绝与网络/500未知按已知接口契约处理；未知要保守，不能仅凭旧接口500推断确定未提交。关闭/离页不回滚已发送请求，也不保证媒体重传不会产生新附件或配额占用。
- 跨模块复核包含原有状态/金额/参数回归和SFC模板解析/生产构建；不能只跑新专项，不能忽略parse.errors或拿组件模拟替代浏览器/真实业务接口/恢复证据。历史报告保留快照，现状见ERP_ASYNC_OPERATIONS。

## ERP 权限回归登记

ERP接口增删或调整授权时，同步审查 `test/permissionmatrix/routes.tsv`、菜单定义及真实资源用例。路由登记单元检查实际路径、企业入口、权限检查数量和所有者限制；不把它称为闭包权限字符串的自动核验。真实HTTP检查必须区分入口拒绝与资源隔离：匿名/平台/无权限空对象拒绝不能替代先成功读取真实数据后，再用同一旧token/原请求验证撤权的用例。下载/导出检查业务码和实际二进制内容，双权限分别撤一个验证；导入任务、结果、文件和重试沿当前企业、当前店铺和提交人/所有者边界。矩阵与运行入口见 [ERP_PERMISSION_MATRIX](./ERP_PERMISSION_MATRIX.md)。
## ERP 规模与负载验证

使用独立多租户本地配置及当轮新建测试企业，旧单租户perfseed/perf.ps1不得套用。cmd/erpbench默认只预览，仅显式execute才读取私有令牌文件并对固定本地ERP GET施压，不自动登录、改密、造数或调整结构。真实业务码/内容校验、在途排空、错误与取消单列；并发是闭环在途请求数，不能换算在线人数。

批量读取夹具必须显著标记，不冒充HTTP建单/业务历史；写入与重放用新键走真实接口并核对最终数量。CPU采样用时间差，当前依赖的CPUPercent是生命周期平均值，不能当阶段即时CPU；区分单核与整机百分比，Windows working set与Go堆不能混写。保护中止、工具错误、未完成阶段保留并排除出最终容量宣称，不删除失败证据。配置、数据库池、角色/范围、权重、样本时长和硬件随结果保存；本机结果不外推2C2G或生产，方法见[ERP_CAPACITY](./ERP_CAPACITY.md)。

## 建单查重访问路径

建单以现有请求键唯一索引和企业/店铺/来源单号唯一索引分别查候选，使用单条UNION派生表去重，保持原OR匹配集合与一次读取。不得改成UNION ALL，否则两条件指向同一记录会误报冲突；不可随意拆成两次独立读取。内层两个物理表分支显式绑定当前tenant_id，外层继续执行租户守卫和当前店铺范围，输入只能参数绑定，不能借性能优化使用System上下文或开放通用复杂关联绕过。

保持既有重放语义：同店铺、来源单号及持久创建指纹一致可返回原结果，包括请求键不同但来源/内容相同的情况；两条件命中不同记录或内容不同仍拒绝。当前授权、事务锁、唯一约束、原子明细/审计继续生效。查询变化必须有旧/新实际结果集合、正向与撤权、四类并发及实际索引计划证据。局部建单提速不等于列表深页或整套ERP的容量变化，见[本轮优化复核](./ERP_ORDER_CREATE_OPTIMIZATION_2026-10-26.md)。

## 订单深分页访问路径

订单页保留精确总数、原过滤和order_id倒序。只在已有实测支持的深偏移使用“覆盖索引选当页ID，再取该页摘要”的单条SQL；内层在LIMIT/OFFSET前应用完整租户、实时店铺和业务过滤，携带tenant_id，外层关联同时约束tenant_id/order_id并保留租户及实时范围守卫。不能将选ID和取摘要拆成应用层两次查询，也不能挪到分页后鉴权。摘要投影保持最小字段，不读取收件资料。

不要把延迟读取无条件套到浅页：本轮原查询在偏移0—5000更快，当前保守门槛为10000；小于门槛保留原SQL和OFFSET。门槛不是数据库优化器的普适保证，改动须补相同数据/授权/配置的交替探针及非空深页、浅页和末页回归。精确COUNT与行读取仍是原来的两次读取，不新增跨请求快照承诺或缓存。

统计保持原实现。工作台GROUP BY后由Go按规范字符串分派，概览使用SQL条件SUM；状态列utf8mb4_unicode_ci下，历史大小写、重音、尾空格等拼写可能使这两种算法不同。不能以性能优化为由统一历史异常口径；普通未知状态仍计入total，工作台状态页签仍忽略所选status并保留店铺/来源筛选。需要改变时先验证旧服务并经过可观察行为审查。本轮纯SUM候选不等价，带兼容检测的候选没有明确收益，均未采用，见[读取优化复核](./ERP_ORDER_READ_OPTIMIZATION_2026-10-26.md)。

## ERP持续业务与故障验收

可靠性测试只使用当轮新建专用企业和工具自建进程，按原键/原内容/原版本重试。停止进程前核对创建句柄、PID及路径，不按宽泛进程名结束服务。事务关卡必须精确匹配企业/请求键/动作，只链接测试工具，不开放生产故障HTTP接口；先独立证明存在未提交修改，再区分回滚重试与提交后重放。观察工具未完成不能直接判为业务缺陷。

混合业务停止调度后先排空，再核对库存余额/流水、预占、订单/包裹及应收应付/核销数量与金额；批量夹具或单纯HTTP200不能替代完整链事实。源库结构/索引及已有企业数据保持原边界，结束撤范围、停用并保留，不能重开已结束企业省造数。

资源统计分开记录自然GC、空闲观察和边界额外诊断GC，保留池上限、open/inUse、等待增量及协程趋势。RSS不立即下降不等于泄漏；CPU取累计时间差，取不到须标记未知或改用可靠采样源，不能写假0。阶段有编译、重启或其他负载须明确记录，不外推生产容量。

运行包仅白名单复制后端、前端、示例配置及说明，禁止把私有配置/账号/令牌/日志/数据库备份打包。校验解压后逐文件哈希，标记源码基线和未提交状态；跨编译不等于目标平台已验。恢复允许保留原服务时，必须通过事务快照、前后全部行/结构及附件摘要一致性判断，发生变化就失败，不能声称已停写。

### 远程选择器避免可见性反馈

远程选择器若在查询前清空选项，不可再用菜单的visible-change触发同一查询：空选项、loading和重新挂载选项都会影响远程菜单可见性，可能形成重复请求/闪烁循环。优先由真实输入获焦、主动搜索或业务弹窗准备阶段加载，绑定对应loading；仅当前请求可更新选项/结束loading，离页、关闭或撤权时清空并作废迟到响应。仅加loading不是消除循环的充分条件，必须实际页面复验。财务账户及销售财务店铺选择器遵循此规则。

可靠性正式入口是`scripts/verify-erp-reliability.ps1`，默认只预览，显式`-Execute`才写新测试企业。quick只验入口/短业务链；standard持续观察；私有脚本不作为唯一复现入口。用法和本地限制见[ERP_RELIABILITY](./ERP_RELIABILITY.md)。

## ERP导航展示分类

ERP业务分类由自有web的`erp-navigation.mjs`维护，只消费后端已经授权的路由；不得通过补齐类别页面、客户端静态注册业务路由或自动改授权来改善菜单。实际路由注册和页面跳转继续使用原URL/组件，展示容器只作唯一侧栏索引和不可点击面包屑。未知ERP扩展页保留入口，不静默丢弃；类别无可见页面时隐藏。新增ERP页面同步登记导航分类，见[ERP_NAVIGATION](./ERP_NAVIGATION.md)。

## ERP控件尺寸与搜索区

自有web的ERP表单、按钮、表格和标签继承系统size；根ElConfigProvider读取appStore.size。不得在业务页写死small或通过单元格padding/line-height强制模拟小尺寸。图像、弹窗宽度和打印纸张尺寸不属于控件size。

列表筛选统一使用ERPQueryBar：字段在默认slot，查询仅走原生form submit，重置调用页面原处理器；新增、导出等业务按钮放toolbar slot或表单外。禁止再为同一输入绑定keyup.enter查询，避免重复提交。选值与清空只更新条件，点击查询/回车后应用；分页和业务快捷筛选沿原入口。布局随容器换行，不以固定控件高度压缩。

重置清除可见及链接带入的隐藏筛选，回到第一页并保留pageSize；日期、文本ID等独立ref一并清除。保留列表默认状态、当前业务页签及待确认的原请求，不能用重置筛选清除写入锁、内容或请求键。拣货待确认期间禁止查询/重置；财务来源搜索与按钮查询必须一致地回到第一页。验证与边界见[ERP表单与搜索复核](./ERP_FORMS_REVIEW_2026-10-28.md)。

## 全局业务弹窗窗口控制

自有web默认使用模板`<el-dialog>`，由dialog-windows插件统一解析到WindowDialog；不用逐页导入或实现最大/最小逻辑。根App保留DialogWindowDock，普通确认提示ElMessageBox与抽屉不在此封装范围。直接导入element-plus的ElDialog属于原生专用入口，不获得窗口控制；新业务页遵循模板默认入口。

最小化仅改变视觉状态，v-model仍为true，不触发before-close/close/closed，不销毁或重建表单；从底部栏恢复原状态。原生Dialog暂时隐藏来释放遮罩、焦点和滚动锁，不能只用CSS藏正文。真正关闭才按原destroy-on-close处理，并只通知一轮关闭事件。离页/卸载必须清掉恢复栏入口，不把最小化当作跨页草稿保存。

最大化覆盖整个视口，正文内部滚动，标题及页脚保持可达；继承系统size、主题和原宽度。show-close=false时默认禁用最小/最大操作；额外保存锁用window-controls-disabled。默认挂载body，特殊布局可显式append-to-body=false；自有ERP的erp-dialog响应式规则继续保留。

before-close保持原取消写法：不调用done也可以取消；返回Promise时在其结束后释放额外的关闭尝试锁。不得让取消/异常导致永久锁定，旧异步回调不能关闭恢复后或新打开的窗口。用法、可选项、测试入口及边界见[全局弹窗](./GLOBAL_DIALOG.md)。


## 全局弹窗拖拽与富文本加载

模板el-dialog默认标题栏可鼠标拖拽，沿原生视口限制；显式:draggable="false"可按窗关闭，最大化由原生禁拖。动作区不把mousedown冒泡给拖动手柄。仅真正新开时重置位置，最小化恢复不复位或重建表单。拖拽与window-controls开关独立，确认提示/抽屉不纳入；见[全局弹窗](./GLOBAL_DIALOG.md)。

全局Editor注册必须从components/Editor/async.mjs进入，不在main.js静态导入Editor/index.vue或Quill；仅挂载使用Editor的页面时加载实现。修改注册入口须检查生产入口的静态依赖图，不得把压缩包大小变化等同于首屏耗时或全站总下载减少。原编辑器的模型和上传规则不由异步入口重写；加载状态与有界失败在异步入口处理。
