-- POD-only bounded batches; never apply to inherited business databases.
CREATE TABLE pod_batch (
 batch_id BIGINT NOT NULL AUTO_INCREMENT,
 workspace_id BIGINT NOT NULL,
 user_id BIGINT NOT NULL,
 request_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 template_id BIGINT NOT NULL,
 template_version BIGINT NOT NULL,
 name VARCHAR(100) NOT NULL,
 execution_mode VARCHAR(20) NOT NULL,
 state VARCHAR(20) NOT NULL DEFAULT 'active',
 impact_json JSON NOT NULL,
 create_by VARCHAR(64) NOT NULL,
 create_time DATETIME(3) NOT NULL,
 update_by VARCHAR(64) NOT NULL,
 update_time DATETIME(3) NOT NULL,
 PRIMARY KEY (batch_id),
 UNIQUE KEY uk_batch_request (workspace_id,request_id),
 KEY idx_batch_owner (workspace_id,create_time,batch_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE pod_batch_item (
 batch_id BIGINT NOT NULL,
 item_no INT NOT NULL,
 workspace_id BIGINT NOT NULL,
 name VARCHAR(100) NOT NULL,
 graph_json JSON NOT NULL,
 state VARCHAR(24) NOT NULL DEFAULT 'waiting',
 project_id BIGINT NOT NULL DEFAULT 0,
 run_id BIGINT NOT NULL DEFAULT 0,
 error_code VARCHAR(64) NOT NULL DEFAULT '',
 checked_at DATETIME(3) NOT NULL,
 create_by VARCHAR(64) NOT NULL,
 create_time DATETIME(3) NOT NULL,
 update_by VARCHAR(64) NOT NULL,
 update_time DATETIME(3) NOT NULL,
 PRIMARY KEY (batch_id,item_no),
 KEY idx_batch_dispatch (state,checked_at,batch_id,item_no),
 KEY idx_batch_waiting_owner (workspace_id,state),
 KEY idx_batch_run (run_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE pod_batch_asset_ref (
 batch_id BIGINT NOT NULL,
 asset_id BIGINT NOT NULL,
 workspace_id BIGINT NOT NULL,
 PRIMARY KEY (batch_id,asset_id),
 KEY idx_batch_asset (asset_id,workspace_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
