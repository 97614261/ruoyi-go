-- ERP branch only. Apply explicitly to the dedicated LOCAL ruoyi_go_mt database.
-- Never run on the Java/main database. No DROP, no automatic startup migration.
CREATE TABLE IF NOT EXISTS erp_sku (
 sku_id BIGINT NOT NULL AUTO_INCREMENT COMMENT 'SKU编号',
 tenant_id BIGINT NOT NULL,
 sku_code VARCHAR(64) NOT NULL COMMENT 'SKU编码',
 product_name VARCHAR(120) NOT NULL COMMENT '商品名称',
 specification VARCHAR(120) NOT NULL DEFAULT '' COMMENT '规格',
 unit VARCHAR(20) NOT NULL DEFAULT '件' COMMENT '单位',
 status CHAR(1) NOT NULL DEFAULT '0' COMMENT '状态',
 create_by VARCHAR(64) DEFAULT '', create_time DATETIME,
 update_by VARCHAR(64) DEFAULT '', update_time DATETIME, remark VARCHAR(500),
 PRIMARY KEY(sku_id), UNIQUE KEY uk_erp_sku_tenant_id(tenant_id,sku_id),
 UNIQUE KEY uk_erp_sku_code(tenant_id,sku_code), KEY ix_erp_sku_name(tenant_id,product_name),
 CONSTRAINT fk_erp_sku_tenant FOREIGN KEY(tenant_id) REFERENCES sys_tenant(tenant_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='商品SKU';
CREATE TABLE IF NOT EXISTS erp_warehouse (
 warehouse_id BIGINT NOT NULL AUTO_INCREMENT COMMENT '仓库编号',
 tenant_id BIGINT NOT NULL,
 warehouse_code VARCHAR(64) NOT NULL COMMENT '仓库编码',
 warehouse_name VARCHAR(120) NOT NULL COMMENT '仓库名称',
 status CHAR(1) NOT NULL DEFAULT '0' COMMENT '状态',
 create_by VARCHAR(64) DEFAULT '', create_time DATETIME,
 update_by VARCHAR(64) DEFAULT '', update_time DATETIME, remark VARCHAR(500),
 PRIMARY KEY(warehouse_id), UNIQUE KEY uk_erp_warehouse_tenant_id(tenant_id,warehouse_id),
 UNIQUE KEY uk_erp_warehouse_code(tenant_id,warehouse_code), KEY ix_erp_warehouse_name(tenant_id,warehouse_name),
 CONSTRAINT fk_erp_warehouse_tenant FOREIGN KEY(tenant_id) REFERENCES sys_tenant(tenant_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='仓库';
CREATE TABLE IF NOT EXISTS erp_document (
 document_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 document_no VARCHAR(64) NOT NULL,
 kind VARCHAR(8) NOT NULL,
 warehouse_id BIGINT NOT NULL,
 warehouse_name VARCHAR(120) NOT NULL,
 status VARCHAR(16) NOT NULL DEFAULT 'DRAFT',
 request_key VARCHAR(64) NOT NULL,
 request_hash CHAR(64) NOT NULL,
 create_by VARCHAR(64) DEFAULT '', create_time DATETIME,
 update_by VARCHAR(64) DEFAULT '', update_time DATETIME, remark VARCHAR(500),
 PRIMARY KEY(document_id), UNIQUE KEY uk_erp_document_id(tenant_id,document_id),
 UNIQUE KEY uk_erp_document_request(tenant_id,request_key), UNIQUE KEY uk_erp_document_no(tenant_id,document_no),
 KEY ix_erp_document_list(tenant_id,kind,document_id), KEY ix_erp_document_status(tenant_id,kind,status,document_id),
 KEY ix_erp_document_warehouse(tenant_id,warehouse_id),
 CONSTRAINT fk_erp_document_tenant FOREIGN KEY(tenant_id) REFERENCES sys_tenant(tenant_id),
 CONSTRAINT fk_erp_document_warehouse FOREIGN KEY(tenant_id,warehouse_id) REFERENCES erp_warehouse(tenant_id,warehouse_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='入库和销售单据';
CREATE TABLE IF NOT EXISTS erp_document_line (
 line_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 document_id BIGINT NOT NULL,
 sku_id BIGINT NOT NULL,
 sku_code VARCHAR(64) NOT NULL,
 product_name VARCHAR(120) NOT NULL,
 specification VARCHAR(120) NOT NULL,
 unit VARCHAR(20) NOT NULL,
 quantity BIGINT NOT NULL,
 create_by VARCHAR(64) DEFAULT '', create_time DATETIME,
 update_by VARCHAR(64) DEFAULT '', update_time DATETIME, remark VARCHAR(500),
 PRIMARY KEY(line_id), UNIQUE KEY uk_erp_document_sku(tenant_id,document_id,sku_id), KEY ix_erp_line_sku(tenant_id,sku_id),
 CONSTRAINT fk_erp_line_document FOREIGN KEY(tenant_id,document_id) REFERENCES erp_document(tenant_id,document_id),
 CONSTRAINT fk_erp_line_sku FOREIGN KEY(tenant_id,sku_id) REFERENCES erp_sku(tenant_id,sku_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='单据明细快照';
CREATE TABLE IF NOT EXISTS erp_stock (
 stock_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 warehouse_id BIGINT NOT NULL,
 sku_id BIGINT NOT NULL,
 on_hand BIGINT NOT NULL DEFAULT 0,
 reserved BIGINT NOT NULL DEFAULT 0,
 create_by VARCHAR(64) DEFAULT '', create_time DATETIME,
 update_by VARCHAR(64) DEFAULT '', update_time DATETIME, remark VARCHAR(500),
 PRIMARY KEY(stock_id), UNIQUE KEY uk_erp_stock(tenant_id,warehouse_id,sku_id), KEY ix_erp_stock_sku(tenant_id,sku_id),
 CONSTRAINT fk_erp_stock_warehouse FOREIGN KEY(tenant_id,warehouse_id) REFERENCES erp_warehouse(tenant_id,warehouse_id),
 CONSTRAINT fk_erp_stock_sku FOREIGN KEY(tenant_id,sku_id) REFERENCES erp_sku(tenant_id,sku_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='库存余额';
CREATE TABLE IF NOT EXISTS erp_stock_ledger (
 ledger_id BIGINT NOT NULL AUTO_INCREMENT,
 tenant_id BIGINT NOT NULL,
 document_id BIGINT NOT NULL,
 document_no VARCHAR(64) NOT NULL,
 warehouse_id BIGINT NOT NULL,
 sku_id BIGINT NOT NULL,
 event VARCHAR(16) NOT NULL,
 on_hand_delta BIGINT NOT NULL,
 reserved_delta BIGINT NOT NULL,
 on_hand_after BIGINT NOT NULL,
 reserved_after BIGINT NOT NULL,
 create_by VARCHAR(64) DEFAULT '', create_time DATETIME,
 update_by VARCHAR(64) DEFAULT '', update_time DATETIME, remark VARCHAR(500),
 PRIMARY KEY(ledger_id), UNIQUE KEY uk_erp_ledger_event(tenant_id,document_id,sku_id,event),
 KEY ix_erp_ledger_stock(tenant_id,warehouse_id,sku_id,ledger_id), KEY ix_erp_ledger_document(tenant_id,document_id),
 CONSTRAINT fk_erp_ledger_document FOREIGN KEY(tenant_id,document_id) REFERENCES erp_document(tenant_id,document_id),
 CONSTRAINT fk_erp_ledger_warehouse FOREIGN KEY(tenant_id,warehouse_id) REFERENCES erp_warehouse(tenant_id,warehouse_id),
 CONSTRAINT fk_erp_ledger_sku FOREIGN KEY(tenant_id,sku_id) REFERENCES erp_sku(tenant_id,sku_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='不可变库存流水';
