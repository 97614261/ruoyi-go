package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"ruoyi-go/internal/model"
)

var ErrPODBudget = errors.New("POD provider estimated budget exhausted")

func checkPODClaim(tx *gorm.DB, c *PODClaim) error {
	var count int64
	err := tx.Model(&model.PODNodeRun{}).Where("run_id=? AND node_id=? AND state='running' AND lease_token=? AND lease_until>CURRENT_TIMESTAMP(3)", c.Run.RunID, c.Node.NodeID, c.Node.LeaseToken).Count(&count).Error
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrPODLease
	}
	return nil
}

// The intent and reservation commit before external I/O. An ambiguous commit must not lead to Submit.
func BeginPODProviderCall(ctx context.Context, c *PODClaim, reservation, budgetMicros int64) (*model.PODProviderCall, bool, error) {
	var row model.PODProviderCall
	fresh := false
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPODGate(tx); err != nil {
			return err
		}
		if err := checkPODClaim(tx, c); err != nil {
			return err
		}
		err := tx.Where("submission_key=?", c.Node.SubmissionKey).Take(&row).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if reservation < 1 || budgetMicros < reservation {
			return ErrPODBudget
		}
		var used int64
		if err := tx.Model(&model.PODProviderCall{}).Select("COALESCE(SUM(reserved_micros+charged_micros),0)").Scan(&used).Error; err != nil {
			return err
		}
		if used > budgetMicros-reservation {
			return ErrPODBudget
		}
		if _, err := lockedPODWorkspace(tx, c.Run.UserID); err != nil {
			return err
		}
		now := podNow()
		row = model.PODProviderCall{SubmissionKey: c.Node.SubmissionKey, RunID: c.Run.RunID, NodeID: c.Node.NodeID, WorkspaceID: c.Run.WorkspaceID, Model: c.Node.ProviderModel, State: "submitted", ReservedMicros: reservation, ReceiptJSON: json.RawMessage(`{}`), CreateBy: "pod-worker", UpdateBy: "pod-worker", CreateTime: now, UpdateTime: now}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		fresh = true
		return nil
	})
	return &row, fresh && err == nil, err
}

func SelectPODProviderCall(ctx context.Context, c *PODClaim) (*model.PODProviderCall, error) {
	var row model.PODProviderCall
	err := DB(ctx).Where("submission_key=? AND run_id=? AND workspace_id=?", c.Node.SubmissionKey, c.Run.RunID, c.Run.WorkspaceID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func SavePODProviderReceipt(ctx context.Context, c *PODClaim, receipt json.RawMessage, charged int64, confirmed bool) error {
	return DB(ctx).Session(&gorm.Session{Logger: logger.Discard}).Transaction(func(tx *gorm.DB) error {
		if err := lockPODGate(tx); err != nil {
			return err
		}
		if err := checkPODClaim(tx, c); err != nil {
			return err
		}
		var row model.PODProviderCall
		if err := tx.Where("submission_key=? AND run_id=?", c.Node.SubmissionKey, c.Run.RunID).Take(&row).Error; err != nil {
			return err
		}
		if row.State == "confirmed" {
			return nil
		}
		if charged < 0 || charged > row.ReservedMicros {
			return ErrPODBudget
		}
		updates := map[string]any{"receipt_json": string(receipt), "update_time": podNow()}
		if confirmed {
			updates["state"] = "confirmed"
			updates["reserved_micros"] = 0
			updates["charged_micros"] = charged
		}
		return tx.Model(&row).Updates(updates).Error
	})
}

func SelectPODProviderAssets(ctx context.Context, key string, workspaceID int64) (map[int]string, error) {
	rows := []model.PODProviderAsset{}
	err := DB(ctx).Where("submission_key=? AND workspace_id=?", key, workspaceID).Find(&rows).Error
	out := map[int]string{}
	for _, row := range rows {
		out[row.ImageIndex] = strconv.FormatInt(row.AssetID, 10)
	}
	return out, err
}

func InsertPODGeneratedAsset(ctx context.Context, c *PODClaim, index int, asset *model.PODAsset, files []model.PODAssetFile, reservation string) (int64, error) {
	var id int64
	err := DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPODGate(tx); err != nil {
			return err
		}
		var w model.PODWorkspace
		if err := tx.Table("pod_workspace").Where("workspace_id=? AND owner_user_id=?", c.Run.WorkspaceID, c.Run.UserID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&w).Error; err != nil {
			return err
		}
		if err := checkPODClaim(tx, c); err != nil {
			return err
		}
		var existing model.PODProviderAsset
		err := tx.Where("submission_key=? AND image_index=?", c.Node.SubmissionKey, index).Take(&existing).Error
		if err == nil {
			id = existing.AssetID
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		asset.WorkspaceID = c.Run.WorkspaceID
		if err := tx.Table("pod_asset").Create(asset).Error; err != nil {
			return err
		}
		for i := range files {
			files[i].AssetID = asset.AssetID
		}
		if err := tx.Table("pod_asset_file").Create(&files).Error; err != nil {
			return err
		}
		now := podNow()
		if err := tx.Create(&model.PODRunAssetRef{RunID: c.Run.RunID, AssetID: asset.AssetID, WorkspaceID: c.Run.WorkspaceID, CreateBy: "pod-worker", UpdateBy: "pod-worker", CreateTime: now, UpdateTime: now}).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.PODProviderAsset{SubmissionKey: c.Node.SubmissionKey, ImageIndex: index, AssetID: asset.AssetID, WorkspaceID: c.Run.WorkspaceID, CreateBy: "pod-worker", UpdateBy: "pod-worker", CreateTime: now, UpdateTime: now}).Error; err != nil {
			return err
		}
		id = asset.AssetID
		return commitPODStorage(tx, c.Run.UserID, reservation, id, files)
	})
	return id, err
}
