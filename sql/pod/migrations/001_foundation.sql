-- POD-only migration. The installer requires a newly created pod_studio_* database.
-- MySQL DDL is not transactional: an incomplete installation stays incomplete.
CREATE TABLE pod_schema_version (
  version varchar(20) NOT NULL PRIMARY KEY,
  checksum char(64) NOT NULL,
  instance_id varchar(64) NOT NULL,
  state varchar(20) NOT NULL,
  create_by varchar(64) NOT NULL DEFAULT '', create_time datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  update_by varchar(64) NOT NULL DEFAULT '', update_time datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  remark varchar(500) NOT NULL DEFAULT ''
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE pod_workspace (
  workspace_id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,
  owner_user_id bigint NOT NULL,
  name varchar(100) NOT NULL,
  status char(1) NOT NULL DEFAULT '0',
  create_by varchar(64) NOT NULL DEFAULT '', create_time datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  update_by varchar(64) NOT NULL DEFAULT '', update_time datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  remark varchar(500) NOT NULL DEFAULT '',
  UNIQUE KEY uk_pod_workspace_owner (owner_user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE pod_workspace_member (
  workspace_id bigint NOT NULL,
  user_id bigint NOT NULL,
  member_role varchar(20) NOT NULL DEFAULT 'owner',
  status char(1) NOT NULL DEFAULT '0',
  create_by varchar(64) NOT NULL DEFAULT '', create_time datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  update_by varchar(64) NOT NULL DEFAULT '', update_time datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  remark varchar(500) NOT NULL DEFAULT '',
  PRIMARY KEY (workspace_id, user_id),
  KEY idx_pod_member_user (user_id, status, workspace_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE pod_project (
  project_id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,
  workspace_id bigint NOT NULL,
  name varchar(100) NOT NULL,
  graph_json json NOT NULL,
  schema_version int NOT NULL DEFAULT 1,
  revision bigint NOT NULL DEFAULT 1,
  del_flag char(1) NOT NULL DEFAULT '0',
  create_by varchar(64) NOT NULL DEFAULT '', create_time datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  update_by varchar(64) NOT NULL DEFAULT '', update_time datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  remark varchar(500) NOT NULL DEFAULT '',
  KEY idx_pod_project_list (workspace_id, del_flag, update_time, project_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
