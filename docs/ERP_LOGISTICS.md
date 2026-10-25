# 物流基础011

当前安装/验收状态见[ERP_LOGISTICS_ACCEPTANCE](./ERP_LOGISTICS_ACCEPTANCE.md)，规则见[设计](./ERP_LOGISTICS_DESIGN.md)。

## 使用

在“物流设置”维护物流公司和配送方式，两页签共用一页。公司/方式编码固定，方式所属公司固定；
名称、启停和备注修改须持久version。公司和方式都启用才能在新包裹选择。

“包裹发货”保留原手工代码/运单入口，新增目录选择。保存物流不扣库存；手工确认发货继续使用原事务。
目录选择保存公司和方式名称快照，改名不自动刷新已有包裹。已有包裹仅改运单沿原快照，即使原方式停用也可维护；
换选其他方式或切回目录的新选择须启用。切回手工会清当前目录关联，旧手工代码不自动映射新公司。
已发货不可改物流；公司/方式停用不阻断已保存包裹的本地手工发货，仓库/店铺/SKU及当前范围仍按原规则检查。

重复运单仅提醒当前用户可见包裹的相同物流代码及运单，不返回隐藏记录，不作全局唯一限制；提示与写入不是原子保证。
发货记录仅列已发货、当前店铺/仓库可读包裹，按物流代码、方式编号、文本运单、录入方式和发货时间筛选导出。
不包含收件人、电话、地址或金额；详情仍走原包裹权限。旧手工记录显示原代码，目录名称按原快照。

未知结果锁定原内容/版本/requestKey，仅显式原请求重试；关闭后先刷新核对，不换键重复提交。
新JSON接口明确业务拒绝为HTTP200/code400，内部或提交结果不明仍code500，旧手工接口响应保持。

## 权限与接口

物流公司及配送方式共享设置权限erp:logistics:list/query/add/edit；设置菜单9200—9203。
发货记录erp:shipment:list/export，菜单9300—9301。定义注册不等于给旧企业开通；还需平台开通、员工角色配置。
平台身份不代入客户业务。

| 路径 | 方法 / 权限 |
|---|---|
| /erp/logistics/carriers、/shippingMethods | GET列表=list、GET/:id=query、POST=add、PUT/:id=edit |
| /erp/logistics/options | GET分页搜索；package:edit，只有启用方式最小ID/代码/名称及公司标签 |
| /erp/logistics/check-tracking | POST；package:edit及当前包裹店铺/仓库范围，仅duplicate布尔值 |
| /erp/package/:packageId/shipping | GET=package:query；data含packageVersion、available、snapshot；PUT=package:edit |
| /erp/logistics/shipments | GET=shipment:list；列表分页并查实时店铺/仓库范围 |
| /erp/logistics/shipments/export | POST=shipment:export；同范围、统一导出限额、无收件DTO的XLSX |

设置写入DTO为code/name/carrierId/status/remark/version/requestKey，新增version=0；只有配送方式填写carrierId。
包裹目录写为shippingMethodId/trackingNo/version/requestKey。租户、审计、快照由服务端赋值。
列表与选项pageNum/pageSize，上限100；查询code精确、name模糊，设置status可筛选；包裹原GET不增加快照字段。
快照GET返回同一读取快照内的packageVersion，页面只与相同版本的原包裹详情组合，避免混合两个时点。

## 显式本地升级

```powershell
go run ./cmd/erplogistics -config configs/multitenant.local.yml
# 停写，完成本轮全库与两附件根配套备份/新恢复核验后：
go run ./cmd/erplogistics -apply -confirm-database ruoyi_go_mt -backup-report <本轮bundle>/verified-report.json
```

只允许本地独立ruoyi_go_mt；CLI拒绝原库/远程/恢复副本。默认只读，启动只验证，不自动建表。
011只CREATE四表，不改001—010；记录后不可原地改写，部分/漂移/孤儿拒绝，DDL中断保留后复核重跑。
没有011时原手工包裹仍可用，快照GET返回available=false；新设置/记录接口提示先安装。

真实接口用例TestERPLogistics，整体入口scripts/verify-erp.ps1 -Integration -Count 2；
结构/事务故障用例须显式ERP_LOGISTICS_RESTORE_DB且等于本轮pre011报告中新副本，禁止对源库故障注入。
不包含承运商API、电子面单、轨迹、运费/扣款、拆包、完整WMS或生产认证。
