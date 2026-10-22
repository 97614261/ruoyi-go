# 多租户开发底座

仅适用于 multi-tenant 和后续 erp 分支。main、原数据库及参考前端保持不变。
本分支使用独立开发库、缓存命名空间、上传目录，以及项目内 web/ 前端副本。
erp分支已具备手工库存示例及仓库范围；开发入口见[ERP_FOUNDATION.md](./ERP_FOUNDATION.md)。本页描述通用租户层，不等于其他分支自动具有ERP功能。

## 安全边界

- 租户身份来自已验证会话，客户数据缺少租户上下文必须失败。
- 平台身份不是业务数据免过滤开关；平台接口另行鉴权。
- 用户和角色 ID 全局唯一，关联必须同租户，租户字段没有业务默认值。
- 文件、缓存、任务、日志同样隔离；禁止按租户清理时执行 TRUNCATE。
- 初始化只能新建不存在的库，不覆盖原数据库；重复或孤立数据必须报告。

## 接口检查

租户列表分页、上限100、主键稳定排序；编码唯一索引；批量关联避免N+1。
数据库与Redis带context和超时；平台接口权限为platform:tenant:*。
错误走统一response，不输出密码/Token/DSN，JSON使用camelCase。

## 验收

两个租户同名账号独立；跨租户详情、修改、删除、授权、导出和附件访问失败。
验证伪造tenantId、缺失上下文、租户停用、客户调用平台接口、缓存串读及日志清理。
完成后补充实际验证结果与启动方法，不将计划写成已完成结果。

## 当前开发环境

- 后端：`http://localhost:8081`，默认仅监听 `127.0.0.1`；健康检查 `/health`。
- 前端：`http://localhost:5173`，源码在 `web/`，Vite 代理指向8081，不指向原8080服务。
- 新库：`ruoyi_go_mt`；Redis DB13，Key统一前缀 `ruoyi_mt:`；上传根目录 `uploadPath-mt/`。
- 企业编码 `platform` 为平台管理；`demo` / `demo-b` 为两个独立客户，账号均为 `admin`。
- 随机密码见本地 `logs/multitenant-accounts.local.json`，配置在 `configs/multitenant.local.yml`。
  两个文件都被 Git 忽略，禁止把内容贴入测试报告或提交仓库。

## 初始化与启动

### 平台超级管理员与租户管理入口

- 企业编码 `platform`，账号 `admin` 是平台超级管理员，沿用本地登录资料里的原密码。
- 登录后左侧有“平台管理 → 租户管理”；右上角账号下拉同样可直达租户管理。
- `demo`、`demo-b` 的 `admin` 是各自企业管理员，看不到平台入口，不能调用平台管理接口。
- 顶栏两行显示当前企业与“用户名 / 角色”；点击可查看完整企业名称、编码和全部有效角色名称。
- 平台超级管理员负责开通、停用、到期管理与所有者交接，不是读取所有客户业务数据的免过滤账号。
- 岗位目前用于用户资料和个人中心展示；功能权限/数据范围仍由角色负责，不自动把岗位当角色。

对于已经初始化的本地新库，只需补齐菜单数据，无须重建库或重新生成密码：

```powershell
go run ./cmd/tenantconsole -config configs/multitenant.local.yml
```

工具仅允许本地 `ruoyi_go_mt`：显式创建SQL003的配额表、为现有企业补默认配额，再事务补齐菜单；不修改原库/账号密码。
DDL不能依赖事务回滚；重复运行不会重置已有配额。固定菜单ID被其他功能占用时拒绝覆盖。
首次 `tenantinit` 会自动完成同样步骤。更新后刷新页面；如果当前是企业账号，先退出并以 `platform` 登录。

### 首次初始化

第一次在没有目标库的本机执行：

```powershell
go run ./cmd/tenantinit -inspect
go run ./cmd/tenantinit
cd web
npm ci
cd ..
.\scripts\dev-mt.ps1
```

脚本不会停止其他进程或抢占已有端口。Node不在PATH时传入 `-NodePath 完整路径`。
不使用脚本也可以分别执行：

```powershell
go run ./cmd/server -config configs/multitenant.local.yml
# 另一个终端，在 web/ 下
npm run dev
```

前端定向测试：在 `web/` 执行 `node --test test/sessionIdentity.test.mjs test/tenantAdmin.test.mjs test/tenantOperations.test.mjs`。
`/getInfo` 的顶层 `identity` 是服务端验证过的当前工作空间元数据，不能作为前端自行授予权限的依据。

## 按企业授权与管理员运维

使用platform账号进入租户管理，每个企业行新增两个操作：

- **功能授权**：可单独选择目录、菜单和按钮；保存时补齐上级目录。勾选目录本身不等于开通全部子功能。
  “全选”选中全部客户功能，“清空”明确撤回全部功能；均须输入企业编码和操作原因，并二次确认。
  保存后该企业用户需要重新登录。撤回功能会删除普通角色的对应授权，重新开通后需要企业管理员重新分配角色权限。
  这是菜单与权限接口的开通范围，不会删除业务数据或主动停止既有后台任务；需整体停止企业使用时应停用企业。
- **重置管理员密码**：只重置当前所有者，不修改员工密码。输入目标企业编码、自己的platform账号密码、新密码两次及原因。
  新密码8到20个字符，单独安全交付；系统不保存明文供事后查看，也不会更新本地旧登录资料文件。
  成功后目标企业全部旧会话失效，员工仍可使用各自原密码重新登录，其他企业和platform会话不受影响。
  输错平台密码会参与该平台账号的登录锁定计数，不能绕开校验暴力重试。

两项操作均校验企业版本；如果其他人已修改企业状态、所有者或授权，请关闭对话框、刷新列表后重新操作。
成功事件在平台“操作日志”中查看，和业务更新同事务；请求/失败日志沿用异步审计。
不要把密码写进“操作原因”。前端不会把开通/重置密码请求写入sessionStorage防重缓存。
如果只看到旧按钮，在当前本地新库执行一次 `go run ./cmd/tenantconsole`，会显式补建配额表，不重建库、不覆写已有配额。

接口：

| 接口 | 权限与数据 |
|---|---|
| GET `/platform/tenant/:tenantId/menus` | `platform:tenant:grant`；data包含tenantId/tenantCode/tenantName/version/menus/checkedKeys |
| PUT `/platform/tenant/:tenantId/menus` | `platform:tenant:grant`；version、confirmTenantCode、menuIds（必传，可空数组）、reason |
| PUT `/platform/tenant/:tenantId/resetOwnerPwd` | `platform:tenant:resetPwd`；version、ownerUserId、confirmTenantCode、operatorPassword、newPassword、confirmPassword、reason；data.retryCounterCleared说明锁定计数是否清理 |

重置后的密码和数据库版本一旦提交就已生效。如果返回retryCounterCleared=false，不要重复重置；等待Redis恢复/最多10分钟锁定窗口再登录。
此功能不等于“强制首次登录改密”，也没有开放平台代登录客户业务的入口。

初始化拒绝覆盖已有 ruoyi_go_mt、已有输出配置或登录资料；拒绝非本地实例以及用输出配置覆盖原配置。
`base.sql` 是参考基线的只读副本，包含 DROP，仅由初始化工具在确认不存在的新库中运行。
不要直接对现有库执行这组文件，也不要把它作为生产在线迁移脚本。
DDL不能依赖一个ROLLBACK撤销；失败应检查新库具体状态，不得自动删库重试。

## 当前实现的规则

- tenant_kind=00是平台内部空间，01是客户；user_type=00普通员工、01企业所有者、02平台管理员。
  普通新增/导入用户不能自行授予平台身份。所有者通过平台交接流程变更，不通过普通用户编辑修改。
- 请求只在登录/获取验证码时提交企业编码，认证后以Redis会话绑定为准。篡改body/header tenantId不能切换企业。
- 用户等已有JSON结构继续由java_contract转换；TenantRecord不接受前端JSON输入，独立会话保存tenantId。
- 平台专属新功能必须使用platform:*权限；旧生成器、服务器、全局缓存、在线管理、菜单管理同样限制。
  角色分配用的菜单树仍可在本企业授权范围内读取。
- 员工ID、角色ID全局自增，不在每企业重复使用1。部门数据范围在租户边界之内计算。
- 登录令牌使用随机UUID；会话索引/权限版本按全局用户ID定位，应用前缀隔离；配置、字典、
  错误密码计数、验证码额外包含tenantId，缓存代数与清理也按企业隔离。
- 媒体GET必须验证所属企业，可使用专用HttpOnly会话Cookie；普通业务API不接受该Cookie认证。
  Cookie不放在URL中，媒体响应private/no-store。当前附件权限粒度是企业，ERP资源级授权另行实现。
- 修改租户状态、到期时间或所有者会递增持久version，旧会话不能在恢复后复活。
  停用阻止后续请求及后续任务触发，不承诺强行中断已经进入执行的事务或不响应context的函数。
- 新企业只复制平台中版本化白名单的公开配置/字典，不复制任意平台设置；平台IP黑名单不作为客户默认值。
- 生成器要求业务表tenant_id为BIGINT，并移除该字段的前端新增、编辑、查询、列表控制；生成模型只由守卫赋值。
  生成仓库使用TenantTable，标量Count/Scan和map更新也必须隔离。新增复杂JOIN前登记表归属并补测试。

## 生命周期与基础配额

### 平台操作

租户列表中的“配额”可查看用量并调整；“更多”中包含重置密码、申请注销、归档、驳回或恢复。
普通企业编辑/所有者交接现在也必须提交version，前端自动携带。发生版本冲突时刷新，不能去掉版本检查强行覆盖。

- 正常/停用 → 申请注销（立即暂停使用、旧会话失效） → 平台确认归档。
- 驳回申请或恢复归档 → **停用**，并非正常；平台还需根据到期时间、审批结果另行启用。
- 全程保留账号、业务数据、附件及编码，不物理删除；归档不是离线备份，也不是合规意义的数据擦除。
- 待注销/已归档不能通过普通编辑、授权、密码重置或调配额绕过恢复流程。
- 企业所有者也可从右上角“企业信息与配额”提交本企业注销申请；普通员工只能查看本企业信息。

### 配额口径

默认客户100个账号、1GiB已登记附件、10MiB单文件。平台可调整，0表示企业不额外限制；单文件永远取企业/服务器上限的较小值。
已有企业初次初始化的额度不低于当前用量；重复tenantconsole不会把手工设置覆盖为默认值。

- 账号数含企业管理员、普通员工和停用账号，不含已逻辑删除账号；删除后可复用名额。
- 附件容量只统计登记的文件元数据，覆盖普通上传、批量上传和头像；不统计日志、导出流、临时文件或崩溃残留。
- 上传失败会清理本请求生成的文件，批量失败会回滚之前登记的文件。操作系统磁盘限制、孤立文件巡检和备份仍需另外实施。
- 账号新增/Excel导入、文件登记在数据库事务内按“企业行→配额行”加锁后计数，不靠前端或单进程锁兜底。
- 新配额不能低于当前账号/容量用量；降低单文件上限只影响新上传，不删除已有文件。
- 调整配额也推进企业版本，用户需重新登录；它不改变角色权限，也不等于商业套餐计费。

新增接口：

| 接口 | 权限/用途 |
|---|---|
| GET `/platform/tenant/:tenantId` | platform:tenant:query，读取当前版本与企业信息 |
| GET/PUT `/platform/tenant/:tenantId/quota` | platform:tenant:quota；写入需version、confirmTenantCode、maxUsers、maxStorageBytes、maxUploadBytes、reason |
| PUT `/platform/tenant/:tenantId/lifecycle` | platform:tenant:lifecycle；version、confirmTenantCode、action、reason；action为requestCancellation/archive/rejectCancellation/restore |
| GET `/tenant/workspace` | 当前企业的名称、状态、配额用量，不接受客户端选择tenantId |
| POST `/tenant/cancellation` | 仅当前企业所有者；action固定requestCancellation，带version、confirmTenantCode、reason |

本分支服务在Redis/任务初始化前只读检查多租户结构，误接原单租户库会拒绝启动；服务不会自动创建或修改表。
现有新库先运行tenantconsole完成SQL003；不要拿原application.yml直接启动本分支。

## 验证与尚未实现

```powershell
go test ./cmd/... ./internal/... ./pkg/... -count=1
go vet ./cmd/... ./internal/... ./pkg/...
$env:MT_TEST_ROOT=(Get-Location).Path
go test ./test/multitenant -count=1 -v
cd web
node --test test/*.test.mjs
npm run build:prod
```

主接口验收只连接本地8081，且校验数据库名为ruoyi_go_mt。故障验收使用本进程Router及自己的连接池，不停止共享服务。
验收会创建测试记录及停用的check-*/tools-*/ops-*/fault-*企业，
不通过全库清空清理它们。结果见 [MULTITENANT_TEST_REPORT.md](./MULTITENANT_TEST_REPORT.md)。
自有前端的防重复提交只使用页面内存，不再缓存请求体到浏览器存储；加载时清理旧版本留下的sessionObj。

通用租户层尚未提供旧生产库/历史附件迁移、付费套餐计费、店铺级权限、生产部署认证和新容量结论。
erp分支已增加仓库及仓库单据范围；附件仍为企业级隔离，不能把仓库权限外推到任意附件/其他业务。
逻辑注销/归档/恢复及基础配额已实现；物理擦除、离线归档和一个自然人跨企业切换未实现。
历史多租户轮次的页面验收情况保留在其测试报告；erp分支同日已完成员工权限与异常操作真实页面补验，见[ERP_TEST_REPORT.md](./ERP_TEST_REPORT.md)顶部，不等于全部平台页面都补验。
当前用户仅要求ERP基础收口，不继续扩展采购、渠道或财务；不要将开发验收等同于“已经可以无风险对外商业运营”。
