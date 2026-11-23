-- Private POD assets only. Apply with podsetup migrate, never edit installed 001.
CREATE TABLE pod_asset (
  asset_id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,
  workspace_id bigint NOT NULL,
  parent_asset_id bigint NOT NULL DEFAULT 0,
  source varchar(20) NOT NULL DEFAULT 'upload',
  original_name varchar(180) NOT NULL,
  media_type varchar(40) NOT NULL,
  byte_size bigint NOT NULL,
  width int NOT NULL, height int NOT NULL,
  revision bigint NOT NULL DEFAULT 1,
  del_flag char(1) NOT NULL DEFAULT '0',
  create_by varchar(64) NOT NULL DEFAULT '', create_time datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  update_by varchar(64) NOT NULL DEFAULT '', update_time datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  remark varchar(500) NOT NULL DEFAULT '',
  KEY idx_pod_asset_list (workspace_id,del_flag,create_time,asset_id),
  KEY idx_pod_asset_parent (parent_asset_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE pod_asset_file (
  asset_id bigint NOT NULL,
  variant varchar(20) NOT NULL,
  storage_key varchar(80) NOT NULL,
  media_type varchar(40) NOT NULL,
  byte_size bigint NOT NULL,
  sha256 char(64) NOT NULL,
  width int NOT NULL, height int NOT NULL,
  create_by varchar(64) NOT NULL DEFAULT '', create_time datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  update_by varchar(64) NOT NULL DEFAULT '', update_time datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  remark varchar(500) NOT NULL DEFAULT '',
  PRIMARY KEY (asset_id,variant),
  UNIQUE KEY uk_pod_asset_file_key (storage_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE pod_project_asset_ref (
  project_id bigint NOT NULL,
  node_id varchar(64) NOT NULL,
  asset_id bigint NOT NULL,
  workspace_id bigint NOT NULL,
  create_by varchar(64) NOT NULL DEFAULT '', create_time datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  update_by varchar(64) NOT NULL DEFAULT '', update_time datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  remark varchar(500) NOT NULL DEFAULT '',
  PRIMARY KEY (project_id,node_id,asset_id),
  KEY idx_pod_asset_ref (asset_id,workspace_id,project_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
