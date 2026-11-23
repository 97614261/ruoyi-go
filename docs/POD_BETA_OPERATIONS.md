# POD 小范围试用：部署与运营

本轮交付单机 API + Worker、两套静态页面。2026-11-15 已完成新增四个文字型号及 Pro 单图真实验收，
范围与价格见 [型号接入报告](./POD_MODEL_ADDITIONS_20261115.md)；额度和其他图片能力仍需分别核实。
本文件是部署步骤，不代表已在 Linux 实机通过。目标机还需验证 TLS、Nginx、systemd、磁盘硬链接和容量。

## 1. 本机先检查

```powershell
./scripts/pod.ps1 -Action doctor
```

或 `go run ./cmd/podsetup -mode doctor -config configs/pod.local.yml`。
输出 JSON：`failed` 退出 1，`warning` 退出 0；警告需要人工判断，不能仅看退出码就宣布全部就绪。
检查读取配置、专用库 001—012、Redis 归属和版本、Worker 心跳、目录、模型主密钥、有效模型/默认值和累计成本。
它不迁移、不认领缓存、不重置账号、不修复队列、不写素材、不调用供应商。
目录存在不代表可写、磁盘足够或支持硬链接；目录模型通过不代表供应商 Key/额度有效，也不保证每位用户均获模型权限。

当前配置累计预算为 **10 元**，不是供应商充值，也不是用户正式点数。供应商实际账单需要管理员核对。

## 2. 构建交付目录

```powershell
./scripts/pod-release.ps1 -Architecture amd64
```

产物在忽略提交的 `.local/releases/pod-linux-amd64-时间/`：

| 目录 | 内容 |
|---|---|
| bin | Linux API、Worker、podsetup；SQL 迁移由程序内嵌 |
| studio / admin | 两端编译后的静态文件，运行时无需 Node.js |
| deploy | 两个 systemd 单元、Nginx 双域名示例 |
| image-ai | 可选 CPU 服务代码、固定依赖和模型校验脚本，不含权重 |
| configs | 不含真实密钥的配置示例，必须配置后使用 |
| docs | 本文、使用说明、本地说明、千问配置说明 |
| manifest.json | 逐文件大小与 SHA256 |

此包不带本机配置、账号、密钥、用户素材、数据库或 CPU 权重，不会自动部署。
arm64 选择对应参数；跨平台构建通过不代表在目标 CPU 上运行通过。

## 3. Linux 同机布局

| 路径 | 放什么 |
|---|---|
| /opt/pod/releases/版本 | 交付目录，程序和静态文件只读 |
| /opt/pod/current | 指向选定版本的软链接 |
| /opt/pod/.local | 工具初始化账号/备份临时目录，只允许 pod 用户读取 |
| /etc/pod/pod.yml | 实际配置，仅 pod 用户可读 |
| /etc/pod/keyring.json | 独立模型主密钥，保留历史 keyId，与 JWT 不同 |
| /var/lib/pod/assets | 用户私有素材，含 catalog 平台素材子目录 |
| /var/lib/pod/public | 继承模块公共上传目录，与私有素材根互不包含 |

由部署者创建 `pod` 系统用户和这些目录，给目录及凭据合理权限。不要把私有目录放进 Nginx 根目录。
两端部署在不同域名根路径：`create.example.com` / `admin.example.com`；示例不能直接挪到子目录使用。
只对外开放 HTTPS/HTTP，MySQL、Redis、API 18110、可选 CPU 18112 监听回环地址。

### 全新实例

1. 从示例另存 `/etc/pod/pod.yml`。填新 `pod_studio_` 专用库、新 instanceId、新独立 Redis DB（非 0）、独立随机 JWT、Linux 绝对素材/公共目录；初始化先使用 `simulation`。
2. 配置本机 MySQL/Redis 连接；不要粘贴现有业务库的库名，也不要覆盖已安装 SQL。迁移/初始化账号需要 DDL 权限，运行账号权限由部署者按本项目查询/写入需求配置。
3. `cd /opt/pod` 后运行 `./current/bin/podsetup -mode init -config /etc/pod/pod.yml`。只创建不存在的库；已存在时拒绝覆盖。初始账号仅写 `.local/accounts.json`，不要放到站点中。
4. 配置独立 keyring：格式为 `activeKeyId`、`keys`（keyId→Base64 32 字节随机 AES key）、`fingerprintKey`（另一份 Base64 32 字节随机值）。使用操作系统密码学随机源，不能复用 JWT 或 AES 值。历史 keyId 和 fingerprintKey 不得随意更换。
5. 配置供应商后按下节启用 managed；`allowPaidCalls` 和累计预算是服务端门闩，不能用来替代供应商付费设置与授权。

### 已有实例迁移

不能把全新初始化当数据迁移。先完整备份并单独保存原 keyring；在新隔离环境验证恢复和历史凭据解密，核对素材 SHA 和引用后再制定切换。
当前 `restore-check` 仅做副本验收，不自动提升为运行实例、不复制 Redis、不切换域名。
不要让两个实例同时领取同一数据库的任务，也不要把 Windows 盘符直接放进 Linux 配置。

### 启动

1. 复制 `deploy/pod-*.service`，确认可执行权限和路径；本地 MySQL/Redis 应先启动。API/Worker 可独立重启，停止信号为 SIGTERM。
2. 若使用环境变量凭据，在两个服务中加载受限的本地 EnvironmentFile，并把变量名加入 `credentialEnvs`；不能只在登录 shell 中设置。
3. CPU 图片服务是可选项。源码位于 `scripts/image-ai`（交付包中为 `image-ai`）。启用时按固定依赖和 SHA256 安装权重，用独立服务在回环 18112 启动；未安装就把 `imageAIURL` 留空，不能用普通插值冒充 AI。README 的 Windows venv 路径在 Linux 上替换为 `bin/python`，Python/轮子兼容仍须目标机验证。
4. `podsetup -mode check` 通过后启动 API/Worker；运行 `doctor` 核对心跳和模型。API `/health` 必须 HTTP 200，依赖故障时会返回 503；它不代表 Worker 或供应商已就绪。
5. Nginx 示例放 `http {}` 中，替换域名和证书路径，确认创作端 `/api/`、管理端 `/prod-api/` 分别剥离前缀转 18110；设置服务端 `trustedProxies: [127.0.0.1/32]`，保持同源代理。
6. 在目标机执行 `nginx -t`、`systemd-analyze verify`，再加载服务和代理；配置仍为示例时不要对外开放。
7. 实际登录两端、复制模板、上传、报价、生成或本地处理、下载、刷新和撤权复查；CPU、权限及存储需目标机实测。

## 4. 管理员开通用户

1. **POD 管理 → 创作者管理**：创建创作者，设置有效期、并发、每日次数、存储限额和允许模型。
2. 在创作者详情分配正式点数，填写原因。超时先核对原操作，不换编号重复发点。测试额度不是正式点数。
3. 把创作端地址和账号私下交给用户；管理员入口不发给普通创作者。初始密码通过受限渠道交付。
4. 用新用户登录，查看“项目 → 第一件作品”的点数和可选模型；给 0 点或没授权图片模型时，页面会解释缺什么。
5. **任务中心**看进度，**用量统计**看调用与估算成本；私有图文必须获得单资源限时授权才可读取，不能只凭 root 随意查看。

## 5. 模型恢复与小额验收

原 Flash/Plus 及新增四个百炼文字型号已有真实样本，Pro 单图生成通过并为默认图片型号。
Turbo 为本次修改前已有启用配置，本轮未复测；2.0/2.0 Pro 和其他厂商缺 Key/定价验证的草稿保持停用。
图片翻译为未接入草稿，Omni 当前只接文字。截图免费额度不代表实时余额，详见 [本轮报告](./POD_MODEL_ADDITIONS_20261115.md)。

1. 在供应商控制台核实账户地域、模型访问、免费额度/计费状态；不把 Key 发到聊天里。不在本站自动开付费。
2. 后台 **供应商 → 配置检查**仅静态验凭据。**模型目录 → 版本**核对官方型号、能力、成本和点数；新价格建版本，旧快照不覆盖。
3. 按具体型号核实额度，不能从 Turbo 耗尽推断 Pro 也耗尽。仅在确认供应商可调用后启用、可见、设默认，并给验收用户授权；保留现有 10 元累计预算和账本。
4. 先预定最多 1 张文生图 + 1 张参考改图；每次显式核对报价、上游调用和成本，单次只生成 1 张。累计预算余量不足时停止；本轮不以“预算上限”授权耗尽整个余额。
5. 核对结果归档、本人下载、跨账号拒绝、模型费用与正式点数预占/结算，再人工核对供应商账单。
6. 失败显示额度/Key/输入原因时先解决原因，不反复重试；未知调用只核对原状态，归档失败只恢复归档，不重新生成。

真实样本通过后才对用户说明“图片已可用”。复制模板成功、doctor 通过、替身测试都不能代替这一步。

## 6. 完整备份与恢复演练

升级前留维护窗口，停止 API/Worker，等待 Worker 的 10 秒心跳过期。

```powershell
./scripts/pod.ps1 -Action stop
./scripts/pod.ps1 -Action bundle -MySQLDump 'mysqldump完整路径'
./scripts/pod.ps1 -Action restore-check -Bundle '刚生成的可信备份目录'
./scripts/pod.ps1 -Action start
```

Linux 对应在 `/opt/pod` 调用 `./current/bin/podsetup -mode bundle/restore-check -config /etc/pod/pod.yml`，分别传 `-mysqldump` / `-bundle`。
工具当前只允许回环 MySQL，SQL 自动副本恢复上限 32 MiB，单素材 256 MiB；超出需要部署者使用 MySQL 客户端和独立隔离的恢复流程，不能跳过完整性核验。

bundle 含 SQL、私人/平台素材及配置/初始账号（如存在）；**不包含独立 keyring，也不包含环境变量值**。必须另做受限、加密、异地的 keyring/凭据备份，旧 keyId 与 fingerprintKey 一起保存。没有主密钥，即使 SQL 恢复也无法解密模型凭据。
恢复时使用匹配的原 keyring。只接受自己可信的备份；恢复工具新建副本库和素材，校验 SHA、引用、版本、账户与凭据，不覆盖源库，不启动 Worker、不连接队列。
副本创建报告不等于线上回滚；真正切换仍需停旧实例、核对原 instanceId/缓存归属和任务意图，防止重复调用。

## 7. 对外试用前仍要验

供应商真实图片样本与账单、目标 Linux/TLS/静态代理/硬链接、按目标机器测容量，以及手机真机与印刷质量。CPU 占用、本机测试数量或二位用户的通过均不能外推服务用户上限。
先给少量受邀用户、有限点数/并发和人工开通；充值支付、团队空间、自动对账、HA 不在这次交付范围。
