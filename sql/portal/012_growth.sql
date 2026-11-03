-- Portal-only v12 -> v13. No sys table structure or role grants change.
CREATE TABLE cms_tag (
 tag_id BIGINT NOT NULL AUTO_INCREMENT, name VARCHAR(40) NOT NULL, status CHAR(1) NOT NULL DEFAULT '0',version BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(tag_id),UNIQUE KEY uk_cms_tag_name(name),KEY idx_cms_tag_status(status,tag_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE cms_revision_ops (
 revision_id BIGINT NOT NULL,featured TINYINT NOT NULL DEFAULT 0,pin_order INT NOT NULL DEFAULT 0,payload TEXT NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(revision_id),KEY idx_cms_ops_order(featured,pin_order,revision_id),CONSTRAINT fk_cms_ops_revision FOREIGN KEY(revision_id) REFERENCES cms_content_revision(revision_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE cms_revision_tag (
 revision_id BIGINT NOT NULL,tag_id BIGINT NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(revision_id,tag_id),KEY idx_cms_tag_revision(tag_id,revision_id),CONSTRAINT fk_cms_rt_revision FOREIGN KEY(revision_id) REFERENCES cms_content_revision(revision_id),CONSTRAINT fk_cms_rt_tag FOREIGN KEY(tag_id) REFERENCES cms_tag(tag_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE cms_visit_daily (
 visit_day DATE NOT NULL,dimension VARCHAR(16) NOT NULL,dimension_value VARCHAR(160) NOT NULL,hits BIGINT NOT NULL DEFAULT 0,
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(visit_day,dimension,dimension_value),KEY idx_cms_visit_dimension(dimension,visit_day)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
INSERT INTO sys_menu(menu_id,menu_name,parent_id,order_num,path,component,is_frame,is_cache,menu_type,visible,status,perms,icon,create_by,create_time,update_by,remark) VALUES
(6090,'内容标签',6000,9,'tag','cms/tag/index',1,0,'C','0','0','cms:tag:list','#','portal',NOW(),'','门户标签'),
(6091,'标签查询',6090,1,'','',1,0,'F','0','0','cms:tag:query','#','portal',NOW(),'',''),
(6092,'标签新增',6090,2,'','',1,0,'F','0','0','cms:tag:add','#','portal',NOW(),'',''),
(6093,'标签修改',6090,3,'','',1,0,'F','0','0','cms:tag:edit','#','portal',NOW(),'',''),
(6094,'访问统计',6000,10,'statistics','cms/statistics/index',1,0,'C','0','0','cms:statistics:list','chart','portal',NOW(),'','仅聚合统计，无访客身份');
