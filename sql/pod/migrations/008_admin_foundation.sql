-- POD-only administration foundation. No legacy task or billing conversion.
CREATE TABLE pod_platform_state (
 state_id BIGINT NOT NULL,
 revision BIGINT NOT NULL DEFAULT 1,
 managed_enabled TINYINT NOT NULL DEFAULT 0,
 default_text_model_id BIGINT NULL,
 default_image_model_id BIGINT NULL,
 legacy_run_max_id BIGINT NOT NULL DEFAULT 0,
 legacy_batch_max_id BIGINT NOT NULL DEFAULT 0,
 cutover_time DATETIME(3) NULL,
 create_by VARCHAR(64) NOT NULL DEFAULT 'podsetup',
 create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME(3) NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (state_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
INSERT INTO pod_platform_state(state_id) VALUES (1);

CREATE TABLE pod_admin_action (
 action_id BIGINT NOT NULL AUTO_INCREMENT,
 actor_user_id BIGINT NOT NULL,
 request_id VARCHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 action_type VARCHAR(64) NOT NULL,
 resource_type VARCHAR(32) NOT NULL,
 resource_id BIGINT NOT NULL,
 state VARCHAR(24) NOT NULL,
 result_json JSON NOT NULL,
 create_by VARCHAR(64) NOT NULL,
 create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME(3) NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (action_id),
 UNIQUE KEY uk_admin_request (actor_user_id,request_id),
 KEY idx_admin_resource (resource_type,resource_id,action_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE pod_admin_audit (
 audit_id BIGINT NOT NULL AUTO_INCREMENT,
 actor_user_id BIGINT NOT NULL,
 owner_user_id BIGINT NOT NULL DEFAULT 0,
 action_type VARCHAR(64) NOT NULL,
 resource_type VARCHAR(32) NOT NULL,
 resource_id BIGINT NOT NULL DEFAULT 0,
 reason VARCHAR(200) NOT NULL DEFAULT '',
 before_json JSON NOT NULL,
 after_json JSON NOT NULL,
 trace_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 outcome VARCHAR(24) NOT NULL,
 create_by VARCHAR(64) NOT NULL,
 create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME(3) NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (audit_id),
 KEY idx_admin_audit_actor (actor_user_id,create_time,audit_id),
 KEY idx_admin_audit_owner (owner_user_id,create_time,audit_id),
 KEY idx_admin_audit_resource (resource_type,resource_id,audit_id),
 KEY idx_admin_audit_time (create_time,audit_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE pod_private_access_grant (
 grant_id BIGINT NOT NULL AUTO_INCREMENT,
 operator_user_id BIGINT NOT NULL,
 owner_user_id BIGINT NOT NULL,
 resource_type VARCHAR(16) NOT NULL,
 resource_id BIGINT NOT NULL,
 purpose VARCHAR(200) NOT NULL,
 expires_at DATETIME(3) NOT NULL,
 revoked TINYINT NOT NULL DEFAULT 0,
 revision BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL,
 create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 update_by VARCHAR(64) NOT NULL DEFAULT '',
 update_time DATETIME(3) NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY (grant_id),
 KEY idx_private_operator (operator_user_id,resource_type,resource_id,expires_at,grant_id),
 KEY idx_private_owner (owner_user_id,grant_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
