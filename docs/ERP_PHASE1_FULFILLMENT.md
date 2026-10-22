# ERP 一期阶段6：事务履约与单包裹手工发货

> 后续导航：阶段7已增加工作台统计、只读审核检查、详情互跳和批量逐单操作，见[ERP_PHASE1_WORKSPACE.md](./ERP_PHASE1_WORKSPACE.md)。本页保留阶段6当时范围及证据，末尾“下一步阶段7”不是当前未实施的结论。

2026-10-21，America/New_York。适用erp分支、独立本地ruoyi_go_mt、一个Go进程。
阶段5已完成，历史范围见[ERP_PHASE1_ORDERS.md](./ERP_PHASE1_ORDERS.md)。本轮完成阶段6及履约最小页面，不等于步骤7—8或完整电商ERP完成。

## 1. 已实现与未提供

已实现：审核预占、审核快照、单包裹、待审核/待发货取消、手工物流/文本运单、本地发货出库；旧库存入口托管边界与实时范围加固。

只使用已安装003，没有新增表/列/索引或改写迁移，启动仍只读校验。未改原Java/main库、参考前端、Redis前缀或第三方依赖。未自动给已有企业开通新菜单；未提交/推送Git，未操作远程服务器。

仍无：真实平台授权/拉单/回传、面单申请/轨迹、拆合单/部分发货、组合商品、售后退款、采购订单和财务。页面只是最小操作入口，统一工作台、批量逐单结果、交付级全量验收留步骤7—8。

## 2. 操作与版本契约

全部接口要求客户工作空间、企业开通、功能权限及实时店铺范围；涉及仓库的包裹读写/审核/已审核取消还需仓库范围。平台不能代入。列表分页100，ID倒序；业务错误使用原response形状，不回显SQL或收件资料。

| 接口 | 请求 / 权限 |
|---|---|
| PUT /erp/order/:orderId/review | 订单version、requestKey、warehouseId；erp:order:review |
| PUT /erp/order/:orderId/cancel | 订单version、requestKey、reason；erp:order:cancel |
| GET /erp/package/list | shopId/warehouseId/orderId/status可选；erp:package:list；不含运单/收件资料 |
| GET /erp/package/:packageId | erp:package:query；含包裹SKU行/手工物流，不含订单收件资料 |
| PUT /erp/package/:packageId/logistics | 包裹version、requestKey、carrierCode、trackingNo；erp:package:edit |
| PUT /erp/package/:packageId/ship | 包裹version、requestKey；erp:package:ship；须先保存物流 |

version须正整数；requestKey为8—64位ASCII字母数字及_:-，前端UUID。取消原因1—500字，不含首尾空白/控制字符；物流代码1—64位字母数字_或-；运单1—128字文本，不转数字，保留前导零。

返回data为orderId、订单version/status、可空packageId/packageVersion和existing。订单与包裹版本不是同一个值：审核订单v2/包裹v1；保存物流订单v3/包裹v2；发货订单v4/包裹v3。物流保存不动库存，但必须推进订单版本，旧取消弹窗须刷新。

事件以企业+订单+动作+requestKey保存hash和原结果；同键同输入返回原结果/现existing=true，不再推进版本/写流水，**不保证返回当前最新版本**。同键不同输入拒绝，新键对终态重复或相反动作拒绝。重放也先校验当前范围及适用启停条件，不能借旧成功结果绕过撤权；前端失败不清草稿/请求键，版本冲突需刷新核对后重新发起。

新菜单7306—7307、7500—7503仅定义登记。新建验收企业遵循已有开户规则，不能据此推断原demo等已获得权限；旧企业若要使用须平台另行明确开通，员工仍需资源范围。

## 3. 事务、数量与快照

审核顺序：当前员工→店铺→订单；批量解析当前启用商品/配对，按内部SKU汇总；锁仓库→排序SKU/余额，建立新SALE并预占，保存原订单行审核快照，建立单包裹与逐原行SKU快照，提交订单/事件/成功审计。新单据/包裹尚未提交，由父订单锁保护；不是先锁旧SKU再反向取旧库存单据。

已有行取消/发货按员工→店铺→订单→已有包裹→已有库存单据→仓库→排序SKU/余额。配对/商品写也先锁店铺，与审核互斥；不循环逐行查库。repository内部库存步骤transitionERPDocumentLocked在同一事务中复用，不能串联两个公开事务或HTTP假装原子提交。现有库存相对锁序不变。

每订单1—100原始行；不同变种可配同一内部SKU，库存凭据按SKU汇总，订单/包裹保留原行。审核每SKU合计1—十亿，沿用库存单据上限；录入阶段允许到ERPMaxStock的一万亿不代表可审核任意数量。未配对/缺货/停用/聚合超限时整体回滚，仍待审核，不残留SALE/包裹/流水/成功审计。

审核订单行保存商品/配对ID、配对版本、SKU/编码/名称/规格/单位；包裹行保存原订单行关联及SKU/名称/单位/数量。终态后当前配对可调整，但历史不随之变化。头/行/订单详情包裹链接用一致快照读取。

待审核取消只变订单；待发货取消原子释放预占、取消SALE和包裹、推进版本/事件；已发货不能取消。停用店铺/仓库/SKU不阻止当前有权者取消释放。发货要求启用店铺/仓库/SKU及已存物流，原子扣实物与预占、写OUT流水、更新包裹/订单。这里只登记本地出库，不证明平台回传、承运商揽收或妥投。

## 4. 旧入口与隐私

独立手工SALE继续按原仓库范围工作。订单托管SALE不能从旧/erp/sales独立编辑、confirm、cancel、ship或通过旧create请求键重放返回；所有者也被拒绝，必须走订单/包裹。

旧详情/列表、库存流水、托管SALE及关联SR读取/写入叠加实时店铺与仓库范围，撤权旧会话下一请求生效；先检查范围再给“托管”提示。范围条件中的嵌套表明确tenant_id，不依赖守卫猜任意子查询。检查不在持有凭据锁后反向锁订单/店铺。旧凭据目前无导出路由，将来新增必须继承范围。

关联SR是受控库存纠错，仍检查来源、原因、累计可退和权限，不改写原商业订单已发货事实；不等于售后/金额退款。

包裹不嵌入订单收件资料，列表也不含运单；订单详情只有package的ID/状态/版本投影，不借order:query泄露物流。完整收件资料仍需订单详情权限；包裹页填运单需要query+edit以载入当前版本，但服务端edit不是靠前端按钮授权。

新动作不挂通用请求/响应OperLog，不写浏览器正文防重缓存；HTTP日志抑制包裹查询串，事务成功审计只有ID/动作/版本，不写取消原因/运单/收件正文。详情与动作保持no-store。

## 5. 实际测试和证据

最终完整命令：

```powershell
$env:GOCACHE='D:\WorkSpace\CodeX\.mt-cache'
.\scripts\verify-erp.ps1 `
  -GoPath 'C:\Program Files\Go\bin\go.exe' `
  -NodePath 'C:\Users\FF\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe' `
  -Integration -Count 2
```

最终结果：test/results/erp-20261022T002627217Z-aca1e694/summary.json，CHECKS_PASSED、integration=REQUESTED。Go格式/单测/vet、前端46项0失败0跳过、生产构建、真实本地多租户集成两轮均通过。UTC目录日期22日对应本地21日晚；不是另一天执行的未核实记录。

定向证据logs/erp-stage6-20261021/fulfillment-first.log与fulfillment-verified.log均通过TestERPFulfillment九组；最终完整回归包含最新跨员工/跨店铺用例。覆盖：

- 同SKU多行聚合、待审后建立配对、审核快照、前导零物流、终态后配对改变不改历史。
- 待审核/待发货取消、缺货/未配对/聚合超量、停止主数据仍能释放。
- 相同/不同请求键并发审核、不同员工发货/取消竞争，以及不同员工/店铺在同仓争抢最后一份库存；不只靠同员工锁隐藏竞争。
- 在审核流水/包裹/事件/成功审计、取消释放流水、发货事件等注入故障，整事务回滚。
- 跨企业、平台、店铺与仓库范围、旧会话撤权、旧托管入口/偷用请求键、关联SR及流水旁路。
- 日志/动作审计不含合成收件资料；前端确认前快照、busy覆盖确认/提交、失败保留草稿/请求键、旧响应失效。

### 真实页面（computer-use技能）

仅新建专用企业fulfill-ui-970671078700（ID10370），按用户允许临时关闭其验证码。两个合成订单831/833均页面审核，833页面取消，831页面保存MANUAL及运单0000123456789后发货，详情只读展示前导零且无收件资料。库存页实际实物18/预占0/可用18，与入库20、发货2一致。

截图：logs/erp-stage6-20261021/ui-packages.png、ui-tracking.png、ui-stock.png。只读报告closeout.json核对两个订单/包裹/库存凭据终态及版本、3凭据/2包裹/7事件/5流水，无重复动作残留；结构三项只读检查通过。

收尾：验证码恢复true，专用企业停用保留记录，界面退出登录；没有重置已有密码或关闭平台/demo/demo-b验证码。menu-registration.json记录登记前后现有授权SHA256相等，closeout.json确认demo/demo-b没有阶段6新菜单。

源码工作区快照保存在logs/erp-stage6-20261021/source-snapshot/，含manifest、tracked.patch与文件副本；这是源码留证，不是Git提交或数据库备份。不覆盖阶段5快照，未涉及的原有改动以清单hash复核。

## 6. 后续与不可外推项

下一步阶段7：统一订单/包裹工作台、可见的配对/缺货提示、详情跳转和审核快照展示、窄屏操作入口、批量逐单结果及交互收口；仍不接真实渠道API。阶段8再做一期验收矩阵、升级/恢复和交付检查。

本轮没有做完整员工GUI点测、真实承运商/平台联调、私有导入目录备份恢复、远程压测/生产容量认证或任意生产库迁移。原demo账号凭据核验按用户决定继续延期，不以专用账号通过替代。单实例本地回归不能外推多副本或“没有任何BUG”。
