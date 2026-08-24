-- 001_perf_indexes.sql
--
-- 给 RuoYi 原始表结构补四个索引。
--
-- 【为什么这个文件在 ruoyi-go/sql/ 而不是改 ry_20260417.sql】
-- 那份 SQL 是只读的参考，Java 版和 Go 版共用。这里只做增量，
-- 谁先建库都行，跑一遍这个脚本即可。
--
-- 【为什么不算违反"表结构一行不改"】
-- 索引对 Java 和 Go 都是透明的：不增删列、不改类型、不改约束语义，
-- 两版并排跑不受任何影响。改的是执行计划，不是数据契约。
--
-- 【实测依据】10 万用户 / 1000 部门 / 50 万条操作日志，
-- 详见 docs/PERF.md 和 docs/DECISIONS.md 2026-08-22 那条。
--
-- 回滚见文件末尾。

-- 登录：WHERE user_name = ? AND del_flag = '0'
-- 建之前 type=ALL 扫 98626 行 / 177ms，建之后 type=ref / 1.5ms。
-- 登录在每个会话的关键路径上，这是四个里最值钱的一个。
ALTER TABLE sys_user ADD INDEX idx_sys_user_name (user_name);

-- 数据权限与部门树筛选：WHERE dept_id IN (...) AND del_flag = '0'
-- 复合索引带上 del_flag，因为每条用户查询都有这个条件，可以走覆盖索引。
ALTER TABLE sys_user ADD INDEX idx_sys_user_dept (dept_id, del_flag);

-- 角色授权页的 NOT IN 子查询：WHERE ur.role_id = ?
-- sys_user_role 的主键是 (user_id, role_id)，只按 role_id 查用不上前缀，
-- 建之前子查询要扫 100320 行。
ALTER TABLE sys_user_role ADD INDEX idx_sys_user_role_role (role_id);

-- 部门树逐层查询：WHERE parent_id = ?
-- 部门数上千后才有意义，代价很低，一并加上。
ALTER TABLE sys_dept ADD INDEX idx_sys_dept_parent (parent_id);

-- ---------------------------------------------------------------
-- 回滚
-- ---------------------------------------------------------------
-- ALTER TABLE sys_user      DROP INDEX idx_sys_user_name;
-- ALTER TABLE sys_user      DROP INDEX idx_sys_user_dept;
-- ALTER TABLE sys_user_role DROP INDEX idx_sys_user_role_role;
-- ALTER TABLE sys_dept      DROP INDEX idx_sys_dept_parent;
--
-- 或者：go run ./cmd/perfseed -unindex
