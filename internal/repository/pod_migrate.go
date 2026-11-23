package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"ruoyi-go/internal/config"
	podsql "ruoyi-go/sql/pod"
	"time"
)

// MigratePODAssets only applies embedded asset migrations to an owned local 001 instance.
// Partial DDL is intentionally not retried or marked ready.
func MigratePODAssets(ctx context.Context, cfg *config.Config) error {
	return migratePODThrough(ctx, cfg, podsql.ProductWorkflowsVersion)
}
func MigratePODOfficialTemplates(ctx context.Context, cfg *config.Config) error {
	return migratePODThrough(ctx, cfg, podsql.OfficialTemplatesVersion)
}
func MigratePODManaged(ctx context.Context, cfg *config.Config) error {
	return migratePODThrough(ctx, cfg, "011")
}
func MigratePODCreators(ctx context.Context, cfg *config.Config) error {
	return migratePODThrough(ctx, cfg, "010")
}
func MigratePODModelCatalog(ctx context.Context, cfg *config.Config) error {
	return migratePODThrough(ctx, cfg, "009")
}
func MigratePODAdminFoundation(ctx context.Context, cfg *config.Config) error {
	return migratePODThrough(ctx, cfg, "008")
}
func migratePODThrough(ctx context.Context, cfg *config.Config, target string) error {
	dsn, err := localPODConnection(cfg)
	if err != nil {
		return err
	}
	dsn.MultiStatements = true
	db, err := sql.Open("mysql", dsn.FormatDSN())
	if err != nil {
		return err
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var locked int
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?,5)", "pod-migrate-"+cfg.POD.DatabaseName).Scan(&locked); err != nil || locked != 1 {
		return errors.New("迁移锁不可用，请稍后重试")
	}
	defer func() {
		release, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(release, "SELECT RELEASE_LOCK(?)", "pod-migrate-"+cfg.POD.DatabaseName)
	}()
	var checksum, instance, state string
	if err := conn.QueryRowContext(ctx, "SELECT checksum,instance_id,state FROM pod_schema_version WHERE version='001'").Scan(&checksum, &instance, &state); err != nil || checksum != podsql.Checksum() || instance != cfg.POD.InstanceID || state != "ready" {
		return errors.New("001 版本或实例不匹配，拒绝迁移")
	}
	for _, migration := range []struct{ version, checksum, body string }{
		{podsql.AssetsVersion, podsql.AssetsChecksum(), podsql.Assets},
		{podsql.AssetNodeIdentityVersion, podsql.AssetNodeIdentityChecksum(), podsql.AssetNodeIdentity},
		{podsql.ExecutionVersion, podsql.ExecutionChecksum(), podsql.Execution},
		{podsql.QwenVersion, podsql.QwenChecksum(), podsql.Qwen},
		{podsql.TemplatesVersion, podsql.TemplatesChecksum(), podsql.Templates},
		{podsql.BatchesVersion, podsql.BatchesChecksum(), podsql.Batches},
		{podsql.AdminFoundationVersion, podsql.AdminFoundationChecksum(), podsql.AdminFoundation},
		{podsql.ModelCatalogVersion, podsql.ModelCatalogChecksum(), podsql.ModelCatalog},
		{podsql.CreatorPointsVersion, podsql.CreatorPointsChecksum(), podsql.CreatorPoints},
		{podsql.ManagedExecutionVersion, podsql.ManagedExecutionChecksum(), podsql.ManagedExecution},
		{podsql.OfficialTemplatesVersion, podsql.OfficialTemplatesChecksum(), podsql.OfficialTemplates},
		{podsql.ProductStudioVersion, podsql.ProductStudioChecksum(), podsql.ProductStudio},
		{podsql.ProductFinalsVersion, podsql.ProductFinalsChecksum(), podsql.ProductFinals},
		{podsql.ProductWorkflowsVersion, podsql.ProductWorkflowsChecksum(), podsql.ProductWorkflows},
	} {
		if migration.version > target {
			break
		}
		err = conn.QueryRowContext(ctx, "SELECT checksum,instance_id,state FROM pod_schema_version WHERE version=?", migration.version).Scan(&checksum, &instance, &state)
		if err == nil {
			if checksum == migration.checksum && instance == cfg.POD.InstanceID && state == "ready" {
				continue
			}
			return fmt.Errorf("%s 已存在但不完整或不匹配；请排查部分 DDL，不自动重试", migration.version)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if migration.version == podsql.ProductWorkflowsVersion {
			var count int
			if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=? AND table_name='pod_product_job' AND column_name IN ('work_kind','output_mask')", cfg.POD.DatabaseName).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				return errors.New("发现未登记的商品工作流字段，拒绝覆盖")
			}
		}
		if migration.version == podsql.ProductFinalsVersion {
			var count int
			if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=? AND table_name='pod_product_final'", cfg.POD.DatabaseName).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				return errors.New("发现未登记商品定稿表，拒绝覆盖")
			}
		}
		if migration.version == podsql.ProductStudioVersion {
			var count int
			if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=? AND table_name IN ('pod_product_pack','pod_product_job')", cfg.POD.DatabaseName).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				return errors.New("发现未登记商品素材包表，拒绝覆盖")
			}
		}
		if migration.version == podsql.AssetsVersion {
			var count int
			if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=? AND table_name IN ('pod_asset','pod_asset_file','pod_project_asset_ref')", cfg.POD.DatabaseName).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				return errors.New("发现未登记素材表，拒绝覆盖")
			}
		}
		if migration.version == podsql.OfficialTemplatesVersion {
			var count int
			if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=? AND table_name IN ('pod_catalog_template','pod_catalog_template_version','pod_catalog_asset','pod_catalog_asset_file','pod_catalog_template_asset_ref','pod_catalog_use_action')", cfg.POD.DatabaseName).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				return errors.New("发现未登记官方模板表，拒绝覆盖")
			}
		}
		if migration.version == podsql.ManagedExecutionVersion {
			var count int
			if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=? AND table_name IN ('pod_execution_quote','pod_execution_snapshot','pod_run_model_binding','pod_call_billing')", cfg.POD.DatabaseName).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				return errors.New("发现未登记 managed 表，拒绝覆盖")
			}
		}
		if migration.version == podsql.CreatorPointsVersion {
			var count int
			if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=? AND table_name IN ('pod_creator_policy','pod_user_model_grant','pod_points_account','pod_points_event','pod_user_daily_usage','pod_storage_account','pod_storage_reservation')", cfg.POD.DatabaseName).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				return errors.New("发现未登记用户权益表，拒绝覆盖")
			}
		}
		if migration.version == podsql.ModelCatalogVersion {
			var count int
			if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=? AND table_name IN ('pod_provider','pod_provider_credential_version','pod_model','pod_model_version')", cfg.POD.DatabaseName).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				return errors.New("发现未登记模型目录表，拒绝覆盖")
			}
		}
		if migration.version == podsql.AdminFoundationVersion {
			var count int
			if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=? AND table_name IN ('pod_platform_state','pod_admin_action','pod_admin_audit','pod_private_access_grant')", cfg.POD.DatabaseName).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				return errors.New("发现未登记管理表，拒绝覆盖")
			}
		}
		if migration.version == podsql.ExecutionVersion {
			var count int
			if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=? AND table_name IN ('pod_run','pod_node_run','pod_run_asset_ref','pod_outbox','pod_credit_account','pod_credit_event','pod_retry_request','pod_execution_gate')", cfg.POD.DatabaseName).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				return errors.New("发现未登记执行表，拒绝覆盖")
			}
		}
		if migration.version == podsql.QwenVersion {
			var count int
			if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=? AND table_name IN ('pod_provider_call','pod_provider_asset')", cfg.POD.DatabaseName).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				return errors.New("发现未登记供应商表，拒绝覆盖")
			}
		}
		if migration.version == podsql.BatchesVersion {
			var count int
			if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=? AND table_name IN ('pod_batch','pod_batch_item','pod_batch_asset_ref')", cfg.POD.DatabaseName).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				return errors.New("发现未登记批量表，拒绝覆盖")
			}
		}
		if migration.version == podsql.TemplatesVersion {
			var count int
			if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=? AND table_name IN ('pod_template','pod_template_version','pod_template_asset_ref','pod_template_action')", cfg.POD.DatabaseName).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				return errors.New("发现未登记模板表，拒绝覆盖")
			}
		}
		if _, err := conn.ExecContext(ctx, "INSERT INTO pod_schema_version(version,checksum,instance_id,state,create_by) VALUES(?,?,?,'installing','podsetup')", migration.version, migration.checksum, cfg.POD.InstanceID); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, migration.body); err != nil {
			return fmt.Errorf("%s DDL 未完成，保留 installing 状态供排查: %w", migration.version, err)
		}
		if migration.version == podsql.AdminFoundationVersion {
			if err := installPODAdminMenus(ctx, conn); err != nil {
				return err
			}
		}
		if _, err := conn.ExecContext(ctx, "UPDATE pod_schema_version SET state='ready',update_by='podsetup',update_time=NOW() WHERE version=? AND state='installing'", migration.version); err != nil {
			return err
		}
	}
	return nil
}
