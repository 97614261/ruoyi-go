package repository

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/types"
	"time"
)

const recoveryOwner = "CONTENT_RECOVERY"

func recoveryConflict() error {
	return errs.WithCode(409, "内容或个人恢复副本已变化；已停止覆盖，请重新读取并核对")
}
func recoveryKey(tx *gorm.DB, kind string, id, user int64) *gorm.DB {
	return tx.Where("user_id=? AND kind=? AND content_id=?", user, kind, id)
}
func recoverySlot(tx *gorm.DB, kind string, id, user int64) (model.CMSRecovery, error) {
	var row model.CMSRecovery
	e := recoveryKey(tx, kind, id, user).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = nil
	}
	return row, e
}
func recoveryToken(row model.CMSRecovery, in model.CMSRecoveryToken) error {
	if row.RecoveryId != in.SlotId || row.Version != in.Version {
		return recoveryConflict()
	}
	return nil
}
func recoveryContentVersion(tx *gorm.DB, kind string, id int64) (int64, error) {
	if id == 0 {
		return 0, nil
	}
	var c model.CMSContent
	e := tx.Select("version").Where("content_id=? AND kind=? AND is_deleted=0", id, kind).Take(&c).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	return c.Version, e
}
func recoveryView(row model.CMSRecovery, current int64) (model.CMSRecoveryView, error) {
	v := model.CMSRecoveryView{CMSRecoveryToken: model.CMSRecoveryToken{SlotId: row.RecoveryId, Version: row.Version}, BaseVersion: row.BaseVersion, ContentVersion: current, BaseMatches: true, SavedAt: row.UpdateTime, ExpiresAt: row.ExpiresAt}
	if row.Payload != "" && time.Time(row.ExpiresAt).After(time.Now()) {
		var d model.CMSRecoveryDraft
		if e := json.Unmarshal([]byte(row.Payload), &d); e != nil {
			return v, e
		}
		v.Found = true
		v.Draft = &d
		v.BaseMatches = row.BaseVersion == current
	}
	return v, nil
}
func CMSGetRecovery(ctx context.Context, kind string, id, user int64) (model.CMSRecoveryView, error) {
	row, e := recoverySlot(cmsDB(ctx), kind, id, user)
	if e != nil {
		return model.CMSRecoveryView{}, e
	}
	current, e := recoveryContentVersion(cmsDB(ctx), kind, id)
	if e != nil {
		return model.CMSRecoveryView{}, e
	}
	return recoveryView(row, current)
}
func recoveryRefs(tx *gorm.DB, slot int64, ids []int64, operator string) error {
	if e := tx.Table("cms_asset_ref").Where("owner_type=? AND owner_id=?", recoveryOwner, slot).Delete(nil).Error; e != nil {
		return e
	}
	if len(ids) == 0 {
		return nil
	}
	rows := []map[string]any{}
	for _, id := range ids {
		rows = append(rows, map[string]any{"asset_id": id, "owner_type": recoveryOwner, "owner_id": slot, "create_by": operator, "create_time": types.Now(), "update_by": "", "remark": "个人自动保存引用"})
	}
	return tx.Table("cms_asset_ref").Create(rows).Error
}
func storeRecovery(tx *gorm.DB, row *model.CMSRecovery, kind string, id, user, base int64, payload, operator string) error {
	now := types.Now()
	expiry := types.Time(time.Time(now).Add(7 * 24 * time.Hour))
	if row.RecoveryId == 0 {
		*row = model.CMSRecovery{UserId: user, Kind: kind, ContentId: id, BaseVersion: base, Payload: payload, Version: 1, ExpiresAt: expiry, CreateBy: operator, CreateTime: now, UpdateBy: operator, UpdateTime: now}
		return tx.Create(row).Error
	}
	values := map[string]any{"payload": payload, "base_version": base, "version": row.Version + 1, "expires_at": expiry, "update_by": operator, "update_time": now}
	if e := tx.Model(&model.CMSRecovery{}).Where("recovery_id=? AND user_id=? AND version=?", row.RecoveryId, user, row.Version).Updates(values).Error; e != nil {
		return e
	}
	row.Payload = payload
	row.BaseVersion = base
	row.Version++
	row.ExpiresAt = expiry
	row.UpdateTime = now
	return nil
}
func CMSPutRecovery(ctx context.Context, kind string, id, user int64, in model.CMSRecoveryInput, ids []int64, payload, operator string) (model.CMSRecoveryView, error) {
	var out model.CMSRecoveryView
	e := cmsLocked(ctx, func(tx *gorm.DB) error {
		current, e := recoveryContentVersion(tx, kind, id)
		if e != nil {
			return e
		}
		if current != in.BaseVersion || (id > 0 && current == 0) {
			return recoveryConflict()
		}
		row, e := recoverySlot(tx, kind, id, user)
		if e != nil {
			return e
		}
		if e = recoveryToken(row, in.CMSRecoveryToken); e != nil {
			return e
		}
		if in.Draft.ChannelId > 0 {
			var n int64
			if e = tx.Model(&model.CmsChannel{}).Where("channel_id=? AND kind=?", in.Draft.ChannelId, kind).Count(&n).Error; e != nil {
				return e
			}
			if n != 1 {
				return errs.New("恢复副本引用的栏目不存在或类型不符")
			}
		}
		if len(ids) > 0 {
			var n int64
			if e = tx.Model(&model.CmsAsset{}).Where("asset_id IN ? AND is_deleted=0", ids).Count(&n).Error; e != nil {
				return e
			}
			if n != int64(len(ids)) {
				return errs.New("恢复副本引用的图片不存在或已删除")
			}
		}
		if e = storeRecovery(tx, &row, kind, id, user, in.BaseVersion, payload, operator); e != nil {
			return e
		}
		if e = recoveryRefs(tx, row.RecoveryId, ids, operator); e != nil {
			return e
		}
		out, e = recoveryView(row, current)
		return e
	})
	return out, e
}

// An empty slot is a versioned tombstone: delayed requests cannot resurrect a cleared/new-form cache.
func CMSClearRecovery(ctx context.Context, kind string, id, user int64, in model.CMSRecoveryToken, operator string) (model.CMSRecoveryView, error) {
	var out model.CMSRecoveryView
	e := cmsLocked(ctx, func(tx *gorm.DB) error {
		row, e := recoverySlot(tx, kind, id, user)
		if e != nil {
			return e
		}
		if e = recoveryToken(row, in); e != nil {
			return e
		}
		current, e := recoveryContentVersion(tx, kind, id)
		if e != nil {
			return e
		}
		if e = storeRecovery(tx, &row, kind, id, user, current, "", operator); e != nil {
			return e
		}
		if e = recoveryRefs(tx, row.RecoveryId, nil, operator); e != nil {
			return e
		}
		out, e = recoveryView(row, current)
		return e
	})
	return out, e
}
func CMSCommitRecovery(ctx context.Context, kind string, id, user int64, in model.CMSRecoveryCommit, r model.CMSRevision, ids []int64, operator string) (model.CMSRecoveryCommitted, error) {
	var out model.CMSRecoveryCommitted
	e := cmsLocked(ctx, func(tx *gorm.DB) error {
		current, e := recoveryContentVersion(tx, kind, id)
		if e != nil {
			return e
		}
		if current != in.BaseVersion || (id > 0 && current == 0) {
			return recoveryConflict()
		}
		row, e := recoverySlot(tx, kind, id, user)
		if e != nil {
			return e
		}
		if e = recoveryToken(row, in.CMSRecoveryToken); e != nil {
			return e
		}
		if row.Payload != "" && time.Time(row.ExpiresAt).After(time.Now()) && row.BaseVersion != in.BaseVersion {
			return recoveryConflict()
		}
		out.ContentId, e = cmsSaveDraftTx(tx, kind, in.Draft, r, ids, operator)
		if e != nil {
			return e
		}
		if e = tx.Model(&model.CMSContent{}).Select("version").Where("content_id=?", out.ContentId).Scan(&out.Version).Error; e != nil {
			return e
		}
		if e = storeRecovery(tx, &row, kind, id, user, in.BaseVersion, "", operator); e != nil {
			return e
		}
		return recoveryRefs(tx, row.RecoveryId, nil, operator)
	})
	return out, e
}
func CMSCleanupRecoveries(ctx context.Context) error {
	return cmsLocked(ctx, func(tx *gorm.DB) error {
		rows := []model.CMSRecovery{}
		if e := tx.Select("recovery_id").Where("expires_at<=?", types.Now()).Order("expires_at,recovery_id").Limit(20).Find(&rows).Error; e != nil {
			return e
		}
		if len(rows) == 0 {
			return nil
		}
		ids := []int64{}
		for _, r := range rows {
			ids = append(ids, r.RecoveryId)
		}
		if e := tx.Table("cms_asset_ref").Where("owner_type=? AND owner_id IN ?", recoveryOwner, ids).Delete(nil).Error; e != nil {
			return e
		}
		return tx.Where("recovery_id IN ?", ids).Delete(&model.CMSRecovery{}).Error
	})
}
