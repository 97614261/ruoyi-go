# ERP最小手工库存闭环

适用分支：erp。只在独立多租户开发库新增表，main原库、Java SQL、参考前端不改。
这是第一版手工库存闭环，不是完整的马帮/店小秘ERP，也不是生产上线认证。
当前ruoyi_go_mt已扩展为ERP开发库；不能切回旧底座分支后仍假定这些菜单/表不存在。
若后续并行运行纯底座与ERP，须分别使用数据库、Redis和上传目录，不能共用同一运行环境。

## 已实现范围

商品/SKU → 仓库 → 入库草稿 → 确认入库 → 销售草稿 → 确认预占 → 确认出库。
出库前取消销售会释放预占；草稿取消不改变库存。

| 操作 | 实物 | 预占 | 可用 |
|---|---:|---:|---:|
| 入库100 | 100 | 0 | 100 |
| 确认销售3 | 100 | 3 | 97 |
| 确认出库3 | 97 | 0 | 97 |

可用=实物−预占，数量为整数基本单位。单行1—十亿、单据1—100行，SKU不可重复，累计实物上限一万亿。
商品单位可为件/箱等，但一条SKU的数量不支持小数或自动单位换算。编码支持字母、数字、点、下划线、短横线，最长64。

## 表和关联

| 表 | 用途 | 关键约束 |
|---|---|---|
| erp_sku | 名称、编码、规格、单位、状态 | 企业内编码唯一 |
| erp_warehouse | 仓库编码、名称、状态 | 企业内编码唯一 |
| erp_document | IN入库 / SALE销售，单号、状态、仓库快照 | 企业+requestKey唯一；单号唯一 |
| erp_document_line | SKU编码、名称、规格、单位、数量快照 | 企业+单据+SKU唯一 |
| erp_stock | 仓库SKU的实物/预占 | 企业+仓库+SKU唯一 |
| erp_stock_ledger | 入库、预占、释放、出库的变化与结余 | 企业+单据+SKU+事件唯一 |

全部表带tenant_id及五个审计字段。业务外键包含tenant_id，跨企业关联在数据库层同样拒绝。
已被单据或库存引用的商品/仓库不能删除；SKU一经引用也不能修改计量单位，防止把原来的件数误显示成箱数。
停用后不能创建或继续确认新的库存动作，但仍允许取消已预占订单以释放库存。
列表最多100条，按唯一主键稳定排序；按编码/租户/关联键查询有索引。名称模糊搜索按企业范围过滤，含前导通配符，
不声称一定命中名称索引范围扫描；大数据量下应实测后优化。

## 状态规则

- 入库：DRAFT→POSTED；DRAFT可取消。POSTED不可直接取消、删除或改数量。
- 销售：DRAFT→RESERVED→SHIPPED；DRAFT/RESERVED可取消，RESERVED取消记RELEASE流水。
- 草稿不提供修改接口；录错可取消重建。退货、出入库冲正和盘点尚未实现，不能用删流水修账。
- 同一终态的重复确认/出库/取消为无副作用成功；状态不允许的动作明确拒绝。
- 创建请求保留requestKey重试，同内容得到原单据；同键不同内容拒绝。不能每次网络重试都换新键。

库存动作先锁单据，再锁仓库、按SKU排序的商品及余额；同一事务写余额、不可变流水、单据状态。
仓库父行是当前的粗粒度串行边界，优先简单正确，不代表已完成高吞吐量优化。
普通操作日志另行异步保存，库存流水本身与库存事务原子提交，是库存变化的必需凭据。

## 初始化和开通

```powershell
# 只创建缺失ERP表并登记菜单；不更改既有企业开通范围
go run ./cmd/erpinit -config configs/multitenant.local.yml
```

工具只接受本地ruoyi_go_mt，运行前先备份；DDL不依赖事务回滚，不在服务启动时执行。
重复运行不覆盖已有表、库存或菜单冲突；后续改表要单独设计迁移，不靠CREATE IF NOT EXISTS更新旧表。

平台账号管理开通，不代入客户业务。平台“租户管理→功能授权”可选择ERP工作台及所需按钮；企业重新登录后生效。
平台自己的导航不展示客户ERP工作台，但菜单管理/授权树仍保留ERP节点供明确开通。
只有用户明确同意给本地demo/demo-b开通时，才执行可选的`-grant-demo`，这也会创建各自默认仓库。
本次用户确认后已仅为demo/demo-b开通并各建默认仓库；其他企业由平台明确授权，不批量开通。
该工具只追加ERP节点，保留历史授权；企业行锁和成功审计与变更同事务。重复执行不会推进版本、重复建仓或重复记成功审计。
企业管理员可再给员工角色分配功能权限；首版权限是企业内功能级，不含仓库级/店铺级授权。

## 接口

| 路径 | 作用/权限 |
|---|---|
| /erp/sku、/erp/warehouse | 生成器CRUD及导出，权限为对应list/query/add/edit/remove/export |
| GET /erp/receipt/list、GET /erp/sales/list | 分页，erp:receipt:list / erp:sales:list |
| POST /erp/receipt、POST /erp/sales | 创建草稿，erp:receipt:add / erp:sales:add |
| GET /erp/{receipt或sales}/:documentId | 明细快照，对应query |
| PUT /erp/{receipt或sales}/:documentId/confirm | 入库或预占，对应confirm |
| PUT /erp/{receipt或sales}/:documentId/cancel | 取消，对应cancel |
| PUT /erp/sales/:documentId/ship | 确认出库，erp:sales:ship |
| GET /erp/stock/list | 余额，只读，erp:stock:list |
| GET /erp/ledger/list | 流水，只读，erp:ledger:list |

创建格式：`warehouseId`、`requestKey`、可选`remark`、`lines:[{skuId,quantity}]`。
类型来自服务端路由，企业来自会话；客户端tenantId/kind/status不能控制归属或直接过账。
数字/字段长度在服务端校验，错误走统一response，JSON camelCase，列表rows/total位于顶层。
新增单据返回data，含documentId、documentNo和快照明细。

## 验证方法

```powershell
go test ./cmd/... ./internal/... ./pkg/... -count=1
go vet ./cmd/... ./internal/... ./pkg/...
$env:MT_TEST_ROOT=(Get-Location).Path
go test ./test/multitenant -count=1 -v
cd web
node --test test/*.test.mjs
npm run build:prod
```

SKU/仓库实际通过现有生成器从新表导入生成，再扩展校验、TableName、引用删除保护和自有页面。
本地来源归档为logs/erp-generated.zip，不能用原始生成配置直接覆盖扩展后的文件。
真实接口测试使用专用新建验收企业，保留单据、库存及流水，结束后停用企业；不清库、不FLUSH、不改现有企业密码。
测试结果见 [ERP_TEST_REPORT.md](./ERP_TEST_REPORT.md)。浏览器交互仍受连接条件限制，不能用构建通过替代。

## 未包含

电商渠道、店铺、物流、财务金额、完整采购订单/供应商、退货冲正、库存盘点、批次/序列号、小数单位、多法人、多币种。
上线前需补单据纠错/冲正流程、业务备份恢复、真实浏览器点验和针对目标服务器的容量测试。
