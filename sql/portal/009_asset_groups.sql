-- Explicit portal-only v9 -> v10. Labels never change storage_key or historical references.
ALTER TABLE cms_asset
 ADD COLUMN group_name VARCHAR(40) NOT NULL DEFAULT '' AFTER name,
 ADD KEY idx_cms_asset_group (is_deleted,group_name,asset_id);
