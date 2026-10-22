# ERP 第一期步骤1：基线核对记录

> 本页保留步骤1当时快照。后续用户另行授权的步骤2已完成本地结构升级，现状见[ERP_PHASE1_SCHEMA.md](./ERP_PHASE1_SCHEMA.md)；不能把下文“不改库”外推到后续轮次。

时间：2026-10-21，America/New_York。范围仅步骤1：核对现有代码、保存源码基线、运行默认开发检查、整理一期设计。
没有实施步骤2—8，没有新增业务接口/页面或表，没有修改本地/原单租户数据库、密码、企业权限或会话，没有启停服务。

## 1. Git与可恢复基线

- 仓库：`D:/WorkSpace/go-space/ruoyi-go`；分支`erp`；HEAD为`c506bc2 docs: record ERP permission and failure UI acceptance`。
- 核对时有10个已跟踪文件改动、6个未跟踪文件，均为上一轮基础文档/验证脚本；业务Go、SQL和web源码没有未提交改动。
- 未执行reset/checkout/stash，没有自动提交或推送，也没有混入当前设计后假称已有干净提交。
- 已保存`logs/erp-phase1-step1-20261021T071216136Z-f7f9db6f/`：`tracked.patch`、16个源文件副本、`manifest.json`及`runtime.json`。
- 副本逐个SHA256与源文件一致；在本轮文档写回前再次核对16项均未变化；补丁对当时工作区的逆向只读检查通过。
- 这是**源码工作区快照，不是Git提交、数据库备份或完整运行环境备份**。已有HEAD由Git保存，快照补齐未提交文件；私有配置、账号记录、数据库、附件未复制。
- 如需恢复，先在独立临时目录核对HEAD/manifest/文件，再选择性恢复；本轮没有向现有工作区执行恢复或覆盖其他用户改动。

## 2. 本地运行环境核对

| 项目 | 本轮观察 |
|---|---|
| Go | go1.25.5 windows/amd64 |
| Node | v24.19.0 |
| 独立配置元数据 | 127.0.0.1:3306 / ruoyi_go_mt，Redis DB13；只读取非秘密字段，不输出DSN/凭据 |
| 命名空间 | 代码使用ruoyi_mt:，初始化上传目录为uploadPath-mt；不代表已连接并核验运行中服务的实际环境 |
| 后端8081 / 前端5173 | HTTP探测均不可用，本轮没有启动、停止或重启 |
| MySQL/Redis实际数据状态 | 本轮未建立业务连接核验；不把配置存在当作连接或数据正常 |

上轮“前后端已运行”是当时快照，不能当作本轮仍运行。服务不可用不阻止步骤1的单元/构建检查，但不能声称当前页面、登录或真实接口回归已通过。
原demo凭据核验继续按用户选择延期；本轮未读取密码内容、发起登录或重置现有密码。

## 3. 默认检查实际结果

2026-10-21 03:12:21—03:13:52 America/New_York（07:12:21—07:13:52 UTC），运行：

```powershell
.\scripts\verify-erp.ps1 -GoPath 'C:\Program Files\Go\bin\go.exe' -NodePath 'C:\Users\FF\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe'
```

| 步骤 | 实际结果 |
|---|---|
| gofmt只读检查 | 通过 |
| cmd/internal/pkg单元测试 | 通过，包含middleware/repository/service；不是运行中服务的授权/库存接口验收 |
| go vet（含多租户测试源码） | 通过 |
| 前端测试 | 23项通过，0失败、0跳过 |
| Vite生产构建 | 通过，仅更新本地忽略的web/dist |

证据：`test/results/erp-20261021T071221259Z-a18c096a/summary.json`及同目录日志。
摘要为`CHECKS_PASSED`、`integration=NOT_REQUESTED`。没有运行旧单租户test包，也没有运行会写入专用企业的多租户集成包。
没有重新测生产容量、浏览器操作或备份恢复；历史真实库存/权限/页面证据仍见[ERP_TEST_REPORT.md](./ERP_TEST_REPORT.md)相应日期，不能改写成今天重新通过。

## 4. 步骤1设计产物

[ERP_PHASE1_DESIGN.md](./ERP_PHASE1_DESIGN.md)确定：

- 一期只做自营备货、自有仓、自发货；手工/文件订单，不承诺真实渠道接入。
- 区分店铺商品、内部SKU、商业订单、包裹、库存凭据，复用现有库存核心。
- 给出逻辑实体、接口/权限、状态机、事务、导入去重/中断重试，以及隐私和配额要求；全部标记待实现。
- 明确订单托管库存单不能从旧接口旁路，必须在步骤6实现共享事务及关联单据守卫。
- 完整列出步骤2—8及验收门槛，不将设计当代码、只读结构检查当DDL迁移或默认检查当真实接口回归。

步骤1可以收尾；下一步是用户另行要求后，进入步骤2的DDL设计、备份与显式升级。当前服务不可用须在以后真实接口/页面验证前处理，不由本步擅自启动。
