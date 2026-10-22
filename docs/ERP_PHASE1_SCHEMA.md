# ERP一期步骤2：DDL、备份与显式升级

> 本页为步骤2结构与演练快照。后续步骤3已实现店铺/渠道目录/店铺范围，见[ERP_PHASE1_SHOPS.md](./ERP_PHASE1_SHOPS.md)；下文空表/无菜单/未启动等描述只对应迁移完成当时，不是后续运行现状。

适用erp分支、本地独立ruoyi_go_mt。2026-10-21步骤2仅结构与迁移工具；商业订单/包裹/导入的业务接口仍未实现。
一期业务设计见[ERP_PHASE1_DESIGN.md](./ERP_PHASE1_DESIGN.md)，步骤1历史基线见[ERP_PHASE1_BASELINE.md](./ERP_PHASE1_BASELINE.md)。

## 1. 本次结构

唯一DDL来源是[003_order_fulfillment.sql](../sql/erp/003_order_fulfillment.sql)，通过仅含资源的sql/erp包嵌入CLI和只读校验器，无新第三方依赖。
没有修改旧32张表；没有登记菜单、开通功能、授权员工、改密码或启停服务。

| 新表 | 作用 |
|---|---|
| erp_shop | 店铺、渠道/站点、币种、状态与版本 |
| erp_shop_access / erp_shop_policy | 员工店铺范围与持久授权版本 |
| erp_shop_item / erp_shop_sku_mapping | 店铺变种、可选外部ID、内部SKU配对与停用/版本 |
| erp_order / erp_order_line / erp_order_event | 商业订单、原始及审核快照、动作幂等事件 |
| erp_package / erp_package_line | 一订单一包裹、运单及关联订单行 |
| erp_import_job / erp_import_result | 私有导入文件元数据、任务状态、按订单/源行结果 |
| erp_schema_migration | 数据库级技术迁移版本与DDL指纹，不是客户业务表 |

12张业务表均有无默认值的tenant_id及五个审计字段；技术迁移表是唯一新增非租户表，不可作为生成业务表使用。
业务关联以企业复合键约束；店铺商品/订单行带店铺范围，包裹明细同时约束订单与包裹，防止同企业错单、错店关联。
具体列长、可空性、默认值、索引及外键以SQL为准，不用GORM AutoMigrate代替DDL。

### 数据约束的明确边界

- 外部单号/商品编码用字符串和大小写敏感字符序，保留前导零；后续接口须拒绝编号首尾空白，不能依赖MySQL5.7的字符比较区分尾随空格。
- 原始金额DECIMAL(20,4)，未知为NULL；币种CHAR(3)。这不是已实现的金额校验、汇率或财务功能。
- 库存凭据引用唯一、一期每订单一个包裹、来源订单及动作幂等键唯一；重复键不是完整幂等算法，仍需后续内容指纹和状态检查。
- MySQL5.7不依靠CHECK保证数量正数、状态流转、可空关联的全有/全无、SALE类型或订单/库存凭据仓库一致；步骤3—6必须在服务/事务内校验并测试。
- 已引用配对行保留，历史审核行保存当时mapping_version和SKU快照，不把当前映射解释成历史映射。
- DDL层面不实现店铺/仓库授权、订单审核或自动库存更新；零业务行并不等于业务流程已验收。

## 2. 显式工具与启动守卫

[cmd/erpphase1](../cmd/erpphase1/main.go)默认只读：

```powershell
go run ./cmd/erpphase1 -config configs/multitenant.local.yml
```

只有显式-apply才写入，还必须：

1. DSN为tcp、环回地址、数据库名严格ruoyi_go_mt；拒绝原库、远程库和恢复库。repository另行复核本地连接；专用恢复命名空间只供显式演练/测试内部调用。
2. 确认完整租户及库存底座已存在。
3. 已停止写流量和任务；工具本身不停止服务，也不能证明外部写入已停止。
4. 提供24小时内的原生备份/恢复报告：源库正确、新恢复副本、全表结构/摘要、附件和源稳定性检查通过，实际database.sql大小及SHA256匹配。
5. 提供-confirm-database ruoyi_go_mt。

```powershell
# 先完成本次备份和独立恢复验证，填入新报告；不是直接复用历史报告。
$phase1BackupReport = '本次已验证备份report.json的绝对路径'
go run ./cmd/erpphase1 -config configs/multitenant.local.yml -apply -confirm-database ruoyi_go_mt -backup-report $phase1BackupReport
```

报告是防误操作的证据门槛，不是不可伪造签名，也不能替代停写及迁移前后数据对照。
本机备份演练辅助程序位于忽略目录，不是可移植正式备份产品；其他环境须先准备审查过的备份/恢复操作，不解除CLI限制绕过。

迁移先检查全部已存在对象，再只CREATE缺失的白名单表；同名异构不ALTER、不覆盖、不自动修复。
同连接GET_LOCK串行执行；DDL失败用独立清洁会话释放锁，保留部分结构。重跑先复核已存在对象，完成后才写唯一版本记录；重复完成运行不推进版本或时间、不重复插入记录。
MySQL DDL隐式提交，失败不宣称ROLLBACK撤销了建表。没有自动DROP/清表或降级脚本；正式回退必须按版本兼容及恢复方案另行审查。

启动在原有租户/库存检查后调用VerifyERPPhase1Schema，仅查询元数据和迁移记录：

- 13张一期表都不存在：允许纯底座/库存环境启动，不自动安装。
- 部分存在、缺版本记录或DDL指纹不符：拒绝启动。
- 完整存在：核对列顺序/类型/可空性/默认值/字符序/自增、索引列序/唯一性/前缀、外键引用库/列序/限制规则及InnoDB表字符序。
- 有意保留兼容的MySQL整数显示宽度差异；不兼容unsigned或不同精度。

SQL资源CRLF归一后计算SHA256；已应用的003不得就地修改，后续结构演进使用新迁移版本。不能以注释调整为由静默改掉已登记指纹。

## 3. 私有导入文件的配额与留存门槛

步骤2只增加元数据，没有上传入口或物理文件。
后续步骤5必须同时接入：

- 专用私有目录、随机file_key、SHA256、file_bytes，不使用公开/profile读取；下载需任务+店铺权限。
- PRESENT与DELETE_PENDING仍计入额度，只有实际文件已删除才可转REMOVED释放额度；清理失败不能先减用量。
- 企业总用量为登记的sys_file容量加未移除导入文件容量；现有普通上传/头像、导入创建、用量展示和配额调整必须一起改，不能只在导入处计数。
- 固定企业行→配额行顺序，在同一事务复核容量并登记；记录删除/留存也遵循一致锁顺序。
- 默认保留7天，失败/未完成任务也有界保留；物理写入与DB登记失败的清理、孤立文件巡检及磁盘上限需要专门测试。
- 原始文件不进入API日志、通用附件列表或Git；错误摘要不保存整行收件信息。

目前配额代码仍统计原sys_file，因新表为空且导入未开放，不引入实际额度遗漏；**上述配额接线和隐私检查未通过前，不得开放导入功能**。

## 4. 本轮真实执行与证据（2026-10-21）

### 备份与恢复

最终使用`logs/backup-restore/20261021_075719_6270da65/`：原生单事务dump恢复至
`ruoyi_go_mt_restore_20261021_075719_6270da65`，32张表、42,902行、38个登记/物理附件一致。
源库在备份期间稳定；没有账号登录核验、改密码、启动应用/Redis、覆盖源库或附件。
报告report.json、database.sql和uploads仅保存在本地忽略目录，不提交Git。

### 恢复副本演练

phase1-trial.json / phase1-trial.log全部通过：

- 未安装时可跳过只读一期检查；第三张表故障注入后保留2张表，缺记录/部分结构拒绝启动。
- DDL失败后独立连接可获取同一迁移锁；重跑补齐，重复完成不改迁移记录。
- 跨企业店铺/员工/SKU/事件操作者/导入操作者，以及同企业错店映射、错店订单行、错单包裹行被FK拒绝。
- 重复来源订单、同订单第二个包裹被唯一约束拒绝；00001与12.3400原样保存。
- 字段、索引、外键漂移均被只读校验和升级工具拒绝；故意漂移只在该恢复库发生并恢复。
- 并行升级被连接锁拒绝；测试数据回滚，旧恢复表未变，源32张表未变。
- 一期只读校验实测39ms。EXPLAIN记录可用索引，但空表选择租户/主键索引，不据此宣称大表性能已验证。

版本化测试`TestERPPhase1RestoreIntegration`在上述恢复库显式运行通过，覆盖幂等、真实FK/前导零、漂移拒绝及修复后校验；日志phase1-versioned-test.log。
可复验命令（仅允许已核验恢复副本，会在其中写回滚夹具及短暂结构漂移）：

```powershell
$env:MT_TEST_ROOT = (Get-Location).Path
$env:ERP_PHASE1_RESTORE_DB = '本次已验证且已安装一期结构的ruoyi_go_mt_restore_库名'
go test ./internal/repository -run '^TestERPPhase1RestoreIntegration$' -count=1 -v
# 结束恢复原环境变量；不要设置成源开发库或生产库。
```

默认单测中该用例明确SKIP，不能用默认检查替代上述显式执行。

### 源开发库实际升级

source-apply.log及source-repeat-apply.log均返回13张表齐全、recorded=true，版本003_order_fulfillment。
DDL指纹：`ebc324c78ec32f39df3e0b36a4fee276bf86ff827dd1dc65f20fa880018db2eb`。
截至2026-10-21 04:05:28 America/New_York（08:05:28 UTC），source-after-verify.log / phase1-source-verify.json只读核对：

- 当前45张表；原32张表结构与42,902行摘要完全不变。
- 12张新业务表0行；技术迁移表1行。
- 38个原附件与备份一致；未修改账号、菜单、角色或授权。
- 租户+库存+一期全部启动结构函数在同一个5秒context内通过，实测252ms；不是实际启动服务或页面验收。

### 自动检查与保留的失败

默认完整检查通过：gofmt、cmd/internal/pkg单测、vet、前端23项测试和构建。
证据`test/results/erp-20261021T080133232Z-fddde3aa/`；未执行多租户业务接口包，也未做页面/性能测试。
专门单测覆盖CREATE白名单/审计/租户字段、FK索引覆盖、列漂移、本地连接双重守卫和备份证据缺失/过期/失败/篡改拒绝。

首轮失败保留于`logs/backup-restore/20261021_073635_0201752d/phase1-trial.json`：
DDL错误污染锁释放会话、元数据JOIN偏慢已修复；一个负例先撞重复键而没有验证目标FK，修正为唯一来源行后重跑。未删除失败证据，未改源库来迎合测试。

## 5. 剩余范围

当前仅完成步骤2。普通CRUD生成、权限/菜单、导入配额接线、订单/库存共享事务、旧入口旁路保护及真实页面都在步骤3—8，不作为本轮通过项。
前后端没有启动；demo凭据继续按用户选择延期。没有正式生产迁移/容量结论，不自动提交或推送Git。
