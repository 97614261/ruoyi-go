-- Portal-only v13 -> v14; opt-in workflow. No role grants or baseline structure changes.
CREATE TABLE cms_collaboration_setting (
 setting_id BIGINT NOT NULL, inquiry_isolation TINYINT NOT NULL DEFAULT 0, manager_ids TEXT NOT NULL,version BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(setting_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE cms_review_policy (
 kind VARCHAR(16) NOT NULL,required TINYINT NOT NULL DEFAULT 0,version BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(kind)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE cms_draft_actor (
 kind VARCHAR(16) NOT NULL,snapshot_key VARCHAR(80) NOT NULL,object_id BIGINT NOT NULL,author_user_id BIGINT NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(kind,snapshot_key),KEY idx_cms_actor_author(author_user_id,kind,object_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE cms_review_request (
 review_id BIGINT NOT NULL AUTO_INCREMENT,kind VARCHAR(16) NOT NULL,object_id BIGINT NOT NULL,snapshot_key VARCHAR(80) NOT NULL,content_version BIGINT NOT NULL,snapshot_hash CHAR(64) NOT NULL,snapshot_json MEDIUMTEXT NOT NULL,author_user_id BIGINT NOT NULL,submitter_user_id BIGINT NOT NULL,reviewer_user_id BIGINT NOT NULL DEFAULT 0,policy_version BIGINT NOT NULL,status VARCHAR(16) NOT NULL,opinion VARCHAR(1000) NOT NULL DEFAULT '',version BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(review_id),KEY idx_cms_review_object(kind,object_id,status,review_id),KEY idx_cms_review_queue(status,kind,review_id),KEY idx_cms_review_submitter(submitter_user_id,status,review_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE cms_review_event (
 event_id BIGINT NOT NULL AUTO_INCREMENT,review_id BIGINT NOT NULL DEFAULT 0,kind VARCHAR(16) NOT NULL,object_id BIGINT NOT NULL DEFAULT 0,action VARCHAR(24) NOT NULL,actor_user_id BIGINT NOT NULL,reason VARCHAR(1000) NOT NULL DEFAULT '',details TEXT NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(event_id),KEY idx_cms_event_review(review_id,event_id),KEY idx_cms_event_policy(action,event_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE cms_inquiry_team (
 inquiry_id BIGINT NOT NULL,owner_user_id BIGINT NOT NULL DEFAULT 0,next_follow_at DATETIME DEFAULT NULL,reminder_sent_version BIGINT NOT NULL DEFAULT 0,version BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(inquiry_id),KEY idx_cms_team_owner(owner_user_id,inquiry_id),KEY idx_cms_team_due(next_follow_at,inquiry_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE cms_inquiry_followup (
 followup_id BIGINT NOT NULL AUTO_INCREMENT,inquiry_id BIGINT NOT NULL,actor_user_id BIGINT NOT NULL,event_type VARCHAR(16) NOT NULL DEFAULT 'FOLLOWUP',text VARCHAR(2000) NOT NULL DEFAULT '',result VARCHAR(200) NOT NULL DEFAULT '',is_erased TINYINT NOT NULL DEFAULT 0,
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(followup_id),KEY idx_cms_followup_inquiry(inquiry_id,followup_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE cms_notification_outbox (
 outbox_id BIGINT NOT NULL AUTO_INCREMENT,event_key VARCHAR(160) NOT NULL,recipient_user_id BIGINT NOT NULL,resource_type VARCHAR(16) NOT NULL,resource_id BIGINT NOT NULL,kind VARCHAR(16) NOT NULL DEFAULT '',event_type VARCHAR(32) NOT NULL,business_version BIGINT NOT NULL,subject VARCHAR(120) NOT NULL,status VARCHAR(16) NOT NULL DEFAULT 'PENDING',attempts INT NOT NULL DEFAULT 0,
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(outbox_id),UNIQUE KEY uk_cms_outbox_event(event_key),KEY idx_cms_outbox_pending(status,outbox_id),KEY idx_cms_outbox_resource(resource_type,resource_id,status),KEY idx_cms_outbox_age(create_time,outbox_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE cms_notification (
 notification_id BIGINT NOT NULL AUTO_INCREMENT,outbox_id BIGINT NOT NULL,recipient_user_id BIGINT NOT NULL,resource_type VARCHAR(16) NOT NULL,resource_id BIGINT NOT NULL,kind VARCHAR(16) NOT NULL DEFAULT '',event_type VARCHAR(32) NOT NULL,business_version BIGINT NOT NULL,subject VARCHAR(120) NOT NULL,read_at DATETIME DEFAULT NULL,expires_at DATETIME NOT NULL,version BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME DEFAULT NULL,remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(notification_id),UNIQUE KEY uk_cms_notification_outbox(outbox_id),KEY idx_cms_notification_inbox(recipient_user_id,read_at,notification_id),KEY idx_cms_notification_resource(resource_type,resource_id),KEY idx_cms_notification_expiry(expires_at,notification_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
INSERT INTO cms_collaboration_setting(setting_id,manager_ids,create_by,create_time) VALUES(1,'[1]','portal',NOW());
INSERT INTO cms_review_policy(kind,create_by,create_time) VALUES('ARTICLE','portal',NOW()),('PRODUCT','portal',NOW()),('PAGE','portal',NOW()),('SITE','portal',NOW());
INSERT INTO sys_menu(menu_id,menu_name,parent_id,order_num,path,component,is_frame,is_cache,menu_type,visible,status,perms,icon,create_by,create_time) VALUES
(6100,'协作设置',6000,11,'workflow','cms/workflow/index',1,0,'C','0','0','cms:workflow:list','peoples','portal',NOW()),
(6101,'协作策略修改',6100,1,'#','',1,0,'F','0','0','cms:workflow:edit','#','portal',NOW()),
(6102,'审核中心',6000,12,'review','cms/review/index',1,0,'C','0','0','cms:review:list','validCode','portal',NOW()),
(6103,'站内消息',6000,13,'notification','cms/notification/index',1,0,'C','0','0','cms:notification:list','message','portal',NOW()),
(6104,'文章审核',6020,10,'#','',1,0,'F','0','0','cms:article:review','#','portal',NOW()),
(6105,'产品审核',6040,10,'#','',1,0,'F','0','0','cms:product:review','#','portal',NOW()),
(6106,'单页审核',6030,10,'#','',1,0,'F','0','0','cms:page:review','#','portal',NOW()),
(6107,'外观审核',6050,10,'#','',1,0,'F','0','0','cms:site:review','#','portal',NOW()),
(6108,'询盘指派',6060,10,'#','',1,0,'F','0','0','cms:inquiry:assign','#','portal',NOW()),
(6109,'全部询盘范围',6060,11,'#','',1,0,'F','0','0','cms:inquiry:all','#','portal',NOW());
