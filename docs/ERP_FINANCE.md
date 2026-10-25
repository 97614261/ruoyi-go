# 采购财务基础（008）

适用erp、本地独立多租户环境。设计见[ERP_FINANCE_DESIGN](./ERP_FINANCE_DESIGN.md)，实际验收以[ERP_FINANCE_ACCEPTANCE](./ERP_FINANCE_ACCEPTANCE.md)为准。不是银行支付、会计总账或生产认证。

## 操作链路

1. 资金账户页新增企业内部记账账户：编码、名称、BANK/CASH/OTHER、币种、启停。没有银行卡号或银行连接；编码/类型/币种创建后固定，资料修改须当前version。
2. 采购已确认并实际收货后，在采购应付页点“从收货登记应付”，核对原收货数量、采购冻结单价和毛额，明确登记。历史收货不自动补账，未收货部分不进入应付。
3. 应付页本页勾选同供应商同币种的应付，点击付款登记，选同币种启用账户，填写本次各单核销额。可分次付、每笔最多100单，金额合计由服务端精确复核，不支持预付款或未分配余额。
4. 采购原批退货发生后，在对应应付详情勾选待登记退货，确认抵减；原毛额不变。有待登记退货时禁止新增付款核销，避免按旧毛额超付。
5. 付款页保留原付款及核销。错误登记只能整笔记账冲销，填写原因、携带原版本；不会撤销真实转账或发起退款。

毛应付−已登记退货抵减=净应付；待付=max(净应付−有效已付,0)，超付=max(有效已付−净应付,0)。退货可以让已付超过净应付，显示“超付待处理”，不伪装成已退款。不阻断真实采购退货，也不自动跨单调账。

供应商对账按当前可读仓库范围、供应商及币种分组，只计已登记应付、抵减、有效核销。待付和超付逐单分别汇总；不把不同单的超付自动抵掉欠款。名称取登记快照的展示值，不作为银行收款人或当前供应商主数据证明。未登记退货必须先处理；此页不是完整企业负债、银行余额、销售结算或利润。

## 权限与范围

- 菜单定义8300—8303：`erp:financeAccount:list/query/add/edit`。
- 8400—8404：`erp:payable:list/query/add/adjust/export`。
- 8500—8503：`erp:payment:list/query/add/reverse`。
- 8600—8601：`erp:financeStatement:list/export`。

新菜单仅登记定义，不替已有企业或员工开通。平台另行明确开通，企业所有者再配置角色及仓库范围。平台身份不能读取客户财务。

财务权限与采购/收货价格权限分开；收货与退货作业原无价格DTO不变。财务来源按payable:add提供所需原价快照，不附供应商联系人。账户企业共享，但必须有独立财务功能权限。

应付/来源/抵减叠加实时仓库范围。一笔付款如果核销多个仓库，必须当前拥有全部仓库范围才能列表、详情、冲销或重放；只拥有部分不返回整笔总额。对账及导出同范围，不能靠前端隐藏保护价格。

页面资料修改需query+edit；付款选择通常需payable:list、payment:add、financeAccount:list；应付详情及抵减通常需payable:query+adjust。仓库筛选控件可另配warehouse:list；操作API不会因为按钮能显示就放宽权限。

## API摘要

| 路径（前缀`/erp/finance`） | 方法 | 权限 |
|---|---|---|
| `/accounts`、`/accounts/:id` | GET列表、GET详情、POST新增、PUT修改 | financeAccount对应权限 |
| `/sources`、`/sources/:id` | GET未登记收货列表、原价预览 | payable:add |
| `/payables`、`/payables/:id` | GET列表、GET详情、POST登记 | payable对应权限 |
| `/payables/:id/returns` | GET原批退货，pending默认true，false看历史 | payable:adjust |
| `/payables/:id/adjust` | POST明确returnIds，最多100 | payable:adjust |
| `/payments`、`/payments/:id` | GET列表、GET详情、POST付款及核销 | payment对应权限 |
| `/payments/:id/reverse` | POST整笔记账冲销 | payment:reverse |
| `/statements` | GET范围内供应商/币种对账 | financeStatement:list |
| `/payables/export`、`/statements/export` | POST受控XLSX | 各自export |

列表pageNum/pageSize，上限100，唯一排序；公共筛选supplierId/warehouseId/accountId/receiptId/currency/status/code按具体资源适用。code为完整账户编码/应付号/付款号/收货号，不是任意搜索SQL。金额JSON始终是字符串，最多16整数+4小数，无浮点/科学计数/自动四舍五入。零价应付有效，付款核销必须正数。

写入version/requestKey持久保存；相同动作键/规范内容/操作者返回当时原结果，而非当前最新状态。重放仍先查当前功能和资源范围。同键异内容拒绝，金额/核销/动作结果与最小审计同事务。账户停用阻止新付款，不阻止有权限的原结果重放或整笔冲销。

页面未知结果固定原键、版本、内容，价格隐藏，只可显式原请求重试；关闭后先查记录，不换键重复付款。详情关闭/失活清空，晚响应作废，不存浏览器持久正文。财务HTTP日志不带查询串，成功审计只存动作/ID/版本，不落价款、备注、原因或正文。

## 显式升级与验证

008七张CREATE表独立于已应用001—007，禁止原地改写已安装脚本。启动只读校验；全无可选，部分/孤立记录/结构漂移拒绝。依赖完整采购004/退货005及原ERP基础。

```powershell
go run ./cmd/erpfinance -config configs/multitenant.local.yml # 默认只读
# 停止写入，先全库+公开/私有附件备份并恢复核验；仅本地独立库
go run ./cmd/erpfinance -apply -confirm-database ruoyi_go_mt -backup-report <本轮bundle/verified-report.json>
.\scripts\verify-erp.ps1 -Integration -Count 2
```

CLI拒绝远程、原库和恢复副本写入；repository恢复测试只允许当轮新建命名副本。所有升级器共享ERP结构锁。初始化中断保留未登记部分结构供复核重跑；已登记缺表或漂移不自动修复。报告门槛不是维护锁/签名证明，操作人仍须停写并核对源前后摘要。

未含销售应收/平台结算、实际转账/退款、预付款、会计期间/总账/税、库存成本/利润、汇兑、多法人、远程/容量/生产升级认证。
