-- Explicit portal-only v10 -> v11. No role grants or existing content URL changes.
CREATE TABLE cms_redirect (
 redirect_id BIGINT NOT NULL AUTO_INCREMENT,
 source_path VARCHAR(200) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 target_id BIGINT NOT NULL,
 status CHAR(1) NOT NULL DEFAULT '1',
 version BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL DEFAULT '', create_time DATETIME NOT NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '', update_time DATETIME DEFAULT NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(redirect_id), UNIQUE KEY uk_cms_redirect_source(source_path),
 KEY idx_cms_redirect_status(status,redirect_id),
 CONSTRAINT fk_cms_redirect_target FOREIGN KEY(target_id) REFERENCES cms_content(content_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='门户旧路径映射，停用保留';
INSERT INTO sys_menu(menu_id,menu_name,parent_id,order_num,path,component,is_frame,is_cache,menu_type,visible,status,perms,icon,create_by,create_time) VALUES
(6080,'旧地址跳转',6000,8,'redirect','cms/redirect/index',1,0,'C','0','0','cms:redirect:list','link','portal-init',NOW()),
(6081,'跳转查询',6080,1,'#','',1,0,'F','0','0','cms:redirect:query','#','portal-init',NOW()),
(6082,'跳转新增',6080,2,'#','',1,0,'F','0','0','cms:redirect:add','#','portal-init',NOW()),
(6083,'跳转修改',6080,3,'#','',1,0,'F','0','0','cms:redirect:edit','#','portal-init',NOW()),
(6084,'跳转启停',6080,4,'#','',1,0,'F','0','0','cms:redirect:publish','#','portal-init',NOW());
