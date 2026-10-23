# 采购步骤4：采购单后台、精确金额与本地验收

日期：2026-10-22。适用 `erp` 分支、独立本地 `ruoyi_go_mt`。本页为本轮快照，不是采购收货闭环或生产认证。

> 后续导航：步骤5分批收货/托管库存及步骤6三页的本地范围已完成，含完整两轮、所有者/员工GUI及配套恢复，见[最终验收第10节](./ERP_PURCHASING_ACCEPTANCE.md#10-仅收货员工实际页面补验与最终收口)。下文占位/尚无收货/无最新恢复保留步骤4当时范围，不是现状。

## 1. 完成与未完成

已完成采购草稿新增/编辑、列表/详情/事件、确认采购、取消、关闭余量；持久版本、动作幂等、精确金额、主数据快照及SKU引用保护已接入。

本阶段只交付后台业务。`web/src/views/erp/purchase/index.vue` 是明确提示后续步骤6的占位入口，不提供采购录入或收货按钮；完整采购/收货工作台及真实浏览器验收仍留步骤6。本轮未进行采购页GUI/手机真机验证，不能借此前供应商的页面证据宣布采购页面完成。

没有实现分批收货、采购托管IN、库存联动、采购退货、成本、应付、付款或财务核算。没有任何采购写动作改变库存。未执行DDL、改已登记001—004、给旧企业自动开通、改真实账号密码、连接远程、提交或推送Git。

## 2. 生成与实现来源

从实际 `erp_purchase_order`、`erp_purchase_order_line` 用现有生成器产出Go四层/路由/Vue/API/菜单原包，再人工补领域事务。泛型生成代码中的金额float64改为本模块字符串模型；没有修改生成器模板、依赖或go.mod，没有把生成的普通删除/导出/菜单SQL直接安装。

原始包和元数据在 `logs/erp-purchase-20261022/`：

| 表/生成包 | SHA256 |
|---|---|
| erp_purchase_order-generated.zip | a11194e8f367f3b3a3e6fbb056966f0630d2a45b658abec62f06907e5b2a0e78 |
| erp_purchase_order_line-generated.zip | 53162d8fec886120fed62635d4c7cd5b5cbcce23912b08f83458c97e27328622 |

实现：`internal/{model,repository,service,handler}/purchase.go`、`internal/router/purchase_routes.go`、`pkg/decimalx/`；现有SKU引用判断、菜单/权限清单及HTTP日志规则同步更新。

## 3. 业务规则

- 采购号由服务端生成；企业、状态、审计字段、初始version=1、累计已收和金额均不能由客户端覆盖。
- 一单固定一个供应商/仓库，创建后不可换；草稿可修改明细、价格、币种及备注。币种归一为三位大写字母，只表示报价币种，不验证外部币种真实性或提供汇兑。
- 1—100行、同SKU不得重复、每行数量1—十亿。按SKU排序形成规范化请求；行号由服务端生成。价格必须是字符串，允许"0"，不能用JSON Number、科学计数法、负数或超过4位小数。
- `pkg/decimalx` 采用标准库big.Int按万分位计算，金额规范化为四位小数字符串。单价、行额、逐行累计总额分别检查DECIMAL(20,4)边界，溢出拒绝，不用浮点或静默四舍五入。
- 确认时重新锁定启用的供应商、仓库、SKU，刷新并冻结供应商/仓库名称、SKU编码/名称/规格/单位；价格、数量与总额不因后续主数据改名而漂移。
- 草稿即构成SKU单位/删除引用保护，取消/关闭后仍保留历史引用。兼容004尚未安装的旧核心ERP：固定的表存在性元数据查询使用局部System上下文，随后业务引用计数恢复原企业上下文；元数据错误拒绝，不能作为“未安装”忽略。
- DRAFT可编辑/确认/取消；CONFIRMED可取消或关闭；PARTIAL_RECEIVED仅关闭余量；COMPLETED/CLOSED/CANCELLED不再操作。取消要求已收全0，关闭要求有未收行，均必须填写原因，保存在closeReason；不把未收量改成已收，也不重新打开终态。
- 停用供应商阻止新建/编辑/确认；取消或关闭不要求主数据启用，但仍检查当前身份、功能及仓库范围。仓库/SKU启用检查复用现有锁定引用实现。

### 事务、锁序与重试

锁序：当前员工 → 供应商 → 已有采购单 → 仓库 → 排序SKU。已有采购单先用实时范围做非锁定定位，锁定后复核不可变供应商/仓库；SKU资料写只锁SKU，不反向获取采购/供应商锁。明细主数据批量读取；确认最多100行有界更新，不宣称全部SQL已合并成一条。

业务、不可变事件、最小成功审计在同一个READ COMMITTED事务内提交。创建另存不可变create_request_key/hash；各动作按采购单+动作+请求键保存结果。指纹包含操作者、目标、动作、输入版本及规范业务内容，同键异内容/异操作者拒绝。

重放先检查当前身份/功能及仓库范围，再返回原事件中的ID/版本/状态；不先用最新版本或主数据停用阻断合法历史重试。创建后编辑、确认后关闭，再重试旧动作，返回的是当时结果而不是最新状态，不重复写业务/事件/审计。客户端仍须另行刷新，不得因超时换键或自动升级version。

## 4. API、权限及隐私

| API | 权限 | 输出 |
|---|---|---|
| GET /erp/purchase/list | erp:purchase:list | rows/total；ID、单号、供应商/仓库ID及名称、币种、状态、版本、创建时间，不回金额/备注 |
| GET /erp/purchase/:purchaseId | erp:purchase:query | data详情及最多100行，包含价格/总额，不带供应商联系方式 |
| GET /erp/purchase/:purchaseId/events | erp:purchase:query | 独立分页的ID/动作/前后状态和版本/时间，不回请求指纹及结果正文 |
| POST /erp/purchase | erp:purchase:add | data最小purchaseId/version/status/existing |
| PUT /erp/purchase/:purchaseId | erp:purchase:edit | 草稿version/requestKey，最小结果 |
| PUT /erp/purchase/:purchaseId/confirm | erp:purchase:confirm | version/requestKey，最小结果 |
| PUT /erp/purchase/:purchaseId/cancel、/close | 对应cancel/close | version/requestKey/reason，最小结果 |

客户工作空间入口、企业开通与员工功能、实时仓库范围分别检查，平台身份不能读取客户业务。列表/事件上限100、稳定唯一排序；不开放无界全量查询、DELETE、export、receipts或receivable路由。功能校验在入口执行，资源写入与撤权通过已有员工锁协调，不把它表述成全部功能权限在事务内重新计算。

菜单7700—7706只登记定义；`menu-registration.json` 记录企业/角色授权登记前后摘要相同。旧企业需要平台显式开通，员工再由企业管理员授权，本轮没有替demo/demo-b开通。

采购域不挂通用正文审计/防重缓存；HTTP日志不记录整个查询串，接口no-store。事件的原结果和成功审计仅保存定位字段，不含价格、备注、取消/关闭原因或联系人。价格不能靠前端隐藏列保护；list不隐含query，edit/confirm也不通过结果响应授予价格读取。

## 5. 实测证据

最终完整验证：

```powershell
$env:GOCACHE='D:\WorkSpace\CodeX\.mt-cache'
.\scripts\verify-erp.ps1 -GoPath 'C:\Program Files\Go\bin\go.exe' -NodePath 'C:\Users\FF\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe' -Integration -Count 2
```

`test/results/erp-20261022T132232532Z-901fd02b/summary.json`：六步全通过，真实集成两轮；前端现有68项、0失败、0跳过，生产构建通过。这68项不是新增采购交互测试，占位页未宣称GUI验收。最终Windows后台另行build并实际启动，单实例本地8081；前端5173继续运行。

采购新增11组真实接口/数据库验收，每轮均通过：

1. 金额类型、0/精确上限、乘法/总额溢出及伪造服务端字段。
2. 创建/编辑/确认的原结果重放，同键异内容拒绝。
3. 两企业/平台/外来关联隔离，采购不改变库存。
4. 确认快照、后续改名不漂移、单位/删除引用保护、供应商停用。
5. 固定供应商/仓库、状态机、不同键同version确认/取消竞争只成功一方。
6. 并发同键创建/确认全部取得同一结果，仅一头/一动作事件。
7. 测试直接构造的PARTIAL_RECEIVED/终态边界：关闭不伪造已收，终态拒绝动作；不是实际收货功能测试。
8. 审计/事件创建故障下，新增、草稿编辑、确认全部回滚。
9. 明细创建/采购头更新故障下，无半次编辑或确认。
10. 员工默认无仓库，授予后可用，同会话撤仓库后读/写/原请求拒绝；撤query/add和list后对应接口立即拒绝，list-only不泄露金额。
11. 分页/排序、事件最小内容、审计隐私、no-store及未实现路由404。

故障注入仅存在测试进程repository回调，不向运行服务器植入故障开关。合成部分状态只写新夹具，不证明采购收货或进度/库存联动。

最终只读检查 `logs/erp-purchase-20261022/final-readonly-check.json`：采购专用企业全部停用保留，采购测试企业范围内库存/库存凭据/流水/收货记录均0；004结构校验通过。本轮没有关闭验证码或改旧密码。完整集成会按既有规则写其他专用企业/订单/附件并清理停用，不是全程只读。

含实时权限谓词的仓库+状态筛选列表在小夹具上记录EXPLAIN：主表使用ix_p4_purchase_status、估计9行，仓库权限子查询用uk_erp_access_user，所有者子查询用主键。只说明该样本计划，不代表全部筛选/无范围查询、真实基数或生产容量。

生成辅助程序曾引用不存在的GoType字段，测试曾引用错误流水类型ERPStockLedger；均编译阶段失败，分别按实际生成器/模型定义修正后重跑，没有通过改模板、删除断言或改业务类型规避。首次8组真实API通过后增加并发/故障/合成状态测试，最终全量以11组版本为准。

## 6. 交付边界与下一步

最终后台：`logs/erp-purchase-20261022/server-final-windows-amd64.exe`，SHA256 `f9a86384949008dea61b3f3b111150f0260e4c435e70c1c15d068a34b30dcf39`。本地先核对进程身份及PENDING/RUNNING导入任务为0，再强制停旧进程并换构建；不是优雅排空认证或远程发布。

没有重做包含本轮采购数据的配套恢复、压测、Linux运行、生产迁移/回退或真机/跨浏览器验收；此前结构/供应商报告只作当时证据。代码仍是未提交工作区。

下一步按[采购计划步骤5](./ERP_PURCHASING_DESIGN.md)：分批收货、同一事务内托管IN与库存过账、采购余量/状态推进、丢响应原请求重放、托管IN及来源PR旧入口防绕过；最后步骤6再做完整可视化工作台、互跳和配套恢复收口。
