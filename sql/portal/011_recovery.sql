-- Explicit portal-only v11 -> v12. Personal recovery cache, no role grants.
CREATE TABLE cms_content_recovery (
 recovery_id BIGINT NOT NULL AUTO_INCREMENT,
 user_id BIGINT NOT NULL,
 kind VARCHAR(16) NOT NULL,
 content_id BIGINT NOT NULL DEFAULT 0,
 base_version BIGINT NOT NULL DEFAULT 0,
 payload MEDIUMTEXT NOT NULL,
 version BIGINT NOT NULL DEFAULT 1,
 expires_at DATETIME NOT NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT '', create_time DATETIME NOT NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '', update_time DATETIME DEFAULT NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(recovery_id), UNIQUE KEY uk_cms_recovery_owner(user_id,kind,content_id),
 KEY idx_cms_recovery_expiry(expires_at,recovery_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='个人自动保存恢复副本，不参与发布';
