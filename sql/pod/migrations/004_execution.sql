-- POD-only execution and simulation credit ledger. Never apply to inherited databases.
CREATE TABLE pod_run (
 run_id bigint NOT NULL AUTO_INCREMENT,
 workspace_id bigint NOT NULL, user_id bigint NOT NULL, project_id bigint NOT NULL,
 project_revision bigint NOT NULL, project_name varchar(100) NOT NULL,
 request_id varchar(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_hash char(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 mode varchar(16) NOT NULL, target_node_id varchar(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
 base_run_id bigint NOT NULL DEFAULT 0, graph_json json NOT NULL, base_outputs_json json NOT NULL,
 state varchar(32) NOT NULL, test_scenario varchar(24) NOT NULL,
 attempt int NOT NULL DEFAULT 1, reserved bigint NOT NULL DEFAULT 0, spent bigint NOT NULL DEFAULT 0,
 settled tinyint NOT NULL DEFAULT 0,
 create_by varchar(64) NOT NULL DEFAULT '', create_time datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 update_by varchar(64) NOT NULL DEFAULT '', update_time datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3), remark varchar(500) NOT NULL DEFAULT '',
 PRIMARY KEY(run_id), UNIQUE KEY uk_pod_run_request(workspace_id,request_id),
 KEY idx_pod_run_history(workspace_id,create_time,run_id),
 KEY idx_pod_run_project(workspace_id,project_id,run_id),
 KEY idx_pod_run_pending(state,workspace_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE pod_node_run (
 run_id bigint NOT NULL, node_id varchar(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 workspace_id bigint NOT NULL, user_id bigint NOT NULL, node_type varchar(32) NOT NULL,
 provider varchar(32) NOT NULL, state varchar(32) NOT NULL, attempt int NOT NULL DEFAULT 0,
 submission_key varchar(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
 lease_token varchar(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '', lease_until datetime(3) NULL,
 input_json json NOT NULL, output_json json NOT NULL,
 error_code varchar(64) NOT NULL DEFAULT '', error_message varchar(200) NOT NULL DEFAULT '',
 create_by varchar(64) NOT NULL DEFAULT '', create_time datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 update_by varchar(64) NOT NULL DEFAULT '', update_time datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3), remark varchar(500) NOT NULL DEFAULT '',
 PRIMARY KEY(run_id,node_id), KEY idx_pod_node_lease(state,lease_until),
 KEY idx_pod_node_user(user_id,state), KEY idx_pod_node_provider(provider,state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE pod_run_asset_ref (
 run_id bigint NOT NULL, asset_id bigint NOT NULL, workspace_id bigint NOT NULL,
 create_by varchar(64) NOT NULL DEFAULT '', create_time datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 update_by varchar(64) NOT NULL DEFAULT '', update_time datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3), remark varchar(500) NOT NULL DEFAULT '',
 PRIMARY KEY(run_id,asset_id), KEY idx_pod_run_asset(asset_id,workspace_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE pod_outbox (
 run_id bigint NOT NULL, active tinyint NOT NULL DEFAULT 1, delivery_seq bigint NOT NULL DEFAULT 0,
 next_delivery_at datetime(3) NOT NULL,
 create_by varchar(64) NOT NULL DEFAULT '', create_time datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 update_by varchar(64) NOT NULL DEFAULT '', update_time datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3), remark varchar(500) NOT NULL DEFAULT '',
 PRIMARY KEY(run_id), KEY idx_pod_outbox_due(active,next_delivery_at,run_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE pod_credit_account (
 workspace_id bigint NOT NULL, available bigint NOT NULL DEFAULT 0, held bigint NOT NULL DEFAULT 0, spent bigint NOT NULL DEFAULT 0,
 create_by varchar(64) NOT NULL DEFAULT '', create_time datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 update_by varchar(64) NOT NULL DEFAULT '', update_time datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3), remark varchar(500) NOT NULL DEFAULT '',
 PRIMARY KEY(workspace_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE pod_credit_event (
 event_id bigint NOT NULL AUTO_INCREMENT, workspace_id bigint NOT NULL, run_id bigint NOT NULL DEFAULT 0,
 event_key varchar(100) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, kind varchar(16) NOT NULL, amount bigint NOT NULL,
 available bigint NOT NULL, held bigint NOT NULL, spent bigint NOT NULL,
 create_by varchar(64) NOT NULL DEFAULT '', create_time datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 update_by varchar(64) NOT NULL DEFAULT '', update_time datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3), remark varchar(500) NOT NULL DEFAULT '',
 PRIMARY KEY(event_id), UNIQUE KEY uk_pod_credit_event(workspace_id,event_key),
 KEY idx_pod_credit_history(workspace_id,event_id), KEY idx_pod_credit_run(run_id,event_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE pod_retry_request (
 workspace_id bigint NOT NULL, request_id varchar(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 run_id bigint NOT NULL, attempt int NOT NULL,
 create_by varchar(64) NOT NULL DEFAULT '', create_time datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 update_by varchar(64) NOT NULL DEFAULT '', update_time datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3), remark varchar(500) NOT NULL DEFAULT '',
 PRIMARY KEY(workspace_id,request_id), KEY idx_pod_retry_run(run_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE pod_execution_gate (
 gate_id int NOT NULL,
 create_by varchar(64) NOT NULL DEFAULT '', create_time datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 update_by varchar(64) NOT NULL DEFAULT '', update_time datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3), remark varchar(500) NOT NULL DEFAULT '',
 PRIMARY KEY(gate_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
INSERT INTO pod_execution_gate(gate_id,create_by) VALUES(1,'podsetup');
