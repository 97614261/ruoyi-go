-- 001_perf_indexes.sql
--
-- 【警告】仅限隔离的压测数据库，禁止在生产库执行。
-- 本文件会修改数据库索引，属于结构变更，不是生产数据库基线的一部分。
-- 执行前必须核对当前连接的数据库实例和库名；仓库中存在本文件不代表生产已建这些索引。
--
-- 给 RuoYi 原始表结构补四个索引。
--
-- 【为什么这个文件在 ruoyi-go/sql/ 而不是改 ry_20260417.sql】
-- 那份 SQL 是冻结的生产基线，Java 版和 Go 版共用。本文件只用于压测环境的
-- before / after 实验，不得合入基线，也不得作为生产初始化步骤。
--
-- 【边界】
-- 索引会改变数据库结构和执行计划。即使不增删列、不改字段类型，也仍属于冻结层；
-- 只有隔离压测库允许临时建立，并在实验结束后按文件末尾命令回滚。
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
