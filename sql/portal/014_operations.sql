-- Portal v14 -> v15: operations 1-4 only. No baseline ALTER and no role grants.
CREATE TABLE cms_data_policy (
 policy_id BIGINT NOT NULL,retention_days INT NOT NULL DEFAULT 0,statuses TEXT NOT NULL,version BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(policy_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE cms_data_cleanup_batch (
 batch_id BIGINT NOT NULL AUTO_INCREMENT,actor_user_id BIGINT NOT NULL,policy_version BIGINT NOT NULL,cutoff DATETIME NOT NULL,statuses TEXT NOT NULL,total_eligible BIGINT NOT NULL,preview_count INT NOT NULL,erased_count INT NOT NULL DEFAULT 0,expires_at DATETIME NOT NULL,status VARCHAR(16) NOT NULL DEFAULT 'PREVIEWED',version BIGINT NOT NULL DEFAULT 1,reason VARCHAR(300) NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(batch_id),KEY idx_cms_cleanup_actor(actor_user_id,batch_id),KEY idx_cms_cleanup_expiry(status,expires_at,batch_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE cms_data_cleanup_item (
 batch_id BIGINT NOT NULL,inquiry_id BIGINT NOT NULL,source_version BIGINT NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(batch_id,inquiry_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE cms_data_event (
 event_id BIGINT NOT NULL AUTO_INCREMENT,actor_user_id BIGINT NOT NULL,action VARCHAR(24) NOT NULL,resource_id BIGINT NOT NULL DEFAULT 0,row_count BIGINT NOT NULL DEFAULT 0,details TEXT NOT NULL,file_sha256 CHAR(64) NOT NULL DEFAULT '',
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(event_id),KEY idx_cms_data_actor(actor_user_id,event_id),KEY idx_cms_data_action(action,event_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE cms_inquiry_clock (
 inquiry_id BIGINT NOT NULL,submitted_at DATETIME NOT NULL,active_at DATETIME NOT NULL,status VARCHAR(16) NOT NULL,is_erased TINYINT NOT NULL DEFAULT 0,source_version BIGINT NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(inquiry_id),KEY idx_cms_clock_retention(is_erased,status,active_at,inquiry_id),KEY idx_cms_clock_submitted(submitted_at,inquiry_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE cms_search_document (
 content_id BIGINT NOT NULL,revision_id BIGINT NOT NULL,title_text TEXT NOT NULL,summary_text TEXT NOT NULL,body_text MEDIUMTEXT NOT NULL,search_text MEDIUMTEXT NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(content_id),KEY idx_cms_search_revision(revision_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE cms_search_posting (
 token_hash BINARY(32) NOT NULL,content_id BIGINT NOT NULL,weight INT NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(token_hash,content_id),KEY idx_cms_posting_content(content_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE cms_search_task (
 content_id BIGINT NOT NULL,desired_revision_id BIGINT DEFAULT NULL,status VARCHAR(16) NOT NULL DEFAULT 'PENDING',attempts INT NOT NULL DEFAULT 0,error_message VARCHAR(200) NOT NULL DEFAULT '',version BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(content_id),KEY idx_cms_search_task(status,content_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
INSERT INTO cms_data_policy(policy_id,statuses,create_by,create_time) VALUES(1,'["DONE","SPAM"]','portal',NOW());
INSERT INTO cms_inquiry_clock(inquiry_id,submitted_at,active_at,status,is_erased,source_version,create_by,create_time) SELECT inquiry_id,create_time,COALESCE(update_time,create_time),status,is_deleted,version,'portal',NOW() FROM cms_inquiry;
INSERT INTO cms_search_task(content_id,desired_revision_id,create_by,create_time) SELECT content_id,published_revision_id,'portal',NOW() FROM cms_content WHERE is_deleted=0 AND is_published=1 AND published_revision_id IS NOT NULL;
INSERT INTO sys_menu(menu_id,menu_name,parent_id,order_num,path,component,is_frame,is_cache,menu_type,visible,status,perms,icon,create_by,create_time) VALUES
(6110,'数据保留与审计',6000,14,'data','cms/data/index',1,0,'C','0','0','cms:data:list','log','portal',NOW()),
(6111,'保留策略设置',6110,1,'#','',1,0,'F','0','0','cms:data:edit','#','portal',NOW()),
(6112,'预览并清理询盘',6110,2,'#','',1,0,'F','0','0','cms:data:cleanup','#','portal',NOW()),
(6113,'数据操作审计',6110,3,'#','',1,0,'F','0','0','cms:data:audit','#','portal',NOW()),
(6114,'询盘导出',6060,12,'#','',1,0,'F','0','0','cms:inquiry:export','#','portal',NOW()),
(6115,'询盘敏感字段导出',6060,13,'#','',1,0,'F','0','0','cms:inquiry:exportPrivate','#','portal',NOW()),
(6116,'搜索索引',6000,15,'search-index','cms/search/index',1,0,'C','0','0','cms:search:list','search','portal',NOW()),
(6117,'搜索索引重建',6116,1,'#','',1,0,'F','0','0','cms:search:edit','#','portal',NOW());
