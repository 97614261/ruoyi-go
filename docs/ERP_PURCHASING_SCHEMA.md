# 采购步骤2：独立004结构、本地恢复与验证记录

日期：2026-10-22（客户端时区America/New_York）。本页为本轮快照；规则变化另加决议，后续实施只加导航，不覆写旧证据。

> 后续导航：步骤3—6采购本地范围已完成，回归/所有者及员工GUI/配套恢复与边界见[最终验收第10节](./ERP_PURCHASING_ACCEPTANCE.md#10-仅收货员工实际页面补验与最终收口)。下文“六张空表/未开放业务”是步骤2当时快照；001—004没有因后续改写。

## 1. 完成范围

用户确认[采购方案](./ERP_PURCHASING_DESIGN.md)后，本轮只实施步骤2：六张空业务表、独立显式升级命令、启动只读门槛、隔离登记及结构测试。新增资源是[004_purchasing.sql](../sql/erp/004_purchasing.sql)，通过[sql/erp/schema.go](../sql/erp/schema.go)嵌入，未修改已应用001/002/003。

没有新增供应商CRUD、采购/收货API、菜单、开通项、业务页面或第三方依赖。没有操作远程、原Java/main库、参考Vue或重置现有账号密码。采购业务状态、金额校验、分批收货事务和权限交集仍待后续步骤，不因本次建表而宣布实现。

### 新表

| 表 | 用途 |
|---|---|
| erp_supplier | 企业供应商资料、启停及持久版本 |
| erp_purchase_order | 采购头、固定供应商/仓库、币种/金额快照、创建指纹及版本 |
| erp_purchase_order_line | 采购行、SKU/单位快照、订购/累计已收、精确单价/行额 |
| erp_purchase_receipt | 每批已过账收货、采购/仓库/操作者、唯一托管IN关联及请求键 |
| erp_purchase_receipt_line | 本批采购行/SKU/数量快照 |
| erp_purchase_event | 动作前后状态/版本、操作者、请求指纹及有界原结果 |

全部含无业务默认值的tenant_id及五个审计字段，加入租户表守卫。FK按企业复合关联，特别约束同企业错采购单、错采购行/错SKU、错采购单仓库和错收货批次；没有级联删除。库内版本仍复用既有技术表erp_schema_migration，只新增004记录，不建第二套迁移历史。

SKU/单位字段沿用现有长度；供应商/采购号按字符串保存，单价/金额DECIMAL(20,4)。**FK不保证**库存凭据一定是同仓库已过账IN、正数量/不超收、状态流转、金额乘加或权限；MySQL5.7不能依赖CHECK补这些，后续repository事务必须重新检查。

## 2. 工具与只读边界

[cmd/erppurchasing](../cmd/erppurchasing/main.go)默认只读：

```powershell
go run ./cmd/erppurchasing -config configs/multitenant.local.yml
```

写入同时需要 `-apply`、`-confirm-database ruoyi_go_mt`、24小时内本次数据库及两类附件恢复证据，并严格拒绝原库、远程、恢复库及非TCP配置。repository再检查环回DSN和SELECT DATABASE；仅内部显式恢复演练允许受限ruoyi_go_mt_restore_命名空间。

```powershell
# 只有确认环境、停止写入并完成新的备份恢复核对后使用；不直接复用历史报告。
$purchasingBackupReport = '本次已核验备份目录中report.json的绝对路径'
go run ./cmd/erppurchasing -config configs/multitenant.local.yml `
  -apply -confirm-database ruoyi_go_mt -backup-report $purchasingBackupReport
```

报告检查源/新恢复副本、完成时间、失败状态、全表摘要及公开/私有附件核验标记，实际database.sql大小和SHA256；拒绝尾随JSON和非普通文件。它只是操作证据门槛，不是不可伪造签名，也不自动停止服务或证明其他写入者已停止。

安装依赖完整租户/库存底座及已登记003。与003共用同数据库命名锁，全部既有对象先检查，只CREATE本版本缺失表；同名异构不ALTER或修复。未登记的中途失败保留部分表，确认后重跑补齐；已登记却缺表视为损坏，拒绝静默重建。成功才新增唯一版本记录，重复完成不改变记录/时间。

DDL有隐式提交，不承诺整批ROLLBACK，没有DROP/清表/自动降级。共用窄CREATE解析/元数据比较实现，但004有独立白名单、版本和指纹，不能作为003的一部分被安装。

后端新增[VerifyERPPurchasingSchema](../internal/repository/erp_purchasing_schema.go)只读门槛：全无表且无记录时可选跳过；部分安装、孤立版本记录、缺记录/指纹不符或列/索引/FK漂移拒绝。启动不自动建表；四道门槛在Redis/业务执行器初始化前执行。新的测试同时固定003和已应用004的指纹。

## 3. 本地备份与恢复

本次先核对原Windows后台PID32536及二进制/配置，确认没有PENDING/RUNNING导入任务，再强制停止该精确进程。**强制终止不等于已验证优雅排空**；这是本地维护窗口，不外推生产停写方案。

备份时间：2026-10-22 09:02:12—09:02:23 UTC。

- 源：本地ruoyi_go_mt，MySQL5.7.20-log，45张表、245,653行。
- 原生单事务dump：21,664,813字节，SHA256 `ee88c365e554ac17c839a7e48981603f9de072edcfe31da5b9076825f8216e2e`。
- 公开文件118、私有导入文件67；各自完整物理清单与全部登记一一核验，公开/私有分别118/67条登记。
- 新恢复库：ruoyi_go_mt_restore_20261022_090212；仅新建，没有覆盖源库或旧副本。
- 全部表DDL、行数及逐行摘要一致，两类文件相对路径/大小/hash及登记一致；恢复演练前后源快照再次完全相同。

本地证据根：`logs/erp-purchasing-schema-20261022/`；dump与文件在其bundle-20261022T090212子目录。创建前收紧新目录ACL，仅当前用户/SYSTEM/Administrators；核验743个新对象无其他Allow规则，临时客户端凭据文件已移除。凭据不在命令行或输出，数据/附件/报告均被Git忽略。备份仍是本地留存，不是已加密异地备份产品。

## 4. 恢复副本测试

[TestERPPurchasingRestoreIntegration](../internal/repository/erp_purchasing_integration_test.go)首轮在刚核验、尚无004的新副本执行，通过：

- 全无结构可选；只有004版本记录而无表时，启动/安装均拒绝。
- 第三张CREATE故障注入后保留前两张、无004登记；启动拒绝，锁已释放，去掉注入后重跑补齐。
- 完成后重复安装不改记录；另一连接持有升级锁时拒绝并行安装。
- 两企业同供应商编码/采购号合法；跨企业供应商、仓库、SKU、库存凭据、操作者引用拒绝。
- 同企业错采购单仓库、错采购行所属单/错SKU、错收货所属单、错事件所属收货拒绝。
- 重复创建键、同IN重复收货、同动作请求键拒绝；00001及12.3400精确保留。
- 字段长度、索引、外键漂移分别被只读门槛及安装器拒绝，不自动修复；测试只修复自己在副本注入的漂移。
- FK业务夹具事务回滚，没有向源开发库写这些采购数据。副本上的自增计数可能因回滚插入而推进，不冒称测试后副本逐字节等于原备份。

另执行原TestERPPhase1RestoreIntegration并通过，验证共用比较逻辑没有破坏003的幂等、真实FK/前导零与漂移拒绝。

可复验（仅本次已核验副本；会安装004、写回滚夹具及短暂结构漂移）：

```powershell
$env:MT_TEST_ROOT = (Get-Location).Path
$env:ERP_PURCHASING_RESTORE_DB = '本次已核验的ruoyi_go_mt_restore_库名'
go test ./internal/repository -run '^TestERPPurchasingRestoreIntegration$' -count=1 -v
# 完成后恢复原环境变量。重复执行已安装副本不再覆盖“首次部分安装”分支。
```

默认单测明确SKIP真实恢复用例，不能代替显式执行；CLI证据缺失/失败/过期/篡改及DSN拒绝另有无数据库单测。

## 5. 源开发库升级结果

通过恢复演练后，再次确认源45表/两类文件摘要不变，使用本次report.json显式执行004并重复执行一次。结果：51张表，6张新业务表全部0行，仅技术迁移表新增004记录，原记录不变。

004指纹：`cba3c0146ca5161406e41af859c5eb95b664b0b4544414109619b6310a8533bd`；003仍为`ebc324c78ec32f39df3e0b36a4fee276bf86ff827dd1dc65f20fa880018db2eb`。

`source-after.json`于09:04:30 UTC、恢复后台前核对：其余原表DDL/行数/摘要完全不变；技术表除新增记录及正常自增计数推进外结构/旧记录不变；两类完整文件清单不变。四个结构门槛在同一5秒context内通过，实测合计402ms，不能外推容量。

随后启动含新只读门槛的本地Windows后台，PID26244，产物SHA256 `d93d0ab4faf7a981e9f622272cae7358455435966e74ec7a86f52c294f5f294b`。后台8081、前端5173健康检查均200。没有启动第二个产品实例，也没有部署Linux/远程。

## 6. 完整回归与索引检查

`scripts/verify-erp.ps1 -Integration -Count 2`最终：`test/results/erp-20261022T090627470Z-cb33ebb3/summary.json`，CHECKS_PASSED、integration=REQUESTED。格式、Go单测、vet、前端60项0失败0跳过、生产构建和真实本地集成两轮全部通过。

完整回归之后只补004固定指纹的单测断言及文档，没有再改产品代码或SQL；再跑默认五步，`test/results/erp-20261022T091604903Z-56d9cca7/summary.json`同样CHECKS_PASSED、integration=NOT_REQUESTED。后者不冒充额外一轮数据库集成。

回归在升级完成并恢复服务后执行，按既有规则新增专用企业/附件/单据/审计，结束停用保留；不能把“DDL前后源旧表不变”扩写成整个回归过程没有写数据库。未重置原demo密码或为真实企业自动开通采购权限。

恢复副本上还做6组候选SQL的只读EXPLAIN（包含实时仓库范围、仓库/状态、供应商、收货/事件历史及SKU引用），保留restore-explain.json。所需候选索引存在，但新表为空，优化器常选企业前缀/唯一键，事件查询还出现filesort；**不据此宣称生产SQL已优化或容量已验证**。后续实际接口/真实分布数据仍须复核，不能为消掉空表filesort改已登记004。

日志中的预期FK/重复键失败及第三张CREATE注入是负例通过证据，不是生产报错被忽略；没有删除失败/故障记录或放宽约束。

## 7. 下一步

进入步骤3：从实际erp_supplier表用现有生成器产出CRUD，再补CAS/编码固定/启停/联系人最小投影/日志隐私与企业隔离测试。采购单事务、收货过账、托管IN/PR保护及三页工作台分别属于后续步骤。

本轮没有自动提交或推送新改动。结构验收不等于完整采购、财务、生产升级/旧版本回退或服务器承载能力认证。
