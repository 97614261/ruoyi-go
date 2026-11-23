CREATE TABLE pod_execution_quote (
 quote_id varchar(36) COLLATE utf8mb4_bin NOT NULL,
 user_id bigint NOT NULL, workspace_id bigint NOT NULL,
 kind varchar(12) NOT NULL, source_hash char(64) NOT NULL,
 selection_json longtext NOT NULL, policy_revision bigint NOT NULL,
 catalog_hash char(64) NOT NULL, points_upper_bound bigint NOT NULL,
 cost_upper_micros bigint NOT NULL, expires_at datetime(3) NOT NULL,
 state varchar(16) NOT NULL DEFAULT 'ready',
 consumed_resource_type varchar(12) NOT NULL DEFAULT '', consumed_resource_id bigint NOT NULL DEFAULT 0,
 consumed_request_id varchar(64) COLLATE utf8mb4_bin DEFAULT NULL,
 create_time datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 PRIMARY KEY(quote_id), KEY idx_quote_owner(user_id,state,expires_at,quote_id), UNIQUE KEY uk_quote_action(user_id,kind,consumed_request_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE pod_execution_snapshot (
 scope_kind varchar(12) COLLATE utf8mb4_bin NOT NULL, scope_id bigint NOT NULL,
 workspace_id bigint NOT NULL, user_id bigint NOT NULL,
 quote_id varchar(36) COLLATE utf8mb4_bin NOT NULL,
 billing_scheme varchar(24) NOT NULL DEFAULT 'managedPointsV1',
 policy_revision bigint NOT NULL, selection_json longtext NOT NULL,
 hold_points bigint NOT NULL DEFAULT 0, unallocated_points bigint NOT NULL DEFAULT 0,
 create_time datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 PRIMARY KEY(scope_kind,scope_id), KEY idx_snapshot_owner(user_id,scope_kind,scope_id), KEY idx_snapshot_quote(quote_id),
 CHECK(hold_points>=0 AND unallocated_points>=0 AND unallocated_points<=hold_points)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE pod_run_model_binding (
 run_id bigint NOT NULL, node_id varchar(64) COLLATE utf8mb4_bin NOT NULL,
 workspace_id bigint NOT NULL, user_id bigint NOT NULL,
 model_id bigint NOT NULL, model_version_id bigint NOT NULL, provider_id bigint NOT NULL,
 selection_json longtext NOT NULL,
 PRIMARY KEY(run_id,node_id), KEY idx_binding_provider(provider_id,run_id,node_id),
 KEY idx_binding_model(model_id,run_id,node_id), KEY idx_binding_owner(user_id,run_id,node_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE pod_call_billing (
 submission_key varchar(128) COLLATE utf8mb4_bin NOT NULL,
 run_id bigint NOT NULL, node_id varchar(64) COLLATE utf8mb4_bin NOT NULL,
 user_id bigint NOT NULL, workspace_id bigint NOT NULL,
 model_version_id bigint NOT NULL, credential_version_id bigint NOT NULL,
 held_points bigint NOT NULL, charged_points bigint NOT NULL DEFAULT 0, released_points bigint NOT NULL DEFAULT 0,
 state varchar(16) NOT NULL DEFAULT 'held', usage_json longtext NOT NULL,
 create_time datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
 PRIMARY KEY(submission_key), KEY idx_billing_run(run_id,node_id,submission_key),
 KEY idx_billing_owner(user_id,state,submission_key), KEY idx_billing_model(model_version_id,submission_key),
 CHECK(held_points>=0 AND charged_points>=0 AND released_points>=0 AND charged_points+released_points<=held_points)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
