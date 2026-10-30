-- Explicit portal v5 -> v6. Durable content schedules; no sys schema or role grants.
CREATE TABLE cms_schedule (
 schedule_id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
 content_id BIGINT NOT NULL,
 kind VARCHAR(16) NOT NULL,
 action VARCHAR(16) NOT NULL,
 content_version BIGINT NOT NULL,
 owner_user_id BIGINT NOT NULL,
 due_at DATETIME NOT NULL,
 status VARCHAR(16) NOT NULL DEFAULT 'PENDING',
 result_message VARCHAR(500) NOT NULL DEFAULT '',
 finished_at DATETIME NULL,
 version BIGINT NOT NULL DEFAULT 1,
 create_by VARCHAR(64) NOT NULL DEFAULT '',create_time DATETIME NOT NULL,
 update_by VARCHAR(64) NOT NULL DEFAULT '',update_time DATETIME NULL,
 remark VARCHAR(500) NOT NULL DEFAULT '',
 pending_content_id BIGINT GENERATED ALWAYS AS (CASE WHEN status='PENDING' THEN content_id ELSE NULL END) STORED,
 UNIQUE KEY uk_schedule_pending(pending_content_id),
 KEY idx_schedule_due(status,due_at,schedule_id),
 KEY idx_schedule_content(kind,content_id,schedule_id),
 CONSTRAINT fk_schedule_content FOREIGN KEY(content_id) REFERENCES cms_content(content_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
