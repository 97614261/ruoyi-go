-- Only the dedicated local ruoyi_go_mt development database. Never the original DB.
CREATE TABLE IF NOT EXISTS sys_tenant_policy (
 tenant_id BIGINT NOT NULL PRIMARY KEY,
 max_users BIGINT NOT NULL DEFAULT 100,
 max_storage_bytes BIGINT NOT NULL DEFAULT 1073741824,
 max_upload_bytes BIGINT NOT NULL DEFAULT 10485760,
 create_by VARCHAR(64) NOT NULL DEFAULT '', create_time DATETIME NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '', update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 CONSTRAINT fk_policy_tenant FOREIGN KEY(tenant_id) REFERENCES sys_tenant(tenant_id)
) ENGINE=InnoDB COMMENT='Tenant limits; zero means unlimited, upload still has server cap';
