# 销售财务基础009

仅企业内部业务记账，不接银行、不执行真实收款/退款，不是平台结算、总账或利润。设计见ERP_SALES_FINANCE_DESIGN，实际验证见ERP_SALES_FINANCE_ACCEPTANCE。

## 使用

1. 平台为指定企业明确开通销售应收/收款/店铺对账，企业分配角色及当前店铺、仓库范围。仅登记菜单不会自动授权旧企业。复用资金账户页，需要额外financeAccount:list/add等权限，不因销售收款自动授予。
2. 从已发货订单显式登记应收。原订单总额必须明确，缺金额不当零；保存总额及原行数量快照，不编造单价，不自动历史入账。
3. 应收页勾选同店铺、同币种单据，选同币种启用内部账户，填写本次各单核销额；服务端精确复核合计、版本和待收。支持分次，每笔1—100单，不支持预收/跨币种/未分配余额。
4. 原订单实物退货发生后，新收款先被阻止。应收详情每次选一笔原退货，人工填写抵减金额和原因；可明确零额，累计不超过原毛额。不能按数量自动估算。后续退货可形成超收，不自动退款。
5. 错误收款只能整笔记账冲销，保留原收款/核销/原因/版本。店铺对账按当前可读店铺、仓库及币种分别汇总待收/超收，不自动跨单抵账；不是全企业/银行余额。

## API及权限

前缀`/erp/salesFinance`，JSON字段camelCase，金额字符串最多16整数+4小数。列表pageNum/pageSize上限100，固定唯一排序；筛选shopId/warehouseId/accountId/orderId/currency/status/code按资源适用，code为完整单号。

- GET sources、sources/:id，POST receivables：erp:receivable:add；来源不返回收件人/电话/地址或运单。
- GET receivables、receivables/:id：erp:receivable:list/query。
- GET receivables/:id/returns?pending=true|false，POST receivables/:id/adjust：erp:receivable:adjust。写入version/requestKey/returnId/amount/reason。
- GET collections、collections/:id，POST collections，POST collections/:id/reverse：erp:collection:list/query/add/reverse。收款固定accountId/shopId/currency/amount/allocations，每行receivableId/version/amount。
- GET statements、POST statements/export：erp:shopStatement:list/export；POST receivables/export：erp:receivable:export。

应收、来源及对账同时查实时店铺/仓库范围。收款列表/详情/冲销/重放须拥有全部核销单据范围，不能因其中一单可读而泄漏整笔总额。平台身份不能代入。销售财务权限不隐含订单收件信息query。

每个写动作持久原键/指纹/操作者/版本，业务和无价款最小成功审计同事务；同键异内容拒绝，重放先查当前范围后返回原结果，不返回最新状态。账户停用不妨碍有权的旧重放/冲销，但禁止新收款。未知结果页面只可固定原请求显式重试；失败/关闭隐藏旧价款、作废晚响应，不持久缓存正文。no-store，HTTP不记录财务查询串。

## 本地升级及验证

009六张CREATE表依赖完整006/008；复用008账户，不改001—008。所有升级器共享锁；启动仅只读校验，全无可选，部分/孤立登记/漂移拒绝。CLI默认只读，写入仅本地ruoyi_go_mt、明确确认及24h内完整数据库+公开/私有文件的已核验恢复报告。

```powershell
go run ./cmd/erpsalesfinance
# 明确停写、完整备份恢复核验后，再显式apply
go run ./cmd/erpsalesfinance -apply -confirm-database ruoyi_go_mt -backup-report <本轮bundle/verified-report.json>
.\scripts\verify-erp.ps1 -Integration -Count 2
```

这不是任意生产升级工具；不要在远程/原库试跑，不原地修改已安装009，不把本地恢复点称作实时备份。不含真实资金、退款登记、平台手续费/汇兑、总账税务、成本利润或生产/容量认证。
