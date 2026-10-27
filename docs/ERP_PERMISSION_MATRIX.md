# ERP 权限回归矩阵

适用于 `erp` 分支的本地多租户 ERP。这里维护检查规则和用例入口；某轮执行结果、数量及数据清理证据见对应验收报告，不能用本文件代替测试结果。

## 1. 判定顺序

1. 会话有效，企业处于可访问状态；平台账号不代入企业业务。
2. 当前企业已开通功能，员工当前角色具有接口权限；所有者也受企业开通范围约束。
3. 按当前企业隔离；员工使用当前仓库、店铺范围，不能依赖登录时的旧快照。
4. 按业务边界进一步限制：导入任务属于提交人或企业所有者；收货权限不附带采购价格；财务、导出、发货和扫码复核分别授权。
5. 原请求重试仍要重查当前边界。以前成功过、拿到过下载地址，均不等于永久授权。

前端按钮仅改善使用体验，实际断言使用真实 HTTP。权限拒绝要检查业务响应码、响应正文和真实文件内容，不能只看 HTTP 200 或按钮是否隐藏。

## 2. 接口登记与自动检查

静态登记：[`test/permissionmatrix/routes.tsv`](../test/permissionmatrix/routes.tsv)。每行记录方法、Gin 路径、必须同时满足的权限、是否仅所有者。清单独立维护，不从运行时自动覆盖生成。

[`TestERPPermissionRouteInventory`](../internal/router/erp_permission_matrix_test.go) 同时核对完整生产构造器与实际注册链，检查新增/删除接口、企业入口、中间件权限检查数量及所有者限制是否与清单一致。测试在首个中间件中终止，不执行鉴权或业务查询。它能检查检查点数量，**不能读取闭包捕获的权限字符串**；相同数量但错误标识仍须代码复核及定向授权测试发现。

[`TestERPPermissionMatrix`](../test/multitenant/erp_permission_matrix_test.go) 在专用新企业中，用匿名、平台、无 ERP 权限员工逐条访问清单，分别要求 401、403、403，且不能附带业务数据。路径参数使用不对应既有数据的固定哨兵，写接口使用有效 JSON 空对象。这只证明入口拒绝，**不证明该接口的完整参数、成功语义或资源隔离**。

新增或调整接口时，应同时审查路由权限标识、菜单定义、清单、具体资源测试及本文映射。双权限接口必须分别去掉一个权限验证，不能只测两者全无。仅所有者接口还要测试“员工有功能权限仍拒绝”。

## 3. 真实资源矩阵

以下均由已有模块套件或本次新增用例检查，不把入口空对象检查算进这一层。

| 模块/通道 | 必须保持的边界 | 真实 HTTP 用例入口 |
|---|---|---|
| 企业和会话 | 跨企业用户/角色不可关联；企业停用、交接和撤权影响旧 token | [隔离](../test/multitenant/isolation_test.go)、[身份](../test/multitenant/identity_test.go)、[高级边界](../test/multitenant/advanced_test.go)、[平台工具](../test/multitenant/admin_tools_test.go) |
| 仓库/库存/原单据 | 仓库默认拒绝；列表、明细、导出与库存动作使用当前范围 | [ERPControls](../test/multitenant/erp_controls_test.go)：`warehouse_default_deny_and_grant`、`warehouse_scope_lists_details_exports_and_revoke` |
| 调拨/预警/核对 | 发出和收货分别授权；跨企业拒绝；导出精确过滤 | [ERPWarehouseOperations](../test/multitenant/erp_warehouse_operations_test.go)：`directional_scope_revocation_and_cross_tenant`、`readonly_reconciliation_and_filtered_XLSX_exports` |
| 店铺和配对 | 授权目标属于本企业；CAS 防止覆盖；店铺范围即时生效 | [ERPShops](../test/multitenant/erp_shop_test.go)、[ERPShopItems](../test/multitenant/erp_shop_item_test.go) |
| 供应商 | 列表不泄露私有联系方式，详情另需 query，企业隔离 | [ERPSuppliers](../test/multitenant/erp_supplier_test.go) |
| 采购与仅收货员工 | receive 不附带 query 或价格读取；撤仓库后原请求也拒绝 | [ERPReceiving](../test/multitenant/erp_receiving_test.go)：`receive_only_privacy_isolation_headers_and_live_revoke` |
| 采购/销售退货 | 绑定原收货或包裹，按当前仓库/店铺访问，重放绑定员工 | [采购退货](../test/multitenant/erp_purchase_return_test.go)、[销售退货](../test/multitenant/erp_sales_return_test.go) |
| 订单/包裹/旧凭证 | 私有明细与摘要分开；审核/发货同时检查店铺和仓库；旧凭证不能绕过 | [ERPOrders](../test/multitenant/erp_order_test.go)、[ERPFulfillment](../test/multitenant/erp_fulfillment_test.go)：`tenant_and_live_shop_warehouse_scope_cover_legacy_vouchers_ledger_and_SR` |
| 异步导入 | 原员工逐单重查功能和店铺；企业所有者不能借重试替原员工提权 | [ERPOrders](../test/multitenant/erp_order_test.go)：`live_employee_scope_revocation_interrupts_and_retry_resumes` |
| 导入任务、结果、原文件、重试 | 同店铺同角色的另一员工仍不能看他人任务；跨企业拒绝；旧 token 撤范围后拒绝；query/import 两门独立 | [ERPPermissionMatrix](../test/multitenant/erp_permission_matrix_test.go)：`real_import_file_results_export_and_old_token_revocation` |
| 订单导出 | 有数据时先验证真实 XLSX；撤店铺后只剩表头；撤 export 后 403；摘要不含收件人 | 同上新增用例；不把空导出当成原来没有数据 |
| 商品与媒体 | 独立读/写/导入/图片权限，企业媒体归属、引用保护与旧会话撤权 | [ERPCatalog](../test/multitenant/erp_catalog_test.go)、[TenantIsolation](../test/multitenant/isolation_test.go) |
| 通用附件/下载删除 | 临时文件仅当前上传者及企业；分段/中断保留；不能删除商品、头像及普通上传 | [CommonDownload](../test/multitenant/common_download_test.go) |
| 应付/付款/供应商对账 | 独立财务权限；全部来源仓库当前可见；跨企业拒绝；导出不泄露私有 DTO | [ERPFinance](../test/multitenant/erp_finance_test.go)：`independent_finance_permission_all_warehouses_cross_tenant_and_live_revocation`、`scoped_statement_currency_separation_exports_and_private_DTOs` |
| 应收/收款/店铺对账 | 不借订单基础权限读价格；当前店铺范围；导出不含收件人 | [ERPSalesFinance](../test/multitenant/erp_sales_finance_test.go)：`independent_price_permission_two_employee_race_tenant_and_live_scopes`、`shop_statement_exports_and_no_private_receiver` |
| 物流 | 包裹资料受当前店铺/仓库限制；下拉最小信息；真实导出精确过滤 | [ERPLogistics](../test/multitenant/erp_logistics_test.go)：`minimal_options_duplicate_warning_and_live_scope_do_not_leak`、`export_exact_filters_text_tracking_and_binary_permission_errors` |
| 经营概览 | overview 不替代业务 list 权限；汇总使用实时资源范围 | [ERPOverview](../test/multitenant/erp_overview_test.go)：`overview_permission_does_not_grant_business_data_and_scopes_are_live` |
| 企业启用 | 仅企业所有者；模板预览不替用户保存或授权 | [ERPEnablement](../test/multitenant/erp_setup_test.go) |
| 拣货/复核发货 | 手工发货权限独立；当前双范围；他人请求键不可重放 | [ERPPicking](../test/multitenant/erp_picking_test.go)：`live_scopes_separate_manual_permission_and_cross_actor_retry` |

## 4. 本地运行

无数据库接口检查（格式、Go 单元、vet、前端测试和构建）：

```powershell
.\scripts\verify-erp.ps1
```

只跑路由登记检查，不连接数据库：

```powershell
go test -mod=readonly ./internal/router -run '^TestERPPermissionRouteInventory$' -count=1
```

真实 HTTP 专项，需要已运行的本地 8081 后端、独立多租户库及私有平台账号记录；会创建专用企业及资源，结束撤回本用例范围并停用保留：

```powershell
$env:MT_TEST_ROOT = (Get-Location).Path
go test -mod=readonly ./test/multitenant -run '^TestERPPermissionMatrix$' -count=1 -v
```

两轮完整本地回归：

```powershell
.\scripts\verify-erp.ps1 -Integration -Count 2
```

套件额外校验专用本地库及 Redis13，不代入原库、远程或既有企业。测试执行前记录企业水位和旧业务/授权/范围摘要；执行后检查新企业停用与范围清理。完整回归的残留范围只可根据该轮实际审计回执精确清理，不可套用旧轮水位、批量重开企业或全库清空。

## 5. 不外推的结论

这不是所有角色组合、所有筛选参数、所有并发时序的穷举。不能把 API 用例当成本轮浏览器点击、硬件扫码、Linux、负载容量、生产认证或独立渗透测试。新风险和新接口仍需要补对应真实资源用例。
