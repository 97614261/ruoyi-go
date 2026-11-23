CREATE TABLE pod_product_pack (
 pack_id bigint NOT NULL AUTO_INCREMENT,
 workspace_id bigint NOT NULL,
 fingerprint char(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 product_name varchar(80) NOT NULL,
 sku varchar(60) NOT NULL DEFAULT '',
 selling_points varchar(400) NOT NULL DEFAULT '',
 protected_details varchar(400) NOT NULL DEFAULT '',
 reference_asset_id bigint NOT NULL,
 create_time datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 PRIMARY KEY(pack_id),
 UNIQUE KEY uk_pod_product_pack(workspace_id,fingerprint),
 KEY idx_pod_product_pack_list(workspace_id,pack_id),
 KEY idx_pod_product_pack_asset(workspace_id,reference_asset_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE pod_product_job (
 project_id bigint NOT NULL,
 workspace_id bigint NOT NULL,
 pack_id bigint NOT NULL,
 request_id varchar(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_hash char(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 PRIMARY KEY(project_id),
 UNIQUE KEY uk_pod_product_job_request(workspace_id,request_id),
 KEY idx_pod_product_job_list(workspace_id,project_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
