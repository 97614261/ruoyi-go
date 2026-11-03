-- Portal v15 -> v16: download center and trusted industry presets only. No baseline ALTER or role grants.
CREATE TABLE cms_download_category (
 category_id BIGINT NOT NULL AUTO_INCREMENT,name VARCHAR(60) NOT NULL,order_num INT NOT NULL DEFAULT 0,enabled TINYINT NOT NULL DEFAULT 1,is_deleted TINYINT NOT NULL DEFAULT 0,version BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(category_id),KEY idx_cms_dlcat_list(is_deleted,enabled,order_num,category_id),KEY idx_cms_dlcat_name(name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE cms_download_file (
 file_id BIGINT NOT NULL AUTO_INCREMENT,storage_key VARCHAR(80) NOT NULL,name VARCHAR(160) NOT NULL,extension VARCHAR(8) NOT NULL,mime_type VARCHAR(120) NOT NULL,byte_size BIGINT NOT NULL,sha256 CHAR(64) NOT NULL,is_deleted TINYINT NOT NULL DEFAULT 0,version BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(file_id),UNIQUE KEY uk_cms_dlfile_storage(storage_key),KEY idx_cms_dlfile_name(is_deleted,name,file_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE cms_download (
 download_id BIGINT NOT NULL AUTO_INCREMENT,working_revision_id BIGINT DEFAULT NULL,published_revision_id BIGINT DEFAULT NULL,revision_seq INT NOT NULL DEFAULT 0,is_published TINYINT NOT NULL DEFAULT 0,is_deleted TINYINT NOT NULL DEFAULT 0,published_at DATETIME DEFAULT NULL,version BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(download_id),KEY idx_cms_download_public(is_deleted,is_published,published_at,download_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE cms_download_revision (
 revision_id BIGINT NOT NULL AUTO_INCREMENT,download_id BIGINT NOT NULL,revision_no INT NOT NULL,category_id BIGINT NOT NULL,file_id BIGINT NOT NULL,title VARCHAR(120) NOT NULL,summary VARCHAR(500) NOT NULL DEFAULT '',
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(revision_id),UNIQUE KEY uk_cms_download_revision(download_id,revision_no),KEY idx_cms_dlrev_file(file_id),KEY idx_cms_dlrev_category(category_id,revision_id),KEY idx_cms_dlrev_title(title,revision_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE cms_preset_application (
 application_id BIGINT NOT NULL AUTO_INCREMENT,actor_user_id BIGINT NOT NULL,request_id CHAR(36) NOT NULL,request_sha256 CHAR(64) NOT NULL,preset_id VARCHAR(32) NOT NULL,preset_version INT NOT NULL,site_version_before BIGINT NOT NULL,site_version_after BIGINT NOT NULL,page_ids TEXT NOT NULL,before_config_json MEDIUMTEXT NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(application_id),UNIQUE KEY uk_cms_preset_request(actor_user_id,request_id),KEY idx_cms_preset_history(application_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
INSERT INTO sys_menu(menu_id,menu_name,parent_id,order_num,path,component,is_frame,is_cache,menu_type,visible,status,perms,icon,create_by,create_time) VALUES
(6120,'资料下载中心',6000,16,'download','cms/download/index',1,0,'C','0','0','cms:download:list','download','portal',NOW()),
(6121,'资料详情',6120,1,'#','',1,0,'F','0','0','cms:download:query','#','portal',NOW()),
(6122,'新增资料',6120,2,'#','',1,0,'F','0','0','cms:download:add','#','portal',NOW()),
(6123,'编辑资料',6120,3,'#','',1,0,'F','0','0','cms:download:edit','#','portal',NOW()),
(6124,'删除资料',6120,4,'#','',1,0,'F','0','0','cms:download:remove','#','portal',NOW()),
(6125,'发布下架资料',6120,5,'#','',1,0,'F','0','0','cms:download:publish','#','portal',NOW()),
(6126,'上传资料附件',6120,6,'#','',1,0,'F','0','0','cms:download:upload','#','portal',NOW()),
(6127,'管理资料分类',6120,7,'#','',1,0,'F','0','0','cms:download:category','#','portal',NOW()),
(6128,'清理无引用附件',6120,8,'#','',1,0,'F','0','0','cms:download:fileRemove','#','portal',NOW()),
(6130,'行业模板与页面预设',6000,17,'presets','cms/presets/index',1,0,'C','0','0','cms:preset:list','build','portal',NOW()),
(6131,'应用行业预设草稿',6130,1,'#','',1,0,'F','0','0','cms:preset:apply','#','portal',NOW());
