ALTER TABLE pod_product_job
  ADD COLUMN work_kind varchar(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'product',
  ADD COLUMN output_mask tinyint unsigned NOT NULL DEFAULT 3;
