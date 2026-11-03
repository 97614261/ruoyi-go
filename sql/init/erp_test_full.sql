-- ruoyi-go erp branch: full EMPTY TEST DATABASE initializer, 2026-11-03.
-- Select a NEW database with utf8mb4_unicode_ci before running the entire file.
-- Includes 82 tables, multitenancy, ERP001-011, dictionaries, menus and 3 owners.
-- platform/admin, demo/admin, demo-b/admin: Test@123456 (TEST ONLY).
-- CAPTCHA remains enabled. No existing database, account or business data is exported.
-- Requires CREATE/ALTER/REFERENCES and CREATE ROUTINE/ALTER ROUTINE/EXECUTE privileges.
-- On any error STOP; DDL has implicit commits. Do not import into an existing database.
SET NAMES utf8mb4 COLLATE utf8mb4_unicode_ci;
DELIMITER $$
CREATE PROCEDURE _ruoyi_erp_test_init_20261103()
SQL SECURITY INVOKER
BEGIN
  IF DATABASE() IS NULL OR DATABASE() IN ('mysql','sys','information_schema','performance_schema','ruoyi_go_mt') THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='ERP_INIT_REFUSED: select a NEW empty test database, not ruoyi_go_mt';
  END IF;
  IF EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE()) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='ERP_INIT_REFUSED: target database is not empty';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name=DATABASE() AND default_character_set_name='utf8mb4' AND default_collation_name='utf8mb4_unicode_ci') THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='ERP_INIT_REFUSED: database must use utf8mb4 / utf8mb4_unicode_ci';
  END IF;
create table sys_dept (
  dept_id           bigint(20)      not null auto_increment    comment '部门id',
  parent_id         bigint(20)      default 0                  comment '父部门id',
  ancestors         varchar(50)     default ''                 comment '祖级列表',
  dept_name         varchar(30)     default ''                 comment '部门名称',
  order_num         int(4)          default 0                  comment '显示顺序',
  leader            varchar(20)     default null               comment '负责人',
  phone             varchar(11)     default null               comment '联系电话',
  email             varchar(50)     default null               comment '邮箱',
  status            char(1)         default '0'                comment '部门状态（0正常 1停用）',
  del_flag          char(1)         default '0'                comment '删除标志（0代表存在 2代表删除）',
  create_by         varchar(64)     default ''                 comment '创建者',
  create_time 	    datetime                                   comment '创建时间',
  update_by         varchar(64)     default ''                 comment '更新者',
  update_time       datetime                                   comment '更新时间',
  primary key (dept_id)
) engine=innodb auto_increment=200 comment = '部门表';

create table sys_user (
  user_id           bigint(20)      not null auto_increment    comment '用户ID',
  dept_id           bigint(20)      default null               comment '部门ID',
  user_name         varchar(30)     not null                   comment '用户账号',
  nick_name         varchar(30)     not null                   comment '用户昵称',
  user_type         varchar(2)      default '00'               comment '用户类型（00系统用户）',
  email             varchar(50)     default ''                 comment '用户邮箱',
  phonenumber       varchar(11)     default ''                 comment '手机号码',
  sex               char(1)         default '0'                comment '用户性别（0男 1女 2未知）',
  avatar            varchar(100)    default ''                 comment '头像地址',
  password          varchar(100)    default ''                 comment '密码',
  status            char(1)         default '0'                comment '账号状态（0正常 1停用）',
  del_flag          char(1)         default '0'                comment '删除标志（0代表存在 2代表删除）',
  login_ip          varchar(128)    default ''                 comment '最后登录IP',
  login_date        datetime                                   comment '最后登录时间',
  pwd_update_date   datetime                                   comment '密码最后更新时间',
  create_by         varchar(64)     default ''                 comment '创建者',
  create_time       datetime                                   comment '创建时间',
  update_by         varchar(64)     default ''                 comment '更新者',
  update_time       datetime                                   comment '更新时间',
  remark            varchar(500)    default null               comment '备注',
  primary key (user_id)
) engine=innodb auto_increment=100 comment = '用户信息表';

create table sys_post
(
  post_id       bigint(20)      not null auto_increment    comment '岗位ID',
  post_code     varchar(64)     not null                   comment '岗位编码',
  post_name     varchar(50)     not null                   comment '岗位名称',
  post_sort     int(4)          not null                   comment '显示顺序',
  status        char(1)         not null                   comment '状态（0正常 1停用）',
  create_by     varchar(64)     default ''                 comment '创建者',
  create_time   datetime                                   comment '创建时间',
  update_by     varchar(64)     default ''			       comment '更新者',
  update_time   datetime                                   comment '更新时间',
  remark        varchar(500)    default null               comment '备注',
  primary key (post_id)
) engine=innodb comment = '岗位信息表';

create table sys_role (
  role_id              bigint(20)      not null auto_increment    comment '角色ID',
  role_name            varchar(30)     not null                   comment '角色名称',
  role_key             varchar(100)    not null                   comment '角色权限字符串',
  role_sort            int(4)          not null                   comment '显示顺序',
  data_scope           char(1)         default '1'                comment '数据范围（1：全部数据权限 2：自定数据权限 3：本部门数据权限 4：本部门及以下数据权限）',
  menu_check_strictly  tinyint(1)      default 1                  comment '菜单树选择项是否关联显示',
  dept_check_strictly  tinyint(1)      default 1                  comment '部门树选择项是否关联显示',
  status               char(1)         not null                   comment '角色状态（0正常 1停用）',
  del_flag             char(1)         default '0'                comment '删除标志（0代表存在 2代表删除）',
  create_by            varchar(64)     default ''                 comment '创建者',
  create_time          datetime                                   comment '创建时间',
  update_by            varchar(64)     default ''                 comment '更新者',
  update_time          datetime                                   comment '更新时间',
  remark               varchar(500)    default null               comment '备注',
  primary key (role_id)
) engine=innodb auto_increment=100 comment = '角色信息表';

create table sys_menu (
  menu_id           bigint(20)      not null auto_increment    comment '菜单ID',
  menu_name         varchar(50)     not null                   comment '菜单名称',
  parent_id         bigint(20)      default 0                  comment '父菜单ID',
  order_num         int(4)          default 0                  comment '显示顺序',
  path              varchar(200)    default ''                 comment '路由地址',
  component         varchar(255)    default null               comment '组件路径',
  query             varchar(255)    default null               comment '路由参数',
  route_name        varchar(50)     default ''                 comment '路由名称',
  is_frame          int(1)          default 1                  comment '是否为外链（0是 1否）',
  is_cache          int(1)          default 0                  comment '是否缓存（0缓存 1不缓存）',
  menu_type         char(1)         default ''                 comment '菜单类型（M目录 C菜单 F按钮）',
  visible           char(1)         default 0                  comment '菜单状态（0显示 1隐藏）',
  status            char(1)         default 0                  comment '菜单状态（0正常 1停用）',
  perms             varchar(100)    default null               comment '权限标识',
  icon              varchar(100)    default '#'                comment '菜单图标',
  create_by         varchar(64)     default ''                 comment '创建者',
  create_time       datetime                                   comment '创建时间',
  update_by         varchar(64)     default ''                 comment '更新者',
  update_time       datetime                                   comment '更新时间',
  remark            varchar(500)    default ''                 comment '备注',
  primary key (menu_id)
) engine=innodb auto_increment=2000 comment = '菜单权限表';

insert into sys_menu values('1', '系统管理', '0', '1', 'system',           null, '', '', 1, 0, 'M', '0', '0', '', 'system',   'admin', sysdate(), '', null, '系统管理目录');

insert into sys_menu values('2', '系统监控', '0', '2', 'monitor',          null, '', '', 1, 0, 'M', '0', '0', '', 'monitor',  'admin', sysdate(), '', null, '系统监控目录');

insert into sys_menu values('3', '系统工具', '0', '3', 'tool',             null, '', '', 1, 0, 'M', '0', '0', '', 'tool',     'admin', sysdate(), '', null, '系统工具目录');

insert into sys_menu values('4', '若依官网', '0', '4', 'http://ruoyi.vip', null, '', '', 0, 0, 'M', '0', '0', '', 'guide',    'admin', sysdate(), '', null, '若依官网地址');

insert into sys_menu values('100',  '用户管理', '1',   '1', 'user',       'system/user/index',        '', '', 1, 0, 'C', '0', '0', 'system:user:list',        'user',          'admin', sysdate(), '', null, '用户管理菜单');

insert into sys_menu values('101',  '角色管理', '1',   '2', 'role',       'system/role/index',        '', '', 1, 0, 'C', '0', '0', 'system:role:list',        'peoples',       'admin', sysdate(), '', null, '角色管理菜单');

insert into sys_menu values('102',  '菜单管理', '1',   '3', 'menu',       'system/menu/index',        '', '', 1, 0, 'C', '0', '0', 'system:menu:list',        'tree-table',    'admin', sysdate(), '', null, '菜单管理菜单');

insert into sys_menu values('103',  '部门管理', '1',   '4', 'dept',       'system/dept/index',        '', '', 1, 0, 'C', '0', '0', 'system:dept:list',        'tree',          'admin', sysdate(), '', null, '部门管理菜单');

insert into sys_menu values('104',  '岗位管理', '1',   '5', 'post',       'system/post/index',        '', '', 1, 0, 'C', '0', '0', 'system:post:list',        'post',          'admin', sysdate(), '', null, '岗位管理菜单');

insert into sys_menu values('105',  '字典管理', '1',   '6', 'dict',       'system/dict/index',        '', '', 1, 0, 'C', '0', '0', 'system:dict:list',        'dict',          'admin', sysdate(), '', null, '字典管理菜单');

insert into sys_menu values('106',  '参数设置', '1',   '7', 'config',     'system/config/index',      '', '', 1, 0, 'C', '0', '0', 'system:config:list',      'edit',          'admin', sysdate(), '', null, '参数设置菜单');

insert into sys_menu values('107',  '通知公告', '1',   '8', 'notice',     'system/notice/index',      '', '', 1, 0, 'C', '0', '0', 'system:notice:list',      'message',       'admin', sysdate(), '', null, '通知公告菜单');

insert into sys_menu values('108',  '日志管理', '1',   '9', 'log',        '',                         '', '', 1, 0, 'M', '0', '0', '',                        'log',           'admin', sysdate(), '', null, '日志管理菜单');

insert into sys_menu values('109',  '在线用户', '2',   '1', 'online',     'monitor/online/index',     '', '', 1, 0, 'C', '0', '0', 'monitor:online:list',     'online',        'admin', sysdate(), '', null, '在线用户菜单');

insert into sys_menu values('110',  '定时任务', '2',   '2', 'job',        'monitor/job/index',        '', '', 1, 0, 'C', '0', '0', 'monitor:job:list',        'job',           'admin', sysdate(), '', null, '定时任务菜单');

insert into sys_menu values('111',  '数据监控', '2',   '3', 'druid',      'monitor/druid/index',      '', '', 1, 0, 'C', '0', '0', 'monitor:druid:list',      'druid',         'admin', sysdate(), '', null, '数据监控菜单');

insert into sys_menu values('112',  '服务监控', '2',   '4', 'server',     'monitor/server/index',     '', '', 1, 0, 'C', '0', '0', 'monitor:server:list',     'server',        'admin', sysdate(), '', null, '服务监控菜单');

insert into sys_menu values('113',  '缓存监控', '2',   '5', 'cache',      'monitor/cache/index',      '', '', 1, 0, 'C', '0', '0', 'monitor:cache:list',      'redis',         'admin', sysdate(), '', null, '缓存监控菜单');

insert into sys_menu values('114',  '缓存列表', '2',   '6', 'cacheList',  'monitor/cache/list',       '', '', 1, 0, 'C', '0', '0', 'monitor:cache:list',      'redis-list',    'admin', sysdate(), '', null, '缓存列表菜单');

insert into sys_menu values('115',  '表单构建', '3',   '1', 'build',      'tool/build/index',         '', '', 1, 0, 'C', '0', '0', 'tool:build:list',         'build',         'admin', sysdate(), '', null, '表单构建菜单');

insert into sys_menu values('116',  '代码生成', '3',   '2', 'gen',        'tool/gen/index',           '', '', 1, 0, 'C', '0', '0', 'tool:gen:list',           'code',          'admin', sysdate(), '', null, '代码生成菜单');

insert into sys_menu values('117',  '系统接口', '3',   '3', 'swagger',    'tool/swagger/index',       '', '', 1, 0, 'C', '0', '0', 'tool:swagger:list',       'swagger',       'admin', sysdate(), '', null, '系统接口菜单');

insert into sys_menu values('500',  '操作日志', '108', '1', 'operlog',    'monitor/operlog/index',    '', '', 1, 0, 'C', '0', '0', 'monitor:operlog:list',    'form',          'admin', sysdate(), '', null, '操作日志菜单');

insert into sys_menu values('501',  '登录日志', '108', '2', 'logininfor', 'monitor/logininfor/index', '', '', 1, 0, 'C', '0', '0', 'monitor:logininfor:list', 'logininfor',    'admin', sysdate(), '', null, '登录日志菜单');

insert into sys_menu values('1000', '用户查询', '100', '1',  '', '', '', '', 1, 0, 'F', '0', '0', 'system:user:query',          '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1001', '用户新增', '100', '2',  '', '', '', '', 1, 0, 'F', '0', '0', 'system:user:add',            '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1002', '用户修改', '100', '3',  '', '', '', '', 1, 0, 'F', '0', '0', 'system:user:edit',           '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1003', '用户删除', '100', '4',  '', '', '', '', 1, 0, 'F', '0', '0', 'system:user:remove',         '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1004', '用户导出', '100', '5',  '', '', '', '', 1, 0, 'F', '0', '0', 'system:user:export',         '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1005', '用户导入', '100', '6',  '', '', '', '', 1, 0, 'F', '0', '0', 'system:user:import',         '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1006', '重置密码', '100', '7',  '', '', '', '', 1, 0, 'F', '0', '0', 'system:user:resetPwd',       '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1007', '角色查询', '101', '1',  '', '', '', '', 1, 0, 'F', '0', '0', 'system:role:query',          '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1008', '角色新增', '101', '2',  '', '', '', '', 1, 0, 'F', '0', '0', 'system:role:add',            '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1009', '角色修改', '101', '3',  '', '', '', '', 1, 0, 'F', '0', '0', 'system:role:edit',           '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1010', '角色删除', '101', '4',  '', '', '', '', 1, 0, 'F', '0', '0', 'system:role:remove',         '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1011', '角色导出', '101', '5',  '', '', '', '', 1, 0, 'F', '0', '0', 'system:role:export',         '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1012', '菜单查询', '102', '1',  '', '', '', '', 1, 0, 'F', '0', '0', 'system:menu:query',          '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1013', '菜单新增', '102', '2',  '', '', '', '', 1, 0, 'F', '0', '0', 'system:menu:add',            '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1014', '菜单修改', '102', '3',  '', '', '', '', 1, 0, 'F', '0', '0', 'system:menu:edit',           '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1015', '菜单删除', '102', '4',  '', '', '', '', 1, 0, 'F', '0', '0', 'system:menu:remove',         '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1016', '部门查询', '103', '1',  '', '', '', '', 1, 0, 'F', '0', '0', 'system:dept:query',          '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1017', '部门新增', '103', '2',  '', '', '', '', 1, 0, 'F', '0', '0', 'system:dept:add',            '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1018', '部门修改', '103', '3',  '', '', '', '', 1, 0, 'F', '0', '0', 'system:dept:edit',           '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1019', '部门删除', '103', '4',  '', '', '', '', 1, 0, 'F', '0', '0', 'system:dept:remove',         '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1020', '岗位查询', '104', '1',  '', '', '', '', 1, 0, 'F', '0', '0', 'system:post:query',          '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1021', '岗位新增', '104', '2',  '', '', '', '', 1, 0, 'F', '0', '0', 'system:post:add',            '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1022', '岗位修改', '104', '3',  '', '', '', '', 1, 0, 'F', '0', '0', 'system:post:edit',           '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1023', '岗位删除', '104', '4',  '', '', '', '', 1, 0, 'F', '0', '0', 'system:post:remove',         '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1024', '岗位导出', '104', '5',  '', '', '', '', 1, 0, 'F', '0', '0', 'system:post:export',         '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1025', '字典查询', '105', '1', '#', '', '', '', 1, 0, 'F', '0', '0', 'system:dict:query',          '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1026', '字典新增', '105', '2', '#', '', '', '', 1, 0, 'F', '0', '0', 'system:dict:add',            '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1027', '字典修改', '105', '3', '#', '', '', '', 1, 0, 'F', '0', '0', 'system:dict:edit',           '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1028', '字典删除', '105', '4', '#', '', '', '', 1, 0, 'F', '0', '0', 'system:dict:remove',         '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1029', '字典导出', '105', '5', '#', '', '', '', 1, 0, 'F', '0', '0', 'system:dict:export',         '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1030', '参数查询', '106', '1', '#', '', '', '', 1, 0, 'F', '0', '0', 'system:config:query',        '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1031', '参数新增', '106', '2', '#', '', '', '', 1, 0, 'F', '0', '0', 'system:config:add',          '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1032', '参数修改', '106', '3', '#', '', '', '', 1, 0, 'F', '0', '0', 'system:config:edit',         '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1033', '参数删除', '106', '4', '#', '', '', '', 1, 0, 'F', '0', '0', 'system:config:remove',       '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1034', '参数导出', '106', '5', '#', '', '', '', 1, 0, 'F', '0', '0', 'system:config:export',       '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1035', '公告查询', '107', '1', '#', '', '', '', 1, 0, 'F', '0', '0', 'system:notice:query',        '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1036', '公告新增', '107', '2', '#', '', '', '', 1, 0, 'F', '0', '0', 'system:notice:add',          '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1037', '公告修改', '107', '3', '#', '', '', '', 1, 0, 'F', '0', '0', 'system:notice:edit',         '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1038', '公告删除', '107', '4', '#', '', '', '', 1, 0, 'F', '0', '0', 'system:notice:remove',       '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1039', '操作查询', '500', '1', '#', '', '', '', 1, 0, 'F', '0', '0', 'monitor:operlog:query',      '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1040', '操作删除', '500', '2', '#', '', '', '', 1, 0, 'F', '0', '0', 'monitor:operlog:remove',     '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1041', '日志导出', '500', '3', '#', '', '', '', 1, 0, 'F', '0', '0', 'monitor:operlog:export',     '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1042', '登录查询', '501', '1', '#', '', '', '', 1, 0, 'F', '0', '0', 'monitor:logininfor:query',   '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1043', '登录删除', '501', '2', '#', '', '', '', 1, 0, 'F', '0', '0', 'monitor:logininfor:remove',  '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1044', '日志导出', '501', '3', '#', '', '', '', 1, 0, 'F', '0', '0', 'monitor:logininfor:export',  '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1045', '账户解锁', '501', '4', '#', '', '', '', 1, 0, 'F', '0', '0', 'monitor:logininfor:unlock',  '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1046', '在线查询', '109', '1', '#', '', '', '', 1, 0, 'F', '0', '0', 'monitor:online:query',       '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1047', '批量强退', '109', '2', '#', '', '', '', 1, 0, 'F', '0', '0', 'monitor:online:batchLogout', '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1048', '单条强退', '109', '3', '#', '', '', '', 1, 0, 'F', '0', '0', 'monitor:online:forceLogout', '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1049', '任务查询', '110', '1', '#', '', '', '', 1, 0, 'F', '0', '0', 'monitor:job:query',          '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1050', '任务新增', '110', '2', '#', '', '', '', 1, 0, 'F', '0', '0', 'monitor:job:add',            '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1051', '任务修改', '110', '3', '#', '', '', '', 1, 0, 'F', '0', '0', 'monitor:job:edit',           '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1052', '任务删除', '110', '4', '#', '', '', '', 1, 0, 'F', '0', '0', 'monitor:job:remove',         '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1053', '状态修改', '110', '5', '#', '', '', '', 1, 0, 'F', '0', '0', 'monitor:job:changeStatus',   '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1054', '任务导出', '110', '6', '#', '', '', '', 1, 0, 'F', '0', '0', 'monitor:job:export',         '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1055', '生成查询', '116', '1', '#', '', '', '', 1, 0, 'F', '0', '0', 'tool:gen:query',             '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1056', '生成修改', '116', '2', '#', '', '', '', 1, 0, 'F', '0', '0', 'tool:gen:edit',              '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1057', '生成删除', '116', '3', '#', '', '', '', 1, 0, 'F', '0', '0', 'tool:gen:remove',            '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1058', '导入代码', '116', '4', '#', '', '', '', 1, 0, 'F', '0', '0', 'tool:gen:import',            '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1059', '预览代码', '116', '5', '#', '', '', '', 1, 0, 'F', '0', '0', 'tool:gen:preview',           '#', 'admin', sysdate(), '', null, '');

insert into sys_menu values('1060', '生成代码', '116', '6', '#', '', '', '', 1, 0, 'F', '0', '0', 'tool:gen:code',              '#', 'admin', sysdate(), '', null, '');

create table sys_user_role (
  user_id   bigint(20) not null comment '用户ID',
  role_id   bigint(20) not null comment '角色ID',
  primary key(user_id, role_id)
) engine=innodb comment = '用户和角色关联表';

create table sys_role_menu (
  role_id   bigint(20) not null comment '角色ID',
  menu_id   bigint(20) not null comment '菜单ID',
  primary key(role_id, menu_id)
) engine=innodb comment = '角色和菜单关联表';

create table sys_role_dept (
  role_id   bigint(20) not null comment '角色ID',
  dept_id   bigint(20) not null comment '部门ID',
  primary key(role_id, dept_id)
) engine=innodb comment = '角色和部门关联表';

create table sys_user_post
(
  user_id   bigint(20) not null comment '用户ID',
  post_id   bigint(20) not null comment '岗位ID',
  primary key (user_id, post_id)
) engine=innodb comment = '用户与岗位关联表';

create table sys_oper_log (
  oper_id           bigint(20)      not null auto_increment    comment '日志主键',
  title             varchar(50)     default ''                 comment '模块标题',
  business_type     int(2)          default 0                  comment '业务类型（0其它 1新增 2修改 3删除）',
  method            varchar(200)    default ''                 comment '方法名称',
  request_method    varchar(10)     default ''                 comment '请求方式',
  operator_type     int(1)          default 0                  comment '操作类别（0其它 1后台用户 2手机端用户）',
  oper_name         varchar(50)     default ''                 comment '操作人员',
  dept_name         varchar(50)     default ''                 comment '部门名称',
  oper_url          varchar(255)    default ''                 comment '请求URL',
  oper_ip           varchar(128)    default ''                 comment '主机地址',
  oper_location     varchar(255)    default ''                 comment '操作地点',
  oper_param        varchar(2000)   default ''                 comment '请求参数',
  json_result       varchar(2000)   default ''                 comment '返回参数',
  status            int(1)          default 0                  comment '操作状态（0正常 1异常）',
  error_msg         varchar(2000)   default ''                 comment '错误消息',
  oper_time         datetime                                   comment '操作时间',
  cost_time         bigint(20)      default 0                  comment '消耗时间',
  primary key (oper_id),
  key idx_sys_oper_log_bt (business_type),
  key idx_sys_oper_log_s  (status),
  key idx_sys_oper_log_ot (oper_time)
) engine=innodb auto_increment=100 comment = '操作日志记录';

create table sys_dict_type
(
  dict_id          bigint(20)      not null auto_increment    comment '字典主键',
  dict_name        varchar(100)    default ''                 comment '字典名称',
  dict_type        varchar(100)    default ''                 comment '字典类型',
  status           char(1)         default '0'                comment '状态（0正常 1停用）',
  create_by        varchar(64)     default ''                 comment '创建者',
  create_time      datetime                                   comment '创建时间',
  update_by        varchar(64)     default ''                 comment '更新者',
  update_time      datetime                                   comment '更新时间',
  remark           varchar(500)    default null               comment '备注',
  primary key (dict_id),
  unique (dict_type)
) engine=innodb auto_increment=100 comment = '字典类型表';

insert into sys_dict_type values(1,  '用户性别', 'sys_user_sex',        '0', 'admin', sysdate(), '', null, '用户性别列表');

insert into sys_dict_type values(2,  '菜单状态', 'sys_show_hide',       '0', 'admin', sysdate(), '', null, '菜单状态列表');

insert into sys_dict_type values(3,  '系统开关', 'sys_normal_disable',  '0', 'admin', sysdate(), '', null, '系统开关列表');

insert into sys_dict_type values(4,  '任务状态', 'sys_job_status',      '0', 'admin', sysdate(), '', null, '任务状态列表');

insert into sys_dict_type values(5,  '任务分组', 'sys_job_group',       '0', 'admin', sysdate(), '', null, '任务分组列表');

insert into sys_dict_type values(6,  '系统是否', 'sys_yes_no',          '0', 'admin', sysdate(), '', null, '系统是否列表');

insert into sys_dict_type values(7,  '通知类型', 'sys_notice_type',     '0', 'admin', sysdate(), '', null, '通知类型列表');

insert into sys_dict_type values(8,  '通知状态', 'sys_notice_status',   '0', 'admin', sysdate(), '', null, '通知状态列表');

insert into sys_dict_type values(9,  '操作类型', 'sys_oper_type',       '0', 'admin', sysdate(), '', null, '操作类型列表');

insert into sys_dict_type values(10, '系统状态', 'sys_common_status',   '0', 'admin', sysdate(), '', null, '登录状态列表');

create table sys_dict_data
(
  dict_code        bigint(20)      not null auto_increment    comment '字典编码',
  dict_sort        int(4)          default 0                  comment '字典排序',
  dict_label       varchar(100)    default ''                 comment '字典标签',
  dict_value       varchar(100)    default ''                 comment '字典键值',
  dict_type        varchar(100)    default ''                 comment '字典类型',
  css_class        varchar(100)    default null               comment '样式属性（其他样式扩展）',
  list_class       varchar(100)    default null               comment '表格回显样式',
  is_default       char(1)         default 'N'                comment '是否默认（Y是 N否）',
  status           char(1)         default '0'                comment '状态（0正常 1停用）',
  create_by        varchar(64)     default ''                 comment '创建者',
  create_time      datetime                                   comment '创建时间',
  update_by        varchar(64)     default ''                 comment '更新者',
  update_time      datetime                                   comment '更新时间',
  remark           varchar(500)    default null               comment '备注',
  primary key (dict_code)
) engine=innodb auto_increment=100 comment = '字典数据表';

insert into sys_dict_data values(1,  1,  '男',       '0',       'sys_user_sex',        '',   '',        'Y', '0', 'admin', sysdate(), '', null, '性别男');

insert into sys_dict_data values(2,  2,  '女',       '1',       'sys_user_sex',        '',   '',        'N', '0', 'admin', sysdate(), '', null, '性别女');

insert into sys_dict_data values(3,  3,  '未知',     '2',       'sys_user_sex',        '',   '',        'N', '0', 'admin', sysdate(), '', null, '性别未知');

insert into sys_dict_data values(4,  1,  '显示',     '0',       'sys_show_hide',       '',   'primary', 'Y', '0', 'admin', sysdate(), '', null, '显示菜单');

insert into sys_dict_data values(5,  2,  '隐藏',     '1',       'sys_show_hide',       '',   'danger',  'N', '0', 'admin', sysdate(), '', null, '隐藏菜单');

insert into sys_dict_data values(6,  1,  '正常',     '0',       'sys_normal_disable',  '',   'primary', 'Y', '0', 'admin', sysdate(), '', null, '正常状态');

insert into sys_dict_data values(7,  2,  '停用',     '1',       'sys_normal_disable',  '',   'danger',  'N', '0', 'admin', sysdate(), '', null, '停用状态');

insert into sys_dict_data values(8,  1,  '正常',     '0',       'sys_job_status',      '',   'primary', 'Y', '0', 'admin', sysdate(), '', null, '正常状态');

insert into sys_dict_data values(9,  2,  '暂停',     '1',       'sys_job_status',      '',   'danger',  'N', '0', 'admin', sysdate(), '', null, '停用状态');

insert into sys_dict_data values(10, 1,  '默认',     'DEFAULT', 'sys_job_group',       '',   '',        'Y', '0', 'admin', sysdate(), '', null, '默认分组');

insert into sys_dict_data values(11, 2,  '系统',     'SYSTEM',  'sys_job_group',       '',   '',        'N', '0', 'admin', sysdate(), '', null, '系统分组');

insert into sys_dict_data values(12, 1,  '是',       'Y',       'sys_yes_no',          '',   'primary', 'Y', '0', 'admin', sysdate(), '', null, '系统默认是');

insert into sys_dict_data values(13, 2,  '否',       'N',       'sys_yes_no',          '',   'danger',  'N', '0', 'admin', sysdate(), '', null, '系统默认否');

insert into sys_dict_data values(14, 1,  '通知',     '1',       'sys_notice_type',     '',   'warning', 'Y', '0', 'admin', sysdate(), '', null, '通知');

insert into sys_dict_data values(15, 2,  '公告',     '2',       'sys_notice_type',     '',   'success', 'N', '0', 'admin', sysdate(), '', null, '公告');

insert into sys_dict_data values(16, 1,  '正常',     '0',       'sys_notice_status',   '',   'primary', 'Y', '0', 'admin', sysdate(), '', null, '正常状态');

insert into sys_dict_data values(17, 2,  '关闭',     '1',       'sys_notice_status',   '',   'danger',  'N', '0', 'admin', sysdate(), '', null, '关闭状态');

insert into sys_dict_data values(18, 99, '其他',     '0',       'sys_oper_type',       '',   'info',    'N', '0', 'admin', sysdate(), '', null, '其他操作');

insert into sys_dict_data values(19, 1,  '新增',     '1',       'sys_oper_type',       '',   'info',    'N', '0', 'admin', sysdate(), '', null, '新增操作');

insert into sys_dict_data values(20, 2,  '修改',     '2',       'sys_oper_type',       '',   'info',    'N', '0', 'admin', sysdate(), '', null, '修改操作');

insert into sys_dict_data values(21, 3,  '删除',     '3',       'sys_oper_type',       '',   'danger',  'N', '0', 'admin', sysdate(), '', null, '删除操作');

insert into sys_dict_data values(22, 4,  '授权',     '4',       'sys_oper_type',       '',   'primary', 'N', '0', 'admin', sysdate(), '', null, '授权操作');

insert into sys_dict_data values(23, 5,  '导出',     '5',       'sys_oper_type',       '',   'warning', 'N', '0', 'admin', sysdate(), '', null, '导出操作');

insert into sys_dict_data values(24, 6,  '导入',     '6',       'sys_oper_type',       '',   'warning', 'N', '0', 'admin', sysdate(), '', null, '导入操作');

insert into sys_dict_data values(25, 7,  '强退',     '7',       'sys_oper_type',       '',   'danger',  'N', '0', 'admin', sysdate(), '', null, '强退操作');

insert into sys_dict_data values(26, 8,  '生成代码', '8',       'sys_oper_type',       '',   'warning', 'N', '0', 'admin', sysdate(), '', null, '生成操作');

insert into sys_dict_data values(27, 9,  '清空数据', '9',       'sys_oper_type',       '',   'danger',  'N', '0', 'admin', sysdate(), '', null, '清空操作');

insert into sys_dict_data values(28, 1,  '成功',     '0',       'sys_common_status',   '',   'primary', 'N', '0', 'admin', sysdate(), '', null, '正常状态');

insert into sys_dict_data values(29, 2,  '失败',     '1',       'sys_common_status',   '',   'danger',  'N', '0', 'admin', sysdate(), '', null, '停用状态');

create table sys_config (
  config_id         int(5)          not null auto_increment    comment '参数主键',
  config_name       varchar(100)    default ''                 comment '参数名称',
  config_key        varchar(100)    default ''                 comment '参数键名',
  config_value      varchar(500)    default ''                 comment '参数键值',
  config_type       char(1)         default 'N'                comment '系统内置（Y是 N否）',
  create_by         varchar(64)     default ''                 comment '创建者',
  create_time       datetime                                   comment '创建时间',
  update_by         varchar(64)     default ''                 comment '更新者',
  update_time       datetime                                   comment '更新时间',
  remark            varchar(500)    default null               comment '备注',
  primary key (config_id)
) engine=innodb auto_increment=100 comment = '参数配置表';

insert into sys_config values(1, '主框架页-默认皮肤样式名称',     'sys.index.skinName',               'skin-blue',     'Y', 'admin', sysdate(), '', null, '蓝色 skin-blue、绿色 skin-green、紫色 skin-purple、红色 skin-red、黄色 skin-yellow' );

insert into sys_config values(2, '用户管理-账号初始密码',         'sys.user.initPassword',            '123456',        'Y', 'admin', sysdate(), '', null, '初始化密码 123456' );

insert into sys_config values(3, '主框架页-侧边栏主题',           'sys.index.sideTheme',              'theme-dark',    'Y', 'admin', sysdate(), '', null, '深色主题theme-dark，浅色主题theme-light' );

insert into sys_config values(4, '账号自助-验证码开关',           'sys.account.captchaEnabled',       'true',          'Y', 'admin', sysdate(), '', null, '是否开启验证码功能（true开启，false关闭）');

insert into sys_config values(5, '账号自助-是否开启用户注册功能', 'sys.account.registerUser',         'false',         'Y', 'admin', sysdate(), '', null, '是否开启注册用户功能（true开启，false关闭）');

insert into sys_config values(6, '用户登录-黑名单列表',           'sys.login.blackIPList',            '',              'Y', 'admin', sysdate(), '', null, '设置登录IP黑名单限制，多个匹配项以;分隔，支持匹配（*通配、网段）');

insert into sys_config values(7, '用户管理-初始密码修改策略',     'sys.account.initPasswordModify',   '1',             'Y', 'admin', sysdate(), '', null, '0：初始密码修改策略关闭，没有任何提示，1：提醒用户，如果未修改初始密码，则在登录时就会提醒修改密码对话框');

insert into sys_config values(8, '用户管理-账号密码更新周期',     'sys.account.passwordValidateDays', '0',             'Y', 'admin', sysdate(), '', null, '密码更新周期（填写数字，数据初始化值为0不限制，若修改必须为大于0小于365的正整数），如果超过这个周期登录系统时，则在登录时就会提醒修改密码对话框');

insert into sys_config values(9, '用户管理-密码字符范围',         'sys.account.chrtype',              '0',             'Y', 'admin', sysdate(), '', null, '默认任意字符范围，0任意（密码可以输入任意字符），1数字（密码只能为0-9数字），2英文字母（密码只能为a-z和A-Z字母），3字母和数字（密码必须包含字母，数字）,4字母数字和特殊字符（目前支持的特殊字符包括：~!@#$%^&*()-=_+）');

create table sys_logininfor (
  info_id        bigint(20)     not null auto_increment   comment '访问ID',
  user_name      varchar(50)    default ''                comment '用户账号',
  ipaddr         varchar(128)   default ''                comment '登录IP地址',
  login_location varchar(255)   default ''                comment '登录地点',
  browser        varchar(50)    default ''                comment '浏览器类型',
  os             varchar(50)    default ''                comment '操作系统',
  status         char(1)        default '0'               comment '登录状态（0成功 1失败）',
  msg            varchar(255)   default ''                comment '提示消息',
  login_time     datetime                                 comment '访问时间',
  primary key (info_id),
  key idx_sys_logininfor_s  (status),
  key idx_sys_logininfor_lt (login_time)
) engine=innodb auto_increment=100 comment = '系统访问记录';

create table sys_job (
  job_id              bigint(20)    not null auto_increment    comment '任务ID',
  job_name            varchar(64)   default ''                 comment '任务名称',
  job_group           varchar(64)   default 'DEFAULT'          comment '任务组名',
  invoke_target       varchar(500)  not null                   comment '调用目标字符串',
  cron_expression     varchar(255)  default ''                 comment 'cron执行表达式',
  misfire_policy      varchar(20)   default '3'                comment '计划执行错误策略（1立即执行 2执行一次 3放弃执行）',
  concurrent          char(1)       default '1'                comment '是否并发执行（0允许 1禁止）',
  status              char(1)       default '0'                comment '状态（0正常 1暂停）',
  create_by           varchar(64)   default ''                 comment '创建者',
  create_time         datetime                                 comment '创建时间',
  update_by           varchar(64)   default ''                 comment '更新者',
  update_time         datetime                                 comment '更新时间',
  remark              varchar(500)  default ''                 comment '备注信息',
  primary key (job_id, job_name, job_group)
) engine=innodb auto_increment=100 comment = '定时任务调度表';

create table sys_job_log (
  job_log_id          bigint(20)     not null auto_increment    comment '任务日志ID',
  job_name            varchar(64)    not null                   comment '任务名称',
  job_group           varchar(64)    not null                   comment '任务组名',
  invoke_target       varchar(500)   not null                   comment '调用目标字符串',
  job_message         varchar(500)                              comment '日志信息',
  status              char(1)        default '0'                comment '执行状态（0正常 1失败）',
  exception_info      varchar(2000)  default ''                 comment '异常信息',
  start_time          datetime                                  comment '执行开始时间',
  end_time            datetime                                  comment '执行结束时间',
  create_time         datetime                                  comment '创建时间',
  primary key (job_log_id)
) engine=innodb comment = '定时任务调度日志表';

create table sys_notice (
  notice_id         int(4)          not null auto_increment    comment '公告ID',
  notice_title      varchar(50)     not null                   comment '公告标题',
  notice_type       char(1)         not null                   comment '公告类型（1通知 2公告）',
  notice_content    longblob        default null               comment '公告内容',
  status            char(1)         default '0'                comment '公告状态（0正常 1关闭）',
  create_by         varchar(64)     default ''                 comment '创建者',
  create_time       datetime                                   comment '创建时间',
  update_by         varchar(64)     default ''                 comment '更新者',
  update_time       datetime                                   comment '更新时间',
  remark            varchar(255)    default null               comment '备注',
  primary key (notice_id)
) engine=innodb auto_increment=10 comment = '通知公告表';

create table sys_notice_read (
  read_id          bigint(20)       not null auto_increment    comment '已读主键',
  notice_id        int(4)           not null                   comment '公告id',
  user_id          bigint(20)       not null                   comment '用户id',
  read_time        datetime         not null                   comment '阅读时间',
  primary key (read_id),
  unique key uk_user_notice (user_id, notice_id)   comment '同一用户同一公告只记录一次'
) engine=innodb auto_increment=1 comment='公告已读记录表';

create table gen_table (
  table_id          bigint(20)      not null auto_increment    comment '编号',
  table_name        varchar(200)    default ''                 comment '表名称',
  table_comment     varchar(500)    default ''                 comment '表描述',
  sub_table_name    varchar(64)     default null               comment '关联子表的表名',
  sub_table_fk_name varchar(64)     default null               comment '子表关联的外键名',
  class_name        varchar(100)    default ''                 comment '实体类名称',
  tpl_category      varchar(200)    default 'crud'             comment '使用的模板（crud单表操作 tree树表操作）',
  tpl_web_type      varchar(30)     default ''                 comment '前端模板类型（element-ui模版 element-plus模版）',
  package_name      varchar(100)                               comment '生成包路径',
  module_name       varchar(30)                                comment '生成模块名',
  business_name     varchar(30)                                comment '生成业务名',
  function_name     varchar(50)                                comment '生成功能名',
  function_author   varchar(50)                                comment '生成功能作者',
  form_col_num      int(1)          default 1                  comment '表单布局（单列 双列 三列）',
  gen_type          char(1)         default '0'                comment '生成代码方式（0zip压缩包 1自定义路径）',
  gen_path          varchar(200)    default '/'                comment '生成路径（不填默认项目路径）',
  options           varchar(1000)                              comment '其它生成选项',
  create_by         varchar(64)     default ''                 comment '创建者',
  create_time 	    datetime                                   comment '创建时间',
  update_by         varchar(64)     default ''                 comment '更新者',
  update_time       datetime                                   comment '更新时间',
  remark            varchar(500)    default null               comment '备注',
  primary key (table_id)
) engine=innodb auto_increment=1 comment = '代码生成业务表';

create table gen_table_column (
  column_id         bigint(20)      not null auto_increment    comment '编号',
  table_id          bigint(20)                                 comment '归属表编号',
  column_name       varchar(200)                               comment '列名称',
  column_comment    varchar(500)                               comment '列描述',
  column_type       varchar(100)                               comment '列类型',
  java_type         varchar(500)                               comment 'JAVA类型',
  java_field        varchar(200)                               comment 'JAVA字段名',
  is_pk             char(1)                                    comment '是否主键（1是）',
  is_increment      char(1)                                    comment '是否自增（1是）',
  is_required       char(1)                                    comment '是否必填（1是）',
  is_insert         char(1)                                    comment '是否为插入字段（1是）',
  is_edit           char(1)                                    comment '是否编辑字段（1是）',
  is_list           char(1)                                    comment '是否列表字段（1是）',
  is_query          char(1)                                    comment '是否查询字段（1是）',
  query_type        varchar(200)    default 'EQ'               comment '查询方式（等于、不等于、大于、小于、范围）',
  html_type         varchar(200)                               comment '显示类型（文本框、文本域、下拉框、复选框、单选框、日期控件）',
  dict_type         varchar(200)    default ''                 comment '字典类型',
  sort              int                                        comment '排序',
  create_by         varchar(64)     default ''                 comment '创建者',
  create_time 	    datetime                                   comment '创建时间',
  update_by         varchar(64)     default ''                 comment '更新者',
  update_time       datetime                                   comment '更新时间',
  primary key (column_id)
) engine=innodb auto_increment=1 comment = '代码生成业务表字段';

-- Minimal customer A identity, before tenant columns are applied.
INSERT INTO sys_dept(dept_id,parent_id,ancestors,dept_name,order_num,status,del_flag,create_by,create_time) VALUES(100,0,'0','示例企业 A',0,'0','0','erp-test-init',NOW());
INSERT INTO sys_role(role_id,role_name,role_key,role_sort,data_scope,menu_check_strictly,dept_check_strictly,status,del_flag,create_by,create_time) VALUES(1,'企业管理员','tenant_admin',0,'1',0,0,'0','0','erp-test-init',NOW());
INSERT INTO sys_post(post_id,post_code,post_name,post_sort,status,create_by,create_time) VALUES(1,'employee','普通员工',0,'0','erp-test-init',NOW());
INSERT INTO sys_user(user_id,dept_id,user_name,nick_name,user_type,password,status,del_flag,pwd_update_date,create_by,create_time) VALUES(1,100,'admin','企业 A 管理员','01','$2a$10$7btkK05hL1td67w8CPzg9OArpsLiJQL..R6oPws1NXB2I8P7zMGk6','0','0',NOW(),'erp-test-init',NOW());
INSERT INTO sys_user_role(user_id,role_id) VALUES(1,1);
INSERT INTO sys_user_post(user_id,post_id) VALUES(1,1);
UPDATE sys_config SET config_value='Test@123456',remark='本地测试初始密码；不用于生产' WHERE config_key='sys.user.initPassword';

-- SOURCE: sql/multitenant/002_tenants.sql
CREATE TABLE sys_tenant (
 tenant_id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
 tenant_code VARCHAR(32) NOT NULL,
 tenant_name VARCHAR(100) NOT NULL,
 tenant_kind CHAR(2) NOT NULL DEFAULT '01',
 owner_user_id BIGINT NULL,
 status CHAR(1) NOT NULL DEFAULT '2',
 version BIGINT NOT NULL DEFAULT 1,
 expire_time DATETIME NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '', create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '', update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 UNIQUE KEY uk_tenant_code(tenant_code)
) ENGINE=InnoDB COMMENT='客户工作空间；00平台，01客户；状态2初始化';
INSERT INTO sys_tenant(tenant_id,tenant_code,tenant_name,tenant_kind,status,create_time)
VALUES(1,'platform','平台管理','00','0',NOW()),(10000,'demo','示例企业 A','01','0',NOW());

ALTER TABLE sys_user ADD tenant_id BIGINT NOT NULL DEFAULT 10000;
ALTER TABLE sys_role ADD tenant_id BIGINT NOT NULL DEFAULT 10000;
ALTER TABLE sys_dept ADD tenant_id BIGINT NOT NULL DEFAULT 10000;
ALTER TABLE sys_post ADD tenant_id BIGINT NOT NULL DEFAULT 10000;
ALTER TABLE sys_config ADD tenant_id BIGINT NOT NULL DEFAULT 10000;
ALTER TABLE sys_dict_type ADD tenant_id BIGINT NOT NULL DEFAULT 10000;
ALTER TABLE sys_dict_data ADD tenant_id BIGINT NOT NULL DEFAULT 10000;
ALTER TABLE sys_notice ADD tenant_id BIGINT NOT NULL DEFAULT 10000;
ALTER TABLE sys_notice_read ADD tenant_id BIGINT NOT NULL DEFAULT 10000;
ALTER TABLE sys_user_role ADD tenant_id BIGINT NOT NULL DEFAULT 10000;
ALTER TABLE sys_user_post ADD tenant_id BIGINT NOT NULL DEFAULT 10000;
ALTER TABLE sys_role_menu ADD tenant_id BIGINT NOT NULL DEFAULT 10000;
ALTER TABLE sys_role_dept ADD tenant_id BIGINT NOT NULL DEFAULT 10000;
ALTER TABLE sys_job ADD tenant_id BIGINT NOT NULL DEFAULT 10000;
ALTER TABLE sys_job_log ADD tenant_id BIGINT NOT NULL DEFAULT 10000;
ALTER TABLE sys_oper_log ADD tenant_id BIGINT NOT NULL DEFAULT 10000;
ALTER TABLE sys_logininfor ADD tenant_id BIGINT NOT NULL DEFAULT 10000;

ALTER TABLE sys_user
 ADD active_guard TINYINT GENERATED ALWAYS AS (CASE WHEN del_flag='0' THEN 1 ELSE NULL END) STORED,
 ADD active_phone VARCHAR(11) GENERATED ALWAYS AS (CASE WHEN del_flag='0' THEN NULLIF(phonenumber,'') ELSE NULL END) STORED,
 ADD active_email VARCHAR(50) GENERATED ALWAYS AS (CASE WHEN del_flag='0' THEN NULLIF(email,'') ELSE NULL END) STORED,
 ADD UNIQUE KEY uk_user_tenant_id(tenant_id,user_id),
 ADD UNIQUE KEY uk_user_tenant_name(tenant_id,user_name,active_guard),
 ADD UNIQUE KEY uk_user_tenant_phone(tenant_id,active_phone),
 ADD UNIQUE KEY uk_user_tenant_email(tenant_id,active_email),
 ADD KEY idx_user_tenant_dept(tenant_id,dept_id,del_flag),
 ALTER tenant_id DROP DEFAULT;
ALTER TABLE sys_role
 ADD active_guard TINYINT GENERATED ALWAYS AS (CASE WHEN del_flag='0' THEN 1 ELSE NULL END) STORED,
 ADD UNIQUE KEY uk_role_tenant_id(tenant_id,role_id),
 ADD UNIQUE KEY uk_role_tenant_key(tenant_id,role_key,active_guard),
 ADD UNIQUE KEY uk_role_tenant_name(tenant_id,role_name,active_guard),
 ALTER tenant_id DROP DEFAULT;
ALTER TABLE sys_dept ADD UNIQUE KEY uk_dept_tenant_id(tenant_id,dept_id),
 ADD KEY idx_dept_tenant_parent(tenant_id,parent_id), ALTER tenant_id DROP DEFAULT;
ALTER TABLE sys_post ADD UNIQUE KEY uk_post_tenant_id(tenant_id,post_id),
 ADD UNIQUE KEY uk_post_tenant_code(tenant_id,post_code),
 ADD UNIQUE KEY uk_post_tenant_name(tenant_id,post_name), ALTER tenant_id DROP DEFAULT;
ALTER TABLE sys_config ADD UNIQUE KEY uk_config_tenant_key(tenant_id,config_key), ALTER tenant_id DROP DEFAULT;
ALTER TABLE sys_dict_type DROP INDEX dict_type,
 ADD UNIQUE KEY uk_dict_tenant_type(tenant_id,dict_type), ALTER tenant_id DROP DEFAULT;
ALTER TABLE sys_dict_data ADD KEY idx_dict_tenant_type(tenant_id,dict_type,status,dict_sort), ALTER tenant_id DROP DEFAULT;
ALTER TABLE sys_notice ADD UNIQUE KEY uk_notice_tenant_id(tenant_id,notice_id), ALTER tenant_id DROP DEFAULT;
ALTER TABLE sys_notice_read DROP INDEX uk_user_notice,
 ADD UNIQUE KEY uk_notice_read_tenant(tenant_id,user_id,notice_id), ALTER tenant_id DROP DEFAULT;
ALTER TABLE sys_job ADD UNIQUE KEY uk_job_tenant_id(tenant_id,job_id), ALTER tenant_id DROP DEFAULT;
ALTER TABLE sys_job_log ADD job_id BIGINT NULL, ADD KEY idx_joblog_tenant_id(tenant_id,job_log_id), ALTER tenant_id DROP DEFAULT;
ALTER TABLE sys_oper_log ADD KEY idx_operlog_tenant_id(tenant_id,oper_id), ALTER tenant_id DROP DEFAULT;
ALTER TABLE sys_logininfor ADD KEY idx_logininfor_tenant_id(tenant_id,info_id), ALTER tenant_id DROP DEFAULT;

CREATE TABLE sys_tenant_menu (
 tenant_id BIGINT NOT NULL,menu_id BIGINT NOT NULL,
 PRIMARY KEY(tenant_id,menu_id),
 CONSTRAINT fk_tm_tenant FOREIGN KEY(tenant_id) REFERENCES sys_tenant(tenant_id),
 CONSTRAINT fk_tm_menu FOREIGN KEY(menu_id) REFERENCES sys_menu(menu_id)
) ENGINE=InnoDB;
ALTER TABLE sys_user_role DROP PRIMARY KEY, ADD PRIMARY KEY(tenant_id,user_id,role_id),
 ADD KEY idx_ur_tenant_role(tenant_id,role_id,user_id), ALTER tenant_id DROP DEFAULT,
 ADD CONSTRAINT fk_ur_user FOREIGN KEY(tenant_id,user_id) REFERENCES sys_user(tenant_id,user_id),
 ADD CONSTRAINT fk_ur_role FOREIGN KEY(tenant_id,role_id) REFERENCES sys_role(tenant_id,role_id);
ALTER TABLE sys_user_post DROP PRIMARY KEY, ADD PRIMARY KEY(tenant_id,user_id,post_id), ALTER tenant_id DROP DEFAULT,
 ADD CONSTRAINT fk_up_user FOREIGN KEY(tenant_id,user_id) REFERENCES sys_user(tenant_id,user_id),
 ADD CONSTRAINT fk_up_post FOREIGN KEY(tenant_id,post_id) REFERENCES sys_post(tenant_id,post_id);
ALTER TABLE sys_role_dept DROP PRIMARY KEY, ADD PRIMARY KEY(tenant_id,role_id,dept_id), ALTER tenant_id DROP DEFAULT,
 ADD CONSTRAINT fk_rd_role FOREIGN KEY(tenant_id,role_id) REFERENCES sys_role(tenant_id,role_id),
 ADD CONSTRAINT fk_rd_dept FOREIGN KEY(tenant_id,dept_id) REFERENCES sys_dept(tenant_id,dept_id);
ALTER TABLE sys_role_menu DROP PRIMARY KEY, ADD PRIMARY KEY(tenant_id,role_id,menu_id), ALTER tenant_id DROP DEFAULT,
 ADD CONSTRAINT fk_rm_role FOREIGN KEY(tenant_id,role_id) REFERENCES sys_role(tenant_id,role_id);

CREATE TABLE sys_file (
 file_id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,tenant_id BIGINT NOT NULL,
 storage_key VARCHAR(255) NOT NULL,original_name VARCHAR(255) NOT NULL,
 size_bytes BIGINT NOT NULL,content_type VARCHAR(100) NOT NULL DEFAULT '',
 uploader_user_id BIGINT NOT NULL,visibility CHAR(1) NOT NULL DEFAULT '0',
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 UNIQUE KEY uk_file_storage(tenant_id,storage_key),
 CONSTRAINT fk_file_tenant FOREIGN KEY(tenant_id) REFERENCES sys_tenant(tenant_id)
) ENGINE=InnoDB;

UPDATE sys_user SET user_type='01' WHERE user_id=1;
UPDATE sys_user SET status='1' WHERE user_id<>1;
UPDATE sys_job SET status='1';
UPDATE sys_role SET role_key='tenant_admin' WHERE role_id=1;
UPDATE sys_tenant SET owner_user_id=1 WHERE tenant_id=10000;
INSERT INTO sys_config(tenant_id,config_name,config_key,config_value,config_type,create_by,create_time,remark)
 SELECT 1,config_name,config_key,config_value,config_type,'bootstrap',NOW(),remark FROM sys_config WHERE tenant_id=10000;
INSERT INTO sys_dict_type(tenant_id,dict_name,dict_type,status,create_by,create_time,remark)
 SELECT 1,dict_name,dict_type,status,'bootstrap',NOW(),remark FROM sys_dict_type WHERE tenant_id=10000;
INSERT INTO sys_dict_data(tenant_id,dict_sort,dict_label,dict_value,dict_type,css_class,list_class,is_default,status,create_by,create_time,remark)
 SELECT 1,dict_sort,dict_label,dict_value,dict_type,css_class,list_class,is_default,status,'bootstrap',NOW(),remark FROM sys_dict_data WHERE tenant_id=10000;
INSERT INTO sys_menu(menu_id,menu_name,parent_id,order_num,path,component,is_frame,is_cache,menu_type,visible,status,perms,icon,create_by,create_time)
 VALUES(5000,'租户管理',1,0,'tenant','platform/tenant/index',1,0,'C','0','0','platform:tenant:list','peoples','bootstrap',NOW());
INSERT INTO sys_tenant_menu SELECT 1,menu_id FROM sys_menu;
INSERT INTO sys_tenant_menu SELECT 10000,menu_id FROM sys_menu WHERE
 COALESCE(perms,'') NOT LIKE 'platform:%' AND COALESCE(perms,'') NOT LIKE 'tool:%'
 AND COALESCE(perms,'') NOT LIKE 'system:menu:%' AND COALESCE(perms,'') NOT LIKE 'monitor:cache:%'
 AND COALESCE(perms,'') NOT LIKE 'monitor:server:%'
 AND COALESCE(perms,'') NOT LIKE 'monitor:online:%'
 AND path NOT IN('tenant','tool','gen','menu','online','cache','cacheList','server','druid','swagger');
DELETE rm FROM sys_role_menu rm LEFT JOIN sys_tenant_menu tm ON tm.tenant_id=rm.tenant_id AND tm.menu_id=rm.menu_id WHERE tm.menu_id IS NULL;
ALTER TABLE sys_role_menu ADD CONSTRAINT fk_rm_grant FOREIGN KEY(tenant_id,menu_id) REFERENCES sys_tenant_menu(tenant_id,menu_id);
ALTER TABLE sys_notice_read ADD CONSTRAINT fk_nr_user FOREIGN KEY(tenant_id,user_id) REFERENCES sys_user(tenant_id,user_id),
 ADD CONSTRAINT fk_nr_notice FOREIGN KEY(tenant_id,notice_id) REFERENCES sys_notice(tenant_id,notice_id);
UPDATE sys_user SET dept_id=NULL WHERE dept_id=0;
ALTER TABLE sys_user ADD CONSTRAINT fk_user_dept FOREIGN KEY(tenant_id,dept_id) REFERENCES sys_dept(tenant_id,dept_id);

-- SOURCE: sql/multitenant/003_tenant_policy.sql
CREATE TABLE IF NOT EXISTS sys_tenant_policy (
 tenant_id BIGINT NOT NULL PRIMARY KEY,
 max_users BIGINT NOT NULL DEFAULT 100,
 max_storage_bytes BIGINT NOT NULL DEFAULT 1073741824,
 max_upload_bytes BIGINT NOT NULL DEFAULT 10485760,
 create_by VARCHAR(64) NOT NULL DEFAULT '', create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '', update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 CONSTRAINT fk_policy_tenant FOREIGN KEY(tenant_id) REFERENCES sys_tenant(tenant_id)
) ENGINE=InnoDB COMMENT='Tenant limits; zero means unlimited, upload still has server cap';

-- SOURCE: sql/erp/001_core.sql
CREATE TABLE IF NOT EXISTS erp_sku (
 sku_id BIGINT NOT NULL AUTO_INCREMENT COMMENT 'SKU编号',
 tenant_id BIGINT NOT NULL,
 sku_code VARCHAR(64) NOT NULL COMMENT 'SKU编码',
 product_name VARCHAR(120) NOT NULL COMMENT '商品名称',
 specification VARCHAR(120) NOT NULL DEFAULT '' COMMENT '规格',
 unit VARCHAR(20) NOT NULL DEFAULT '件' COMMENT '单位',
 status CHAR(1) NOT NULL DEFAULT '0' COMMENT '状态',
 create_by VARCHAR(64) DEFAULT '', create_time DATETIME,
 update_by VARCHAR(64) DEFAULT '', update_time DATETIME, remark VARCHAR(500),
 PRIMARY KEY(sku_id), UNIQUE KEY uk_erp_sku_tenant_id(tenant_id,sku_id),
 UNIQUE KEY uk_erp_sku_code(tenant_id,sku_code), KEY ix_erp_sku_name(tenant_id,product_name),
 CONSTRAINT fk_erp_sku_tenant FOREIGN KEY(tenant_id) REFERENCES sys_tenant(tenant_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='商品SKU';
CREATE TABLE IF NOT EXISTS erp_warehouse (
 warehouse_id BIGINT NOT NULL AUTO_INCREMENT COMMENT '仓库编号',
 tenant_id BIGINT NOT NULL,
 warehouse_code VARCHAR(64) NOT NULL COMMENT '仓库编码',
 warehouse_name VARCHAR(120) NOT NULL COMMENT '仓库名称',
 status CHAR(1) NOT NULL DEFAULT '0' COMMENT '状态',
 create_by VARCHAR(64) DEFAULT '', create_time DATETIME,
 update_by VARCHAR(64) DEFAULT '', update_time DATETIME, remark VARCHAR(500),
 PRIMARY KEY(warehouse_id), UNIQUE KEY uk_erp_warehouse_tenant_id(tenant_id,warehouse_id),
 UNIQUE KEY uk_erp_warehouse_code(tenant_id,warehouse_code), KEY ix_erp_warehouse_name(tenant_id,warehouse_name),
 CONSTRAINT fk_erp_warehouse_tenant FOREIGN KEY(tenant_id) REFERENCES sys_tenant(tenant_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='仓库';
CREATE TABLE IF NOT EXISTS erp_document (
 document_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 document_no VARCHAR(64) NOT NULL,
 kind VARCHAR(8) NOT NULL,
 warehouse_id BIGINT NOT NULL,
 warehouse_name VARCHAR(120) NOT NULL,
 status VARCHAR(16) NOT NULL DEFAULT 'DRAFT',
 request_key VARCHAR(64) NOT NULL,
 request_hash CHAR(64) NOT NULL,
 create_by VARCHAR(64) DEFAULT '', create_time DATETIME,
 update_by VARCHAR(64) DEFAULT '', update_time DATETIME, remark VARCHAR(500),
 PRIMARY KEY(document_id), UNIQUE KEY uk_erp_document_id(tenant_id,document_id),
 UNIQUE KEY uk_erp_document_request(tenant_id,request_key), UNIQUE KEY uk_erp_document_no(tenant_id,document_no),
 KEY ix_erp_document_list(tenant_id,kind,document_id), KEY ix_erp_document_status(tenant_id,kind,status,document_id),
 KEY ix_erp_document_warehouse(tenant_id,warehouse_id),
 CONSTRAINT fk_erp_document_tenant FOREIGN KEY(tenant_id) REFERENCES sys_tenant(tenant_id),
 CONSTRAINT fk_erp_document_warehouse FOREIGN KEY(tenant_id,warehouse_id) REFERENCES erp_warehouse(tenant_id,warehouse_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='入库和销售单据';
CREATE TABLE IF NOT EXISTS erp_document_line (
 line_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 document_id BIGINT NOT NULL,
 sku_id BIGINT NOT NULL,
 sku_code VARCHAR(64) NOT NULL,
 product_name VARCHAR(120) NOT NULL,
 specification VARCHAR(120) NOT NULL,
 unit VARCHAR(20) NOT NULL,
 quantity BIGINT NOT NULL,
 create_by VARCHAR(64) DEFAULT '', create_time DATETIME,
 update_by VARCHAR(64) DEFAULT '', update_time DATETIME, remark VARCHAR(500),
 PRIMARY KEY(line_id), UNIQUE KEY uk_erp_document_sku(tenant_id,document_id,sku_id), KEY ix_erp_line_sku(tenant_id,sku_id),
 CONSTRAINT fk_erp_line_document FOREIGN KEY(tenant_id,document_id) REFERENCES erp_document(tenant_id,document_id),
 CONSTRAINT fk_erp_line_sku FOREIGN KEY(tenant_id,sku_id) REFERENCES erp_sku(tenant_id,sku_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='单据明细快照';
CREATE TABLE IF NOT EXISTS erp_stock (
 stock_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 warehouse_id BIGINT NOT NULL,
 sku_id BIGINT NOT NULL,
 on_hand BIGINT NOT NULL DEFAULT 0,
 reserved BIGINT NOT NULL DEFAULT 0,
 create_by VARCHAR(64) DEFAULT '', create_time DATETIME,
 update_by VARCHAR(64) DEFAULT '', update_time DATETIME, remark VARCHAR(500),
 PRIMARY KEY(stock_id), UNIQUE KEY uk_erp_stock(tenant_id,warehouse_id,sku_id), KEY ix_erp_stock_sku(tenant_id,sku_id),
 CONSTRAINT fk_erp_stock_warehouse FOREIGN KEY(tenant_id,warehouse_id) REFERENCES erp_warehouse(tenant_id,warehouse_id),
 CONSTRAINT fk_erp_stock_sku FOREIGN KEY(tenant_id,sku_id) REFERENCES erp_sku(tenant_id,sku_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='库存余额';
CREATE TABLE IF NOT EXISTS erp_stock_ledger (
 ledger_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 document_id BIGINT NOT NULL,
 document_no VARCHAR(64) NOT NULL,
 warehouse_id BIGINT NOT NULL,
 sku_id BIGINT NOT NULL,
 event VARCHAR(16) NOT NULL,
 on_hand_delta BIGINT NOT NULL,
 reserved_delta BIGINT NOT NULL,
 on_hand_after BIGINT NOT NULL,
 reserved_after BIGINT NOT NULL,
 create_by VARCHAR(64) DEFAULT '', create_time DATETIME,
 update_by VARCHAR(64) DEFAULT '', update_time DATETIME, remark VARCHAR(500),
 PRIMARY KEY(ledger_id), UNIQUE KEY uk_erp_ledger_event(tenant_id,document_id,sku_id,event),
 KEY ix_erp_ledger_stock(tenant_id,warehouse_id,sku_id,ledger_id), KEY ix_erp_ledger_document(tenant_id,document_id),
 CONSTRAINT fk_erp_ledger_document FOREIGN KEY(tenant_id,document_id) REFERENCES erp_document(tenant_id,document_id),
 CONSTRAINT fk_erp_ledger_warehouse FOREIGN KEY(tenant_id,warehouse_id) REFERENCES erp_warehouse(tenant_id,warehouse_id),
 CONSTRAINT fk_erp_ledger_sku FOREIGN KEY(tenant_id,sku_id) REFERENCES erp_sku(tenant_id,sku_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='不可变库存流水';

-- SOURCE: sql/erp/002_controls.sql
ALTER TABLE erp_document ADD COLUMN version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE erp_document ADD COLUMN source_document_id BIGINT NULL;
ALTER TABLE erp_document ADD COLUMN reversal TINYINT(1) NOT NULL DEFAULT 0;
ALTER TABLE erp_document ADD INDEX ix_erp_document_source (tenant_id,source_document_id,status,document_id);
ALTER TABLE erp_document ADD CONSTRAINT fk_erp_document_source FOREIGN KEY (tenant_id,source_document_id) REFERENCES erp_document(tenant_id,document_id);
ALTER TABLE erp_document_line ADD COLUMN expected_on_hand BIGINT NOT NULL DEFAULT 0;
ALTER TABLE erp_document_line ADD COLUMN expected_reserved BIGINT NOT NULL DEFAULT 0;
ALTER TABLE erp_document_line ADD COLUMN expected_version BIGINT NOT NULL DEFAULT 0;
ALTER TABLE erp_stock ADD COLUMN version BIGINT NOT NULL DEFAULT 0;
CREATE TABLE IF NOT EXISTS erp_warehouse_access (
 access_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 user_id BIGINT NOT NULL,
 warehouse_id BIGINT NOT NULL,
 create_by VARCHAR(64) DEFAULT '', create_time DATETIME,
 update_by VARCHAR(64) DEFAULT '', update_time DATETIME, remark VARCHAR(500),
 PRIMARY KEY(access_id), UNIQUE KEY uk_erp_access_user(tenant_id,user_id,warehouse_id),
 KEY ix_erp_access_warehouse(tenant_id,warehouse_id),
 CONSTRAINT fk_erp_access_user FOREIGN KEY(tenant_id,user_id) REFERENCES sys_user(tenant_id,user_id),
 CONSTRAINT fk_erp_access_warehouse FOREIGN KEY(tenant_id,warehouse_id) REFERENCES erp_warehouse(tenant_id,warehouse_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='员工仓库数据权限';
CREATE TABLE IF NOT EXISTS erp_warehouse_policy (
 policy_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL, user_id BIGINT NOT NULL, version BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) DEFAULT '', create_time DATETIME,
 update_by VARCHAR(64) DEFAULT '', update_time DATETIME, remark VARCHAR(500),
 PRIMARY KEY(policy_id), UNIQUE KEY uk_erp_policy_user(tenant_id,user_id),
 CONSTRAINT fk_erp_policy_user FOREIGN KEY(tenant_id,user_id) REFERENCES sys_user(tenant_id,user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='仓库授权持久版本';

-- SOURCE: sql/erp/003_order_fulfillment.sql
CREATE TABLE erp_shop (
 shop_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 shop_code VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 shop_name VARCHAR(120) NOT NULL,
 channel_code VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 site_code VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 currency CHAR(3) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 status CHAR(1) NOT NULL DEFAULT '0',
 version BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (shop_id),
 UNIQUE KEY uk_p1_shop_id (tenant_id,shop_id),
 UNIQUE KEY uk_p1_shop_code (tenant_id,shop_code),
 KEY ix_p1_shop_status (tenant_id,status,shop_id),
 CONSTRAINT fk_p1_shop_tenant FOREIGN KEY (tenant_id) REFERENCES sys_tenant(tenant_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_shop_access (
 access_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 user_id BIGINT NOT NULL,
 shop_id BIGINT NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (access_id),
 UNIQUE KEY uk_p1_access_user (tenant_id,user_id,shop_id),
 KEY ix_p1_access_shop (tenant_id,shop_id),
 CONSTRAINT fk_p1_access_user FOREIGN KEY (tenant_id,user_id) REFERENCES sys_user(tenant_id,user_id),
 CONSTRAINT fk_p1_access_shop FOREIGN KEY (tenant_id,shop_id) REFERENCES erp_shop(tenant_id,shop_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_shop_policy (
 policy_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 user_id BIGINT NOT NULL,
 version BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (policy_id),
 UNIQUE KEY uk_p1_policy_user (tenant_id,user_id),
 CONSTRAINT fk_p1_policy_user FOREIGN KEY (tenant_id,user_id) REFERENCES sys_user(tenant_id,user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_shop_item (
 item_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 shop_id BIGINT NOT NULL,
 shop_item_code VARCHAR(128) COLLATE utf8mb4_bin NOT NULL,
 external_product_id VARCHAR(128) COLLATE utf8mb4_bin NULL,
 external_variant_id VARCHAR(128) COLLATE utf8mb4_bin NULL,
 title VARCHAR(200) NOT NULL,
 status CHAR(1) NOT NULL DEFAULT '0',
 version BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (item_id),
 UNIQUE KEY uk_p1_item_id (tenant_id,item_id),
 UNIQUE KEY uk_p1_item_shop_id (tenant_id,shop_id,item_id),
 UNIQUE KEY uk_p1_item_code (tenant_id,shop_id,shop_item_code),
 UNIQUE KEY uk_p1_item_variant (tenant_id,shop_id,external_variant_id),
 KEY ix_p1_item_status (tenant_id,shop_id,status,item_id),
 CONSTRAINT fk_p1_item_shop FOREIGN KEY (tenant_id,shop_id) REFERENCES erp_shop(tenant_id,shop_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_shop_sku_mapping (
 mapping_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 shop_id BIGINT NOT NULL,
 shop_item_id BIGINT NOT NULL,
 sku_id BIGINT NOT NULL,
 status CHAR(1) NOT NULL DEFAULT '0',
 version BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (mapping_id),
 UNIQUE KEY uk_p1_mapping_item (tenant_id,shop_item_id),
 UNIQUE KEY uk_p1_mapping_id (tenant_id,shop_item_id,mapping_id),
 KEY ix_p1_mapping_item_scope (tenant_id,shop_id,shop_item_id),
 KEY ix_p1_mapping_sku (tenant_id,sku_id),
 CONSTRAINT fk_p1_mapping_item FOREIGN KEY (tenant_id,shop_id,shop_item_id) REFERENCES erp_shop_item(tenant_id,shop_id,item_id),
 CONSTRAINT fk_p1_mapping_sku FOREIGN KEY (tenant_id,sku_id) REFERENCES erp_sku(tenant_id,sku_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_order (
 order_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 shop_id BIGINT NOT NULL,
 order_no VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 source_type VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 source_order_no VARCHAR(128) COLLATE utf8mb4_bin NOT NULL,
 create_request_key VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 create_request_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 status VARCHAR(16) NOT NULL DEFAULT 'PENDING_REVIEW',
 version BIGINT NOT NULL DEFAULT 1,
 receiver_name VARCHAR(120) NOT NULL,
 receiver_phone VARCHAR(64) NOT NULL DEFAULT '',
 receiver_country CHAR(2) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 receiver_region VARCHAR(120) NOT NULL DEFAULT '',
 receiver_city VARCHAR(120) NOT NULL DEFAULT '',
 receiver_postcode VARCHAR(32) NOT NULL DEFAULT '',
 receiver_address VARCHAR(500) NOT NULL,
 currency CHAR(3) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 original_amount DECIMAL(20,4) NULL,
 ordered_at DATETIME NOT NULL,
 warehouse_id BIGINT NULL,
 stock_document_id BIGINT NULL,
 reviewed_at DATETIME NULL,
 shipped_at DATETIME NULL,
 cancelled_at DATETIME NULL,
 cancel_reason VARCHAR(500) NOT NULL DEFAULT '',
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (order_id),
 UNIQUE KEY uk_p1_order_id (tenant_id,order_id),
 UNIQUE KEY uk_p1_order_shop_id (tenant_id,shop_id,order_id),
 UNIQUE KEY uk_p1_order_warehouse_id (tenant_id,order_id,warehouse_id),
 UNIQUE KEY uk_p1_order_source (tenant_id,shop_id,source_order_no),
 UNIQUE KEY uk_p1_order_request (tenant_id,create_request_key),
 UNIQUE KEY uk_p1_order_no (tenant_id,order_no),
 UNIQUE KEY uk_p1_order_stock (tenant_id,stock_document_id),
 KEY ix_p1_order_list (tenant_id,shop_id,status,order_id),
 KEY ix_p1_order_status (tenant_id,status,order_id),
 KEY ix_p1_order_warehouse (tenant_id,warehouse_id,order_id),
 CONSTRAINT fk_p1_order_shop FOREIGN KEY (tenant_id,shop_id) REFERENCES erp_shop(tenant_id,shop_id),
 CONSTRAINT fk_p1_order_warehouse FOREIGN KEY (tenant_id,warehouse_id) REFERENCES erp_warehouse(tenant_id,warehouse_id),
 CONSTRAINT fk_p1_order_stock FOREIGN KEY (tenant_id,stock_document_id) REFERENCES erp_document(tenant_id,document_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_order_line (
 line_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 shop_id BIGINT NOT NULL,
 order_id BIGINT NOT NULL,
 source_line_no VARCHAR(128) COLLATE utf8mb4_bin NOT NULL,
 shop_item_code VARCHAR(128) COLLATE utf8mb4_bin NOT NULL,
 source_title VARCHAR(200) NOT NULL,
 quantity BIGINT NOT NULL,
 shop_item_id BIGINT NULL,
 mapping_id BIGINT NULL,
 mapping_version BIGINT NULL,
 sku_id BIGINT NULL,
 sku_code VARCHAR(64) NULL,
 product_name VARCHAR(120) NULL,
 specification VARCHAR(120) NULL,
 unit VARCHAR(20) NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (line_id),
 UNIQUE KEY uk_p1_line_order_id (tenant_id,order_id,line_id),
 UNIQUE KEY uk_p1_line_source (tenant_id,order_id,source_line_no),
 UNIQUE KEY uk_p1_line_resolved (tenant_id,order_id,line_id,sku_id),
 KEY ix_p1_line_order_scope (tenant_id,shop_id,order_id),
 KEY ix_p1_line_item (tenant_id,shop_id,shop_item_id),
 KEY ix_p1_line_mapping (tenant_id,shop_item_id,mapping_id),
 KEY ix_p1_line_sku (tenant_id,sku_id),
 CONSTRAINT fk_p1_line_order FOREIGN KEY (tenant_id,shop_id,order_id) REFERENCES erp_order(tenant_id,shop_id,order_id),
 CONSTRAINT fk_p1_line_item FOREIGN KEY (tenant_id,shop_id,shop_item_id) REFERENCES erp_shop_item(tenant_id,shop_id,item_id),
 CONSTRAINT fk_p1_line_mapping FOREIGN KEY (tenant_id,shop_item_id,mapping_id) REFERENCES erp_shop_sku_mapping(tenant_id,shop_item_id,mapping_id),
 CONSTRAINT fk_p1_line_sku FOREIGN KEY (tenant_id,sku_id) REFERENCES erp_sku(tenant_id,sku_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_order_event (
 event_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 order_id BIGINT NOT NULL,
 actor_user_id BIGINT NOT NULL,
 action VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_key VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 from_status VARCHAR(16) NOT NULL,
 to_status VARCHAR(16) NOT NULL,
 from_version BIGINT NOT NULL,
 to_version BIGINT NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (event_id),
 UNIQUE KEY uk_p1_event_request (tenant_id,order_id,action,request_key),
 KEY ix_p1_event_order (tenant_id,order_id,event_id),
 KEY ix_p1_event_actor (tenant_id,actor_user_id),
 CONSTRAINT fk_p1_event_order FOREIGN KEY (tenant_id,order_id) REFERENCES erp_order(tenant_id,order_id),
 CONSTRAINT fk_p1_event_actor FOREIGN KEY (tenant_id,actor_user_id) REFERENCES sys_user(tenant_id,user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_package (
 package_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 order_id BIGINT NOT NULL,
 warehouse_id BIGINT NOT NULL,
 package_no VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 status VARCHAR(16) NOT NULL DEFAULT 'READY_TO_SHIP',
 version BIGINT NOT NULL DEFAULT 1,
 carrier_code VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
 tracking_no VARCHAR(128) COLLATE utf8mb4_bin NOT NULL DEFAULT '',
 shipped_at DATETIME NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (package_id),
 UNIQUE KEY uk_p1_package_id (tenant_id,order_id,package_id),
 UNIQUE KEY uk_p1_package_order (tenant_id,order_id),
 UNIQUE KEY uk_p1_package_no (tenant_id,package_no),
 KEY ix_p1_package_order_scope (tenant_id,order_id,warehouse_id),
 KEY ix_p1_package_list (tenant_id,warehouse_id,status,package_id),
 CONSTRAINT fk_p1_package_order FOREIGN KEY (tenant_id,order_id,warehouse_id) REFERENCES erp_order(tenant_id,order_id,warehouse_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_package_line (
 line_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 order_id BIGINT NOT NULL,
 package_id BIGINT NOT NULL,
 order_line_id BIGINT NOT NULL,
 sku_id BIGINT NOT NULL,
 quantity BIGINT NOT NULL,
 sku_code VARCHAR(64) NOT NULL,
 product_name VARCHAR(120) NOT NULL,
 unit VARCHAR(20) NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (line_id),
 UNIQUE KEY uk_p1_package_line (tenant_id,package_id,order_line_id),
 KEY ix_p1_package_line_scope (tenant_id,order_id,package_id),
 KEY ix_p1_package_line_order (tenant_id,order_id,order_line_id,sku_id),
 CONSTRAINT fk_p1_package_line_package FOREIGN KEY (tenant_id,order_id,package_id) REFERENCES erp_package(tenant_id,order_id,package_id),
 CONSTRAINT fk_p1_package_line_order FOREIGN KEY (tenant_id,order_id,order_line_id,sku_id) REFERENCES erp_order_line(tenant_id,order_id,line_id,sku_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_import_job (
 job_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 shop_id BIGINT NOT NULL,
 actor_user_id BIGINT NOT NULL,
 request_key VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 status VARCHAR(16) NOT NULL DEFAULT 'PENDING',
 version BIGINT NOT NULL DEFAULT 1,
 attempt BIGINT NOT NULL DEFAULT 0,
 file_key VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 file_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 file_bytes BIGINT NOT NULL,
 file_state VARCHAR(16) NOT NULL DEFAULT 'PRESENT',
 retain_until DATETIME NOT NULL,
 total_rows BIGINT NOT NULL DEFAULT 0,
 created_orders BIGINT NOT NULL DEFAULT 0,
 existing_orders BIGINT NOT NULL DEFAULT 0,
 failed_orders BIGINT NOT NULL DEFAULT 0,
 started_at DATETIME NULL,
 finished_at DATETIME NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (job_id),
 UNIQUE KEY uk_p1_job_id (tenant_id,shop_id,job_id),
 UNIQUE KEY uk_p1_job_request (tenant_id,actor_user_id,request_key),
 UNIQUE KEY uk_p1_job_file (tenant_id,file_key),
 KEY ix_p1_job_actor (tenant_id,actor_user_id,status,job_id),
 KEY ix_p1_job_shop (tenant_id,shop_id,status,job_id),
 KEY ix_p1_job_quota (tenant_id,file_state,file_bytes),
 KEY ix_p1_job_retention (file_state,retain_until,job_id),
 CONSTRAINT fk_p1_job_shop FOREIGN KEY (tenant_id,shop_id) REFERENCES erp_shop(tenant_id,shop_id),
 CONSTRAINT fk_p1_job_actor FOREIGN KEY (tenant_id,actor_user_id) REFERENCES sys_user(tenant_id,user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_import_result (
 result_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 shop_id BIGINT NOT NULL,
 job_id BIGINT NOT NULL,
 source_row BIGINT NOT NULL,
 source_order_no VARCHAR(128) COLLATE utf8mb4_bin NOT NULL DEFAULT '',
 input_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 result_code VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 error_summary VARCHAR(500) NOT NULL DEFAULT '',
 order_id BIGINT NULL,
 attempt BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (result_id),
 UNIQUE KEY uk_p1_result_row (tenant_id,job_id,source_row),
 KEY ix_p1_result_job_scope (tenant_id,shop_id,job_id),
 KEY ix_p1_result_order (tenant_id,shop_id,order_id),
 CONSTRAINT fk_p1_result_job FOREIGN KEY (tenant_id,shop_id,job_id) REFERENCES erp_import_job(tenant_id,shop_id,job_id),
 CONSTRAINT fk_p1_result_order FOREIGN KEY (tenant_id,shop_id,order_id) REFERENCES erp_order(tenant_id,shop_id,order_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_schema_migration (
 migration_id BIGINT NOT NULL AUTO_INCREMENT,
 version_key VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 ddl_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (migration_id),
 UNIQUE KEY uk_p1_migration_version (version_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- SOURCE: sql/erp/004_purchasing.sql
CREATE TABLE erp_supplier (
 supplier_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 supplier_code VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 supplier_name VARCHAR(120) NOT NULL,
 contact_name VARCHAR(64) NOT NULL DEFAULT '',
 phone VARCHAR(64) NOT NULL DEFAULT '',
 email VARCHAR(128) NOT NULL DEFAULT '',
 address VARCHAR(500) NOT NULL DEFAULT '',
 status CHAR(1) NOT NULL DEFAULT '0',
 version BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (supplier_id),
 UNIQUE KEY uk_p4_supplier_id (tenant_id,supplier_id),
 UNIQUE KEY uk_p4_supplier_code (tenant_id,supplier_code),
 KEY ix_p4_supplier_status (tenant_id,status,supplier_id),
 CONSTRAINT fk_p4_supplier_tenant FOREIGN KEY (tenant_id) REFERENCES sys_tenant(tenant_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_purchase_order (
 purchase_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 purchase_no VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 supplier_id BIGINT NOT NULL,
 warehouse_id BIGINT NOT NULL,
 supplier_code VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 supplier_name VARCHAR(120) NOT NULL,
 warehouse_name VARCHAR(120) NOT NULL,
 currency CHAR(3) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 status VARCHAR(24) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'DRAFT',
 version BIGINT NOT NULL DEFAULT 1,
 create_request_key VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 create_request_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 ordered_amount DECIMAL(20,4) NOT NULL,
 confirmed_at DATETIME NULL,
 closed_at DATETIME NULL,
 cancelled_at DATETIME NULL,
 close_reason VARCHAR(500) NOT NULL DEFAULT '',
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (purchase_id),
 UNIQUE KEY uk_p4_purchase_id (tenant_id,purchase_id),
 UNIQUE KEY uk_p4_purchase_warehouse (tenant_id,purchase_id,warehouse_id),
 UNIQUE KEY uk_p4_purchase_no (tenant_id,purchase_no),
 UNIQUE KEY uk_p4_purchase_request (tenant_id,create_request_key),
 KEY ix_p4_purchase_status (tenant_id,status,purchase_id),
 KEY ix_p4_purchase_warehouse (tenant_id,warehouse_id,status,purchase_id),
 KEY ix_p4_purchase_supplier (tenant_id,supplier_id,purchase_id),
 CONSTRAINT fk_p4_purchase_supplier FOREIGN KEY (tenant_id,supplier_id) REFERENCES erp_supplier(tenant_id,supplier_id),
 CONSTRAINT fk_p4_purchase_warehouse FOREIGN KEY (tenant_id,warehouse_id) REFERENCES erp_warehouse(tenant_id,warehouse_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_purchase_order_line (
 line_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 purchase_id BIGINT NOT NULL,
 line_no BIGINT NOT NULL,
 sku_id BIGINT NOT NULL,
 sku_code VARCHAR(64) NOT NULL,
 product_name VARCHAR(120) NOT NULL,
 specification VARCHAR(120) NOT NULL,
 unit VARCHAR(20) NOT NULL,
 ordered_quantity BIGINT NOT NULL,
 received_quantity BIGINT NOT NULL DEFAULT 0,
 unit_price DECIMAL(20,4) NOT NULL,
 line_amount DECIMAL(20,4) NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (line_id),
 UNIQUE KEY uk_p4_line_identity (tenant_id,purchase_id,line_id,sku_id),
 UNIQUE KEY uk_p4_line_no (tenant_id,purchase_id,line_no),
 UNIQUE KEY uk_p4_line_sku (tenant_id,purchase_id,sku_id),
 KEY ix_p4_line_sku (tenant_id,sku_id),
 CONSTRAINT fk_p4_line_purchase FOREIGN KEY (tenant_id,purchase_id) REFERENCES erp_purchase_order(tenant_id,purchase_id),
 CONSTRAINT fk_p4_line_sku FOREIGN KEY (tenant_id,sku_id) REFERENCES erp_sku(tenant_id,sku_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_purchase_receipt (
 receipt_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 receipt_no VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 purchase_id BIGINT NOT NULL,
 warehouse_id BIGINT NOT NULL,
 stock_document_id BIGINT NOT NULL,
 actor_user_id BIGINT NOT NULL,
 request_key VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 posted_at DATETIME NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (receipt_id),
 UNIQUE KEY uk_p4_receipt_id (tenant_id,purchase_id,receipt_id),
 UNIQUE KEY uk_p4_receipt_no (tenant_id,receipt_no),
 UNIQUE KEY uk_p4_receipt_stock (tenant_id,stock_document_id),
 UNIQUE KEY uk_p4_receipt_request (tenant_id,purchase_id,request_key),
 KEY ix_p4_receipt_purchase_wh (tenant_id,purchase_id,warehouse_id),
 KEY ix_p4_receipt_warehouse (tenant_id,warehouse_id,receipt_id),
 KEY ix_p4_receipt_actor (tenant_id,actor_user_id),
 CONSTRAINT fk_p4_receipt_purchase_wh FOREIGN KEY (tenant_id,purchase_id,warehouse_id) REFERENCES erp_purchase_order(tenant_id,purchase_id,warehouse_id),
 CONSTRAINT fk_p4_receipt_stock FOREIGN KEY (tenant_id,stock_document_id) REFERENCES erp_document(tenant_id,document_id),
 CONSTRAINT fk_p4_receipt_actor FOREIGN KEY (tenant_id,actor_user_id) REFERENCES sys_user(tenant_id,user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_purchase_receipt_line (
 line_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 receipt_id BIGINT NOT NULL,
 purchase_id BIGINT NOT NULL,
 purchase_line_id BIGINT NOT NULL,
 sku_id BIGINT NOT NULL,
 quantity BIGINT NOT NULL,
 sku_code VARCHAR(64) NOT NULL,
 product_name VARCHAR(120) NOT NULL,
 specification VARCHAR(120) NOT NULL,
 unit VARCHAR(20) NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (line_id),
 UNIQUE KEY uk_p4_receipt_line (tenant_id,receipt_id,purchase_line_id),
 KEY ix_p4_receipt_line_receipt (tenant_id,purchase_id,receipt_id),
 KEY ix_p4_receipt_line_purchase (tenant_id,purchase_id,purchase_line_id,sku_id),
 CONSTRAINT fk_p4_receipt_line_receipt FOREIGN KEY (tenant_id,purchase_id,receipt_id) REFERENCES erp_purchase_receipt(tenant_id,purchase_id,receipt_id),
 CONSTRAINT fk_p4_receipt_line_purchase FOREIGN KEY (tenant_id,purchase_id,purchase_line_id,sku_id) REFERENCES erp_purchase_order_line(tenant_id,purchase_id,line_id,sku_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_purchase_event (
 event_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 purchase_id BIGINT NOT NULL,
 receipt_id BIGINT NULL,
 actor_user_id BIGINT NOT NULL,
 action VARCHAR(24) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_key VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 from_status VARCHAR(24) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 to_status VARCHAR(24) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 from_version BIGINT NOT NULL,
 to_version BIGINT NOT NULL,
 result_json VARCHAR(2000) NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (event_id),
 UNIQUE KEY uk_p4_event_request (tenant_id,purchase_id,action,request_key),
 KEY ix_p4_event_purchase (tenant_id,purchase_id,event_id),
 KEY ix_p4_event_receipt (tenant_id,purchase_id,receipt_id),
 KEY ix_p4_event_actor (tenant_id,actor_user_id),
 CONSTRAINT fk_p4_event_purchase FOREIGN KEY (tenant_id,purchase_id) REFERENCES erp_purchase_order(tenant_id,purchase_id),
 CONSTRAINT fk_p4_event_receipt FOREIGN KEY (tenant_id,purchase_id,receipt_id) REFERENCES erp_purchase_receipt(tenant_id,purchase_id,receipt_id),
 CONSTRAINT fk_p4_event_actor FOREIGN KEY (tenant_id,actor_user_id) REFERENCES sys_user(tenant_id,user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- SOURCE: sql/erp/005_purchase_returns.sql
CREATE TABLE erp_purchase_return_order (
 return_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 return_no VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 purchase_id BIGINT NOT NULL,
 receipt_id BIGINT NOT NULL,
 warehouse_id BIGINT NOT NULL,
 stock_document_id BIGINT NOT NULL,
 actor_user_id BIGINT NOT NULL,
 request_key VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 from_version BIGINT NOT NULL,
 to_version BIGINT NOT NULL,
 result_json VARCHAR(2000) NOT NULL,
 reason VARCHAR(500) NOT NULL,
 posted_at DATETIME NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (return_id),
 UNIQUE KEY uk_p5_return_id (tenant_id,purchase_id,receipt_id,return_id),
 UNIQUE KEY uk_p5_return_no (tenant_id,return_no),
 UNIQUE KEY uk_p5_return_stock (tenant_id,stock_document_id),
 UNIQUE KEY uk_p5_return_request (tenant_id,receipt_id,request_key),
 UNIQUE KEY uk_p5_return_version (tenant_id,receipt_id,to_version),
 KEY ix_p5_return_receipt (tenant_id,purchase_id,receipt_id),
 KEY ix_p5_return_purchase_wh (tenant_id,purchase_id,warehouse_id),
 KEY ix_p5_return_warehouse (tenant_id,warehouse_id,return_id),
 KEY ix_p5_return_actor (tenant_id,actor_user_id),
 CONSTRAINT fk_p5_return_receipt FOREIGN KEY (tenant_id,purchase_id,receipt_id) REFERENCES erp_purchase_receipt(tenant_id,purchase_id,receipt_id),
 CONSTRAINT fk_p5_return_purchase_wh FOREIGN KEY (tenant_id,purchase_id,warehouse_id) REFERENCES erp_purchase_order(tenant_id,purchase_id,warehouse_id),
 CONSTRAINT fk_p5_return_stock FOREIGN KEY (tenant_id,stock_document_id) REFERENCES erp_document(tenant_id,document_id),
 CONSTRAINT fk_p5_return_actor FOREIGN KEY (tenant_id,actor_user_id) REFERENCES sys_user(tenant_id,user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_purchase_return_line (
 line_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 return_id BIGINT NOT NULL,
 receipt_id BIGINT NOT NULL,
 receipt_line_id BIGINT NOT NULL,
 purchase_id BIGINT NOT NULL,
 purchase_line_id BIGINT NOT NULL,
 sku_id BIGINT NOT NULL,
 quantity BIGINT NOT NULL,
 sku_code VARCHAR(64) NOT NULL,
 product_name VARCHAR(120) NOT NULL,
 specification VARCHAR(120) NOT NULL,
 unit VARCHAR(20) NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (line_id),
 UNIQUE KEY uk_p5_return_line (tenant_id,return_id,purchase_line_id),
 KEY ix_p5_return_line_head (tenant_id,purchase_id,receipt_id,return_id),
 KEY ix_p5_return_line_receipt (tenant_id,receipt_id,purchase_line_id),
 KEY ix_p5_return_line_purchase (tenant_id,purchase_id,purchase_line_id,sku_id),
 CONSTRAINT fk_p5_line_head FOREIGN KEY (tenant_id,purchase_id,receipt_id,return_id) REFERENCES erp_purchase_return_order(tenant_id,purchase_id,receipt_id,return_id),
 CONSTRAINT fk_p5_line_receipt FOREIGN KEY (tenant_id,receipt_id,purchase_line_id) REFERENCES erp_purchase_receipt_line(tenant_id,receipt_id,purchase_line_id),
 CONSTRAINT fk_p5_line_purchase FOREIGN KEY (tenant_id,purchase_id,purchase_line_id,sku_id) REFERENCES erp_purchase_order_line(tenant_id,purchase_id,line_id,sku_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- SOURCE: sql/erp/006_sales_returns.sql
CREATE TABLE erp_sales_return_order (
 return_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 return_no VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 order_id BIGINT NOT NULL,
 shop_id BIGINT NOT NULL,
 package_id BIGINT NOT NULL,
 warehouse_id BIGINT NOT NULL,
 stock_document_id BIGINT NOT NULL,
 actor_user_id BIGINT NOT NULL,
 request_key VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 from_version BIGINT NOT NULL,
 to_version BIGINT NOT NULL,
 result_json VARCHAR(2000) NOT NULL,
 reason VARCHAR(500) NOT NULL,
 posted_at DATETIME NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (return_id),
 UNIQUE KEY uk_s6_return_id (tenant_id,order_id,package_id,return_id),
 UNIQUE KEY uk_s6_return_no (tenant_id,return_no),
 UNIQUE KEY uk_s6_return_stock (tenant_id,stock_document_id),
 UNIQUE KEY uk_s6_return_request (tenant_id,package_id,request_key),
 UNIQUE KEY uk_s6_return_version (tenant_id,package_id,to_version),
 KEY ix_s6_return_package (tenant_id,order_id,package_id),
 KEY ix_s6_return_shop (tenant_id,shop_id,order_id),
 KEY ix_s6_return_wh (tenant_id,order_id,warehouse_id),
 KEY ix_s6_return_list (tenant_id,shop_id,warehouse_id,return_id),
 KEY ix_s6_return_actor (tenant_id,actor_user_id),
 CONSTRAINT fk_s6_return_package FOREIGN KEY (tenant_id,order_id,package_id) REFERENCES erp_package(tenant_id,order_id,package_id),
 CONSTRAINT fk_s6_return_shop FOREIGN KEY (tenant_id,shop_id,order_id) REFERENCES erp_order(tenant_id,shop_id,order_id),
 CONSTRAINT fk_s6_return_wh FOREIGN KEY (tenant_id,order_id,warehouse_id) REFERENCES erp_order(tenant_id,order_id,warehouse_id),
 CONSTRAINT fk_s6_return_stock FOREIGN KEY (tenant_id,stock_document_id) REFERENCES erp_document(tenant_id,document_id),
 CONSTRAINT fk_s6_return_actor FOREIGN KEY (tenant_id,actor_user_id) REFERENCES sys_user(tenant_id,user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_sales_return_line (
 line_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 return_id BIGINT NOT NULL,
 package_id BIGINT NOT NULL,
 package_line_id BIGINT NOT NULL,
 order_id BIGINT NOT NULL,
 order_line_id BIGINT NOT NULL,
 sku_id BIGINT NOT NULL,
 quantity BIGINT NOT NULL,
 sku_code VARCHAR(64) NOT NULL,
 product_name VARCHAR(120) NOT NULL,
 unit VARCHAR(20) NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (line_id),
 UNIQUE KEY uk_s6_return_line (tenant_id,return_id,order_line_id),
 KEY ix_s6_line_head (tenant_id,order_id,package_id,return_id),
 KEY ix_s6_line_package (tenant_id,package_id,order_line_id),
 KEY ix_s6_line_order (tenant_id,order_id,order_line_id,sku_id),
 CONSTRAINT fk_s6_line_head FOREIGN KEY (tenant_id,order_id,package_id,return_id) REFERENCES erp_sales_return_order(tenant_id,order_id,package_id,return_id),
 CONSTRAINT fk_s6_line_package FOREIGN KEY (tenant_id,package_id,order_line_id) REFERENCES erp_package_line(tenant_id,package_id,order_line_id),
 CONSTRAINT fk_s6_line_order FOREIGN KEY (tenant_id,order_id,order_line_id,sku_id) REFERENCES erp_order_line(tenant_id,order_id,line_id,sku_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- SOURCE: sql/erp/007_warehouse_operations.sql
CREATE TABLE erp_warehouse_profile (
 profile_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 warehouse_id BIGINT NOT NULL,
 address VARCHAR(300) NOT NULL DEFAULT '',
 manager VARCHAR(64) NOT NULL DEFAULT '',
 version BIGINT NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (profile_id),
 UNIQUE KEY uk_w7_profile (tenant_id,warehouse_id),
 CONSTRAINT fk_w7_profile_wh FOREIGN KEY (tenant_id,warehouse_id) REFERENCES erp_warehouse(tenant_id,warehouse_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_stock_alert (
 alert_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 warehouse_id BIGINT NOT NULL,
 sku_id BIGINT NOT NULL,
 threshold_quantity BIGINT NULL,
 version BIGINT NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (alert_id),
 UNIQUE KEY uk_w7_alert (tenant_id,warehouse_id,sku_id),
 KEY ix_w7_alert_sku (tenant_id,sku_id),
 CONSTRAINT fk_w7_alert_wh FOREIGN KEY (tenant_id,warehouse_id) REFERENCES erp_warehouse(tenant_id,warehouse_id),
 CONSTRAINT fk_w7_alert_sku FOREIGN KEY (tenant_id,sku_id) REFERENCES erp_sku(tenant_id,sku_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_transfer (
 transfer_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 transfer_no VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 source_warehouse_id BIGINT NOT NULL,
 target_warehouse_id BIGINT NOT NULL,
 source_warehouse_name VARCHAR(120) NOT NULL,
 target_warehouse_name VARCHAR(120) NOT NULL,
 status VARCHAR(16) NOT NULL,
 version BIGINT NOT NULL,
 actor_user_id BIGINT NOT NULL,
 request_key VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 result_json VARCHAR(2000) NOT NULL,
 out_document_id BIGINT NULL,
 in_document_id BIGINT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (transfer_id),
 UNIQUE KEY uk_w7_transfer_id (tenant_id,transfer_id),
 UNIQUE KEY uk_w7_transfer_no (tenant_id,transfer_no),
 UNIQUE KEY uk_w7_transfer_key (tenant_id,request_key),
 UNIQUE KEY uk_w7_transfer_out (tenant_id,out_document_id),
 UNIQUE KEY uk_w7_transfer_in (tenant_id,in_document_id),
 KEY ix_w7_transfer_src (tenant_id,source_warehouse_id,status,transfer_id),
 KEY ix_w7_transfer_dst (tenant_id,target_warehouse_id,status,transfer_id),
 KEY ix_w7_transfer_actor (tenant_id,actor_user_id),
 CONSTRAINT fk_w7_transfer_src FOREIGN KEY (tenant_id,source_warehouse_id) REFERENCES erp_warehouse(tenant_id,warehouse_id),
 CONSTRAINT fk_w7_transfer_dst FOREIGN KEY (tenant_id,target_warehouse_id) REFERENCES erp_warehouse(tenant_id,warehouse_id),
 CONSTRAINT fk_w7_transfer_actor FOREIGN KEY (tenant_id,actor_user_id) REFERENCES sys_user(tenant_id,user_id),
 CONSTRAINT fk_w7_transfer_out FOREIGN KEY (tenant_id,out_document_id) REFERENCES erp_document(tenant_id,document_id),
 CONSTRAINT fk_w7_transfer_in FOREIGN KEY (tenant_id,in_document_id) REFERENCES erp_document(tenant_id,document_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_transfer_line (
 line_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 transfer_id BIGINT NOT NULL,
 sku_id BIGINT NOT NULL,
 quantity BIGINT NOT NULL,
 sku_code VARCHAR(64) NOT NULL,
 product_name VARCHAR(120) NOT NULL,
 specification VARCHAR(120) NOT NULL,
 unit VARCHAR(20) NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (line_id),
 UNIQUE KEY uk_w7_transfer_line (tenant_id,transfer_id,sku_id),
 KEY ix_w7_transfer_line_sku (tenant_id,sku_id),
 CONSTRAINT fk_w7_transfer_line_head FOREIGN KEY (tenant_id,transfer_id) REFERENCES erp_transfer(tenant_id,transfer_id),
 CONSTRAINT fk_w7_transfer_line_sku FOREIGN KEY (tenant_id,sku_id) REFERENCES erp_sku(tenant_id,sku_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_transfer_action (
 action_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 transfer_id BIGINT NOT NULL,
 actor_user_id BIGINT NOT NULL,
 action VARCHAR(16) NOT NULL,
 request_key VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 result_json VARCHAR(2000) NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (action_id),
 UNIQUE KEY uk_w7_transfer_action (tenant_id,transfer_id,request_key),
 KEY ix_w7_transfer_action_actor (tenant_id,actor_user_id),
 CONSTRAINT fk_w7_transfer_action_head FOREIGN KEY (tenant_id,transfer_id) REFERENCES erp_transfer(tenant_id,transfer_id),
 CONSTRAINT fk_w7_transfer_action_actor FOREIGN KEY (tenant_id,actor_user_id) REFERENCES sys_user(tenant_id,user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- SOURCE: sql/erp/008_finance.sql
CREATE TABLE erp_fin_account (
 account_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 account_code VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 account_name VARCHAR(120) NOT NULL,
 account_type VARCHAR(8) NOT NULL,
 currency CHAR(3) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 status CHAR(1) NOT NULL,
 version BIGINT NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (account_id),
 UNIQUE KEY uk_f8_account_id (tenant_id,account_id),
 UNIQUE KEY uk_f8_account_code (tenant_id,account_code),
 KEY ix_f8_account_status (tenant_id,status,account_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_fin_payable (
 payable_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 payable_no VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 receipt_id BIGINT NOT NULL,
 purchase_id BIGINT NOT NULL,
 supplier_id BIGINT NOT NULL,
 warehouse_id BIGINT NOT NULL,
 supplier_name VARCHAR(120) NOT NULL,
 receipt_no VARCHAR(64) NOT NULL,
 currency CHAR(3) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 gross_amount DECIMAL(20,4) NOT NULL,
 credit_amount DECIMAL(20,4) NOT NULL,
 paid_amount DECIMAL(20,4) NOT NULL,
 version BIGINT NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (payable_id),
 UNIQUE KEY uk_f8_payable_id (tenant_id,payable_id),
 UNIQUE KEY uk_f8_payable_source (tenant_id,payable_id,purchase_id,receipt_id),
 UNIQUE KEY uk_f8_payable_batch (tenant_id,payable_id,receipt_id),
 UNIQUE KEY uk_f8_payable_no (tenant_id,payable_no),
 UNIQUE KEY uk_f8_payable_receipt (tenant_id,receipt_id),
 KEY ix_f8_payable_supplier (tenant_id,supplier_id,currency,payable_id),
 KEY ix_f8_payable_wh (tenant_id,warehouse_id,payable_id),
 KEY ix_f8_payable_purchase (tenant_id,purchase_id),
 KEY ix_f8_payable_receipt_fk (tenant_id,purchase_id,receipt_id),
 CONSTRAINT fk_f8_payable_receipt FOREIGN KEY (tenant_id,purchase_id,receipt_id) REFERENCES erp_purchase_receipt(tenant_id,purchase_id,receipt_id),
 CONSTRAINT fk_f8_payable_purchase FOREIGN KEY (tenant_id,purchase_id) REFERENCES erp_purchase_order(tenant_id,purchase_id),
 CONSTRAINT fk_f8_payable_supplier FOREIGN KEY (tenant_id,supplier_id) REFERENCES erp_supplier(tenant_id,supplier_id),
 CONSTRAINT fk_f8_payable_wh FOREIGN KEY (tenant_id,warehouse_id) REFERENCES erp_warehouse(tenant_id,warehouse_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_fin_payable_line (
 line_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 payable_id BIGINT NOT NULL,
 receipt_line_id BIGINT NOT NULL,
 receipt_id BIGINT NOT NULL,
 purchase_line_id BIGINT NOT NULL,
 sku_code VARCHAR(64) NOT NULL,
 product_name VARCHAR(120) NOT NULL,
 unit VARCHAR(20) NOT NULL,
 quantity BIGINT NOT NULL,
 unit_price DECIMAL(20,4) NOT NULL,
 line_amount DECIMAL(20,4) NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (line_id),
 UNIQUE KEY uk_f8_payable_line (tenant_id,payable_id,receipt_line_id),
 KEY ix_f8_payable_line_receipt (tenant_id,receipt_id,purchase_line_id),
 KEY ix_f8_payable_line_head (tenant_id,payable_id,receipt_id),
 CONSTRAINT fk_f8_payable_line_head FOREIGN KEY (tenant_id,payable_id,receipt_id) REFERENCES erp_fin_payable(tenant_id,payable_id,receipt_id),
 CONSTRAINT fk_f8_payable_line_receipt FOREIGN KEY (tenant_id,receipt_id,purchase_line_id) REFERENCES erp_purchase_receipt_line(tenant_id,receipt_id,purchase_line_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_fin_credit (
 credit_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 payable_id BIGINT NOT NULL,
 return_id BIGINT NOT NULL,
 purchase_id BIGINT NOT NULL,
 receipt_id BIGINT NOT NULL,
 return_no VARCHAR(64) NOT NULL,
 amount DECIMAL(20,4) NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (credit_id),
 UNIQUE KEY uk_f8_credit_return (tenant_id,return_id),
 KEY ix_f8_credit_payable (tenant_id,payable_id,credit_id),
 KEY ix_f8_credit_head (tenant_id,payable_id,purchase_id,receipt_id),
 KEY ix_f8_credit_return (tenant_id,purchase_id,receipt_id,return_id),
 CONSTRAINT fk_f8_credit_head FOREIGN KEY (tenant_id,payable_id,purchase_id,receipt_id) REFERENCES erp_fin_payable(tenant_id,payable_id,purchase_id,receipt_id),
 CONSTRAINT fk_f8_credit_return FOREIGN KEY (tenant_id,purchase_id,receipt_id,return_id) REFERENCES erp_purchase_return_order(tenant_id,purchase_id,receipt_id,return_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_fin_payment (
 payment_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 payment_no VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 account_id BIGINT NOT NULL,
 account_name VARCHAR(120) NOT NULL,
 supplier_id BIGINT NOT NULL,
 supplier_name VARCHAR(120) NOT NULL,
 currency CHAR(3) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 amount DECIMAL(20,4) NOT NULL,
 status VARCHAR(16) NOT NULL,
 version BIGINT NOT NULL,
 reverse_reason VARCHAR(500) NOT NULL DEFAULT '',
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (payment_id),
 UNIQUE KEY uk_f8_payment_id (tenant_id,payment_id),
 UNIQUE KEY uk_f8_payment_no (tenant_id,payment_no),
 KEY ix_f8_payment_supplier (tenant_id,supplier_id,currency,payment_id),
 KEY ix_f8_payment_account (tenant_id,account_id,payment_id),
 CONSTRAINT fk_f8_payment_account FOREIGN KEY (tenant_id,account_id) REFERENCES erp_fin_account(tenant_id,account_id),
 CONSTRAINT fk_f8_payment_supplier FOREIGN KEY (tenant_id,supplier_id) REFERENCES erp_supplier(tenant_id,supplier_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_fin_allocation (
 allocation_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 payment_id BIGINT NOT NULL,
 payable_id BIGINT NOT NULL,
 amount DECIMAL(20,4) NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (allocation_id),
 UNIQUE KEY uk_f8_allocation (tenant_id,payment_id,payable_id),
 KEY ix_f8_allocation_payable (tenant_id,payable_id,payment_id),
 CONSTRAINT fk_f8_allocation_payment FOREIGN KEY (tenant_id,payment_id) REFERENCES erp_fin_payment(tenant_id,payment_id),
 CONSTRAINT fk_f8_allocation_payable FOREIGN KEY (tenant_id,payable_id) REFERENCES erp_fin_payable(tenant_id,payable_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_fin_action (
 action_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 actor_user_id BIGINT NOT NULL,
 action VARCHAR(24) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 target_id BIGINT NOT NULL,
 request_key VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 result_json VARCHAR(2000) NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (action_id),
 UNIQUE KEY uk_f8_action_key (tenant_id,request_key),
 KEY ix_f8_action_target (tenant_id,action,target_id,action_id),
 KEY ix_f8_action_actor (tenant_id,actor_user_id),
 CONSTRAINT fk_f8_action_actor FOREIGN KEY (tenant_id,actor_user_id) REFERENCES sys_user(tenant_id,user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- SOURCE: sql/erp/009_sales_finance.sql
CREATE TABLE erp_sf_receivable (
 receivable_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 receivable_no VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 order_id BIGINT NOT NULL,
 package_id BIGINT NOT NULL,
 shop_id BIGINT NOT NULL,
 warehouse_id BIGINT NOT NULL,
 shop_name VARCHAR(120) NOT NULL,
 order_no VARCHAR(64) NOT NULL,
 currency CHAR(3) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 gross_amount DECIMAL(20,4) NOT NULL,
 credit_amount DECIMAL(20,4) NOT NULL,
 received_amount DECIMAL(20,4) NOT NULL,
 version BIGINT NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (receivable_id),
 UNIQUE KEY uk_s9_rec_id (tenant_id,receivable_id),
 UNIQUE KEY uk_s9_rec_source (tenant_id,receivable_id,order_id,package_id),
 UNIQUE KEY uk_s9_rec_no (tenant_id,receivable_no),
 UNIQUE KEY uk_s9_rec_order (tenant_id,order_id),
 KEY ix_s9_rec_shop (tenant_id,shop_id,currency,receivable_id),
 KEY ix_s9_rec_wh (tenant_id,warehouse_id,receivable_id),
 KEY ix_s9_rec_package (tenant_id,order_id,package_id),
 KEY ix_s9_rec_order_shop (tenant_id,shop_id,order_id),
 KEY ix_s9_rec_order_wh (tenant_id,order_id,warehouse_id),
 CONSTRAINT fk_s9_rec_package FOREIGN KEY (tenant_id,order_id,package_id) REFERENCES erp_package(tenant_id,order_id,package_id),
 CONSTRAINT fk_s9_rec_order_shop FOREIGN KEY (tenant_id,shop_id,order_id) REFERENCES erp_order(tenant_id,shop_id,order_id),
 CONSTRAINT fk_s9_rec_order_wh FOREIGN KEY (tenant_id,order_id,warehouse_id) REFERENCES erp_order(tenant_id,order_id,warehouse_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_sf_receivable_line (
 line_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 receivable_id BIGINT NOT NULL,
 order_id BIGINT NOT NULL,
 order_line_id BIGINT NOT NULL,
 sku_code VARCHAR(64) NOT NULL,
 product_name VARCHAR(120) NOT NULL,
 unit VARCHAR(20) NOT NULL,
 quantity BIGINT NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (line_id),
 UNIQUE KEY uk_s9_line (tenant_id,receivable_id,order_line_id),
 KEY ix_s9_line_order (tenant_id,order_id,order_line_id),
 CONSTRAINT fk_s9_line_head FOREIGN KEY (tenant_id,receivable_id) REFERENCES erp_sf_receivable(tenant_id,receivable_id),
 CONSTRAINT fk_s9_line_order FOREIGN KEY (tenant_id,order_id,order_line_id) REFERENCES erp_order_line(tenant_id,order_id,line_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_sf_credit (
 credit_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 receivable_id BIGINT NOT NULL,
 return_id BIGINT NOT NULL,
 order_id BIGINT NOT NULL,
 package_id BIGINT NOT NULL,
 return_no VARCHAR(64) NOT NULL,
 amount DECIMAL(20,4) NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (credit_id),
 UNIQUE KEY uk_s9_credit_return (tenant_id,return_id),
 KEY ix_s9_credit_head (tenant_id,receivable_id,order_id,package_id),
 KEY ix_s9_credit_return (tenant_id,order_id,package_id,return_id),
 CONSTRAINT fk_s9_credit_head FOREIGN KEY (tenant_id,receivable_id,order_id,package_id) REFERENCES erp_sf_receivable(tenant_id,receivable_id,order_id,package_id),
 CONSTRAINT fk_s9_credit_return FOREIGN KEY (tenant_id,order_id,package_id,return_id) REFERENCES erp_sales_return_order(tenant_id,order_id,package_id,return_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_sf_collection (
 collection_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 collection_no VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 account_id BIGINT NOT NULL,
 account_name VARCHAR(120) NOT NULL,
 shop_id BIGINT NOT NULL,
 shop_name VARCHAR(120) NOT NULL,
 currency CHAR(3) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 amount DECIMAL(20,4) NOT NULL,
 status VARCHAR(16) NOT NULL,
 version BIGINT NOT NULL,
 reverse_reason VARCHAR(500) NOT NULL DEFAULT '',
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (collection_id),
 UNIQUE KEY uk_s9_collection_id (tenant_id,collection_id),
 UNIQUE KEY uk_s9_collection_no (tenant_id,collection_no),
 KEY ix_s9_collection_shop (tenant_id,shop_id,currency,collection_id),
 KEY ix_s9_collection_account (tenant_id,account_id,collection_id),
 CONSTRAINT fk_s9_collection_shop FOREIGN KEY (tenant_id,shop_id) REFERENCES erp_shop(tenant_id,shop_id),
 CONSTRAINT fk_s9_collection_account FOREIGN KEY (tenant_id,account_id) REFERENCES erp_fin_account(tenant_id,account_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_sf_allocation (
 allocation_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 collection_id BIGINT NOT NULL,
 receivable_id BIGINT NOT NULL,
 amount DECIMAL(20,4) NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (allocation_id),
 UNIQUE KEY uk_s9_allocation (tenant_id,collection_id,receivable_id),
 KEY ix_s9_allocation_rec (tenant_id,receivable_id,collection_id),
 CONSTRAINT fk_s9_allocation_collection FOREIGN KEY (tenant_id,collection_id) REFERENCES erp_sf_collection(tenant_id,collection_id),
 CONSTRAINT fk_s9_allocation_rec FOREIGN KEY (tenant_id,receivable_id) REFERENCES erp_sf_receivable(tenant_id,receivable_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_sf_action (
 action_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 actor_user_id BIGINT NOT NULL,
 action VARCHAR(24) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 target_id BIGINT NOT NULL,
 request_key VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 result_json VARCHAR(2000) NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (action_id),
 UNIQUE KEY uk_s9_action_key (tenant_id,request_key),
 KEY ix_s9_action_target (tenant_id,action,target_id,action_id),
 KEY ix_s9_action_actor (tenant_id,actor_user_id),
 CONSTRAINT fk_s9_action_actor FOREIGN KEY (tenant_id,actor_user_id) REFERENCES sys_user(tenant_id,user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- SOURCE: sql/erp/010_catalog.sql
CREATE TABLE erp_product_category (
 category_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 category_code VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 category_name VARCHAR(120) NOT NULL,
 status CHAR(1) NOT NULL,
 version BIGINT NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (category_id),
 UNIQUE KEY uk_c10_category_id (tenant_id,category_id),
 UNIQUE KEY uk_c10_category_code (tenant_id,category_code),
 CONSTRAINT fk_c10_category_tenant FOREIGN KEY (tenant_id) REFERENCES sys_tenant(tenant_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_product (
 product_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 product_code VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 product_name VARCHAR(120) NOT NULL,
 category_id BIGINT NULL,
 version BIGINT NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (product_id),
 UNIQUE KEY uk_c10_product_id (tenant_id,product_id),
 UNIQUE KEY uk_c10_product_code (tenant_id,product_code),
 KEY ix_c10_product_category (tenant_id,category_id,product_id),
 CONSTRAINT fk_c10_product_tenant FOREIGN KEY (tenant_id) REFERENCES sys_tenant(tenant_id),
 CONSTRAINT fk_c10_product_category FOREIGN KEY (tenant_id,category_id) REFERENCES erp_product_category(tenant_id,category_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_sku_profile (
 profile_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 product_id BIGINT NOT NULL,
 sku_id BIGINT NOT NULL,
 barcode VARCHAR(64) COLLATE utf8mb4_bin NULL,
 weight_grams BIGINT NULL,
 length_mm BIGINT NULL,
 width_mm BIGINT NULL,
 height_mm BIGINT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (profile_id),
 UNIQUE KEY uk_c10_profile_sku (tenant_id,sku_id),
 UNIQUE KEY uk_c10_profile_barcode (tenant_id,barcode),
 KEY ix_c10_profile_product (tenant_id,product_id,sku_id),
 CONSTRAINT fk_c10_profile_product FOREIGN KEY (tenant_id,product_id) REFERENCES erp_product(tenant_id,product_id),
 CONSTRAINT fk_c10_profile_sku FOREIGN KEY (tenant_id,sku_id) REFERENCES erp_sku(tenant_id,sku_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_product_image (
 image_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 product_id BIGINT NOT NULL,
 storage_key VARCHAR(255) NOT NULL,
 position INT NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (image_id),
 UNIQUE KEY uk_c10_image_position (tenant_id,product_id,position),
 UNIQUE KEY uk_c10_image_file (tenant_id,product_id,storage_key),
 KEY ix_c10_image_file (tenant_id,storage_key),
 CONSTRAINT fk_c10_image_product FOREIGN KEY (tenant_id,product_id) REFERENCES erp_product(tenant_id,product_id),
 CONSTRAINT fk_c10_image_file FOREIGN KEY (tenant_id,storage_key) REFERENCES sys_file(tenant_id,storage_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_catalog_action (
 action_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 request_key VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 request_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 actor_id BIGINT NOT NULL,
 action_kind VARCHAR(24) NOT NULL,
 result_json TEXT NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (action_id),
 UNIQUE KEY uk_c10_action_key (tenant_id,request_key),
 CONSTRAINT fk_c10_action_tenant FOREIGN KEY (tenant_id) REFERENCES sys_tenant(tenant_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- SOURCE: sql/erp/011_logistics.sql
CREATE TABLE erp_carrier (
 carrier_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 carrier_code VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 carrier_name VARCHAR(120) NOT NULL,
 status CHAR(1) NOT NULL,
 version BIGINT NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (carrier_id),
 UNIQUE KEY uk_l11_carrier_id (tenant_id,carrier_id),
 UNIQUE KEY uk_l11_carrier_code (tenant_id,carrier_code),
 CONSTRAINT fk_l11_carrier_tenant FOREIGN KEY (tenant_id) REFERENCES sys_tenant(tenant_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_shipping_method (
 shipping_method_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 carrier_id BIGINT NOT NULL,
 method_code VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 method_name VARCHAR(120) NOT NULL,
 status CHAR(1) NOT NULL,
 version BIGINT NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (shipping_method_id),
 UNIQUE KEY uk_l11_method_id (tenant_id,carrier_id,shipping_method_id),
 UNIQUE KEY uk_l11_method_code (tenant_id,method_code),
 KEY ix_l11_method_list (tenant_id,status,shipping_method_id),
 CONSTRAINT fk_l11_method_carrier FOREIGN KEY (tenant_id,carrier_id) REFERENCES erp_carrier(tenant_id,carrier_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_package_shipping (
 shipping_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 order_id BIGINT NOT NULL,
 package_id BIGINT NOT NULL,
 carrier_id BIGINT NOT NULL,
 shipping_method_id BIGINT NOT NULL,
 carrier_code VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 carrier_name VARCHAR(120) NOT NULL,
 method_code VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 method_name VARCHAR(120) NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (shipping_id),
 UNIQUE KEY uk_l11_shipping_package (tenant_id,package_id),
 KEY ix_l11_shipping_order (tenant_id,order_id,package_id),
 KEY ix_l11_shipping_method (tenant_id,carrier_id,shipping_method_id),
 CONSTRAINT fk_l11_shipping_package FOREIGN KEY (tenant_id,order_id,package_id) REFERENCES erp_package(tenant_id,order_id,package_id),
 CONSTRAINT fk_l11_shipping_method FOREIGN KEY (tenant_id,carrier_id,shipping_method_id) REFERENCES erp_shipping_method(tenant_id,carrier_id,shipping_method_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE erp_logistics_action (
 action_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 request_key VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 actor_id BIGINT NOT NULL,
 action_kind VARCHAR(24) NOT NULL,
 result_json TEXT NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',
 create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (action_id),
 UNIQUE KEY uk_l11_action_key (tenant_id,request_key),
 CONSTRAINT fk_l11_action_tenant FOREIGN KEY (tenant_id) REFERENCES sys_tenant(tenant_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Explicit current DDL records: 003-011; 001/002 have no migration rows.
INSERT INTO erp_schema_migration(version_key,ddl_sha256,create_by,create_time,remark) VALUES
('003_order_fulfillment','ebc324c78ec32f39df3e0b36a4fee276bf86ff827dd1dc65f20fa880018db2eb','erp-test-init',NOW(),'fresh empty database initialization; source DDL SHA256'),
('004_purchasing','cba3c0146ca5161406e41af859c5eb95b664b0b4544414109619b6310a8533bd','erp-test-init',NOW(),'fresh empty database initialization; source DDL SHA256'),
('005_purchase_returns','8b94fd34e75644cf7d1e3c1fe2fedc9e580ccb0c7c69c49857fa3f9b86a10781','erp-test-init',NOW(),'fresh empty database initialization; source DDL SHA256'),
('006_sales_returns','b915c1de7ed2c44df3ceddac456bca11676840abdf5d3a4696b3df2933c7fd46','erp-test-init',NOW(),'fresh empty database initialization; source DDL SHA256'),
('007_warehouse_operations','a3301879926643a297cf4626c3ecc27382ebd5578a5147e5b460bfe5b03a0a7f','erp-test-init',NOW(),'fresh empty database initialization; source DDL SHA256'),
('008_finance','6ba0c311f8783e687fd35492867cc2bf0acf22b7216cebfa29b4b928c04f495d','erp-test-init',NOW(),'fresh empty database initialization; source DDL SHA256'),
('009_sales_finance','26d9ed65f10246cb4207bae9617fcd529cdbdec66776eb5f70d083f91cd7449a','erp-test-init',NOW(),'fresh empty database initialization; source DDL SHA256'),
('010_catalog','62534663d51de9422cb60654276fa638c3f17dc690e3f98a6d8c98db7b1b2687','erp-test-init',NOW(),'fresh empty database initialization; source DDL SHA256'),
('011_logistics','834668e64be8601122f07725b28d6139bf140c124dcf267a2e6fd15a0511614b','erp-test-init',NOW(),'fresh empty database initialization; source DDL SHA256');

-- Platform and customer B owners; these are new identities, never copied credentials.
INSERT INTO sys_dept(dept_id,tenant_id,parent_id,ancestors,dept_name,order_num,status,del_flag,create_by,create_time) VALUES
(200,1,0,'0','平台管理',0,'0','0','erp-test-init',NOW()),(201,10001,0,'0','示例企业 B',0,'0','0','erp-test-init',NOW());
INSERT INTO sys_tenant(tenant_id,tenant_code,tenant_name,tenant_kind,owner_user_id,status,version,create_by,create_time) VALUES(10001,'demo-b','示例企业 B','01',NULL,'0',1,'erp-test-init',NOW());
INSERT INTO sys_role(role_id,tenant_id,role_name,role_key,role_sort,data_scope,menu_check_strictly,dept_check_strictly,status,del_flag,create_by,create_time) VALUES
(100,1,'平台超级管理员','platform_admin',0,'1',0,0,'0','0','erp-test-init',NOW()),(101,10001,'企业管理员','tenant_admin',0,'1',0,0,'0','0','erp-test-init',NOW());
INSERT INTO sys_user(user_id,tenant_id,dept_id,user_name,nick_name,user_type,password,status,del_flag,pwd_update_date,create_by,create_time) VALUES
(100,1,200,'admin','平台管理员','02','$2a$10$7btkK05hL1td67w8CPzg9OArpsLiJQL..R6oPws1NXB2I8P7zMGk6','0','0',NOW(),'erp-test-init',NOW()),(101,10001,201,'admin','企业 B 管理员','01','$2a$10$7btkK05hL1td67w8CPzg9OArpsLiJQL..R6oPws1NXB2I8P7zMGk6','0','0',NOW(),'erp-test-init',NOW());
UPDATE sys_tenant SET owner_user_id=100 WHERE tenant_id=1;
UPDATE sys_tenant SET owner_user_id=101 WHERE tenant_id=10001;
UPDATE sys_role SET role_name='企业管理员' WHERE tenant_id=10000 AND role_id=1;
INSERT INTO sys_user_role(tenant_id,user_id,role_id) VALUES(1,100,100),(10001,101,101);
INSERT INTO sys_tenant_policy(tenant_id,max_users,max_storage_bytes,max_upload_bytes,create_by,create_time) VALUES
(1,0,0,10485760,'erp-test-init',NOW()),(10000,100,1073741824,10485760,'erp-test-init',NOW()),(10001,100,1073741824,10485760,'erp-test-init',NOW());
INSERT INTO sys_config(tenant_id,config_name,config_key,config_value,config_type,create_by,create_time,remark)
SELECT 10001,config_name,config_key,config_value,config_type,'erp-test-init',NOW(),remark FROM sys_config WHERE tenant_id=1 AND config_key IN ('sys.index.skinName','sys.index.sideTheme','sys.account.chrtype','sys.account.initPasswordModify','sys.account.passwordValidateDays','sys.account.captchaEnabled','sys.account.registerUser','sys.user.initPassword','sys.login.blackIPList');
INSERT INTO sys_dict_type(tenant_id,dict_name,dict_type,status,create_by,create_time,remark)
SELECT 10001,dict_name,dict_type,status,'erp-test-init',NOW(),remark FROM sys_dict_type WHERE tenant_id=1 AND dict_type IN ('sys_user_sex','sys_show_hide','sys_normal_disable','sys_job_status','sys_job_group','sys_yes_no','sys_notice_type','sys_notice_status','sys_oper_type','sys_common_status');
INSERT INTO sys_dict_data(tenant_id,dict_sort,dict_label,dict_value,dict_type,css_class,list_class,is_default,status,create_by,create_time,remark)
SELECT 10001,dict_sort,dict_label,dict_value,dict_type,css_class,list_class,is_default,status,'erp-test-init',NOW(),remark FROM sys_dict_data WHERE tenant_id=1 AND dict_type IN ('sys_user_sex','sys_show_hide','sys_normal_disable','sys_job_status','sys_job_group','sys_yes_no','sys_notice_type','sys_notice_status','sys_oper_type','sys_common_status');

-- Menus are exported from current code; ERP sidebar grouping stays in web/.
INSERT INTO sys_menu(`menu_id`,`menu_name`,`parent_id`,`order_num`,`path`,`component`,`query`,`route_name`,`is_frame`,`is_cache`,`menu_type`,`visible`,`status`,`perms`,`icon`,create_by,create_time) VALUES
(5100,'平台管理',0,0,'platform',NULL,NULL,'','1','1','M','0','0','platform:tenant:list','tree','erp-test-init',NOW()),
(5000,'租户管理',5100,0,'tenant','platform/tenant/index',NULL,'','1','1','C','0','0','platform:tenant:list','peoples','erp-test-init',NOW()),
(5001,'开通企业',5000,0,'',NULL,NULL,'','1','1','F','0','0','platform:tenant:add','','erp-test-init',NOW()),
(5002,'管理企业',5000,0,'',NULL,NULL,'','1','1','F','0','0','platform:tenant:edit','','erp-test-init',NOW()),
(5003,'企业功能授权',5000,0,'',NULL,NULL,'','1','1','F','0','0','platform:tenant:grant','','erp-test-init',NOW()),
(5004,'重置企业管理员密码',5000,0,'',NULL,NULL,'','1','1','F','0','0','platform:tenant:resetPwd','','erp-test-init',NOW()),
(5005,'企业详情',5000,0,'',NULL,NULL,'','1','1','F','0','0','platform:tenant:query','','erp-test-init',NOW()),
(5006,'企业配额',5000,0,'',NULL,NULL,'','1','1','F','0','0','platform:tenant:quota','','erp-test-init',NOW()),
(5007,'企业生命周期',5000,0,'',NULL,NULL,'','1','1','F','0','0','platform:tenant:lifecycle','','erp-test-init',NOW())
ON DUPLICATE KEY UPDATE `menu_name`=VALUES(`menu_name`),`parent_id`=VALUES(`parent_id`),`order_num`=VALUES(`order_num`),`path`=VALUES(`path`),`component`=VALUES(`component`),`query`=VALUES(`query`),`route_name`=VALUES(`route_name`),`is_frame`=VALUES(`is_frame`),`is_cache`=VALUES(`is_cache`),`menu_type`=VALUES(`menu_type`),`visible`=VALUES(`visible`),`status`=VALUES(`status`),`perms`=VALUES(`perms`),`icon`=VALUES(`icon`);
INSERT INTO sys_menu(`menu_id`,`menu_name`,`parent_id`,`order_num`,`path`,`component`,`query`,`route_name`,`is_frame`,`is_cache`,`menu_type`,`visible`,`status`,`perms`,`icon`,create_by,create_time) VALUES
(6000,'ERP工作台',0,0,'erp',NULL,NULL,'','1','1','M','0','0','','shopping','erp-test-init',NOW()),
(6100,'商品SKU',6000,0,'sku','erp/sku/index',NULL,'','1','1','C','0','0','erp:sku:list','list','erp-test-init',NOW()),
(6101,'商品SKU-query',6100,0,'',NULL,NULL,'','1','1','F','0','0','erp:sku:query','','erp-test-init',NOW()),
(6102,'商品SKU-add',6100,0,'',NULL,NULL,'','1','1','F','0','0','erp:sku:add','','erp-test-init',NOW()),
(6103,'商品SKU-edit',6100,0,'',NULL,NULL,'','1','1','F','0','0','erp:sku:edit','','erp-test-init',NOW()),
(6104,'商品SKU-remove',6100,0,'',NULL,NULL,'','1','1','F','0','0','erp:sku:remove','','erp-test-init',NOW()),
(6105,'商品SKU-export',6100,0,'',NULL,NULL,'','1','1','F','0','0','erp:sku:export','','erp-test-init',NOW()),
(6200,'仓库',6000,0,'warehouse','erp/warehouse/index',NULL,'','1','1','C','0','0','erp:warehouse:list','list','erp-test-init',NOW()),
(6201,'仓库-query',6200,0,'',NULL,NULL,'','1','1','F','0','0','erp:warehouse:query','','erp-test-init',NOW()),
(6202,'仓库-add',6200,0,'',NULL,NULL,'','1','1','F','0','0','erp:warehouse:add','','erp-test-init',NOW()),
(6203,'仓库-edit',6200,0,'',NULL,NULL,'','1','1','F','0','0','erp:warehouse:edit','','erp-test-init',NOW()),
(6204,'仓库-remove',6200,0,'',NULL,NULL,'','1','1','F','0','0','erp:warehouse:remove','','erp-test-init',NOW()),
(6205,'仓库-export',6200,0,'',NULL,NULL,'','1','1','F','0','0','erp:warehouse:export','','erp-test-init',NOW()),
(6206,'仓库-grant',6200,0,'',NULL,NULL,'','1','1','F','0','0','erp:warehouse:grant','','erp-test-init',NOW()),
(6300,'采购入库',6000,0,'receipt','erp/receipt/index',NULL,'','1','1','C','0','0','erp:receipt:list','list','erp-test-init',NOW()),
(6301,'采购入库-query',6300,0,'',NULL,NULL,'','1','1','F','0','0','erp:receipt:query','','erp-test-init',NOW()),
(6302,'采购入库-add',6300,0,'',NULL,NULL,'','1','1','F','0','0','erp:receipt:add','','erp-test-init',NOW()),
(6303,'采购入库-confirm',6300,0,'',NULL,NULL,'','1','1','F','0','0','erp:receipt:confirm','','erp-test-init',NOW()),
(6304,'采购入库-cancel',6300,0,'',NULL,NULL,'','1','1','F','0','0','erp:receipt:cancel','','erp-test-init',NOW()),
(6305,'采购入库-edit',6300,0,'',NULL,NULL,'','1','1','F','0','0','erp:receipt:edit','','erp-test-init',NOW()),
(6400,'销售出库',6000,0,'sales','erp/sales/index',NULL,'','1','1','C','0','0','erp:sales:list','list','erp-test-init',NOW()),
(6401,'销售出库-query',6400,0,'',NULL,NULL,'','1','1','F','0','0','erp:sales:query','','erp-test-init',NOW()),
(6402,'销售出库-add',6400,0,'',NULL,NULL,'','1','1','F','0','0','erp:sales:add','','erp-test-init',NOW()),
(6403,'销售出库-confirm',6400,0,'',NULL,NULL,'','1','1','F','0','0','erp:sales:confirm','','erp-test-init',NOW()),
(6404,'销售出库-cancel',6400,0,'',NULL,NULL,'','1','1','F','0','0','erp:sales:cancel','','erp-test-init',NOW()),
(6405,'销售出库-ship',6400,0,'',NULL,NULL,'','1','1','F','0','0','erp:sales:ship','','erp-test-init',NOW()),
(6406,'销售出库-edit',6400,0,'',NULL,NULL,'','1','1','F','0','0','erp:sales:edit','','erp-test-init',NOW()),
(6500,'库存余额',6000,0,'stock','erp/stock/index',NULL,'','1','1','C','0','0','erp:stock:list','list','erp-test-init',NOW()),
(6600,'库存流水',6000,0,'ledger','erp/ledger/index',NULL,'','1','1','C','0','0','erp:ledger:list','list','erp-test-init',NOW()),
(6700,'销售退货/冲销',6000,0,'salesReturn','erp/salesReturn/index',NULL,'','1','1','C','0','0','erp:salesReturn:list','list','erp-test-init',NOW()),
(6701,'销售退货/冲销-query',6700,0,'',NULL,NULL,'','1','1','F','0','0','erp:salesReturn:query','','erp-test-init',NOW()),
(6702,'销售退货/冲销-add',6700,0,'',NULL,NULL,'','1','1','F','0','0','erp:salesReturn:add','','erp-test-init',NOW()),
(6703,'销售退货/冲销-confirm',6700,0,'',NULL,NULL,'','1','1','F','0','0','erp:salesReturn:confirm','','erp-test-init',NOW()),
(6704,'销售退货/冲销-cancel',6700,0,'',NULL,NULL,'','1','1','F','0','0','erp:salesReturn:cancel','','erp-test-init',NOW()),
(6705,'销售退货/冲销-edit',6700,0,'',NULL,NULL,'','1','1','F','0','0','erp:salesReturn:edit','','erp-test-init',NOW()),
(6800,'采购退货/冲销',6000,0,'purchaseReturn','erp/purchaseReturn/index',NULL,'','1','1','C','0','0','erp:purchaseReturn:list','list','erp-test-init',NOW()),
(6801,'采购退货/冲销-query',6800,0,'',NULL,NULL,'','1','1','F','0','0','erp:purchaseReturn:query','','erp-test-init',NOW()),
(6802,'采购退货/冲销-add',6800,0,'',NULL,NULL,'','1','1','F','0','0','erp:purchaseReturn:add','','erp-test-init',NOW()),
(6803,'采购退货/冲销-confirm',6800,0,'',NULL,NULL,'','1','1','F','0','0','erp:purchaseReturn:confirm','','erp-test-init',NOW()),
(6804,'采购退货/冲销-cancel',6800,0,'',NULL,NULL,'','1','1','F','0','0','erp:purchaseReturn:cancel','','erp-test-init',NOW()),
(6805,'采购退货/冲销-edit',6800,0,'',NULL,NULL,'','1','1','F','0','0','erp:purchaseReturn:edit','','erp-test-init',NOW()),
(6900,'库存盘点',6000,0,'count','erp/count/index',NULL,'','1','1','C','0','0','erp:count:list','list','erp-test-init',NOW()),
(6901,'库存盘点-query',6900,0,'',NULL,NULL,'','1','1','F','0','0','erp:count:query','','erp-test-init',NOW()),
(6902,'库存盘点-add',6900,0,'',NULL,NULL,'','1','1','F','0','0','erp:count:add','','erp-test-init',NOW()),
(6903,'库存盘点-confirm',6900,0,'',NULL,NULL,'','1','1','F','0','0','erp:count:confirm','','erp-test-init',NOW()),
(6904,'库存盘点-cancel',6900,0,'',NULL,NULL,'','1','1','F','0','0','erp:count:cancel','','erp-test-init',NOW()),
(6905,'库存盘点-edit',6900,0,'',NULL,NULL,'','1','1','F','0','0','erp:count:edit','','erp-test-init',NOW()),
(7000,'渠道目录',6000,0,'channel','erp/channel/index',NULL,'','1','1','C','0','0','erp:channel:list','list','erp-test-init',NOW()),
(7100,'店铺管理',6000,0,'shop','erp/shop/index',NULL,'','1','1','C','0','0','erp:shop:list','list','erp-test-init',NOW()),
(7101,'店铺管理-query',7100,0,'',NULL,NULL,'','1','1','F','0','0','erp:shop:query','','erp-test-init',NOW()),
(7102,'店铺管理-add',7100,0,'',NULL,NULL,'','1','1','F','0','0','erp:shop:add','','erp-test-init',NOW()),
(7103,'店铺管理-edit',7100,0,'',NULL,NULL,'','1','1','F','0','0','erp:shop:edit','','erp-test-init',NOW()),
(7104,'店铺管理-grant',7100,0,'',NULL,NULL,'','1','1','F','0','0','erp:shop:grant','','erp-test-init',NOW()),
(7200,'店铺商品配对',6000,0,'shopItem','erp/shopItem/index',NULL,'','1','1','C','0','0','erp:shopItem:list','list','erp-test-init',NOW()),
(7201,'店铺商品配对-query',7200,0,'',NULL,NULL,'','1','1','F','0','0','erp:shopItem:query','','erp-test-init',NOW()),
(7202,'店铺商品配对-add',7200,0,'',NULL,NULL,'','1','1','F','0','0','erp:shopItem:add','','erp-test-init',NOW()),
(7203,'店铺商品配对-edit',7200,0,'',NULL,NULL,'','1','1','F','0','0','erp:shopItem:edit','','erp-test-init',NOW()),
(7204,'店铺商品配对-map',7200,0,'',NULL,NULL,'','1','1','F','0','0','erp:shopItem:map','','erp-test-init',NOW()),
(7300,'订单录入',6000,0,'order','erp/order/index',NULL,'','1','1','C','0','0','erp:order:list','list','erp-test-init',NOW()),
(7301,'订单录入-query',7300,0,'',NULL,NULL,'','1','1','F','0','0','erp:order:query','','erp-test-init',NOW()),
(7302,'订单录入-add',7300,0,'',NULL,NULL,'','1','1','F','0','0','erp:order:add','','erp-test-init',NOW()),
(7303,'订单录入-edit',7300,0,'',NULL,NULL,'','1','1','F','0','0','erp:order:edit','','erp-test-init',NOW()),
(7304,'订单录入-import',7300,0,'',NULL,NULL,'','1','1','F','0','0','erp:order:import','','erp-test-init',NOW()),
(7305,'订单录入-export',7300,0,'',NULL,NULL,'','1','1','F','0','0','erp:order:export','','erp-test-init',NOW()),
(7306,'订单录入-review',7300,0,'',NULL,NULL,'','1','1','F','0','0','erp:order:review','','erp-test-init',NOW()),
(7307,'订单录入-cancel',7300,0,'',NULL,NULL,'','1','1','F','0','0','erp:order:cancel','','erp-test-init',NOW()),
(7400,'订单导入任务',6000,0,'importJob','erp/importJob/index',NULL,'','1','1','C','0','0','erp:importJob:list','list','erp-test-init',NOW()),
(7401,'订单导入任务-query',7400,0,'',NULL,NULL,'','1','1','F','0','0','erp:importJob:query','','erp-test-init',NOW()),
(7500,'包裹发货',6000,0,'package','erp/package/index',NULL,'','1','1','C','0','0','erp:package:list','list','erp-test-init',NOW()),
(7501,'包裹发货-query',7500,0,'',NULL,NULL,'','1','1','F','0','0','erp:package:query','','erp-test-init',NOW()),
(7502,'包裹发货-edit',7500,0,'',NULL,NULL,'','1','1','F','0','0','erp:package:edit','','erp-test-init',NOW()),
(7503,'包裹发货-ship',7500,0,'',NULL,NULL,'','1','1','F','0','0','erp:package:ship','','erp-test-init',NOW()),
(7600,'供应商管理',6000,0,'supplier','erp/supplier/index',NULL,'','1','1','C','0','0','erp:supplier:list','list','erp-test-init',NOW()),
(7601,'供应商管理-query',7600,0,'',NULL,NULL,'','1','1','F','0','0','erp:supplier:query','','erp-test-init',NOW()),
(7602,'供应商管理-add',7600,0,'',NULL,NULL,'','1','1','F','0','0','erp:supplier:add','','erp-test-init',NOW()),
(7603,'供应商管理-edit',7600,0,'',NULL,NULL,'','1','1','F','0','0','erp:supplier:edit','','erp-test-init',NOW()),
(7700,'采购单',6000,0,'purchase','erp/purchase/index',NULL,'','1','1','C','0','0','erp:purchase:list','list','erp-test-init',NOW()),
(7701,'采购单-query',7700,0,'',NULL,NULL,'','1','1','F','0','0','erp:purchase:query','','erp-test-init',NOW()),
(7702,'采购单-add',7700,0,'',NULL,NULL,'','1','1','F','0','0','erp:purchase:add','','erp-test-init',NOW()),
(7703,'采购单-edit',7700,0,'',NULL,NULL,'','1','1','F','0','0','erp:purchase:edit','','erp-test-init',NOW()),
(7704,'采购单-confirm',7700,0,'',NULL,NULL,'','1','1','F','0','0','erp:purchase:confirm','','erp-test-init',NOW()),
(7705,'采购单-cancel',7700,0,'',NULL,NULL,'','1','1','F','0','0','erp:purchase:cancel','','erp-test-init',NOW()),
(7706,'采购单-close',7700,0,'',NULL,NULL,'','1','1','F','0','0','erp:purchase:close','','erp-test-init',NOW()),
(7800,'采购收货记录',6000,0,'purchaseReceipt','erp/purchaseReceipt/index',NULL,'','1','1','C','0','0','erp:purchaseReceipt:list','list','erp-test-init',NOW()),
(7801,'采购收货记录-query',7800,0,'',NULL,NULL,'','1','1','F','0','0','erp:purchaseReceipt:query','','erp-test-init',NOW()),
(7900,'采购收货作业',6000,0,'purchaseReceive','erp/purchaseReceive/index',NULL,'','1','1','C','0','0','erp:purchase:receive','list','erp-test-init',NOW()),
(8000,'采购退货',6000,0,'purchaseReturnOrder','erp/purchaseReturnOrder/index',NULL,'','1','1','C','0','0','erp:purchaseReturnOrder:list','list','erp-test-init',NOW()),
(8001,'采购退货-query',8000,0,'',NULL,NULL,'','1','1','F','0','0','erp:purchaseReturnOrder:query','','erp-test-init',NOW()),
(8002,'采购退货-return',8000,0,'',NULL,NULL,'','1','1','F','0','0','erp:purchaseReturnOrder:return','','erp-test-init',NOW()),
(8100,'订单退货入库',6000,0,'salesReturnOrder','erp/salesReturnOrder/index',NULL,'','1','1','C','0','0','erp:salesReturnOrder:list','list','erp-test-init',NOW()),
(8101,'订单退货入库-query',8100,0,'',NULL,NULL,'','1','1','F','0','0','erp:salesReturnOrder:query','','erp-test-init',NOW()),
(8102,'订单退货入库-return',8100,0,'',NULL,NULL,'','1','1','F','0','0','erp:salesReturnOrder:return','','erp-test-init',NOW()),
(8200,'仓库调拨',6000,0,'transfer','erp/transfer/index',NULL,'','1','1','C','0','0','erp:transfer:list','list','erp-test-init',NOW()),
(8201,'仓库调拨-query',8200,0,'',NULL,NULL,'','1','1','F','0','0','erp:transfer:query','','erp-test-init',NOW()),
(8202,'仓库调拨-add',8200,0,'',NULL,NULL,'','1','1','F','0','0','erp:transfer:add','','erp-test-init',NOW()),
(8203,'仓库调拨-edit',8200,0,'',NULL,NULL,'','1','1','F','0','0','erp:transfer:edit','','erp-test-init',NOW()),
(8204,'仓库调拨-dispatch',8200,0,'',NULL,NULL,'','1','1','F','0','0','erp:transfer:dispatch','','erp-test-init',NOW()),
(8205,'仓库调拨-receive',8200,0,'',NULL,NULL,'','1','1','F','0','0','erp:transfer:receive','','erp-test-init',NOW()),
(8206,'仓库调拨-cancel',8200,0,'',NULL,NULL,'','1','1','F','0','0','erp:transfer:cancel','','erp-test-init',NOW()),
(8300,'财务资金账户',6000,0,'financeAccount','erp/financeAccount/index',NULL,'','1','1','C','0','0','erp:financeAccount:list','list','erp-test-init',NOW()),
(8301,'财务资金账户-query',8300,0,'',NULL,NULL,'','1','1','F','0','0','erp:financeAccount:query','','erp-test-init',NOW()),
(8302,'财务资金账户-add',8300,0,'',NULL,NULL,'','1','1','F','0','0','erp:financeAccount:add','','erp-test-init',NOW()),
(8303,'财务资金账户-edit',8300,0,'',NULL,NULL,'','1','1','F','0','0','erp:financeAccount:edit','','erp-test-init',NOW()),
(8400,'采购应付',6000,0,'payable','erp/payable/index',NULL,'','1','1','C','0','0','erp:payable:list','list','erp-test-init',NOW()),
(8401,'采购应付-query',8400,0,'',NULL,NULL,'','1','1','F','0','0','erp:payable:query','','erp-test-init',NOW()),
(8402,'采购应付-add',8400,0,'',NULL,NULL,'','1','1','F','0','0','erp:payable:add','','erp-test-init',NOW()),
(8403,'采购应付-adjust',8400,0,'',NULL,NULL,'','1','1','F','0','0','erp:payable:adjust','','erp-test-init',NOW()),
(8404,'采购应付-export',8400,0,'',NULL,NULL,'','1','1','F','0','0','erp:payable:export','','erp-test-init',NOW()),
(8500,'采购付款',6000,0,'payment','erp/payment/index',NULL,'','1','1','C','0','0','erp:payment:list','list','erp-test-init',NOW()),
(8501,'采购付款-query',8500,0,'',NULL,NULL,'','1','1','F','0','0','erp:payment:query','','erp-test-init',NOW()),
(8502,'采购付款-add',8500,0,'',NULL,NULL,'','1','1','F','0','0','erp:payment:add','','erp-test-init',NOW()),
(8503,'采购付款-reverse',8500,0,'',NULL,NULL,'','1','1','F','0','0','erp:payment:reverse','','erp-test-init',NOW()),
(8600,'供应商对账',6000,0,'financeStatement','erp/financeStatement/index',NULL,'','1','1','C','0','0','erp:financeStatement:list','list','erp-test-init',NOW()),
(8601,'供应商对账-export',8600,0,'',NULL,NULL,'','1','1','F','0','0','erp:financeStatement:export','','erp-test-init',NOW()),
(8700,'销售应收',6000,0,'receivable','erp/receivable/index',NULL,'','1','1','C','0','0','erp:receivable:list','list','erp-test-init',NOW()),
(8701,'销售应收-query',8700,0,'',NULL,NULL,'','1','1','F','0','0','erp:receivable:query','','erp-test-init',NOW()),
(8702,'销售应收-add',8700,0,'',NULL,NULL,'','1','1','F','0','0','erp:receivable:add','','erp-test-init',NOW()),
(8703,'销售应收-adjust',8700,0,'',NULL,NULL,'','1','1','F','0','0','erp:receivable:adjust','','erp-test-init',NOW()),
(8704,'销售应收-export',8700,0,'',NULL,NULL,'','1','1','F','0','0','erp:receivable:export','','erp-test-init',NOW()),
(8800,'销售收款',6000,0,'collection','erp/collection/index',NULL,'','1','1','C','0','0','erp:collection:list','list','erp-test-init',NOW()),
(8801,'销售收款-query',8800,0,'',NULL,NULL,'','1','1','F','0','0','erp:collection:query','','erp-test-init',NOW()),
(8802,'销售收款-add',8800,0,'',NULL,NULL,'','1','1','F','0','0','erp:collection:add','','erp-test-init',NOW()),
(8803,'销售收款-reverse',8800,0,'',NULL,NULL,'','1','1','F','0','0','erp:collection:reverse','','erp-test-init',NOW()),
(8900,'店铺对账',6000,0,'shopStatement','erp/shopStatement/index',NULL,'','1','1','C','0','0','erp:shopStatement:list','list','erp-test-init',NOW()),
(8901,'店铺对账-export',8900,0,'',NULL,NULL,'','1','1','F','0','0','erp:shopStatement:export','','erp-test-init',NOW()),
(9000,'商品档案',6000,0,'product','erp/product/index',NULL,'','1','1','C','0','0','erp:product:list','list','erp-test-init',NOW()),
(9001,'商品档案-query',9000,0,'',NULL,NULL,'','1','1','F','0','0','erp:product:query','','erp-test-init',NOW()),
(9002,'商品档案-add',9000,0,'',NULL,NULL,'','1','1','F','0','0','erp:product:add','','erp-test-init',NOW()),
(9003,'商品档案-edit',9000,0,'',NULL,NULL,'','1','1','F','0','0','erp:product:edit','','erp-test-init',NOW()),
(9004,'商品档案-import',9000,0,'',NULL,NULL,'','1','1','F','0','0','erp:product:import','','erp-test-init',NOW()),
(9005,'商品档案-export',9000,0,'',NULL,NULL,'','1','1','F','0','0','erp:product:export','','erp-test-init',NOW()),
(9006,'商品档案-image',9000,0,'',NULL,NULL,'','1','1','F','0','0','erp:product:image','','erp-test-init',NOW()),
(9100,'商品分类',6000,0,'productCategory','erp/productCategory/index',NULL,'','1','1','C','0','0','erp:productCategory:list','list','erp-test-init',NOW()),
(9101,'商品分类-add',9100,0,'',NULL,NULL,'','1','1','F','0','0','erp:productCategory:add','','erp-test-init',NOW()),
(9102,'商品分类-edit',9100,0,'',NULL,NULL,'','1','1','F','0','0','erp:productCategory:edit','','erp-test-init',NOW()),
(9200,'物流设置',6000,0,'logistics','erp/logistics/index',NULL,'','1','1','C','0','0','erp:logistics:list','list','erp-test-init',NOW()),
(9201,'物流设置-query',9200,0,'',NULL,NULL,'','1','1','F','0','0','erp:logistics:query','','erp-test-init',NOW()),
(9202,'物流设置-add',9200,0,'',NULL,NULL,'','1','1','F','0','0','erp:logistics:add','','erp-test-init',NOW()),
(9203,'物流设置-edit',9200,0,'',NULL,NULL,'','1','1','F','0','0','erp:logistics:edit','','erp-test-init',NOW()),
(9300,'发货记录',6000,0,'shipment','erp/shipment/index',NULL,'','1','1','C','0','0','erp:shipment:list','list','erp-test-init',NOW()),
(9301,'发货记录-export',9300,0,'',NULL,NULL,'','1','1','F','0','0','erp:shipment:export','','erp-test-init',NOW()),
(9400,'经营概览',6000,0,'overview','erp/overview/index',NULL,'','1','1','C','0','0','erp:overview:list','list','erp-test-init',NOW()),
(9500,'企业启用',6000,0,'setup','erp/setup/index',NULL,'','1','1','C','0','0','erp:setup:list','list','erp-test-init',NOW()),
(9600,'拣货与打包',6000,0,'picking','erp/picking/index',NULL,'','1','1','C','0','0','erp:picking:list','list','erp-test-init',NOW()),
(9601,'拣货与打包-query',9600,0,'',NULL,NULL,'','1','1','F','0','0','erp:picking:query','','erp-test-init',NOW()),
(9602,'拣货与打包-check',9600,0,'',NULL,NULL,'','1','1','F','0','0','erp:picking:check','','erp-test-init',NOW()),
(9603,'拣货与打包-ship',9600,0,'',NULL,NULL,'','1','1','F','0','0','erp:picking:ship','','erp-test-init',NOW()),
(6501,'库存导出',6500,0,'',NULL,NULL,'','1','1','F','0','0','erp:stock:export','','erp-test-init',NOW()),
(6502,'预警配置',6500,0,'',NULL,NULL,'','1','1','F','0','0','erp:stock:alert','','erp-test-init',NOW()),
(6503,'只读库存核对',6500,0,'',NULL,NULL,'','1','1','F','0','0','erp:stock:reconcile','','erp-test-init',NOW()),
(6601,'流水导出',6600,0,'',NULL,NULL,'','1','1','F','0','0','erp:ledger:export','','erp-test-init',NOW());
INSERT INTO sys_tenant_menu(tenant_id,menu_id)
SELECT 1,m.menu_id FROM sys_menu m WHERE m.menu_id IN (5100,5000,5001,5002,5003,5004,5005,5006,5007) AND NOT EXISTS(SELECT 1 FROM sys_tenant_menu g WHERE g.tenant_id=1 AND g.menu_id=m.menu_id);
INSERT INTO sys_tenant_menu(tenant_id,menu_id)
SELECT t.tenant_id,m.menu_id FROM sys_tenant t CROSS JOIN sys_menu m
WHERE t.tenant_id IN (10000,10001) AND m.status='0' AND COALESCE(m.perms,'') NOT LIKE 'platform:%' AND COALESCE(m.perms,'') NOT LIKE 'tool:%' AND COALESCE(m.perms,'') NOT LIKE 'system:menu:%' AND COALESCE(m.perms,'') NOT LIKE 'monitor:cache:%' AND COALESCE(m.perms,'') NOT LIKE 'monitor:server:%' AND COALESCE(m.perms,'') NOT LIKE 'monitor:online:%' AND m.path NOT IN('platform','tenant','tool','gen','menu','online','cache','cacheList','server','druid','swagger')
AND NOT EXISTS(SELECT 1 FROM sys_tenant_menu g WHERE g.tenant_id=t.tenant_id AND g.menu_id=m.menu_id);
INSERT INTO sys_role_menu(tenant_id,role_id,menu_id)
SELECT r.tenant_id,r.role_id,g.menu_id FROM sys_role r JOIN sys_tenant_menu g ON g.tenant_id=r.tenant_id
WHERE r.role_key IN ('platform_admin','tenant_admin');
INSERT INTO erp_warehouse(warehouse_id,tenant_id,warehouse_code,warehouse_name,status,create_by,create_time) VALUES
(1,10000,'DEFAULT','默认仓库','0','erp-test-init',NOW()),(2,10001,'DEFAULT','默认仓库','0','erp-test-init',NOW());

END$$
CALL _ruoyi_erp_test_init_20261103()$$
DROP PROCEDURE _ruoyi_erp_test_init_20261103$$
DELIMITER ;
SELECT 'ERP_INIT_COMPLETE' AS result,DATABASE() AS database_name,
(SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()) AS table_count,
(SELECT COUNT(*) FROM erp_schema_migration) AS migration_count;
