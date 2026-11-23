# POD 第 3 步：供应商与多模型目录

日期：2026-11-13。本报告是本轮实现及验收的快照；稳定约束见 CLAUDE.md，原因见 DECISIONS.md。

## 结论与范围

新八步计划的第 3 步已完成本地实现和验收，schema 已由 001—008 升至 001—009。
管理员可配置供应商、只写凭据、多个文字/图片模型、不可变能力/成本/点数版本及默认模型。
支持多个供应商配置记录，不代表已接入多个厂商协议；当前只开放千问 qwen-v1、北京地域和服务端明确的模型白名单。

本步只配置目录，不调用供应商、不证明真实 Key 有效，不将原 qwen/simulation 任务转换为新模型计费。
managed 仍为 false，正式点数账户尚未实现，创作端模型选择与报价留给第 5 步；目录点数规则只是后续执行的配置。
本轮没有提高原 5 元累计估算预算，没有充值或增加并发，没有新增真实模型费用。

只操作本项目本地 pod_studio_dev、专用 Redis 6387/DB15 及独立恢复副本；未操作 ERP、门户、原 6379、远程或生产。
保留原未提交改动，没有自动提交或推送 Git。

## 页面位置与使用顺序

管理端：`http://127.0.0.1:5178/`，使用原本地 admin；密码仍在忽略提交的 `.local/accounts.json`。
普通用户继续使用 `http://127.0.0.1:5177/`，不会获得模型管理权限。

1. 进入 **POD 管理 → 模型管理 → 供应商与凭据**，新建供应商，填写固定编码、名称及官方根地址。
2. 点该行 **轮换凭据**，选择加密保存或部署白名单中的环境变量引用，填写操作原因。旧值不会回显；保存新增凭据版本。
3. 切到 **模型目录**，新建模型，选择供应商及文字/图片类型。创建后默认停用；展示标记仅作为后续创作端配置。
4. 点 **新版本**，选择官方模型，填写能力限制、超时、成本和点数规则及原因。保存新增版本并切换当前版本，不覆盖历史。
5. 点 **版本** 查看历史版本，展开行可看能力、成本、点数、官方地址和规格摘要。点 **检查配置** 仅做本地校验。
6. 配置完整后可依次启用供应商和模型；**默认模型** Tab 只接受有效版本、模型与供应商均已启用的同类型记录。

供应商创建后编码、协议、地域与 endpoint 固定，编辑只能改名称。更换地域、端点或计费主体需新建供应商。
模型创建后编码、供应商和类型固定；能力或费率变动用新版本，名称、展示与排序走元信息编辑。
当前无通用物理删除入口，停用不会删除旧版本或改写旧调用；默认指针也不会因停用自动换成另一模型。

页面沿用原若依 Element Plus 和全局 size。每次变更需要原因、requestId 及相应 revision。
提交结果不明时，先 **核对原操作**，必要时 **重填原操作**，沿用原编号、目标、revision 和内容，不能换编号重复提交。
浏览器仅保存安全元信息；Key 不会保存，重填凭据必须重新输入。存储无法持久化编号时阻止提交。

## 当前支持规格

| 项目 | 当前限制 |
|---|---|
| 协议和地域 | qwen-v1 / cn-beijing；官方 HTTPS 根域名，拒绝任意路径、端口、Header、请求模板与内网 URL |
| 文字模型 | qwen-flash、qwen-plus、qwen-turbo；本轮接口夹具覆盖 flash、plus 两种同类型模型 |
| 文字能力 | 输入 1—32000 字节，输出 1—2048 Token；不能混入图片能力字段 |
| 图片模型 | qwen-image-2.1-turbo |
| 图片能力 | 提示词 1—1200 字，输出 1—4 张，参考图 0—3 张；允许比例为五种现有比例的不重复非空子集 |
| 比例 | 1:1、3:4、4:3、9:16、16:9 |
| 超时 | 10—300 秒 |
| 成本规则 v1 | CNY / micros，1 元 = 1000000 micros；文字按每百万输入/输出 Token，图片按每张 |
| 点数规则 v1 | points 整数，文字按每百万 Token，图片按每张；独立于人民币和原测试单位 |
| 费率上限 | Token 单价 <= 1000000000000，图片单价 <= 1000000000；均为非负整数，0 是明确的零费率 |

ID、版本、revision、micros、点数使用十进制字符串，前端不转 Number 计算价格。
费率由管理员核对后填写，不抓取厂商价格、不把测试夹具的价格当成官方报价。
一份模型版本同时固定官方 ID、协议版本、供应商地域/endpoint、能力、成本、点数、超时和完整规格摘要。
未支持的阶梯、缓存/思考 Token、视频、任意请求透传不能只靠填 JSON 开放。

## 主密钥与凭据

示例配置见 `configs/pod.example.yml`。本机私有配置已引用独立文件 `.local/keys/catalog-keyring.json`；不要在聊天或日志打印内容。

```yaml
pod:
  catalog:
    keyringFile: "D:/WorkSpace/CodeX/pod-studio/.local/keys/catalog-keyring.json"
    credentialEnvs: [POD_CATALOG_TEST_KEY]
```

这是配置项位置示例，合并进现有 pod 块，保留文件原缩进；不能重复创建 pod 或改变原 execution/qwen 归属。
本机白名单是本地虚拟凭据测试用途，没有把原千问 Key 自动复制到目录。

keyring 为不超过 32 KiB 的普通 JSON 文件，字段为 activeKeyId、keys（keyId 到 Base64 密钥的映射）、fingerprintKey。
AES-256 密钥及 HMAC fingerprintKey 均为独立的 32 字节随机值，不能相同、不能复用 JWT；最多保留八个 AES keyId。
主密钥只在服务端读取，不存数据库、不返回前端。路径为空时允许读目录、拒绝管理写入；无效文件拒绝初始化，不退回明文。

凭据用随机 nonce 的 AES-256-GCM 加密，AAD 绑定供应商与凭据版本；复制到另一供应商/版本或篡改后不能解密。
env 来源只存变量名并验证部署白名单，DTO 不返回变量名或变量值。
响应与审计不返回 secret、密文、nonce、envName、keyId、HMAC 请求指纹或供应商原始响应。
真实 Key 由管理员在本地自行录入；本次 GUI 只检查空密码框，未进行真实凭据提交。

轮换供应商 Key 新增凭据版本；轮换加密主密钥时加入新 keyId、切 activeKeyId，保留历史 keyId 对应密钥和原 fingerprintKey。
不能直接替换 fingerprintKey，否则原未知操作的 HMAC 重放比较失效。当前没有批量重加密/迁移指纹工具。
主密钥文件不进入普通 bundle，必须单独受限备份。仅有 SQL 无法找回历史密文凭据；env 实际值也必须另行保管。

## 接口、安全与实现

handler → service → repository；只有 repository 操作数据库。17 个目录接口的具体路径和权限见 [接口清单](./POD_PLATFORM_API.md)。
CRUD 先使用既有代码生成器产生 provider/catalogmodel 骨架，记录保留在 `.local/generated/pod-catalog/`，再补专用 DTO、事务和页面；生成证据有定向测试。
新增 009 四表，008 及其之前已安装 SQL 未回写；迁移和初始化同步 schema checksum 与恢复工具。

- 入口要求 POD 管理权限，具体接口再检查 provider/model 对应权限；普通创作者不能直接调用。
- 写事务按执行门、平台、供应商、模型顺序锁定，并复核账号、角色、启用菜单及具体权限，拒绝使用已撤销权限的旧 LoginUser。
- actor+requestId 唯一，独立 HMAC 比较完整请求；同内容重放返回原结果，异内容或旧 revision 返回业务 409。
- 业务写、安全审计和 action 结果同事务；action 查询只限本人，并重新要求原操作权限。
- 审计使用有类型安全前后快照，支持目录动作与 failure 结果；读取快照失败时中止事务，不写假空快照。
- 服务校验拒绝写固定安全失败事件；坏 JSON、未知字段、Content-Type 或入口超限在解析阶段拒绝，不保证生成业务变更审计。
- 专用请求客户端不缓存正文、不打印完整错误；写入请求严格 JSON 且最多 128 KiB，不走通用 RepeatSubmit 正文缓存或 OperLog。
- 前端待确认元信息严格验证 op/path/method/ID/revision，不恢复任意路径或混入的 secret；关闭、提交和卸载清空凭据输入。

配置检查验证类型规格、完整摘要、供应商固定身份和当前凭据来源，并不要求为了检查而启用模型。
检查全程不发送供应商请求，不证明 Key 有效、模型已开通或实际生成质量合格。

## 已完成验证

完整回归先停本项目 API/Worker、保留专用 Redis，使用显式 simulation 测试配置；不运行历史默认 test/ 集成。

```powershell
.\scripts\pod.ps1 -Action stop
.\scripts\pod.ps1 -Action test -TestConfig .local/image-work-integration.yml
.\scripts\pod.ps1 -Action start
```

本轮完整脚本已通过，日志 `.local/catalog-full-checks.log`：Go 单元/专项集成、vet、build，创作端 15 文件 105 项测试及构建，管理端 8 项测试及生产构建。
之后补恢复完整性、审计快照、action 校验和测试夹具还原，重新通过：

```powershell
go test ./internal/config ./internal/service ./pkg/modelcatalog ./cmd/podsetup
# 先停 API/Worker，并由 pod-env.ps1 配好本地 Go 缓存；仅使用显式 simulation 测试配置
$env:POD_TEST_CONFIG = (Resolve-Path .local/image-work-integration.yml).Path
go test ./test/pod -run '^TestPODFoundation$/model_catalog' -count=1
go vet ./internal/... ./pkg/... ./cmd/... ./sql/pod
go build ./...
git diff --check
```

页面收尾后又跑管理端 8 项测试与生产构建，日志 `.local/catalog-final-web-build.log`；已有大 chunk 提示保留，不当成功能失败。
没有在最终文档变更后无理由重跑整套有副作用的集成测试。

专项覆盖包括：401/403、普通创作者拒绝、撤权后事务拒绝；加密/env 来源、轮换、缺主密钥只读；不可变版本、摘要篡改拒绝；
两个文字模型及一个图片模型各两个版本；类型不匹配默认值拒绝；幂等重放/异内容 409、并发 revision 竞争；action 仅本人；敏感字段不进入 DTO/审计/浏览器待确认记录。
测试安装 HTTP DefaultTransport tripwire，拒绝真实供应商网络调用。

真实浏览器使用原 admin 完成本次已授权的验证码登录，验证了：

| 页面项目 | 结果 |
|---|---|
| 模型目录 | 列表、第二页、切 Tab 回第一页、名称无结果及清除恢复通过 |
| 历史版本 | 图片模型 v2/v1 可读，展开显示能力、费率、官方地址和摘要 |
| 新版本表单 | 从当前图片版本预填能力与费率，通过；只查看后取消 |
| 供应商与凭据 | 安全状态元信息正常，轮换框不回显旧 Key；只查看后取消 |
| 默认模型 | 当前两类未设置、正式模式未启用；搜索停用测试模型无可选项，取消保留原值 |
| 检查配置 | 明确提示本地检查通过、未调用供应商、不证明 Key 有效 |
| 最终页面 | 热更新正常，模型目录保留给用户，截图 `.local/catalog-ui/model-directory.png` |

GUI 未录入真实 API Key，也未提交启用/默认值变更；写入、轮换和竞争由专用接口测试覆盖，不称作所有 GUI 写分支已验收。
收尾补了存储读取异常不导致目录页初始化失败、旧弹窗请求错误不覆盖新弹窗的防护；前端测试/构建通过，未专门模拟浏览器禁存或网络乱序。

## 备份与恢复证据

| 证据 | 本地位置 |
|---|---|
| 008 升级前完整 bundle | `.local/backups/bundle-20261113-140036-2365173308` |
| 008→009 独立副本报告 | `.local/pod_studio_restore_20261113_140349_596df081-3305825004/restore-report.json` |
| 当前有效 009 完整 bundle | `.local/backups/bundle-20261113-144112-3719393923` |
| 009 完整副本恢复报告 | `.local/pod_studio_restore_20261113_144258_1be12122-2418576973/restore-report.json` |

009 恢复快照：项目 11、运行 21、引用 46、批次 1、管理审计 409；供应商 8、凭据版本 24、模型 20、模型版本 40。
62 个素材文件 SHA256 校验通过，40 个历史模型版本完整规格校验通过，16 个加密凭据历史版本解密通过。
env 引用只校验白名单，报告 environmentCredentialValuesVerified=false。
报告 sourceUntouched=true、workerStarted=false、queueConnected=false；副本没有连接原 Redis 或启动 Worker。
这些数字属于备份当时快照，之后页面访问会新增审计，不作为当前库总数。

本轮曾发现两处验证问题并修复：

1. 篡改测试使用 GORM Model 更新时同时改了内存对象，导致还原摘要仍是零。改为独立保存原摘要，恢复后再次检查；只修复本轮 local-test 虚拟夹具并清理失败测试自己的临时账号。
2. 恢复报告连续 GORM Scan 复用计数结构会清零之前字段。改用目录局部结构再合并，重新做完整 bundle 和恢复，以本节最终报告为准。

早期失败副本与备份保留现场，不作为通过证据；临时修复测试已删除。
本轮 local-test 供应商/模型及虚拟历史凭据保留供界面/恢复验收，全部停用、隐藏；默认恢复为空，managed=false，临时权限账号按准确 ID 清理。

## 未验收边界与下一步

未验证真实 Key、多模型生成、真实费率/供应商账单、Linux/TLS/生产切换和手机真机；没有把目录替身测试当成真实供应商成功。
当前多厂商适配、正式点数、用户模型授权和创作端报价仍不能使用，不应对外开放正式收费模式。

下一步是第 4 步：用户资格/到期/停用、允许模型范围、正式点数账户与人工发放/扣减、每日提交及存储限制。
这些完成后，第 5 步才把 studio 模型选择、服务端报价、managed 执行及点数结算接起来。
