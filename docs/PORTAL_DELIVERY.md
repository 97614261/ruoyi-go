# 门户部署、备份与恢复

2026-10-30；当前结构v6（29表），business资源v3、editorial资源v1。只描述portal分支。

## 发布包

从源码执行 scripts/package-portal.ps1 -TargetOS windows 或 linux；已有本轮Vite产物可加 -SkipAdmin。
脚本生成dist下带时间戳的ZIP，内含Go服务、初装/升级/恢复检查工具、配置示例、使用说明和SHA256SUMS。
包中不含本地私有配置、账号、日志、数据库备份或上传素材。服务器不需要Node；MySQL、Redis与持久化素材目录要单独准备。
Linux包是交叉编译产物，本轮没有Linux机器，不能算Linux运行验收。ZIP解压后需给四个程序执行权限。

所有命令都在解压后的包根目录执行，保留工作目录；默认配置路径 configs/portal.local.yml。

## 全新安装

1. 准备MySQL和Redis。安装工具当前只允许本机3306上的全新ruoyi_go_portal与空Redis DB14；已有库一律拒绝覆盖。
2. 复制configs/portal.example.yml为configs/bootstrap.local.yml，填写MySQL DSN（库名ruoyi_go_portal，尚未创建）、Redis连接、临时至少32字符随机JWT密钥。该临时密钥只为读取引导配置；安装后工具会生成新的JWT和管理员密码。
3. Windows先运行 .\portal-init.exe 查看计划，再运行 .\portal-init.exe -admin-config configs/bootstrap.local.yml -confirm；Linux对应 ./portal-init。
4. 安装生成configs/portal.local.yml与logs/portal-accounts.local.json；它们含凭证，限制文件访问权限。不要公开；引导配置安装后也按私有凭证管理。
5. 修改私有配置portal.publicURL为实际HTTP/HTTPS域名源地址，用于canonical与站点地图；需要代理时配置监听地址，默认仅127.0.0.1:8085。
6. 运行 .\portal.exe 或 ./portal。配置缺失、SQL源版本不一致或结构漂移会停止启动，不自动迁移。
7. 后台/admin/登录，创建栏目、素材和内容；站点外观在「门户管理 → 站点与外观」发布。

新装命令本身不会覆盖现有库。当前环境已实际验证初期新库创建、v3→v4→v5→v6升级；本轮没有另开全新MySQL实例重演v6初装，不能把编译通过当作新机实装。

## 已有门户升级

先停门户，备份数据库、私有配置和整个私有素材目录。只备SQL无法恢复图片。
在包根目录先运行portal-migrate（默认只读计划），确认本地目标、备份及版本，再加 -confirm。
迁移按版本顺序执行，旧指纹不匹配、菜单ID冲突或DDL失败就停止，保留现场；MySQL DDL不能假装事务回滚。
不要重复执行初装/迁移SQL，也不要重算指纹掩盖漂移。处理失败前保留原包和备份。
恢复旧程序前必须确认它支持当前数据库/外观资源版本；不能仅替换exe假装数据库已降级。

## 备份与恢复演练

本轮提供portal-recovery-check工具：默认只预览；确认停服务后加 -confirm。
它使用官方mysqldump/mysql客户端，默认Windows官方安装目录。Linux显式加 -mysql-bin /usr/bin；恢复工具本轮仅在Windows实际执行。
工具从已验证的私有门户配置生成本次恢复目录内的mysql-client.local.cnf，并通过--defaults-file使用；不依赖其他项目的客户端连接文件，命令行另外固定本地TCP主机与端口。此文件含凭据，必须私有。
工具只接受TCP本地3306的ruoyi_go_portal、Redis DB14、127.0.0.1监听配置及空闲8085端口（配置端口也必须8085）；创建全新ruoyi_go_portal_restore_时间戳，不复用、不覆盖、不删除任何库。
产物在logs/portal-step56/recovery-时间戳：database.sql、private下配置和账号、assets、restored-assets、manifest.local.json。
实际比较每张表的SHOW CREATE定义与CHECKSUM TABLE EXTENDED值，再将素材复制至恢复目录比较SHA256；原SQL备份本身也有SHA256。
数据库校验值用于恢复一致性核对，不是对抗篡改的签名；备份清单及备份文件应一起受控保存。

副本仅用于核验，工具不会启动Go或改当前配置指向它。生产启动守卫仍只接受固定ruoyi_go_portal，不能直接把恢复副本库名填进配置绕过守卫。
真正灾备切换应在新的目标MySQL环境创建固定名称ruoyi_go_portal，导入核准备份，恢复同批配置/素材和匹配程序，先核验结构/引用/登录，再切入口；此操作需单独的运维窗口，不在本工具中覆盖当前库。
备份包含数据库、私有连接和账号凭据，不得发到公共位置。不要把恢复目录作为Web静态目录。

## 本轮验证边界

接口/权限/CAS、两模板完整页面、真实后台预览发布恢复、Windows运行、完整本地恢复副本、附件SHA256与160次8并发读检查已执行。
没有远程部署、Linux实际运行、长期稳定性或生产规模容量结论；本地小样本延迟不能换算适用用户数。
系统保持单进程部署，已实现内容定时发布/下架；没有实现多实例协调或任意第三方模板上传。

## 定时任务与升级

v5升级v6新增cms_schedule，须备份后显式运行portal-migrate；旧v5程序不支持v6库，不能只替换回旧exe。
任务按北京时间保存，每秒有界扫描最多10条，执行存在排队/数据库耗时，不承诺精准到秒。服务需持续运行。
停机后错过24小时以内的任务在启动后补执行；超过24小时标记失败，需人工重新安排。要避免维护后的补执行，停机前先取消对应任务。
任务执行会重查创建人的启用状态和当前发布权限、内容版本、栏目和素材。修改草稿后原任务失败，需要取消/重新安排；不会自动追随新草稿。
任务状态与内容发布在同一事务提交；系统/数据库错误保持待执行重试，业务条件失败保留原因。取消只能由创建人或超级管理员执行，且仍须具有模块publish权限。
恢复旧备份也可能恢复其中的待执行任务，切入口前应先核对任务列表和创建人权限。
本轮v6恢复到全新副本29表及素材SHA256核验通过，证据见PORTAL_SCHEDULE_ACCEPTANCE。
