package repository

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math"
	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/errs"
)

// A failed/unknown commit retains its reservation. Time alone never releases it.
func PODReserveStorage(ctx context.Context, user int64, key string, size int64, archive bool) error {
	if size <= 0 || size > 64<<20 || len(key) > 120 {
		return errs.WithCode(400, "存储预留参数无效")
	}
	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		if !archive {
			if _, err := lockedPODWorkspace(tx, user); err != nil {
				return err
			}
		} else {
			var w model.PODWorkspace
			if err := tx.Table("pod_workspace").Where("owner_user_id=?", user).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&w).Error; err != nil {
				return err
			}
		}
		return reservePODStorageTx(tx, user, key, size)
	})
}

func reservePODStorageTx(tx *gorm.DB, user int64, key string, size int64) error {
	if size <= 0 || size > 64<<20 || len(key) > 120 {
		return errs.WithCode(400, "存储预留参数无效")
	}
	var p model.PODCreatorPolicy
	if err := tx.Where("user_id=?", user).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&p).Error; err != nil {
		return err
	}
	var a model.PODStorageAccount
	if err := tx.Where("user_id=?", user).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&a).Error; err != nil {
		return err
	}
	var n int64
	if err := tx.Table("pod_storage_reservation").Where("user_id=? AND operation_key=?", user, key).Count(&n).Error; err != nil {
		return err
	}
	if n != 0 {
		return errs.WithCode(409, "原存储操作尚需核对，未重复写入文件")
	}
	if a.UsedBytes < 0 || a.HeldBytes < 0 || a.UsedBytes > p.MaxStorageBytes || a.HeldBytes > p.MaxStorageBytes-a.UsedBytes || size > p.MaxStorageBytes-a.UsedBytes-a.HeldBytes {
		return errs.WithCode(409, "存储空间不足，请联系管理员；回收站和缩略图也占空间")
	}
	if a.Revision == math.MaxInt64 {
		return errors.New("storage revision overflow")
	}
	if err := tx.Exec("UPDATE pod_storage_account SET held_bytes=held_bytes+?,revision=revision+1,update_by='pod',update_time=NOW(3) WHERE user_id=?", size, user).Error; err != nil {
		return err
	}
	return tx.Exec("INSERT INTO pod_storage_reservation(user_id,operation_key,reserved_bytes,state,lease_until,result_asset_ids_json,create_by) VALUES(?,?,?,'held',DATE_ADD(NOW(3),INTERVAL 10 MINUTE),'[]','pod')", user, key, size).Error
}

func commitPODStorage(tx *gorm.DB, user int64, key string, asset int64, files []model.PODAssetFile) error {
	var a model.PODStorageAccount
	if err := tx.Where("user_id=?", user).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&a).Error; err != nil {
		return err
	}
	var r model.PODStorageReservation
	if err := tx.Where("user_id=? AND operation_key=? AND state='held'", user, key).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&r).Error; err != nil {
		return err
	}
	var n int64
	for _, f := range files {
		if f.ByteSize < 0 || n > math.MaxInt64-f.ByteSize {
			return errors.New("storage size overflow")
		}
		n += f.ByteSize
	}
	if n > r.ReservedBytes || a.HeldBytes < r.ReservedBytes || a.UsedBytes > math.MaxInt64-n || a.Revision == math.MaxInt64 {
		return errors.New("storage reservation invariant failed")
	}
	if err := tx.Exec("UPDATE pod_storage_account SET held_bytes=held_bytes-?,used_bytes=used_bytes+?,revision=revision+1,update_by='pod',update_time=NOW(3) WHERE user_id=?", r.ReservedBytes, n, user).Error; err != nil {
		return err
	}
	ids, _ := json.Marshal([]int64{asset})
	return tx.Exec("UPDATE pod_storage_reservation SET state='committed',committed_bytes=?,result_asset_ids_json=?,update_by='pod',update_time=NOW(3) WHERE reservation_id=?", n, string(ids), r.ReservationID).Error
}

// Only called after the caller has removed every file from a known failed write.
func PODReleaseStorage(ctx context.Context, user int64, key string) error {
	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		var w model.PODWorkspace
		if err := tx.Table("pod_workspace").Where("owner_user_id=?", user).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&w).Error; err != nil {
			return err
		}
		var a model.PODStorageAccount
		if err := tx.Where("user_id=?", user).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&a).Error; err != nil {
			return err
		}
		var r model.PODStorageReservation
		if err := tx.Where("user_id=? AND operation_key=?", user, key).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&r).Error; err != nil {
			return err
		}
		if r.State != "held" {
			return nil
		}
		if a.HeldBytes < r.ReservedBytes || a.Revision == math.MaxInt64 {
			return errors.New("storage release invariant failed")
		}
		if err := tx.Exec("UPDATE pod_storage_account SET held_bytes=held_bytes-?,revision=revision+1,update_by='pod',update_time=NOW(3) WHERE user_id=?", r.ReservedBytes, user).Error; err != nil {
			return err
		}
		return tx.Model(&r).Updates(map[string]any{"state": "released", "update_by": "pod", "update_time": gorm.Expr("NOW(3)")}).Error
	})
}

func PODVerifyCreatorAccounts(ctx context.Context) error {
	var invalid int64
	checks := []string{
		"SELECT COUNT(*) FROM pod_points_account a WHERE a.granted_total<>(SELECT COALESCE(SUM(e.amount),0) FROM pod_points_event e WHERE e.user_id=a.user_id AND e.kind='grant') OR a.deducted_total<>(SELECT COALESCE(SUM(e.amount),0) FROM pod_points_event e WHERE e.user_id=a.user_id AND e.kind='deduct')",
		"SELECT COUNT(*) FROM pod_points_account WHERE available<0 OR held<0 OR spent<0 OR granted_total<0 OR deducted_total<0 OR revision<1 OR CAST(available AS DECIMAL(30,0))+held+spent<>CAST(granted_total AS DECIMAL(30,0))-deducted_total",
		"SELECT COUNT(*) FROM pod_storage_account s JOIN pod_creator_policy p ON p.user_id=s.user_id WHERE s.used_bytes<0 OR s.held_bytes<0 OR s.used_bytes<>(SELECT COALESCE(SUM(f.byte_size),0) FROM pod_asset a JOIN pod_asset_file f ON f.asset_id=a.asset_id WHERE a.workspace_id=p.workspace_id) OR s.held_bytes<>(SELECT COALESCE(SUM(reserved_bytes),0) FROM pod_storage_reservation r WHERE r.user_id=s.user_id AND r.state='held')",
		"SELECT COUNT(*) FROM pod_creator_policy p LEFT JOIN pod_points_account a ON a.user_id=p.user_id LEFT JOIN pod_storage_account s ON s.user_id=p.user_id LEFT JOIN pod_workspace w ON w.workspace_id=p.workspace_id AND w.owner_user_id=p.user_id WHERE a.user_id IS NULL OR s.user_id IS NULL OR w.workspace_id IS NULL OR p.status NOT IN ('active','suspended') OR p.max_concurrent_nodes<1 OR p.max_pending_runs<1 OR p.max_daily_submissions<1 OR p.max_storage_bytes<1",
	}
	for _, q := range checks {
		if err := DB(ctx).Raw(q).Scan(&invalid).Error; err != nil {
			return err
		}
		if invalid != 0 {
			return errors.New("用户权益、点数或存储账户校验失败")
		}
	}
	return nil
}
