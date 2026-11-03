-- Explicit v6 -> v7. Inquiry metadata and separate permissions, no grants.
CREATE TABLE cms_inquiry(inquiry_id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,name VARCHAR(60) NOT NULL,contact VARCHAR(120) NOT NULL,message VARCHAR(2000) NOT NULL,status VARCHAR(16) NOT NULL DEFAULT 'NEW',note VARCHAR(500) NOT NULL DEFAULT '',version BIGINT NOT NULL DEFAULT 1,is_deleted TINYINT NOT NULL DEFAULT 0,create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME NULL,remark VARCHAR(500) NOT NULL DEFAULT '',KEY idx_inquiry_status(is_deleted,status,inquiry_id),KEY idx_inquiry_list(is_deleted,inquiry_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
INSERT INTO sys_menu(menu_id,menu_name,parent_id,order_num,path,component,is_frame,is_cache,menu_type,visible,status,perms,icon,create_by,create_time) VALUES
(6060,'询盘留言',6000,7,'inquiry','cms/inquiry/index',1,0,'C','0','0','cms:inquiry:list','message','portal-init',NOW()),
(6061,'询盘详情',6060,1,'#','',1,0,'F','0','0','cms:inquiry:query','#','portal-init',NOW()),
(6062,'处理询盘',6060,2,'#','',1,0,'F','0','0','cms:inquiry:edit','#','portal-init',NOW()),
(6063,'清除询盘',6060,3,'#','',1,0,'F','0','0','cms:inquiry:remove','#','portal-init',NOW());
