-- Explicit v2 -> v3. Add immutable article/page revisions; do not alter original sys_* schema.
CREATE TABLE cms_content (
 content_id BIGINT NOT NULL AUTO_INCREMENT,
 kind VARCHAR(16) NOT NULL,
 slug VARCHAR(80) NOT NULL,
 working_revision_id BIGINT DEFAULT NULL,
 published_revision_id BIGINT DEFAULT NULL,
 revision_seq INT NOT NULL DEFAULT 0,
 is_published TINYINT NOT NULL DEFAULT 0,
 ever_published TINYINT NOT NULL DEFAULT 0,
 is_deleted TINYINT NOT NULL DEFAULT 0,
 published_at DATETIME DEFAULT NULL,
 version BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL DEFAULT '', create_time DATETIME NOT NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '', update_time DATETIME DEFAULT NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(content_id), UNIQUE KEY uk_cms_content_slug(kind,slug),
 KEY idx_cms_content_admin(kind,is_deleted,content_id),
 KEY idx_cms_content_public(kind,is_deleted,is_published,published_at,content_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='门户内容与发布指针';

CREATE TABLE cms_content_revision (
 revision_id BIGINT NOT NULL AUTO_INCREMENT,
 content_id BIGINT NOT NULL,
 revision_no INT NOT NULL,
 channel_id BIGINT DEFAULT NULL,
 title VARCHAR(120) NOT NULL,
 summary VARCHAR(500) NOT NULL DEFAULT '',
 body_html MEDIUMTEXT NOT NULL,
 author VARCHAR(60) NOT NULL DEFAULT '',
 cover_asset_id BIGINT DEFAULT NULL,
 cover_alt_text VARCHAR(200) NOT NULL DEFAULT '',
 seo_title VARCHAR(150) NOT NULL DEFAULT '',
 seo_description VARCHAR(300) NOT NULL DEFAULT '',
 create_by VARCHAR(64) NOT NULL DEFAULT '', create_time DATETIME NOT NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '', update_time DATETIME DEFAULT NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(revision_id),
 UNIQUE KEY uk_cms_revision_number(content_id,revision_no),
 UNIQUE KEY uk_cms_revision_owner(content_id,revision_id),
 KEY idx_cms_revision_title(title),
 CONSTRAINT fk_cms_revision_content FOREIGN KEY(content_id) REFERENCES cms_content(content_id),
 CONSTRAINT fk_cms_revision_channel FOREIGN KEY(channel_id) REFERENCES cms_channel(channel_id),
 CONSTRAINT fk_cms_revision_cover FOREIGN KEY(cover_asset_id) REFERENCES cms_asset(asset_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='不可变内容修订';

ALTER TABLE cms_content
 ADD CONSTRAINT fk_cms_content_working FOREIGN KEY(content_id,working_revision_id) REFERENCES cms_content_revision(content_id,revision_id),
 ADD CONSTRAINT fk_cms_content_published FOREIGN KEY(content_id,published_revision_id) REFERENCES cms_content_revision(content_id,revision_id);

INSERT INTO sys_menu(menu_id,menu_name,parent_id,order_num,path,component,query,is_frame,is_cache,menu_type,visible,status,perms,icon,create_by,create_time,update_by,update_time,remark) VALUES
(6020,'文章管理',6000,3,'article','cms/article/index','',1,0,'C','0','0','cms:article:list','documentation','portal-init',NOW(),'',NULL,''),
(6021,'文章查询与预览',6020,1,'#','','',1,0,'F','0','0','cms:article:query','#','portal-init',NOW(),'',NULL,''),
(6022,'文章新增',6020,2,'#','','',1,0,'F','0','0','cms:article:add','#','portal-init',NOW(),'',NULL,''),
(6023,'文章修改与恢复草稿',6020,3,'#','','',1,0,'F','0','0','cms:article:edit','#','portal-init',NOW(),'',NULL,''),
(6024,'文章发布与下架',6020,4,'#','','',1,0,'F','0','0','cms:article:publish','#','portal-init',NOW(),'',NULL,''),
(6025,'文章删除',6020,5,'#','','',1,0,'F','0','0','cms:article:remove','#','portal-init',NOW(),'',NULL,''),
(6030,'单页管理',6000,4,'page','cms/page/index','',1,0,'C','0','0','cms:page:list','form','portal-init',NOW(),'',NULL,''),
(6031,'单页查询与预览',6030,1,'#','','',1,0,'F','0','0','cms:page:query','#','portal-init',NOW(),'',NULL,''),
(6032,'单页新增',6030,2,'#','','',1,0,'F','0','0','cms:page:add','#','portal-init',NOW(),'',NULL,''),
(6033,'单页修改与恢复草稿',6030,3,'#','','',1,0,'F','0','0','cms:page:edit','#','portal-init',NOW(),'',NULL,''),
(6034,'单页发布与下架',6030,4,'#','','',1,0,'F','0','0','cms:page:publish','#','portal-init',NOW(),'',NULL,''),
(6035,'单页删除',6030,5,'#','','',1,0,'F','0','0','cms:page:remove','#','portal-init',NOW(),'',NULL,'');
