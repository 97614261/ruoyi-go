# 仓库基础增强（007）

适用erp分支、独立本地ruoyi_go_mt、自有web。设计见[ERP_WAREHOUSE_DESIGN](./ERP_WAREHOUSE_DESIGN.md)，证据见[ERP_WAREHOUSE_ACCEPTANCE](./ERP_WAREHOUSE_ACCEPTANCE.md)。不是完整WMS或生产认证。

## 功能与边界

- 仓库：可选负责人/地址、状态筛选；编辑提交详情的持久version，同事务更新，冲突后人工重新核对。负责人不是仓库授权；私有资料只在warehouse:query返回，不进库存/流水或通用正文日志。
- 库存：有可用、零实物、全部预占、低库存筛选；零库存只列已有余额行，不生成仓库×SKU笛卡尔积。不同SKU/单位不直接相加。
- 追溯：库存→流水→来源单据，预占来源仅列当前可读单据，不冒充全部预占明细。流水名称/单位采用原凭据快照，支持完整单号、事件、时间筛选。原单query和店铺/仓库范围不因链接放宽。
- 预警：仓库＋SKU最低可用库存，可用<=阈值提示。0有效、未配置null；显式null清除但保留版本防ABA，首次version=0，漏字段不能清除。不自动采购/推送。
- 调拨：同企业不同仓，整单DRAFT→IN_TRANSIT→COMPLETED；草稿可改商品数量/备注及取消，两仓不可转移。调出仅扣源可用、不动reserved，在途不可售，收货才增目标实物。已调出不可删除/直接取消，目的仓或SKU停用会阻止收货，须先处理主数据或另设计补偿，不直接改余额。
- 核对：只读一致快照，按当前仓库范围分页；对全部流水聚合但不泄露隐藏店铺原单。包括余额缺失的流水组合，零净额也标balanceMissing=true，不当作0库存/一致，不自动建余额。每页通过不是整仓通过，多页不保证同一瞬间。

## 权限与操作

库存/流水导出分别stock:export、ledger:export；配置stock:alert，核对stock:reconcile。仓库编辑仍仅当前所有者且需warehouse:edit。预占来源同时需stock:list、sales:query和实时范围，不返回收件资料。

调拨权限erp:transfer:list/query/add/edit/dispatch/receive/cancel。list/query可读涉及本人任一仓侧的明确调拨，但不授权读取对方仓库存；add/edit/cancel查两仓，dispatch查源，receive查目标。页面确认需读取详情，因此操作角色通常也配置list/query；写接口本身不隐含query。

新菜单只登记定义6501—6503、6601、8200—8206，不替既有企业/员工开通，不改demo/demo-b。平台明确开通后，企业另外分配功能和仓库范围。

调拨TOUT/TIN凭据仅由调拨事务过账，旧CRUD/退货不能绕过；草稿也冻结引用SKU单位。创建和动作保存原结果，重放先查当前身份、功能和对应仓库。未知结果沿用原键/版本/内容重试；关闭后先查单据，不换键重复调出/收货。页面不持久缓存正文。

## 显式本地升级

```powershell
# 默认只读
go run ./cmd/erpwarehouse -config configs/multitenant.local.yml
# 停止写入；先核验完整数据库＋公开/私有附件的备份及恢复，再使用本次bundle报告
go run ./cmd/erpwarehouse -apply -confirm-database ruoyi_go_mt -backup-report <本次bundle中的verified-report.json>
```

007仅CREATE五表，旧001—006冻结；CLI拒绝远程、原库、恢复副本，检查24小时内证据及原始SQL摘要，和旧升级器共享锁。初装DDL中断保留部分结构，复核后重跑；已登记后缺表、结构漂移不能自动修复。启动只读，不自动迁移/授权。不证明旧二进制可回退或新增强能不升级运行。

旧流水没有独立时间/事件索引，依赖企业/仓库/SKU范围，不声称全量搜索性能。当前只验功能正确性，不是容量压测。

## 验证及未包含

```powershell
.\scripts\verify-erp.ps1 -Integration -Count 2
# schema故障测试额外指定MT_TEST_ROOT、ERP_WAREHOUSE_RESTORE_DB，且只能是当轮新建恢复副本
go test ./internal/repository -run '^TestERPWarehouseRestoreIntegration$' -count=1 -v
```

只在本轮新企业验收，结束撤权/恢复验证码/停用保留，不重开旧夹具。真实页面、正确性并发、备份恢复、性能容量是不同证据。

007本模块未包含分批调拨、损耗/退回、库位/库区、批次/序列号/效期、不可售/残次隔离、成本估值或真实渠道API。后续已独立提供[单包裹拣货与扫码复核](./ERP_PICKING.md)，内部应收应付见008/009；这些不构成库位WMS或库存成本模块。退货默认回原仓实物，本版本不是带质检隔离的完整售后仓。
