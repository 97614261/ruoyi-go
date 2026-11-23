-- POD official templates. All assets are platform-owned, never personal aliases.
CREATE TABLE pod_catalog_template (
 template_id BIGINT NOT NULL AUTO_INCREMENT, code VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 name VARCHAR(100) NOT NULL, description VARCHAR(500) NOT NULL DEFAULT '', status VARCHAR(16) NOT NULL DEFAULT 'draft',
 current_version_id BIGINT NULL, published_version_id BIGINT NULL, revision BIGINT NOT NULL DEFAULT 1, sort INT NOT NULL DEFAULT 0,
 create_by VARCHAR(64) NOT NULL, create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 update_by VARCHAR(64) NOT NULL DEFAULT '', update_time DATETIME(3) NULL, remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(template_id), UNIQUE KEY uk_official_code(code), KEY idx_official_status(status,sort,template_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
CREATE TABLE pod_catalog_template_version (
 version_id BIGINT NOT NULL AUTO_INCREMENT, template_id BIGINT NOT NULL, version BIGINT NOT NULL,
 graph_json JSON NOT NULL, spec_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, published_at DATETIME(3) NULL,
 create_by VARCHAR(64) NOT NULL, create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 update_by VARCHAR(64) NOT NULL DEFAULT '', update_time DATETIME(3) NULL, remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(version_id), UNIQUE KEY uk_official_version(template_id,version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
CREATE TABLE pod_catalog_asset (
 asset_id BIGINT NOT NULL AUTO_INCREMENT, name VARCHAR(200) NOT NULL, media_type VARCHAR(64) NOT NULL,
 byte_size BIGINT NOT NULL, width INT NOT NULL, height INT NOT NULL, license_note VARCHAR(500) NOT NULL,
 status VARCHAR(16) NOT NULL DEFAULT 'active', revision BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL, create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 update_by VARCHAR(64) NOT NULL DEFAULT '', update_time DATETIME(3) NULL, remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(asset_id), KEY idx_official_asset(status,create_time,asset_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
CREATE TABLE pod_catalog_asset_file (
 asset_id BIGINT NOT NULL, variant VARCHAR(16) NOT NULL, storage_key VARCHAR(100) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 byte_size BIGINT NOT NULL, sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, media_type VARCHAR(64) NOT NULL, width INT NOT NULL, height INT NOT NULL,
 create_by VARCHAR(64) NOT NULL, create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 update_by VARCHAR(64) NOT NULL DEFAULT '', update_time DATETIME(3) NULL, remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(asset_id,variant), UNIQUE KEY uk_official_storage(storage_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
CREATE TABLE pod_catalog_template_asset_ref (
 version_id BIGINT NOT NULL, node_id VARCHAR(64) NOT NULL, asset_id BIGINT NOT NULL,
 create_by VARCHAR(64) NOT NULL, create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 update_by VARCHAR(64) NOT NULL DEFAULT '', update_time DATETIME(3) NULL, remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(version_id,node_id,asset_id), KEY idx_official_reference(asset_id,version_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
CREATE TABLE pod_catalog_use_action (
 action_id BIGINT NOT NULL AUTO_INCREMENT, user_id BIGINT NOT NULL, workspace_id BIGINT NOT NULL,
 request_id VARCHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, request_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 version_id BIGINT NOT NULL, state VARCHAR(16) NOT NULL, copy_map_json JSON NOT NULL, graph_json JSON NOT NULL, name VARCHAR(100) NOT NULL,
 project_id BIGINT NOT NULL DEFAULT 0, storage_operation_key VARCHAR(120) NOT NULL,
 create_by VARCHAR(64) NOT NULL, create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 update_by VARCHAR(64) NOT NULL DEFAULT '', update_time DATETIME(3) NULL, remark VARCHAR(500) NOT NULL DEFAULT '',
 PRIMARY KEY(action_id), UNIQUE KEY uk_official_use(user_id,request_id), KEY idx_official_use_state(state,action_id), KEY idx_official_use_version(version_id,action_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
