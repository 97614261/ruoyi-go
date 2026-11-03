# 第二阶段本地收口报告

2026-11-02，portal分支，Windows。结论：功能1—5和本地验收完成，原第6步Linux生产性能实测因无环境未完成；这不等于完整生产验收。

## 本轮交付

- 询盘：前台「联系与留言」，Go模板无JS表单；后台「门户管理 → 询盘留言」遮蔽列表、独立详情、待处理/已处理/垃圾、备注、CAS和个人字段擦除。
- 防刷：证明10分钟有效、至少2秒填写、IP绑定、Cookie比对、原子消费、防重复、诱饵字段，同IP提交5/小时、表单30/小时、全站1000/24小时；Redis失败停止接收。是应用层兜底，不是DDoS防护。
- 隐私：不自动发邮件短信；list不返回正文/处理备注，query独立授权，edit/remove分离；模板/Vue按文本转义，操作日志及请求查询串不记录询盘内容。清除活跃表不清历史私有备份，需自行制定保留策略。
- v6→v7显式升级：新增cms_inquiry及6060—6063菜单，30表，不改旧sys结构/普通角色授权；生成器先产出九文件骨架再专用改造，日志/骨架保留在本轮私有验收目录。
- 两条会话测试已修正并增强：直接读Redis验证索引范围，随后鉴权验证懒刷新；损坏revision验证撤销当前及同用户另一个会话，而非只匹配文案。没有削弱会话实现。

## 实际验证

全包`go test -count=1 ./...`在显式本地集成标志下通过，包含原基础接口与门户全套。`go vet ./...`、Vite生产构建通过。
询盘覆盖同意项/跨源/缺Cookie/诱饵/一次性重放、遮蔽列表、详情/修改/清除权限拒绝、CAS和活跃表个人字段擦除、审计无正文；防刷Lua另测年龄、IP、限额、重放和取消请求失败关闭。
真实浏览器从前台提交测试留言，在后台查看和处理为已处理；核对备注落库后仅清除该测试条，验证码恢复true、后台会话退出。
v7实际停机错过dueAt后重启补执行一次；同一发布与任务状态事务在v7重新验证故障回滚。测试内容软删除、测试角色/账号撤权停用；历史仍按规则保留。
恢复副本ruoyi_go_portal_restore_1793582296126996200：30表定义/数据校验、附件SHA256一致；源库未覆盖，副本未启动。资料logs/portal-step56/recovery-1793582296126996200含私有凭据，不能公开。
本地160次/8并发读取：失败0，P95 6.608ms；仅短时Windows小样本，不代表生产容量或长期稳定性。

## 当前访问与注意

前台http://127.0.0.1:18085/，后台http://127.0.0.1:18085/admin/，留言/inquiry。8085由其他Java工程使用，未停止它；旧门户记录PID已失效，因此只按新归属启动18085。
原私有配置备份logs/portal-phase2-final/original-config.local.yml；升级前SQL备份logs/pre-v7-20261102090143.local.sql。没有重置管理员密码。
恢复工具使用显式`-listen-port 18085`且要求对应门户已停止；默认8085守卫仍有效，数据库固定本机3306/ruoyi_go_portal，Redis14。

## 仍待外部环境

没有Linux运行、实际生产并发/长稳、新机v7初装、域名HTTPS切流验收。已检查本机WSL未安装发行版、无Docker命令，没有自动安装或远程部署。Linux交付包只有交叉编译。
第6步的Linux生产性能测试保留待验，不能为了写“第二阶段结束”而改成通过。后续阶段建议见PORTAL_NEXT_PHASES。

## 证据

目录D:/WorkSpace/CodeX/.portal-phase2-final-20261102/：all-final.log（全包真实回归）、legacy-all.log、sessions.log、anti-spam.log、atomic-v7.log、gui-cleanup.log、restart-prepare/verify.log、recovery-v7.log、load.log、frontend.log、vet.log、inquiry-submit.png和inquiry-admin.png。
首次模板页面未注册/源地址不匹配导致的失败日志保留为排障记录，最终通过见inquiry-final.log及all-final.log。验收目录含私有资料，不提交公共仓库。
本地集成运行：GOWORK=off、PORTAL_INTEGRATION=1、PORTAL_TEST_PORT=18085，然后go test -count=1 ./...；GUI清理和重启模式默认跳过，需要各自明确标志。不要用默认8085测试当前Java工程。
未自动提交或推送Git。
