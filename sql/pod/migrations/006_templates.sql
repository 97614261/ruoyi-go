-- Personal, versioned templates. Never rewrite an installed earlier migration.
CREATE TABLE pod_template (
  template_id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,
  workspace_id bigint NOT NULL,
  name varchar(100) NOT NULL,
  description varchar(500) NOT NULL DEFAULT '',
  current_version bigint NOT NULL DEFAULT 1,
  revision bigint NOT NULL DEFAULT 1,
  del_flag char(1) NOT NULL DEFAULT '0',
  create_by varchar(64) NOT NULL DEFAULT '', create_time datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  update_by varchar(64) NOT NULL DEFAULT '', update_time datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  remark varchar(500) NOT NULL DEFAULT '',
  KEY idx_pod_template_list (workspace_id,del_flag,update_time,template_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE pod_template_version (
  template_id bigint NOT NULL,
  version bigint NOT NULL,
  workspace_id bigint NOT NULL,
  graph_json json NOT NULL,
  create_by varchar(64) NOT NULL DEFAULT '', create_time datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  update_by varchar(64) NOT NULL DEFAULT '', update_time datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  remark varchar(500) NOT NULL DEFAULT '',
  PRIMARY KEY (template_id,version),
  KEY idx_pod_template_version_space (workspace_id,template_id,version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE pod_template_asset_ref (
  template_id bigint NOT NULL,
  version bigint NOT NULL,
  node_id varchar(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  asset_id bigint NOT NULL,
  workspace_id bigint NOT NULL,
  create_by varchar(64) NOT NULL DEFAULT '', create_time datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  update_by varchar(64) NOT NULL DEFAULT '', update_time datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  remark varchar(500) NOT NULL DEFAULT '',
  PRIMARY KEY (template_id,version,node_id,asset_id),
  KEY idx_pod_template_asset (asset_id,workspace_id),
  KEY idx_pod_template_ref_space (workspace_id,template_id,version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE pod_template_action (
  workspace_id bigint NOT NULL,
  request_id char(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  action varchar(20) NOT NULL,
  request_hash char(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  template_id bigint NOT NULL,
  version bigint NOT NULL,
  project_id bigint NOT NULL DEFAULT 0,
  create_by varchar(64) NOT NULL DEFAULT '', create_time datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  update_by varchar(64) NOT NULL DEFAULT '', update_time datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  remark varchar(500) NOT NULL DEFAULT '',
  PRIMARY KEY (workspace_id,request_id),
  KEY idx_pod_template_action_template (template_id,version),
  KEY idx_pod_template_action_project (project_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
