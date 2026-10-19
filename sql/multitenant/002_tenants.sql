-- Only run after base.sql in a brand-new, empty ruoyi_go_mt database.
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

-- Preserve the original customer IDs, but they are no longer platform identities.
UPDATE sys_user SET user_type='01' WHERE user_id=1;
UPDATE sys_user SET status='1' WHERE user_id<>1;
UPDATE sys_job SET status='1';
UPDATE sys_role SET role_key='tenant_admin' WHERE role_id=1;
UPDATE sys_tenant SET owner_user_id=1 WHERE tenant_id=10000;
-- Platform configuration/dictionaries are explicit onboarding defaults, not another customer's live values.
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
-- user_id=0 was a Java "no department" sentinel; preserve as NULL before adding the relationship.
UPDATE sys_user SET dept_id=NULL WHERE dept_id=0;
ALTER TABLE sys_user ADD CONSTRAINT fk_user_dept FOREIGN KEY(tenant_id,dept_id) REFERENCES sys_dept(tenant_id,dept_id);
