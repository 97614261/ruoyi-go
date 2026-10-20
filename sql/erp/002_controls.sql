-- ERP local development evolution, explicitly applied by erpinit only.
-- Never apply to main/Java or production. Back up before DDL (MySQL DDL commits).
ALTER TABLE erp_document ADD COLUMN version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE erp_document ADD COLUMN source_document_id BIGINT NULL;
ALTER TABLE erp_document ADD COLUMN reversal TINYINT(1) NOT NULL DEFAULT 0;
ALTER TABLE erp_document ADD INDEX ix_erp_document_source (tenant_id,source_document_id,status,document_id);
ALTER TABLE erp_document ADD CONSTRAINT fk_erp_document_source FOREIGN KEY (tenant_id,source_document_id) REFERENCES erp_document(tenant_id,document_id);
ALTER TABLE erp_document_line ADD COLUMN expected_on_hand BIGINT NOT NULL DEFAULT 0;
ALTER TABLE erp_document_line ADD COLUMN expected_reserved BIGINT NOT NULL DEFAULT 0;
ALTER TABLE erp_document_line ADD COLUMN expected_version BIGINT NOT NULL DEFAULT 0;
ALTER TABLE erp_stock ADD COLUMN version BIGINT NOT NULL DEFAULT 0;
CREATE TABLE IF NOT EXISTS erp_warehouse_access (
 access_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 user_id BIGINT NOT NULL,
 warehouse_id BIGINT NOT NULL,
 create_by VARCHAR(64) DEFAULT '', create_time DATETIME,
 update_by VARCHAR(64) DEFAULT '', update_time DATETIME, remark VARCHAR(500),
 PRIMARY KEY(access_id), UNIQUE KEY uk_erp_access_user(tenant_id,user_id,warehouse_id),
 KEY ix_erp_access_warehouse(tenant_id,warehouse_id),
 CONSTRAINT fk_erp_access_user FOREIGN KEY(tenant_id,user_id) REFERENCES sys_user(tenant_id,user_id),
 CONSTRAINT fk_erp_access_warehouse FOREIGN KEY(tenant_id,warehouse_id) REFERENCES erp_warehouse(tenant_id,warehouse_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='员工仓库数据权限';
CREATE TABLE IF NOT EXISTS erp_warehouse_policy (
 policy_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL, user_id BIGINT NOT NULL, version BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) DEFAULT '', create_time DATETIME,
 update_by VARCHAR(64) DEFAULT '', update_time DATETIME, remark VARCHAR(500),
 PRIMARY KEY(policy_id), UNIQUE KEY uk_erp_policy_user(tenant_id,user_id),
 CONSTRAINT fk_erp_policy_user FOREIGN KEY(tenant_id,user_id) REFERENCES sys_user(tenant_id,user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='仓库授权持久版本';
