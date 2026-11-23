-- POD-only creator policy, independent formal points and resource accounting.
CREATE TABLE pod_creator_policy (
 user_id BIGINT NOT NULL PRIMARY KEY, workspace_id BIGINT NOT NULL, status VARCHAR(16) NOT NULL, valid_until DATETIME(3) DEFAULT NULL,
 max_concurrent_nodes INT NOT NULL, max_pending_runs INT NOT NULL, max_daily_submissions INT NOT NULL, max_storage_bytes BIGINT NOT NULL,
 revision BIGINT NOT NULL DEFAULT 1, last_dispatched_at DATETIME(3) DEFAULT NULL,
 UNIQUE KEY uk_workspace(workspace_id), KEY idx_expiry(status,valid_until,user_id), KEY idx_dispatch(last_dispatched_at,user_id),
 create_by VARCHAR(64) NOT NULL DEFAULT '', create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 update_by VARCHAR(64) NOT NULL DEFAULT '', update_time DATETIME(3) DEFAULT NULL, remark VARCHAR(500) NOT NULL DEFAULT ''
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE pod_user_model_grant (
 user_id BIGINT NOT NULL, model_id BIGINT NOT NULL, enabled TINYINT(1) NOT NULL DEFAULT 1, revision BIGINT NOT NULL DEFAULT 1,
 PRIMARY KEY(user_id,model_id), KEY idx_model(model_id,enabled,user_id),
 create_by VARCHAR(64) NOT NULL DEFAULT '', create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 update_by VARCHAR(64) NOT NULL DEFAULT '', update_time DATETIME(3) DEFAULT NULL, remark VARCHAR(500) NOT NULL DEFAULT ''
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE pod_points_account (
 user_id BIGINT NOT NULL PRIMARY KEY, available BIGINT NOT NULL DEFAULT 0, held BIGINT NOT NULL DEFAULT 0, spent BIGINT NOT NULL DEFAULT 0,
 granted_total BIGINT NOT NULL DEFAULT 0, deducted_total BIGINT NOT NULL DEFAULT 0, revision BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL DEFAULT '', create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 update_by VARCHAR(64) NOT NULL DEFAULT '', update_time DATETIME(3) DEFAULT NULL, remark VARCHAR(500) NOT NULL DEFAULT ''
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE pod_points_event (
 event_id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, user_id BIGINT NOT NULL, workspace_id BIGINT NOT NULL, run_id BIGINT DEFAULT NULL, batch_id BIGINT DEFAULT NULL,
 submission_key VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '', event_key VARCHAR(100) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 kind VARCHAR(16) NOT NULL, amount BIGINT NOT NULL, available_after BIGINT NOT NULL, held_after BIGINT NOT NULL, spent_after BIGINT NOT NULL,
 reason VARCHAR(200) NOT NULL, actor_user_id BIGINT NOT NULL,
 UNIQUE KEY uk_event(user_id,event_key), KEY idx_user(user_id,event_id), KEY idx_run(run_id,event_id), KEY idx_batch(batch_id,event_id), KEY idx_submission(submission_key,event_id),
 create_by VARCHAR(64) NOT NULL DEFAULT '', create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 update_by VARCHAR(64) NOT NULL DEFAULT '', update_time DATETIME(3) DEFAULT NULL, remark VARCHAR(500) NOT NULL DEFAULT ''
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE pod_user_daily_usage (
 user_id BIGINT NOT NULL, business_date DATE NOT NULL, accepted_submissions INT NOT NULL DEFAULT 0, PRIMARY KEY(user_id,business_date),
 create_by VARCHAR(64) NOT NULL DEFAULT '', create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 update_by VARCHAR(64) NOT NULL DEFAULT '', update_time DATETIME(3) DEFAULT NULL, remark VARCHAR(500) NOT NULL DEFAULT ''
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE pod_storage_account (
 user_id BIGINT NOT NULL PRIMARY KEY, used_bytes BIGINT NOT NULL DEFAULT 0, held_bytes BIGINT NOT NULL DEFAULT 0, revision BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL DEFAULT '', create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 update_by VARCHAR(64) NOT NULL DEFAULT '', update_time DATETIME(3) DEFAULT NULL, remark VARCHAR(500) NOT NULL DEFAULT ''
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE pod_storage_reservation (
 reservation_id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, user_id BIGINT NOT NULL, operation_key VARCHAR(120) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 reserved_bytes BIGINT NOT NULL, committed_bytes BIGINT NOT NULL DEFAULT 0, state VARCHAR(16) NOT NULL, lease_until DATETIME(3) DEFAULT NULL,
 result_asset_ids_json JSON NOT NULL, UNIQUE KEY uk_operation(user_id,operation_key), KEY idx_state(state,lease_until,reservation_id), KEY idx_user(user_id,reservation_id),
 create_by VARCHAR(64) NOT NULL DEFAULT '', create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 update_by VARCHAR(64) NOT NULL DEFAULT '', update_time DATETIME(3) DEFAULT NULL, remark VARCHAR(500) NOT NULL DEFAULT ''
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Preserve existing workspace revocations; do not grant models or convert simulation credits.
INSERT INTO pod_creator_policy(user_id,workspace_id,status,max_concurrent_nodes,max_pending_runs,max_daily_submissions,max_storage_bytes,create_by)
 SELECT w.owner_user_id,w.workspace_id,CASE WHEN w.status='0' AND u.status='0' AND u.del_flag='0' AND EXISTS(SELECT 1 FROM pod_workspace_member m WHERE m.workspace_id=w.workspace_id AND m.user_id=w.owner_user_id AND m.status='0') THEN 'active' ELSE 'suspended' END,1,10,1000,1073741824,'podsetup'
 FROM pod_workspace w JOIN sys_user u ON u.user_id=w.owner_user_id;
INSERT INTO pod_points_account(user_id,create_by) SELECT user_id,'podsetup' FROM pod_creator_policy;
INSERT INTO pod_storage_account(user_id,used_bytes,create_by)
 SELECT p.user_id,COALESCE(SUM(f.byte_size),0),'podsetup' FROM pod_creator_policy p LEFT JOIN pod_asset a ON a.workspace_id=p.workspace_id LEFT JOIN pod_asset_file f ON f.asset_id=a.asset_id GROUP BY p.user_id;
