# 门户单租户基线来源

000_ruoyi_base.sql是参考RuoYi-Vue-master/sql/ry_20260417.sql的原样来源副本，含20张核心表；保留上游MIT许可证LICENSE.ruoyi。
不要手动执行这个原始副本，也不要把它当作已有库的升级脚本。它保留了历史DROP语句便于来源核对；
portalinit显式安装器会先删除DROP语句，并且只允许在自己刚创建的空门户库内执行。

默认go run ./cmd/portalinit只预览，不连接数据库。真正安装需要-confirm和-admin-config，固定本机新库ruoyi_go_portal，已有库拒绝。
新增CMS表应使用后续显式迁移并升级结构版本/守卫，不重跑基线。基线哈希统一LF，用于绑定来源，不把跨平台换行变化当作新数据库版本。
本轮未导入Quartz表或001_perf_indexes.sql；安装和验证见docs/PORTAL_STEP1_ACCEPTANCE.md。
