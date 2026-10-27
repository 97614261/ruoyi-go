# ERP 本地可靠性验收入口

本入口用于已安装多租户及 ERP 001—011 的本地开发环境。默认只预览；不会自动安装表、修改索引、重置已有密码、重启正式服务或连接远程。验收范围和本轮结果见[2026-10-27收口报告](./ERP_CLOSEOUT_2026-10-27.md)，此前30分钟实测保留在[可靠性报告](./ERP_RELIABILITY_REVIEW_2026-10-26.md)。

## 如何运行

在源码仓库根目录使用 PowerShell：

```powershell
# 默认、-Plan 都只显示计划，不读取私有配置或连接数据库。
./scripts/verify-erp-reliability.ps1
./scripts/verify-erp-reliability.ps1 -Plan -Profile standard

# 明确执行：会新建测试企业并写入合成业务数据。
./scripts/verify-erp-reliability.ps1 -Execute -Profile quick
./scripts/verify-erp-reliability.ps1 -Execute -Profile standard

# 无数据库的常规代码检查；现在也覆盖可靠性工具的格式/单元/vet。
./scripts/verify-erp.ps1
```

Go 未在 PATH 时传 `-GoPath 'C:\Program Files\Go\bin\go.exe'`。配置和平台账号记录默认使用被 Git 忽略的 `configs/multitenant.local.yml`、`logs/multitenant-accounts.local.json`，可分别通过 `-Config`、`-Accounts` 指定本地私有文件。工具核对平台密码与 bcrypt 记录，不匹配即失败，不重置密码；账号记录里历史控制字符只在内存兼容解析，不改原文件。

执行限制：MySQL 必须是回环地址、TCP、`ruoyi_go_mt`；Redis 必须是回环地址和 DB13。附件路径必须位于源码仓库内，URL 前缀为 `/profile`。须预先安装源码依赖、Go 和外置 MySQL/Redis；本入口不会下载安装软件。事务观察还需读取 `information_schema.innodb_trx` 的权限，缺失时报失败，不以跳过冒充通过。

当前执行采样依赖 Windows 原生进程累计时间；其他平台仅可编译检查或预览，不能据此声称 Linux 运行验收。没有 Linux 时可完成当前本地轮次，未来实际部署再验目标环境。

## 两种档位

| 档位 | 自然基线 | 混合业务调度 | 自然冷却 | 用途 |
|---|---:|---:|---:|---|
| quick | 15秒 | 2分钟 | 30秒 | 验证工具、完整链和故障流程是否能运行 |
| standard | 2分钟 | 30分钟 | 5分钟 | 持续业务、资源趋势与排空核对 |

档位时间不含编译、造数、六类故障、独立摘要/对账和子进程退出。停止调度后等待在途完整链结束，不把调度截止时的半条链排除在结果外。quick 不替代 standard 的资源趋势证据；两者均不直接换算生产并发、在线人数或2核2G容量。

## 测什么

- 每轮唯一目录 `logs/erp-verify-*`，包装脚本限制本机账户访问；只创建该轮三个新企业。保存起始企业水位及旧业务/授权/范围表摘要。不得复用、重新启用已结束企业。
- 真实 API 链：采购→确认→收货→应付/付款核销→订单→审核预占→手工运单→扫码复核/发货→应收/收款核销，并验证原请求重放。两个写入者与四个读取者，有固定停顿；这不是无停顿的吞吐极限测试。
- 订单审核和采购付款各覆盖提交前真实客户端超时、提交前终止自建子进程、证明已提交后重启。提交前先独立读取 INNODB_TRX，确认有未提交修改；普通连接仍只见原事实；中断后证明回滚，再原内容重试。
- 故障关卡仅链接 `test/erpreliability`；正式 `cmd/server` 不包含关卡，也没有故障/调试 HTTP 入口。子进程使用随机回环端口，不启动定时任务或导入 worker，保留正式路由、授权、事务和连接池设置。
- 独立 SQL 核对库存余额/流水、预占、单据数量以及应收应付/有效核销；HTTP 200 不能单独判为通过。自然 Go 堆、协程、数据库池指标与边界额外诊断 GC 分开记录；Windows CPU 使用累计时间差，只采集本轮测试服务和测试客户端，不把本机数据库总占用归因给某个接口。
- 正常结束撤回测试员工范围、注销并停用保留三个企业，再核对旧表摘要。业务/传输错误均计数，不剔除失败请求。

## 证据与失败处理

`run-complete.json`、`fault-results.json`、`facts-*.json`、`fixture-cleanup.json`、`old-data-verified.json` 共同组成通过证据；`native/` 和 `children/` 保留进程与运行时样本。令牌不写进夹具 JSON，私有日志及账号不得提交 Git 或装进运行包。

普通失败会尝试按本轮企业 ID、编码、所有者和起始水位清理，失败仍退出非零。强制结束整个控制器、断电等不保证其 defer 执行；这时保留目录，先核对 `fixtures.json` 中每个身份及数据库状态，再按平台正常入口撤回该轮范围、停用保留。不要重跑并重新启用旧企业，也不要通配删除 Redis 或按进程名结束正式服务。清理记录缺失不能算通过。

本入口不自动提交或推送 Git。当前交付仍须记录提交基线、工作区状态、源文件哈希与运行包逐文件清单；本地通过不等于正式环境已发布。
