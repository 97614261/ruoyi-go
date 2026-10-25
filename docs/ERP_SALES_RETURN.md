# 订单退货入库（数量基础版）

适用`erp`分支、本地独立多租户环境。设计见[设计](./ERP_SALES_RETURN_DESIGN.md)，真实结果见[验收](./ERP_SALES_RETURN_ACCEPTANCE.md)。不把设计、测试用例或生成器产物当成已验收证明。

## 能做什么

已发货订单→原包裹→填写原行退货数量/原因→立即退回原仓→查不可变退货历史。
允许分批退回；每个原行累计不能超过已发数量。两条店铺变种映射同SKU也分别限额，入库凭据再按SKU汇总。
增加实物、不改预占，退货头行、托管SR、余额、不可变流水及最小成功审计在同一事务。
原订单/包裹保持SHIPPED及原版本、数量、物流事实；毛发5/退2/净发3，不把原发货改成3。

只接收`version/requestKey/reason/lines[{packageLineId,quantity}]`，不接收租户/仓库/价格/库存凭据/状态覆盖。
退货版本独立于订单/包裹物流版本，原包裹下退货头数量+1。每次读取后固定原版本和请求键；不确定时用原请求重试，不能换键重复入库。
关闭前未确认的请求，之后先查退货历史或让管理员核对，不用刷新后盲目重发。

## 权限与隐私

菜单8100—8102：`erp:salesReturnOrder:list/query/return`，与旧库存`erp:salesReturn:*`不同。
list读取可退包裹/退货记录分页；query读只读退货详情；return读最小可退明细并提交。
同时检查当前企业、功能、实时店铺和原仓库范围；平台身份不得代入。重放也不能绕过撤权。
不隐含订单/包裹详情权限，不返回收件人、地址、电话、运单、报价或内部请求指纹。原因只在退货详情，不进列表或成功审计。
新菜单仅登记定义，未给demo/demo-b或任何既有企业自动开通；需要平台另行开通，再由企业分配员工功能、店铺与仓库。

| 方法和路径 | 功能权限 |
|---|---|
| GET `/erp/salesReturnOrder/returnable/list` | list |
| GET `/erp/salesReturnOrder/list` | list |
| GET `/erp/salesReturnOrder/:returnId` | query |
| GET `/erp/package/:packageId/returnable` | return |
| POST `/erp/package/:packageId/returns` | return |

列表`pageNum/pageSize`，上限100，按ID唯一兜底；明细/每次提交最多100行。字段camelCase，普通业务错误仍用统一RuoYi响应。
可退列表可按shopId/warehouseId/orderId/packageId/packageNo筛选；退货列表按shopId/warehouseId/orderId/packageId/returnNo。
订单/包裹详情可跳转原包裹退货，仍单独检查新功能及来源范围，链接参数不能授予权限。

## 旧库存退货兼容

旧库存SR没有原订单行证据，不能猜分到原行。
已有非本模块已过账SR的原SALE，本模块拒绝再登记商业退货；列表`legacyStockReturn=true`，`netShippedQuantity=null`表示不能按原行确定，不是0。
`returnedQuantity`仅是本模块已归属数量，该阻断状态下不能把它解释为包含旧纠错的全部实际退回量；页面显示“需核对”。
已产生商业退货的原SALE，旧入口不能再新增/确认同源SR或整单冲销；此前未过账草稿可按原权限取消，不可再确认。
两路在原SALE行锁下互斥。历史依旧按实时范围读取，不修改旧记录或回填猜测分摊。
本模块托管SR不能走旧入口编辑、确认、取消、创建键重放或再作为退货来源。

## 结构、升级和验证

006仅新增`erp_sales_return_order/line`；不修改001—005。应用启动只读校验；完全未安装可选，部分安装、孤立版本、哈希/列/索引/FK漂移拒绝。
结构依赖完整003及基础多租户/库存控制；不依赖采购004/005的业务表。实际本地006安装前，先在新恢复副本测试失败重试/漂移拒绝/命名锁，再显式安装。

```powershell
# 默认只读，不给已有企业授权，不自动启动/修复结构
go run ./cmd/erpsalesreturn
# 必须先停写并备份恢复核验；仅限本地ruoyi_go_mt
go run ./cmd/erpsalesreturn -apply -confirm-database ruoyi_go_mt -backup-report <已核验报告路径>
.\scripts\verify-erp.ps1
# 只在明确授权的专用本地测试数据内运行，建立并保留停用企业
.\scripts\verify-erp.ps1 -Integration -Count 2
```

CLI的报告门槛不是维护锁或签名证明，操作人仍须停写、核对备份、比对源前后表和附件。恢复副本与本地辅助程序不等于生产迁移产品。
本轮测试依用户2026-10-23持续授权：仅当轮新建本地测试企业/账号/角色/店铺/仓库，GUI验证码临时关闭后恢复；结束撤权及停用保留，不复用旧停用企业。

## 尚不包括

退款/退运费、财务/应收/税/成本、质量检验/报废/冻结库存、换货、跨仓退货、多包裹/部分发货、审批、平台售后API。
不表示远程部署、Linux、容量、生产迁移/回退或所有浏览器/手机真机认证。
