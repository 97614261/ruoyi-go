CREATE TABLE pod_product_final (
  project_id bigint NOT NULL,
  workspace_id bigint NOT NULL,
  node_id varchar(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  run_id bigint NOT NULL,
  asset_id bigint NOT NULL,
  PRIMARY KEY (project_id, node_id),
  KEY idx_pod_product_final_asset (workspace_id, asset_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
