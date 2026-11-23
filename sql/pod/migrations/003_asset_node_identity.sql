-- Graph node IDs are ASCII and case-sensitive. Preserve that identity in the reference primary key.
ALTER TABLE pod_project_asset_ref MODIFY node_id varchar(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL;
