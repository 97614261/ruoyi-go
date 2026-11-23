-- POD-only provider receipts and private generated assets. Do not apply to inherited business databases.
ALTER TABLE pod_run ADD COLUMN execution_mode varchar(16) NOT NULL DEFAULT 'simulation' AFTER mode;
ALTER TABLE pod_node_run ADD COLUMN provider_model varchar(80) NOT NULL DEFAULT '' AFTER provider;

CREATE TABLE pod_provider_call (
  submission_key varchar(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  run_id bigint NOT NULL,
  node_id varchar(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  workspace_id bigint NOT NULL,
  model varchar(80) NOT NULL,
  state varchar(24) NOT NULL,
  reserved_micros bigint NOT NULL DEFAULT 0,
  charged_micros bigint NOT NULL DEFAULT 0,
  receipt_json json NOT NULL,
  create_by varchar(64) NOT NULL DEFAULT '',
  create_time datetime(3) NOT NULL,
  update_by varchar(64) NOT NULL DEFAULT '',
  update_time datetime(3) NOT NULL,
  remark varchar(500) NOT NULL DEFAULT '',
  PRIMARY KEY (submission_key),
  KEY idx_provider_call_run (run_id,node_id),
  KEY idx_provider_call_state (state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE pod_provider_asset (
  submission_key varchar(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  image_index int NOT NULL,
  asset_id bigint NOT NULL,
  workspace_id bigint NOT NULL,
  create_by varchar(64) NOT NULL DEFAULT '',
  create_time datetime(3) NOT NULL,
  update_by varchar(64) NOT NULL DEFAULT '',
  update_time datetime(3) NOT NULL,
  remark varchar(500) NOT NULL DEFAULT '',
  PRIMARY KEY (submission_key,image_index),
  UNIQUE KEY uk_provider_asset (asset_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
