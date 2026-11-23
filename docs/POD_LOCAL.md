# 本地运行 POD Studio

适用：Windows PowerShell，本项目目录 `D:\WorkSpace\CodeX\pod-studio`。
本轮只使用本地环境，不连接远程服务器。

就绪检查：`./scripts/pod.ps1 -Action doctor`，只读、不调用模型，报告中的 warning 必须看原因。
小范围试用的交付包、Linux 示例和运营步骤见 [POD_BETA_OPERATIONS.md](./POD_BETA_OPERATIONS.md)。

## 1. 当前环境与账号

| 项目 | 位置 |
|---|---|
| 创作端 | `http://127.0.0.1:5177/` |
| 管理端 | `http://127.0.0.1:5178/`，`web/`；使用 admin 或明确分配管理角色的账号 |
| 后端健康检查 | `http://127.0.0.1:18110/health` |
| 可选本机图片 AI | `http://127.0.0.1:18112/health`，当前已安装并启用 |
| MySQL | 独立新库 `pod_studio_dev` |
| Redis | POD 专用 127.0.0.1:6387，DB 15；原 6379 服务未改变 |
| 私有素材目录 | `.local/assets`，原图与缩略图；不通过 /profile 公开 |
| 本地配置 | `configs/pod.local.yml`，忽略提交 |
| 账号密码 | `.local/accounts.json`，忽略提交；文件 state 应为 `ready` |

用 `pod_creator` 登录创作端；`pod_creator_b` 用于两个普通账号的隔离测试。
`admin` 是专用库系统管理员，默认未开通个人空间；可显式开通自己的空间，不会获得其他人的项目访问权。
日常创作与隔离验收使用两个普通账号。
账号均使用随机密码，不能套用 RuoYi 默认密码；验证码以当前本地设置为准，本轮没有修改源库验证码开关。
查看密码可在本地编辑器打开账号文件，不要把它或配置文件上传到 Git、工单或聊天。

`web/` 已复用于 POD 管理端，保留原若依样式与全局 size。进入后台需 `podAdmin:access`，各功能仍独立校验权限。
2026-11-18 核对发现：源库 `admin` 的密码与 `.local/accounts.json` 记录不匹配，未重置源库密码；
请使用你当前维护的密码登录，或单独安排重置并同步本地记录。两个创作者的本地密码记录匹配，原有管理权限未改变。
登录只可记住账号；管理 Token 使用当前标签页的独立 sessionStorage，退出清理动态路由，不共用创作端会话。

管理页面位置：POD 管理 → 运营概览 → 平台概况 / 管理操作审计。概览读取真实记录，审计支持日期、操作者、结果筛选与详情。
模型管理已实现，三个 Tab 为模型目录、供应商与凭据、默认模型，使用和验收见 [模型目录报告](./POD_MODEL_CATALOG_REPORT_20261113.md)。
创作者管理、任务中心、官方模板、用量统计均已接入真实功能；官方模板本地测试条目在 2026-11-14 清理后为空，可按需要重新维护正式模板。
新增 `pod_admin`、`pod_operator` 两个角色，但不自动分配给任何已有创作者；pod_operator 默认仅有 access、overview、creator/task 元信息、usage 只读权限，不含审计或系统泛管理权限。
生产部署需分别构建 studio 和 web，再为 web 的 `/prod-api` 配置到本 Go API 的反向代理；本轮没有验证生产部署。
可选 `VITE_POD_STUDIO_URL` 指定无管理权限提示页的创作端链接；开发默认 5177，生产未配置则不显示本地地址。

### 项目与画布怎么用

1. 登录后在“项目”点击新建，输入名称，进入画布。
2. 左侧加号打开节点工具，也可用快捷按钮添加文本/图片/生图，或插入工作流预设；拖动标题移动，从输出圆点连到同类型输入圆点。
3. 文本节点直接输入，可点“AI 润色”预览后替换；生图节点直接选比例、数量并点“立即生成”。右上角设置按钮打开更多参数及连线表单。
4. 等待顶部显示“已保存”，刷新可继续。手动保存用 `Ctrl+S`，复制用 `Ctrl+D`，撤销用 `Ctrl+Z`。
5. 返回列表可重命名、移入回收站；切到回收站可恢复，内容保留。

当前可保存画布、上传私有素材，使用 managed 模型目录。累计估算预算 10 元、并发 1；
2026-11-18 源库保留文字默认 `qwen-flash`、图片默认 `qwen-image-2.1-pro`，静态启用可见为 6 个文字、2 个图片模型。
11-14 全图片停用/清空默认值属于历史状态，已由 11-15 的 Pro 单图真实样本更新；不代表供应商免费额度已实时同步。
本轮新增五家厂商的预设配置，未新增真实供应商调用。能力、后台配置、缺项及验收边界见 [多厂商报告](./POD_MULTI_PROVIDER_REPORT_20261118.md)。

### 多厂商模型怎么配置

1. 管理端 → POD 管理 → 模型管理 → 供应商与凭据：选择供应商，点“配置凭据”，填该厂商自己的 API Key。
2. 模型目录：固定编码是本系统内的唯一标识，3—64 位、以小写字母开头，允许小写字母、数字和半角 `_`、`-`、`.`，例如 `bbroot-gpt-image-2.6-sunburst`。请求上游用的是新版本里单独填写的「官方模型 ID」。维护能力/价格时，空白价格需要补齐，免费明确填 `0`；核对后启用供应商和模型、开启展示。
3. 创作者管理：显式把所需模型授予用户。设置默认模型不会自动授予其他人的模型权限。
4. 中转站：新建供应商时选“自定义 / OpenAI 兼容渠道”，填该站文档的 Base URL（如 `https://bian.bbroot.com/v1`）。同时有供应商和模型编辑权限的管理员可在「供应商与凭据 → 编辑」修正同一域名下的接口路径。保存后已有模型自动新增版本，保留价格、凭据、默认模型和授权；新任务用新版本，旧任务不换地址。换域名、协议、地域或计费主体请新建供应商。
5. 文字可选 Chat/Responses；图片可选 URL/Base64、参考图、张数、尺寸和质量。填渠道的真实价格，不能照抄官方价格。
6. 支持 `b64_json` 的渠道可在图片模型新版本选择「Base64 · 直接存本地」。图片校验后先落在服务器私有素材根目录的 `receipts/`，归档完成后原图与缩略图进入私有素材库；MySQL 只保留回执元数据和引用，不需要调大 `max_allowed_packet`。API 和 Worker 应使用同一私有素材根；完整备份同时包含未归档回执，不要手动清理待恢复文件。已提交的 URL 任务不改回执格式。

重复安装预设不会覆盖既有版本、密钥、默认值和用户授权。仅限本地专用开发库：

```powershell
. ./scripts/pod-env.ps1
go run ./cmd/podcatalog -mode plan
go run ./cmd/podcatalog -mode apply
```

### 火山方舟（豆包）

已支持 Seed 2.0 Mini LTS、Seed 2.1 Lite 文字和 Seedream 5.0 Flash 文生图、参考图生图。
Flash 每次输出 1 张、最多 3 张参考图；走 URL 回执与私有归档，不需要改 MySQL 单包大小。
当前不接入视频、图层拆分或按像素与输入图另收费的型号。

只导入方舟的停用条目：`go run ./cmd/podcatalog -mode apply -provider volcengine`（先加载上面的 PowerShell 环境）。
升级后重启 API 和 Worker。登录管理端 → **POD 管理 → 模型管理 → 供应商与凭据**，给「火山引擎 / 火山方舟」配置已开通所需模型的方舟 API Key；
在「模型目录」核对价格、启用并展示，在「创作者管理」授权给要用的人。不需要改现有默认模型，也不要把 Key 发到聊天。

2026-11-18 核对 [方舟官方价目](https://docs.volcengine.com/docs/ark/model-pricing?lang=zh&redirect=1)：
Flash 0.12 元/成功输出张，参考图不另收费；常规在线推理的 Mini LTS 是输入 0.20、输出 2.00 元/百万 Token（短输入档），
2.1 Lite 是输入 0.80、输出 2.70 元/百万 Token。预设不计缓存优惠，实际账单以方舟为准。
不添加 [2026-11-24 关停](https://docs.volcengine.com/docs/ark/model-deprecation-notice?lang=zh) 的 Seedream 5.0 Lite。

节点操作需确认本次付费，上游 AI 也可能重新计费。2026-11-13 实际登录的上传/制作/模板/批量链路已验证；
真实 3/4 张、多参考成功样本因千问免费额度耗尽且仅免费调用而阻塞，不能通过提高本站预算解决。
结果和未测项见 [交付报告](./POD_DELIVERY_REPORT_20261113.md)。七类配置、完整快捷键与接口见
[POD_STEP2_REPORT.md](./POD_STEP2_REPORT.md)。

保存失败会保留本机草稿；版本冲突时先下载草稿或另存项目，再决定是否放弃修改并重新加载。
旧草稿恢复不会自动覆盖服务端新版本。草稿按账号、空间和项目隔离在当前浏览器，未加密，不能代替备份。
换浏览器只能看到已提交到服务端的内容。

### 图片交互怎么用

1. 连接“图片输入 → 图片处理”，点处理节点“调整参数”，选择“裁剪”。从图上拖出选框，框内移动、四角改大小；需要重新选择时点“重新框选”。
2. “先旋转”之后再裁剪；下方数字为 0—1 的比例，起点加保留尺寸不能超过 1。方向键微调、Shift 加快，“恢复全图”清除裁剪区域。
3. 拖动过程中只预览，松手后自动保存；Escape 或取消拖动不会保存半成品。一次框选对应一次撤销，保存后重开能复现。
4. 执行完成，在图片结果旁点“对比”。可并排或单看前/后，棋盘表示透明；100%/200%/400% 按原图像素查看。处理前来自该结果的真实输入，修改画布不会换掉历史对比来源。
5. 商品预览点“调整印花布局”，可拖动，也可点水平/垂直/全部居中。“布局复位”恢复默认位置和大小，保留正反面与底色；像素提示是二维预览坐标。
6. 同一标签页刷新项目会恢复上次选择的运行，并继续查询未完成状态；参数面板显示来源版本与失败原因。参数修改后需要重新执行，恢复记录不会自动调用模型。

读取失败可重试；运行不存在或权限失效时清除旧记录，不保留可操作的旧结果。运行编号按账号/空间/项目保存在当前标签页，换浏览器或关闭标签页后不能据此恢复。
本轮验收与限制见 [图片交互报告](./POD_IMAGE_INTERACTION_REPORT_20261112.md)。

### 个人模板怎么用

1. 打开已有项目，在画布顶部点“保存为模板”；填写名称和说明，可新建模板，也可给已有模板保存新版本。提交前会先保存当前画布。
2. 返回后在顶部“模板”进入“我的模板”，或在画布左侧点“我的模板”。“内置预设”和自己的模板分开显示。
3. 点“使用模板”，选定版本，修改这次的文字或图片，点“创建项目并打开”。这是独立项目，不会自动执行或收费。
4. 在新画布继续调整，确认后再执行。以后保存模板新版本，不会改变之前创建的项目。
5. 模板列表可改名、改说明、复制、移入回收站和恢复；详情可复制指定的历史版本。删除和恢复都保留历史图片引用。

提交超时会显示“重试原提交/原创建”，模板页也可“重试原操作”；沿用原输入和请求编号核对结果，避免重复创建。
待确认记录只保存在当前浏览器标签页的 sessionStorage，不能跨浏览器恢复，核对完成前不要清理该记录或重复提交。
若被其他窗口更新，重新加载后再操作。模板仅本人可见，未开放共享与永久清理；详见 [个人模板报告](./POD_TEMPLATE_REPORT_20261112.md)。

### 批量怎么用

1. 工作流模板 → 我的模板 → 使用模板 → 批量制作，选择已单份跑通的固定版本。
2. 每份填写自己的文字/图片，可复制输入再改；每批最多 10 份，不自动交叉组合。
3. 点“核对份数与费用”，确认后提交。预览不占预算，实际执行仍可能因预算或配置变化失败。
4. 运行与用量 → 批量，查看每份实际状态、打开项目/运行、下载已归档图片 ZIP。
5. “停止待派发项”只停未派发部分；已派发去运行详情取消。已全部派发不等于全部成功。

每人最多 20、实例最多 200 个待派发项；成功节点图片分 print/mockup，含处理/预览中间结果，排除图片输入。
ZIP 最多 100 图、100 MiB，超限请在项目中分别下载。提交不明用“重试原批次提交”，不要另开一批重复做。
证据和限制见 [批量报告](./POD_BATCH_REPORT_20261113.md)。

图上限可在本地配置的 `pod` 下设置，修改后重启 API：

| 参数 | 默认值 | 范围 |
|---|---|---|
| `maxNodes` | 100 | 1–100 |
| `maxEdges` | 300 | 1–300 |
| `maxGraphBytes` | 1048576 | 1–8388608 字节 |

不填或填 0 使用默认值，负值或超过上限会拒绝启动。单节点配置另有 64 KiB 上限，HTTP 请求体上限独立生效。

### 素材怎么用

顶部“素材”可上传 PNG/JPEG/WebP、查看原图和下载原图；未被项目引用时可移入回收站并恢复。
图片输入节点的“选择或上传图片”可从图库选择，每个节点最多 8 张；确认后等待画布保存。
图库引用数包含回收站项目、历史运行、模板的全部历史版本和批次。仅项目引用可通过恢复项目、移除图片并保存来解除；历史运行、模板版本与批次引用继续保留。
删除只改变状态，不释放磁盘；没有自动清空回收站或按年龄删除文件的任务。

默认单图 10 MiB、2400 万像素、最长边 12000px。`pod.maxUploadBytes` 可调整至最高 20 MiB，
`pod.maxImagePixels` 最高 4000 万；0/不填使用默认。缩略图最长边 480px，下载保留原文件。
完整边界和测试限制见 [POD_STEP3_REPORT.md](./POD_STEP3_REPORT.md)。

### 图片处理和 T 恤怎么用

1. 进入项目画布，左侧“工作流预设”选择“已有印花做 T 恤”，为图片输入节点选择素材并等待保存。
2. 选中图片处理节点，在右侧设置裁剪、尺寸、去背景或 2/4 倍放大。纯色去背景适合干净底色；复杂背景可选“AI 抠图 · CPU 模型”。裁剪支持框选和数值比例。
3. 执行处理；选中商品预览节点，右侧可拖动印花，也可用方向键、滑条调整位置和大小，选择正反面及底色。
4. 等待设置保存，执行商品预览和对应导出节点。印花导出连接处理结果，商品展示导出连接 T 恤节点；节点结果及运行详情可预览、下载。
5. 印花优先选 PNG 保留透明，JPEG 会铺白底。填写目标印刷毫米尺寸查看有效 DPI，低于 150 会提示；印刷要求以生产商为准。

预设插入后不会自动执行。只有本地图片节点的流程不调用千问；含上游 AI 的分支重新执行仍可能计费。
二维 T 恤是位置和配色示意，不能代替实际布料效果或生产文件验收。处理结果保存为私有派生素材，原图保留。
使用与证据见 [图片制作报告](./POD_IMAGE_WORK_REPORT_20261112.md)。

## 2. 已初始化后的日常命令

Windows 下双击项目根目录的 [start.bat](../start.bat)。在本机 MySQL 已运行的情况下，脚本会启动 POD 专用 Redis、后端、两个前端和已配置的任务服务及图片 AI，等健康检查通过后显示地址。启动窗口可在就绪后关闭，后台服务会继续运行。只检查启动前置条件可运行 `start.bat -Check`。

用完双击 [stop.bat](../stop.bat)。它先停止本项目记录的服务，再关闭 POD 专用 Redis，本机 MySQL 继续运行。停止会中断正在执行的任务，建议等生成完成再关。

依赖 Go 1.25+、Node.js 22.12+、pnpm 11，以及已运行的本地 MySQL/Redis。
把这些工具加入当前终端 PATH；依赖版本以 `go.mod`、`studio/package.json`、`web/package.json` 和锁文件为准。

```powershell
Set-Location D:\WorkSpace\CodeX\pod-studio

# 启动或核对本机 POD 专用 Redis；重复启动仅核对记录进程，不会再开一份
.\scripts\pod-redis.ps1 -Action start

# 只读检查数据库版本、实例和 Redis 归属
.\scripts\pod.ps1 -Action check

# 首次安装前端依赖；已经安装过可跳过
Push-Location studio
pnpm install --frozen-lockfile
Pop-Location
Push-Location web
pnpm install --frozen-lockfile
Pop-Location

# 启动本项目 API、创作端、管理端、当前模式 Worker，以及已配置的本机图片 AI
.\scripts\pod.ps1 -Action start

# 运行完成后，只停止脚本记录的本项目进程
.\scripts\pod.ps1 -Action stop
```

启动前检查端口，不自动杀占用进程。进程记录在 `.local/processes.json`，停止时同时核对 PID、路径和启动时间，
防止误停 PID 被复用的其他程序。新进程的路径确认可读后才记录，停止时先验证全部记录再结束进程。
启动命令只报告进程已拉起，仍需打开健康检查和创作端确认就绪。
后端日志 `.local/server.out.log` / `.local/server.err.log`，前端日志 `.local/studio.out.log` / `.local/studio.err.log`，Worker 日志 `.local/worker.out.log` / `.local/worker.err.log`。
开发 stop 会强制结束核对过身份的进程；不是优雅停机，未完成调用由租约恢复。正常信号关闭的 Worker 最多排空 15 秒。

`scripts/pod-env.ps1` 仅调整当前 PowerShell 进程的 Go 缓存和临时目录至 `.local/`，
避免当前机器外部临时目录权限导致测试失败，不修改全局系统环境变量。

## 3. 新机器从零初始化

当前机器已完成此节，日常不要重复执行 init。初始化会导入独立系统基线与 001—011，生成账号，
将继承的普通示例账号停用、定时任务停用、自助注册关闭，不修改原业务库。

先检查本机 Redis 所选库是否被使用；工具会拒绝无归属标记的非空库和其他实例标记，不会 FLUSHDB。
DB 15 是当前机器的选择，不保证其他机器也为空；需要换库时显式传 `-RedisDB`。

```powershell
# 仅复用 configs/application.yml 中的本机回环连接资料，生成独立配置
.\scripts\pod.ps1 -Action prepare -RedisDB 15

# 检查生成的配置确认库名、端口与私有目录归属，再创建新库
.\scripts\pod.ps1 -Action init
.\scripts\pod.ps1 -Action check
```

prepare 拒绝覆盖现有本地配置；也可以根据 `configs/pod.example.yml` 手动填写独立配置。
JWT 必须替换为独立随机值，库名必须为 `pod_studio_*`。
init 拒绝任何已存在的目标数据库和已有账号文件，不作为重置或升级命令使用。
MySQL DDL 无事务回滚：中途失败会保留新库和 prepared 状态账号供排查，不能直接标为 ready 或反复覆盖。

本地初始化、备份和恢复检查工具只支持回环地址 MySQL；现有实例可读不等于有创建新库权限。
数据库连接用户与文件系统权限应按实际环境配置，本工具不自动创建数据库用户或修改 Windows ACL。

### 已有 POD 001 的本地升级

本机已完成 002/003/004/005/006/007/008/009/010，不需要重新 init。其他已有独立 POD 环境先停写并备份，再执行：

```powershell
.\scripts\pod.ps1 -Action stop
.\scripts\pod.ps1 -Action backup -MySQLDump 'C:\Program Files\MySQL\MySQL Server 5.7\bin\mysqldump.exe'
.\scripts\pod.ps1 -Action migrate
.\scripts\pod.ps1 -Action check
.\scripts\pod.ps1 -Action start
```

migrate 执行内嵌的 002 素材表、003 节点 ID 比较规则、004 执行与账本表、005 千问调用记录与归档映射、006 个人模板四表、007 批量三表、008 管理基础四表及菜单/角色、009 模型目录四表、010 创作者权益与账户七表，核对本地实例与版本；不会导入系统基线或覆盖账号。
已经 ready 的迁移可重复检查。部分 DDL 失败保留 installing，需排查后再处理，不能删除版本记录后直接重跑。

## 4. 测试

```powershell
.\scripts\pod.ps1 -Action test -TestConfig .local/image-work-integration.yml
```

先执行 stop 停止本项目 API/Worker，保留专用 Redis；脚本不会替你停 Worker。该命令运行 Go 单元测试、POD 专项集成、vet/build、studio 与 web 测试和构建。必须显式指定 simulation 测试配置，不能传 qwen 执行配置。
`.local/image-work-integration.yml` 是本机已准备的测试配置；新环境先从本机专用配置准备独立 simulation 副本，保持同一实例/专用库和私有素材根，不改生产或旧系统库。
集成套件检查 Worker 心跳，仍在消费时拒绝故障注入；不会自动停其他进程。
管理端测试自动发现 `web/test/pod-*.test.mjs`，新增 POD 用例按此命名，无须在脚本手工追加文件名。没有匹配文件时明确失败。
跨模块链路在 `managed_execution/platform_chain`：官方模板发布/复制、报价/幂等执行/结算、管理用量/审计、跨用户隔离及撤回模型范围。供应商使用离线替身，撤回后新报价按现有契约返回业务码 409，不代表真实供应商调用成功。
CPU 实体测试需另行启动已安装的本机图片服务，并在当前终端设置 `$env:POD_IMAGE_AI_TEST_URL='http://127.0.0.1:18112'` 后运行测试；不要为此启动 API/Worker。未设置时该用例跳过。队列崩溃测试在其他空间存在活动/待确认运行时跳过，不能为跑测试删除或改写这些记录。
POD 集成只读取显式本地配置；未配置 `POD_TEST_CONFIG` 时直接运行该包会跳过，跳过不能算验收通过。
专项测试会使用本项目两个创作账号，临时停用本人的成员/空间后恢复；
还会创建测试项目、素材与临时受限账号，按精确 ID 清理本轮资源、对应原图/缩略图及登录会话、验证码。
不对 ERP、门户或其他逻辑库做清理，不修改 demo/demo-b。

需要分开排查时：

```powershell
. .\scripts\pod-env.ps1
go test ./internal/... ./pkg/... ./cmd/... ./sql/pod
$env:POD_TEST_CONFIG = Join-Path $podProjectRoot '.local/image-work-integration.yml'
go test ./test/pod -count=1 -v
go vet ./...
go build ./...
Push-Location studio
pnpm test
pnpm build
Pop-Location
```

继承的根 `test/` 套件有旧数据库默认值，不在此项目使用无选择的 `go test ./...` 或旧 `scripts/test.ps1`。
不要为了跑过测试而把 POD 配置指向继承业务库。

## 5. 完整备份与独立恢复检查

升级前只备份 SQL 可继续使用 backup；需要配套私有图片和配置时用 bundle：

```powershell
.\scripts\pod.ps1 -Action stop
# 等 Worker 的 10 秒心跳过期；仍存在时工具会拒绝。保留专用 Redis 供核对归属。
.\scripts\pod.ps1 -Action backup -MySQLDump 'C:\Program Files\MySQL\MySQL Server 5.7\bin\mysqldump.exe'

# 配套 SQL、storage、配置快照、账号文件（存在时）和逐文件 SHA256 清单
.\scripts\pod.ps1 -Action bundle -MySQLDump 'C:\Program Files\MySQL\MySQL Server 5.7\bin\mysqldump.exe'

# 将路径替换为本次输出的可信本地 bundle；不要使用 .partial
.\scripts\pod.ps1 -Action restore-check -Bundle '.local/backups/bundle-20261113-144112-3719393923'
.\scripts\pod.ps1 -Action start
```

使用已安装且兼容的 mysqldump；不是下载安装命令。备份先验证 001 基线与实例归属，以允许升级前或新迁移故障时备份；
它不替代要求 001—011 全部 ready 的启动/check。只导出声明的专用库，
凭据放入临时 option 文件，不出现在进程参数中，退出时删除临时文件。
输出位于 `.local/backups/`，成功结束并关闭文件后才改为 `.sql`，未完成文件保留 `.partial`，不能用于恢复。

bundle 要求本实例 API 停止、Worker 心跳过期、Redis 归属正确。最多 10000 个素材文件、素材合计 2 GiB、单文件 256 MiB；
超过范围使用专门运维备份。停止后不要另起写进程；检查心跳不是数据库全局写锁。
输出先保留 `.partial`，所有文件复制和 SHA256 清单完成后才改为正式目录。

restore-check 只接受**当前实例的可信本地 bundle**：先校验路径白名单、重复、长度和 SHA256，
再创建唯一的新 `pod_studio_restore_*` 库及独立素材目录，按 bundle 核对其原 001—007、001—008、001—009、001—010 或 001—011 schema 和每个 asset_file 的 SHA256。
SQL 上限 32 MiB，失败保留副本，不删库、不覆盖源数据。它不会连接 Redis、启动 Worker、切换配置或生成可直接误启的副本配置。
因此这只验证本地数据与素材可恢复，不等于生产切换或恢复后的全应用启动演练。

创作版交付时的历史备份 `bundle-20261113-010008-176666460` 已实际恢复，
001—007、60 个素材文件校验通过，项目 11、运行 20、引用 44、批次 1；详情见 [交付报告](./POD_DELIVERY_REPORT_20261113.md)。
原库、原素材和恢复副本均保留。SQL 单文件不包含图片或 Redis，不能代替完整备份。

008 管理基础升级前，可用可信的 007 bundle 演练升级。以下命令只在新恢复副本安装 008，不升级源库，也不连接队列：

```powershell
.\scripts\pod.ps1 -Action restore-check -Bundle '.local/backups/bundle-20261113-111345-399455592' -UpgradeAdmin
```

普通 `restore-check` 保留 bundle 的原 schema，不自动升级；`-UpgradeAdmin` 仅用于恢复检查，不用于启动或原库迁移。
2026-11-13 的 007→008 副本演练及升级后 008 完整 bundle 恢复均已通过，详细路径、计数和边界见 [管理基础报告](./POD_ADMIN_FOUNDATION_REPORT_20261113.md)。

备份含密码哈希、随机账号密码、连接资料、JWT，当前配置内的模型密钥也可能在快照中；
忽略提交和 SHA256 不等于加密或外部真实性证明。仅在受限位置保管，不能上传 Git 或发送聊天。
环境变量中的秘密、模型目录主密钥文件、Python 依赖/权重不包含在此包，另行安全保管和安装。


### 模型目录主密钥和 009 历史恢复

当前 schema 为 001—012。本机独立 keyring 在 `.local/keys/catalog-keyring.json`，私有配置只引用绝对路径，不回显内容。
`pod.catalog.keyringFile` 为空时目录只读；`credentialEnvs` 是允许引用的环境变量名清单，不是变量值。
主密钥结构、轮换及使用步骤见 [模型目录报告](./POD_MODEL_CATALOG_REPORT_20261113.md)，不要把 keyring、配置或账号文件提交 Git。

```powershell
# 新的 008→009 副本演练，不改变源库
.\scripts\pod.ps1 -Action restore-check -Bundle '.local/backups/bundle-20261113-140036-2365173308' -UpgradeCatalog
# 历史 009 完整 bundle 恢复，不接队列、不启动副本 Worker
.\scripts\pod.ps1 -Action restore-check -Bundle '.local/backups/bundle-20261113-144112-3719393923'
```

以上两项本机均已通过；旧 007/008 报告与 bundle 保留作历史证据，不代表当前 schema。
009 恢复还校验历史模型摘要及解密历史凭据；环境变量的实际值是外部依赖，不宣称已恢复。
主密钥与 fingerprintKey 必须单独受限备份，丢失后不能仅靠 SQL 找回凭据或原幂等指纹。

## 6. 测试执行与独立 Redis

此段说明 simulation 测试模式；日常配置目前为 managed，旧 qwen 历史见下一节。simulation 默认全局并发 2、用户 1、测试供应商 2；等待运行上限全局 100、用户 10。
首次执行一次性发放 1000 测试单位；测试文本每成功节点消耗 1，输入节点 0。这里只是测试账本，不是付费 AI。
画布顶部「测试执行」可选择场景、整条/单节点执行；顶部「运行」查看历史与额度，详见 [第四阶段报告](./POD_STEP4_REPORT.md)。

Asynq 至少需要 Redis 4。当前机器原 Redis 3 不兼容，因此仅 POD 改用便携 Redis 5.0.14.1（开发用），位于 `.local/tools/redis5/`。
独立配置和持久化在 `.local/redis5/`；绑定回环 6387、独立随机密码、DB 15、AOF everysec、128 MiB/noeviction。原 6379 及旧缓存未清理。
压缩包来源和 SHA256 见第四阶段报告；不把可执行文件或私密配置提交 Git。新机器需要自行准备兼容实例，prepare 不会下载安装 Redis。

```powershell
# 本机便携实例的启动；已运行时核对记录的进程身份
.\scripts\pod-redis.ps1 -Action start

# 配置切到新的专用空 Redis 时，先核对 001—011，再原子认领空库
# 其他实例标记或无标记非空库一律拒绝；不导入、不清理旧缓存
go run ./cmd/podsetup -mode claim-cache -config configs/pod.local.yml

# 使用完毕：先停 API/Worker，等待心跳过期，再正常保存/停止专用 Redis
.\scripts\pod.ps1 -Action stop
.\scripts\pod-redis.ps1 -Action stop
```

Redis 停止脚本核对 PID、可执行路径和启动时间，要求 API/Worker 已停止；Go 工具另核对本地 6387、实例标记和其他逻辑库，发送 SHUTDOWN SAVE 后由脚本确认进程退出。
Windows 便携 Redis 关闭连接可能表现为网络重置，不以重试命令或“端口没响应”替代进程身份确认。不强杀 Redis，也不操作 Windows 共享 Redis 服务。
独立运行 Worker：`go run ./cmd/worker -config configs/pod.local.yml`。自动启动由 pod.ps1 根据本地显式 simulation/qwen/managed 配置执行。
生产选择维护中的独立 Redis，并补 Linux 验收；本地 Windows 验证不等于生产交付。

## 7. 千问真实模式

### 查看本人用量

在创作端左侧进入“运行与用量”，切换“我的用量”。画布执行面板也有同一入口。
顶部显示本人累计估算费用、预占、模型调用和已确认生图数；明细点“查看运行”能看失败原因与当前结果。
画布内只打开当前项目运行，其他项目从左侧运行主页面查看。用量汇总始终是本人全部项目，不是当前页或当前画布。
执行前列出上游 AI、图片数和逐项预占；已确认提交网络中断时沿用“重试原提交”，不把新草稿混入旧请求。
本地图片处理没有模型费；但节点按钮执行所需上游，含 AI 的上游也会重新计费。
“测试额度”是模拟单位，不能抵扣人民币。实例累计预算不是个人余额；预算不足请看用量并联系部署者核对，不能删记录释放预算。
归档失败会恢复原结果下载；未知调用仍保留预占。目前没有人工改费用的入口。
统计口径及未验收项见 [用量报告](./POD_USAGE_REPORT_20261112.md)。

### 当前配置

2026-11-14：用户授权累计预算改为 `budgetFen: 1000`（10 元），API/Worker 都使用同一配置。
千问 Key 已加密保存到模型目录，`qwen-flash` 为默认文字、`qwen-plus` 可选；两者本次真实 managed 调用通过。
四个图片型号已保存能力/价格版本，但当前 Key 的图片免费额度耗尽，暂时停用；补齐百炼侧额度后再验证启用，不自动开通付费或切换模型重试。
另建 DeepSeek、智谱 GLM、硅基流动、OpenAI 停用供应商和模型草稿，缺各自 Key、实际可用型号和已核实成本时不能启用。
后台入口为「POD 管理 → 模型管理 → 供应商与凭据 / 模型目录 / 默认模型」。补凭据和启用步骤见
[本轮维护报告](./POD_MODEL_MAINTENANCE_REPORT_20261114.md)。不把 Key 发到聊天、前端或 Git。

下面保留 2026-11-12—13 的历史情况，不能作为当前目录和预算说明：

以下为切换前 qwen 的历史配置。第 5 步后本地改为 managed，累计估算预算仍 5 元、并发仍 1；旧 Key 留在原忽略配置，不导入目录。
2026-11-12 按用户追加试用要求从 100 分改为 500 分；保留历史用量和预占，不充值百炼账户、不自动发起生成。
短文字、节点内润色、单参考图、五种比例和两张输出已有真实样本；供应商账单及未覆盖组合仍待验收。
2026-11-13 的 3/4 图、多参考尝试被 HTTP 403 / AllocationQuota.FreeTierOnly 拒绝：免费额度耗尽且仅免费调用。
本地累计预算仍 5 元，累计估算 1.000385 元，本轮新增 0；没有开通供应商付费或自动重试。
模型选择、环境变量备选来源、付费确认、故障处理及小样本步骤见 [POD_QWEN.md](./POD_QWEN.md)。
切换模式必须重启 API 和 Worker；不要修改已提交运行的模式或清空调用记录来重置预算。

## 8. 可选本机 CPU 图片 AI

完整安装步骤见 [scripts/image-ai/README.md](../scripts/image-ai/README.md)。依赖和模型位于忽略提交的
`.local/image-ai-venv/`、`.local/image-ai-models/`，需 Python 3.10；不会打进 Go 可执行文件。
安装后在本地 `pod.execution` 下设置 `imageAIURL: http://127.0.0.1:18112`，重启全部本地进程。
`pod.ps1 -Action start` 会先启动模型服务、等待就绪，再启动 API/Worker/前端，stop 一并停止记录的进程。
清空 URL 可关闭 AI 选项，普通图片处理仍可使用；配置存在但服务不可用时任务会明确失败，不改用普通插值冒充 AI。

```powershell
Invoke-RestMethod http://127.0.0.1:18112/health
& .local/image-ai-venv/Scripts/python.exe scripts/image-ai/smoke.py --output .local/image-ai-smoke
```

仅回环访问、并发 1，不调用千问、不自动下载权重。CPU 首次加载可能较慢，复杂边缘和大图需另验质量、耗时与内存。
日志为 `.local/image-ai.out.log`、`.local/image-ai.err.log`。部署到新机器需配套安装环境、权重和许可证；当前没有 Linux 生产验收结论。

## 9. 构建产物与交付组成

`pod.ps1 -Action start` 会编译 `.local/bin/pod-server.exe` 和 `pod-worker.exe`；
`studio/` 中运行 `pnpm build` 产出 `studio/dist/`。前端不是服务端模板，生产需静态服务器与同源反向代理，
开发 Vite 的 `/api` 代理配置不会自动带入静态包。
运行还需要独立 MySQL/Redis、受限配置与素材目录；启用 CPU AI 时另带 Python 环境、权重及许可证。
这些是当前交付组成，不是无需依赖的单文件 EXE，也没有完成 Linux/TLS/反向代理部署验收。

## 创作者权益（第 4 步）

管理端：POD 管理 → 创作者管理。新用户开通后点数为零、模型范围为空，分别在模型范围和调整点数中配置。
创作端：顶部“资格与点数”看本人有效期、限额和正式流水。managed 正式消费已接通；不要把这里的点数当旧测试额度或人民币余额。

```powershell
# 009 历史 bundle → 新建 010 副本（不改源库）
.\scripts\pod.ps1 -Action restore-check -Bundle '<可信009备份目录>' -UpgradeCreators
# 历史 010 bundle 恢复后额外校验点数恒等式/流水累计和存储账户
.\scripts\pod.ps1 -Action restore-check -Bundle '<可信010备份目录>'
```

初始账号名 3—20 位 ASCII，密码 8—20 字；页面不保存密码。结果不明时先“核对原操作”，重填原内容时沿用原编号，不另建请求。
存储未知写入/提交保留预留，需要精确核对；当前没有按超时自动释放或永久清理入口。

## managed 正式执行（第 5 步，当前本地模式）

本地已安装 001—011 并显式设置 `pod.execution.mode=managed`；API/Worker 同步启动。
`qwen.allowPaidCalls/budgetFen` 保留为全实例总成本闸（当前 500 分），不会每加一个模型再获得一份预算。
managed 只读模型目录的凭据版本，不读取旧 YAML Key；没有自动导入真实 Key、扩大预算或测试付费生成。

1. 后台“模型管理”配置供应商凭据、启用模型和完整能力/成本/点数版本，按需选默认模型。
2. “创作者管理”明确授权对应模型、发放点数；旧用户不会自动获得这些权限或把测试余额换成点数。
3. 创作端文本节点选润色模型，文字/生图节点选生成模型，保存后执行。
4. 报价弹窗显示最多预占点数、节点/模型版本，勾选确认后才提交。有效期 10 分钟，改输入需重报价。
5. “运行与用量 → 正式点数”或“资格与点数”看账户/流水；运行/批次详情看本次预占、已用、释放。

本地浏览器账号尚无真实目录模型授权，因此会显示“暂无已授权的可用模型”；这是正确的拒绝，不代表已经配置可付费使用。
用于自动化的假供应商/账号只限本地替身，验收后模型停用、账号暂停并保留账本；未知调用保留预占作为恢复证据。
它们的成本数值是替身，不是新增供应商账单；本机总闸仍统计持久调用记录，不得删记录重置预算。

```powershell
# 010 历史可信备份 → 新 011 副本，不改变源库或队列
.\scripts\pod.ps1 -Action restore-check -Bundle '<可信010备份目录>' -UpgradeManaged
# 当前 011 完整恢复，校验报价、绑定、点数账户/流水、未知预占和凭据关联
.\scripts\pod.ps1 -Action restore-check -Bundle '<可信011备份目录>'
```

先停止 API/Worker，等待 10 秒心跳自然过期；同时停止集成测试等一切写库工具，再生成 bundle。
不要在回归测试运行时备份。恢复报告须 `sourceUntouched=true`、`queueConnected=false`、`workerStarted=false`。
具体结果见 [执行报告](./POD_MANAGED_EXECUTION_REPORT_20261113.md)。

## 任务运营和官方模板（第 6、7 步）

本地 schema 现为 001—012，012 文件为 `sql/pod/migrations/012_official_templates.sql`，已安装后不可回写。
管理端任务/批次/用量/私有授权及官方模板入口已接通；创作端“工作流模板 → 官方模板”读取发布版本并复制到本人空间。
实际使用步骤见 [使用说明](./POD_USER_GUIDE.md)，证据与限制见 [任务报告](./POD_OPERATIONS_CATALOG_REPORT_20261113.md)。

平台素材物理根由 `pod.storageRoot/catalog` 派生，私人根仍为 `pod.storageRoot` 的平面文件键。
复制先预留存储并持久化计划，再写文件、最后创建项目；中断保留原计划与空间，重试原请求。
目标本地文件系统须支持硬链接；文件发布不覆盖已有目标，冲突或不支持时明确失败，不使用覆盖式回退。
备份覆盖两类素材、尚未完成复制的文件及数据库；恢复保留 `copying` 和 `held`，不因时间过去自动退款或执行模型。

```powershell
# 已停 API/Worker、停止集成测试并等 Worker 心跳自然过期后生成配套包
.\scripts\pod.ps1 -Action bundle -MySQLDump 'C:\Program Files\MySQL\MySQL Server 5.7\bin\mysqldump.exe'
# 当前 012 包恢复到全新的本地库和目录，不覆盖源库，不接 Redis
.\scripts\pod.ps1 -Action restore-check -Bundle '<可信012完整备份目录>'
# 011 历史包只在新副本演练到 012
.\scripts\pod.ps1 -Action restore-check -Bundle '<可信011完整备份目录>' -UpgradeOfficial
```

检查恢复报告的 `officialDataVerified`、`officialCounts`、`verifiedOfficialFiles` 和 `verifiedPendingCopyFiles`，
同时核对 `sourceUntouched=true`、`queueConnected=false`、`workerStarted=false`。
主密钥继续单独受限备份，不能用普通 bundle 替代。第 8 步本地全链路、CPU 与 012 恢复已通过，证据见 [本地收口报告](./POD_PLATFORM_CLOSEOUT_REPORT_20261113.md)；正式部署、真实供应商与账单等仍须单独验收。


## 配置与入门模板（2026-11-14）

管理端「POD 管理 → 模型管理」按以下顺序使用：

1. 在「供应商与凭据」选择厂商，保存对应厂商的 API Key；不要把千问 Key 填到其他厂商。
2. 在「模型目录」按供应商筛选，查看缺 Key、缺能力与定价版本、停用或未展示等提示。
3. 保存模型版本时，成本直接填人民币元：文字分别为每百万输入/输出 Token，图片为每张。
   例如每张 0.20 元填 `0.20`；最多六位小数。免费必须明确填 `0`，空白会报错。点数另填整数。
4. 核对能力、成本和点数后启用并授予用户模型范围。配置就绪只代表本地条件满足，不证明真实 Key 有效。

创作端模型下方显示型号、版本和点数单价；执行前确认弹窗显示服务端冻结报价、张数和最多预占点数。
取消确认不调用模型；默认模型变化不会替换已保存的明确选择。

三套正式入门模板位于「工作流模板 → 官方模板」：文字生成印花、参考图改图、印花处理与商品预览。
参考图和预览模板须先选择本人素材；复制后打开独立画布，自主选可用模型并确认运行。第三套基础去白底和二维预览不调用付费模型。
当前图片模型因供应商额度耗尽保持停用，前两套能复制编辑，待恢复图片额度并验证启用后才可生成。

新环境安装使用以下命令；本地开发库本轮已安装，不需重复。先停止本项目 API/Worker，等待心跳自然过期并做好 bundle。

```powershell
. .\scripts\pod-env.ps1
go run ./cmd/podsetup -mode seed-templates -config configs/pod.local.yml
```

命令核对专库、Redis 归属及离线状态，通过正常服务的审计/幂等路径创建并发布，不执行模型。
重复运行保留已有安装结果；管理员后来编辑或下架的模板不会被重新覆盖、发布。安装中断后若草稿被编辑，命令明确停止。

任务详情会显示处理建议：Key/权限/额度问题先找管理员；输入或型号错误回画布修改后重新报价；限流稍后确认失败再手动恢复。
调用结果待确认时只刷新原任务并核对账单，保留运行编号；归档待恢复只继续下载原结果，避免重复生图。
本轮代码与验收见 [易用性报告](./POD_CREATION_USABILITY_REPORT_20261114.md)。
