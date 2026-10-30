-- Only for explicit portal migration v1 -> v2. No original sys_* schema is altered.
CREATE TABLE cms_asset (
 asset_id BIGINT NOT NULL AUTO_INCREMENT,
 storage_key VARCHAR(64) NOT NULL,
 name VARCHAR(150) NOT NULL,
 alt_text VARCHAR(200) NOT NULL DEFAULT '',
 mime_type VARCHAR(40) NOT NULL,
 byte_size BIGINT NOT NULL,
 width INT NOT NULL,
 height INT NOT NULL,
 is_deleted TINYINT NOT NULL DEFAULT 0,
 version BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL DEFAULT '', create_time DATETIME NOT NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '', update_time DATETIME DEFAULT NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(asset_id), UNIQUE KEY uk_cms_asset_key(storage_key),
 KEY idx_cms_asset_list(is_deleted,asset_id), KEY idx_cms_asset_name(is_deleted,name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='门户私有素材';

CREATE TABLE cms_channel (
 channel_id BIGINT NOT NULL AUTO_INCREMENT,
 parent_id BIGINT NOT NULL DEFAULT 0,
 kind VARCHAR(16) NOT NULL,
 name VARCHAR(60) NOT NULL,
 slug VARCHAR(80) NOT NULL,
 order_num INT NOT NULL DEFAULT 0,
 status CHAR(1) NOT NULL DEFAULT '0',
 cover_asset_id BIGINT DEFAULT NULL,
 seo_title VARCHAR(150) NOT NULL DEFAULT '',
 seo_description VARCHAR(300) NOT NULL DEFAULT '',
 version BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL DEFAULT '', create_time DATETIME NOT NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '', update_time DATETIME DEFAULT NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(channel_id), UNIQUE KEY uk_cms_channel_slug(kind,slug),
 KEY idx_cms_channel_parent(parent_id,order_num,channel_id),
 KEY idx_cms_channel_filter(kind,status,order_num,channel_id), KEY idx_cms_channel_name(name),
 CONSTRAINT fk_cms_channel_cover FOREIGN KEY(cover_asset_id) REFERENCES cms_asset(asset_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='门户栏目';

CREATE TABLE cms_asset_ref (
 ref_id BIGINT NOT NULL AUTO_INCREMENT,
 asset_id BIGINT NOT NULL,
 owner_type VARCHAR(32) NOT NULL,
 owner_id BIGINT NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '', create_time DATETIME NOT NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '', update_time DATETIME DEFAULT NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(ref_id), UNIQUE KEY uk_cms_asset_ref(owner_type,owner_id,asset_id),
 KEY idx_cms_asset_ref_asset(asset_id),
 CONSTRAINT fk_cms_asset_ref FOREIGN KEY(asset_id) REFERENCES cms_asset(asset_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='门户素材引用（业务服务维护，不生成公开CRUD）';

INSERT INTO sys_menu(menu_id,menu_name,parent_id,order_num,path,component,query,is_frame,is_cache,menu_type,visible,status,perms,icon,create_by,create_time,update_by,update_time,remark) VALUES
(6000,'门户管理',0,2,'cms',NULL,'',1,0,'M','0','0','','documentation','portal-init',NOW(),'',NULL,'新门户模块'),
(6001,'栏目管理',6000,1,'channel','cms/channel/index','',1,0,'C','0','0','cms:channel:list','tree','portal-init',NOW(),'',NULL,''),
(6002,'栏目查询',6001,1,'#','','',1,0,'F','0','0','cms:channel:query','#','portal-init',NOW(),'',NULL,''),
(6003,'栏目新增',6001,2,'#','','',1,0,'F','0','0','cms:channel:add','#','portal-init',NOW(),'',NULL,''),
(6004,'栏目修改',6001,3,'#','','',1,0,'F','0','0','cms:channel:edit','#','portal-init',NOW(),'',NULL,''),
(6005,'栏目删除',6001,4,'#','','',1,0,'F','0','0','cms:channel:remove','#','portal-init',NOW(),'',NULL,''),
(6010,'素材管理',6000,2,'asset','cms/asset/index','',1,0,'C','0','0','cms:asset:list','upload','portal-init',NOW(),'',NULL,''),
(6011,'素材预览',6010,1,'#','','',1,0,'F','0','0','cms:asset:query','#','portal-init',NOW(),'',NULL,''),
(6012,'素材上传',6010,2,'#','','',1,0,'F','0','0','cms:asset:upload','#','portal-init',NOW(),'',NULL,''),
(6013,'素材修改',6010,3,'#','','',1,0,'F','0','0','cms:asset:edit','#','portal-init',NOW(),'',NULL,''),
(6014,'素材删除',6010,4,'#','','',1,0,'F','0','0','cms:asset:remove','#','portal-init',NOW(),'',NULL,'');
