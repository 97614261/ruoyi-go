package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"ruoyi-go/internal/model"
	podsql "ruoyi-go/sql/pod"
)

// PODPersonalWorkspace checks both ownership and active membership. Administrator
// privileges do not bypass this resource boundary.
func PODPersonalWorkspace(ctx context.Context, userID int64) (*model.PODWorkspace, error) {
	var workspace model.PODWorkspace
	err := DB(ctx).Table("pod_workspace AS w").
		Select("w.workspace_id, w.name").
		Joins("JOIN pod_workspace_member AS m ON m.workspace_id = w.workspace_id AND m.user_id = ? AND m.status = '0'", userID).
		Where("w.owner_user_id = ? AND w.status = '0'", userID).Take(&workspace).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &workspace, nil
}

func VerifyPODSchema(ctx context.Context, instanceID string) error {
	if err := VerifyPODSchema014(ctx, instanceID); err != nil {
		return err
	}
	return verifyPODVersion(ctx, instanceID, podsql.ProductWorkflowsVersion, podsql.ProductWorkflowsChecksum())
}
func VerifyPODSchema014(ctx context.Context, instanceID string) error {
	if err := VerifyPODSchema013(ctx, instanceID); err != nil {
		return err
	}
	return verifyPODVersion(ctx, instanceID, podsql.ProductFinalsVersion, podsql.ProductFinalsChecksum())
}
func VerifyPODSchema013(ctx context.Context, instanceID string) error {
	if err := VerifyPODSchema012(ctx, instanceID); err != nil {
		return err
	}
	return verifyPODVersion(ctx, instanceID, podsql.ProductStudioVersion, podsql.ProductStudioChecksum())
}
func VerifyPODSchema012(ctx context.Context, instanceID string) error {
	if err := VerifyPODSchema011(ctx, instanceID); err != nil {
		return err
	}
	return verifyPODVersion(ctx, instanceID, podsql.OfficialTemplatesVersion, podsql.OfficialTemplatesChecksum())
}
func VerifyPODSchema011(ctx context.Context, instanceID string) error {
	if err := VerifyPODSchema010(ctx, instanceID); err != nil {
		return err
	}
	return verifyPODVersion(ctx, instanceID, podsql.ManagedExecutionVersion, podsql.ManagedExecutionChecksum())
}
func VerifyPODSchema010(ctx context.Context, instanceID string) error {
	if err := VerifyPODSchema009(ctx, instanceID); err != nil {
		return err
	}
	return verifyPODVersion(ctx, instanceID, podsql.CreatorPointsVersion, podsql.CreatorPointsChecksum())
}
func VerifyPODSchema009(ctx context.Context, instanceID string) error {
	if err := VerifyPODSchema008(ctx, instanceID); err != nil {
		return err
	}
	return verifyPODVersion(ctx, instanceID, podsql.ModelCatalogVersion, podsql.ModelCatalogChecksum())
}

func VerifyPODSchema008(ctx context.Context, instanceID string) error {
	if err := VerifyPODSchema007(ctx, instanceID); err != nil {
		return err
	}
	if err := verifyPODVersion(ctx, instanceID, podsql.AdminFoundationVersion, podsql.AdminFoundationChecksum()); err != nil {
		return err
	}
	var state model.PODPlatformState
	if err := DB(ctx).Table("pod_platform_state").Where("state_id = 1").Take(&state).Error; err != nil {
		return fmt.Errorf("POD 管理状态缺失")
	}
	return nil
}

// Historical bundles remain verifiable without silently migrating their data.
func VerifyPODSchema007(ctx context.Context, instanceID string) error {
	if err := VerifyPODFoundation(ctx, instanceID); err != nil {
		return err
	}
	if err := verifyPODVersion(ctx, instanceID, podsql.AssetsVersion, podsql.AssetsChecksum()); err != nil {
		return err
	}
	if err := verifyPODVersion(ctx, instanceID, podsql.AssetNodeIdentityVersion, podsql.AssetNodeIdentityChecksum()); err != nil {
		return err
	}
	if err := verifyPODVersion(ctx, instanceID, podsql.ExecutionVersion, podsql.ExecutionChecksum()); err != nil {
		return err
	}
	if err := verifyPODVersion(ctx, instanceID, podsql.QwenVersion, podsql.QwenChecksum()); err != nil {
		return err
	}
	if err := verifyPODVersion(ctx, instanceID, podsql.TemplatesVersion, podsql.TemplatesChecksum()); err != nil {
		return err
	}
	return verifyPODVersion(ctx, instanceID, podsql.BatchesVersion, podsql.BatchesChecksum())
}

func VerifyPODFoundation(ctx context.Context, instanceID string) error {
	return verifyPODVersion(ctx, instanceID, podsql.Version, podsql.Checksum())
}

func verifyPODVersion(ctx context.Context, instanceID, version, checksum string) error {
	var row struct {
		Checksum   string
		InstanceID string
		State      string
	}
	err := DB(ctx).Table("pod_schema_version").Where("version = ?", version).Take(&row).Error
	if err != nil {
		return fmt.Errorf("POD 数据库缺少版本 %s，请运行 podsetup 检查或 migrate", version)
	}
	if row.Checksum != checksum || row.InstanceID != instanceID || row.State != "ready" {
		return fmt.Errorf("POD 数据库版本、校验和或实例归属不匹配，禁止启动")
	}
	return nil
}
