# 采购退货（005）

适用`erp`分支、本地独立多租户环境。是采购/分批收货的数量纠错，不是退款、成本、应付、审批或完整采购财务。

## 使用

平台显式为企业开通“采购退货”，员工须有list/query/return及仓库范围。8000/8001/8002仅登记定义，不替已有企业开通，不执行`-grant-demo`。

`/erp/purchaseReturnOrder`分“原收货退货”“退货历史”。按完整单号搜索；原批显示累计收货/已退/净收货，本次数量初始0。填写数量及原因，确认即扣库存；历史只读，不可改删撤销。原收货页提供退货链接。接口和页面均不含价格，不只是隐藏列；return不隐含采购/供应商query。

收10退2：原收货仍10，已退2，净收8；采购已收、状态、version不回退，不自动补收2，需要补货另建采购。只能退原批未退量且实物－预占须足够；只扣实物，预占不变。停用主数据允许历史纠错，当前企业/功能/仓库范围必须仍有效。

## 重试

退货version属于原收货批次：初始1，成功后+1，不是采购version。一次1—100行，每行1—十亿整数，原因必填最多500字，不换行。

首次发送固定收货ID、版本、键、原因和数量；失败锁表单，只沿用原请求重试。响应丢失不代表未扣库存；放弃重试后先查历史，不直接换键再做。同键同操作者同内容回原结果，不是最新状态；异内容/操作者拒绝。曾成功也不能绕过当前撤权。关窗/失活/离页清详情、作废晚响应，不在local/sessionStorage存正文。

## 显式结构升级

005只新增两表，001—004不改。启动只读；部分安装、缺登记、漂移拒绝。新业务表带tenant_id和审计字段。实际表先用现有生成器产出ZIP留证，再按领域事务扩展，未开放原生成CRUD或执行生成菜单SQL。

先停写、原生备份全库及公开/私有文件，恢复新库逐表逐文件核验，再运行：

```powershell
go run ./cmd/erppurchasereturn -config configs/multitenant.local.yml
# 上行为默认只读；下行为核验后显式写入，仅允许本地ruoyi_go_mt
go run ./cmd/erppurchasereturn -config configs/multitenant.local.yml -apply -confirm-database ruoyi_go_mt -backup-report <本地核验报告路径>
```

报告需24小时内、同目录database.sql大小/SHA相符、全库及公开和私有导入兄弟目录恢复全部通过。门槛不是签名证明或自动停机机制。工具只CREATE/登记005，依赖完整004，与003/004共用命名锁，不写菜单/账号/授权/业务种子，不ALTER/DROP或自动修复。不得解除DSN守卫冒充生产升级器。

## 开发验证

设计、接口权限、锁序和七项预审见[设计](./ERP_PURCHASE_RETURN_DESIGN.md)。退货头/行、托管PR、余额、流水、最小成功审计同事务；库存仅接受repository私有且持久关联匹配的来源证明。旧入口不能操作托管IN/PR或通过创建重放绕过。

```powershell
scripts/verify-erp.ps1 # 无数据库：格式、Go测试/vet、前端测试及构建
scripts/verify-erp.ps1 -Integration -Count 2 # 有写入：仅明确授权的新本地测试企业
```

专项`TestERPPurchaseReturns`；结构演练`TestERPPurchaseReturnRestoreIntegration`须设置MT_TEST_ROOT和新恢复库ERP_PURCHASE_RETURN_RESTORE_DB，不得指源库。结果见[本轮验收](./ERP_PURCHASE_RETURN_ACCEPTANCE.md)；接口、组件、真实页面、恢复与生产验证分别记录。
