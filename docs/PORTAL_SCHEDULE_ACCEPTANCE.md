# 门户第二阶段第4步：定时发布与下架验收

2026-10-30，portal分支，本地Windows。当前门户结构v6/29表；本报告是本轮快照，后续现状以计划与新报告为准。

## 已交付

- 文章、产品、单页行内「定时任务」：按北京时间安排发布/下架，分页查看状态、结果和创建人，取消待执行任务。
- 每篇最多一条待执行任务；版本绑定创建时内容，修改后失败，不会误发新稿。
- 创建/取消检查publish权限，查询检查query权限；执行重新读取账号及当前角色菜单权限。取消限创建人或超级管理员。
- 启动扫描与每秒轮询，每批最多10条，每轮15秒context；取消后排空worker再关闭依赖。端口绑定失败不启动任务。
- 任务结果与内容变更同事务提交；业务条件失败保存原因，数据库/系统错误回滚并保留待执行状态重试，同批其他任务继续处理。
- 停机后24小时内错过的任务补执行，超过失败；只做单次内容任务，不做周期任务或外观切换。执行会排队，不承诺准点到秒。
- 构建清空专用前端生成目录，避免旧哈希资源反复嵌入后端包。

## 本地实测通过

1. 完整门户接口回归通过：原有登录、草稿隔离、CAS、富文本、栏目/素材引用、发布下架、外观与巡检继续通过。
2. 定时发布和定时下架成功；重复待执行、错误类型、匿名、非法时间、非创建人取消、重复取消均拒绝。
3. 改稿、停用创建人、撤回发布权限、移走本轮图片、停用栏目、错过超过24小时均失败；不改变公开状态，结果保留原因。文件随后恢复，测试账号/角色撤权停用，内容软删除。
4. 三类内容均有实际定时发布用例；未发布内容不能安排下架。终态任务再次扫描不重复改变内容版本。
5. repository故障注入：发布变更后在保存任务终态时注入错误，内容和任务共同回滚；移除故障后重试只提交一次。
6. 创建持久任务后真正停止门户进程，过dueAt再启动，任务补执行成功且内容版本只增加一次，随后软删除测试内容。
7. 真实浏览器完成新建草稿、创建任务、取消、再次安排近期任务、刷新显示成功；截图schedule-ui-proof.png。验证码已恢复true，页面测试会话退出。
8. 本地v5升级前备份：logs/portal-schedule/pre-v6-20261030075058.local.sql。升级仅新增cms_schedule及版本记录，原sys表结构未改。
9. 实际恢复到全新ruoyi_go_portal_restore_1793318738524570300，29表定义和数据校验一致、素材SHA256一致；源库未覆盖、副本未启动。资料位于logs/portal-step56/recovery-1793318738524570300，含私有凭据，不公开。

实现包测试（internal/pkg/portal/sql/cmd）及全包go vet通过；后台Vite构建通过。Windows已运行，Linux仅交叉构建。

## 扩大检查发现的两条失败

额外运行`go test ./...`时，旧`ruoyi-go/test`基础套件有2条失败，不能写成全项目通过。该套件按其默认配置访问本地ry-vue，创建并按zz_test_前缀清理测试记录；本轮未改ERP工作区或数据库结构，未向其安装CMS表。

- `TestRefreshOnlineUsersByIDUsesSessionIndex`：旧断言要求未索引会话继续保留stale:permission/revision0，但断言调用GetLoginUser时当前权限版本机制已触发懒刷新。后续应区分“直接读取Redis验证索引刷新范围”和“鉴权读取验证懒刷新”，不能为测试恢复旧权限。
- `TestPermissionRefreshFailsClosedOnInvalidRevision`：错误实际为“已安全撤销用户全部会话”，旧断言匹配“已安全撤销会话”失败，在后续会话删除断言前退出。后续应验证实际会话撤销结果和范围，避免只依赖文案。当前失败不能替代后续状态核验。

本轮保留这两条失败及日志，没有跳过、弱化它们或修改会话业务代码；建议下一轮先校准测试并复核安全撤销，再开始询盘留言。

## 证据与重跑

日志根目录：D:/WorkSpace/CodeX/.portal-schedule-20261030/。
integration.log（完整门户）、kinds.log（三类内容/栏目）、atomic.log（故障回滚）、restart-prepare.log与restart-verify.log（真重启）、recovery.log（v6恢复）、unit-scoped.log、vet.log、frontend.log。
unit-final.log保留全包运行的2条失败，不将它替换成通过日志。页面截图schedule-ui-proof.png。

运行前设置GOWORK=off、GOTOOLCHAIN=local；本地接口测试显式PORTAL_INTEGRATION=1：

```powershell
go test -count=1 -v ./test/portal
go test -count=1 -v ./internal/repository -run TestCMSScheduleAtomicCommit
go test ./internal/... ./pkg/... ./portal/... ./sql/portal ./cmd/...
go vet ./...
```

重启测试还需显式PORTAL_SCHEDULE_RESTART=prepare/verify并由外部停止/启动已核实归属的门户进程；默认跳过，不能把准备模式当作验证完成。专用GUI/负载测试默认也跳过，须按各自条件单独执行。

未执行新机v6初装、Linux运行、生产规模容量或长期压力测试。保留历史修订/素材/任务记录，不把软删除称作物理清空。未自动提交或推送Git；第二阶段5—6未开始。
