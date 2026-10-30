当前状态（2026-10-29）：第1步本地已完成，独立库和登录已验收；下文未安装/未启动是工程接入阶段的历史快照。当前入口及证据以 [第1步验收](./PORTAL_STEP1_ACCEPTANCE.md) 为准。

# 门户工程第一步：接入与边界

2026-10-29。这里说明基础接入，不代表CMS或正式门户已经完成。

## 当前代码

- 默认配置不开启门户，原API继续注册；显式开启时追加首页、模板静态资源和`/admin/`后台。
- 首页目前显示“门户建设中”，不是正式首页布局；数据库内容、搜索、发布和模板切换尚未接入。
- `admin-web/`为参考RuoYi-Vue3的自有副本，保留MIT LICENSE，不含ERP源码和私有配置。
- 后台基础路径和Vue Router base均为`/admin/`，重新登录/退出跳转保持在该目录。
- 生产后台直接调用原根路径API，不另建prod-api转发；开发模式仍用dev-api代理到8085。
- 门户Token使用独立`Portal-Admin-Token`浏览器Cookie，不覆盖原后台Token。
- 门户模板与后台构建文件嵌入Go；资源路由白名单不公开源码、README、.env或模板文件。
- 后台只对扩展名为空的客户端导航返回index；缺失JS/CSS等返回404，不能用HTML掩盖资源错误。
- 静态资源有ETag；门户模板资源版本变更必须同步manifest版本和模板中的资源链接。
- 后台index和临时门户首页不缓存。原API不经过SPA兜底。

## 独立配置

模板在`configs/portal.example.yml`。后续实际启用时复制成已忽略的`configs/portal.local.yml`。
通过进程环境设置`MYSQL_DSN`和`JWT_SECRET`，需要时设置`REDIS_PASSWORD`，不要把凭证写入提交文件。

| 项目 | 开发约定 |
|---|---|
| Go端口 | 8085 |
| Vue开发端口 | 5175，入口`/admin/` |
| MySQL | 必须为独立`ruoyi_go_portal`，本轮未创建 |
| Redis | 候选专用DB14，实际启用前核实没有其他应用占用；本轮未连接或清理 |
| 原兼容上传 | 独立`uploadPath-portal-public`，仍使用原`/profile` URL |
| CMS私有素材 | 后续使用`uploadPath-portal-private`，不得放入原匿名profile存储 |
| 定时任务 | 门户模式默认不启动，可显式runJobs；需新库任务审核后再启用 |

当前门户启动前先检查资源是否完整，避免启动后才发现后台未构建。独立库未准备之前不要尝试用原库顶替。
原有数据库、账号、JWT密钥和ERP运行服务没有更改；不代表本轮已验证真实登录。

## 构建

在开发/构建机安装匹配工具链，首次在`admin-web/`执行`npm ci`，之后可在仓库根目录运行：

```powershell
.\scripts\build-portal.ps1 -TargetOS windows
.\scripts\build-portal.ps1 -TargetOS linux
```

已经构建后台时可加`-SkipAdmin`。服务器仅运行生成的Go二进制；配置、MySQL/Redis和附件持久化仍需准备。
Linux交叉构建不等于Linux实机验收。不能只复制二进制后丢弃附件目录或以为数据库被打包进程序。

## 独立库的安装前置（后续执行）

1. 明确单租户基线SQL的实际来源，核对Go主分支额外依赖的表和固定契约数据，不能直接复制ERP多租户库。
2. 核验备份，再显式新建门户专用库；导入经核对的基线，形成可重复的安装清单和结构版本。
3. CMS新表和菜单脚本放`sql/portal/`，启动只读校验，不自动建表/改表；新菜单不扩权既有账号。
4. 本地专用测试账号用于验收，实际密码不写进公开文档；旧用户、原库与ERP不参与修改。
5. 新库/Redis隔离就绪后才运行真实登录、菜单和上传测试，并补数据库/附件配套恢复演练。

本轮仅落实代码隔离与配置门槛。库安装、结构守卫、CMS权限及后台登录验收仍是后续工作，不能把示例配置当作已安装环境。

## 门户发布包的默认配置

用build-portal.ps1生成的门户包默认读取configs/portal.local.yml，并要求portal.enabled=true；
即使误传原configs/application.yml，也会在数据库初始化前拒绝。原server普通构建不改变默认路径。
示例文件不含可用数据库凭证，当前没有创建该本地配置或数据库。勿用普通server默认启动代替门户部署命令。

本地安装入口：go run ./cmd/portalinit（默认仅预览）；显式安装需-confirm和-admin-config，不允许覆盖已有库。原始单租户SQL副本和许可证在sql/portal；CMS后续迁移不能通过重放000脚本升级。
