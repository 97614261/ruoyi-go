package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

// SelectNoticeTopWithReadStatus 查最近的公告并带上当前用户的已读状态。
//
// 对应 Java 版 SysNoticeReadMapper.selectNoticeListWithReadStatus。
// 用 left join 判断已读，不含 notice_content。
func SelectNoticeTopWithReadStatus(ctx context.Context, userID int64, limit int) ([]model.NoticeTopItem, error) {
	if limit <= 0 {
		limit = 5
	}

	var list []model.NoticeTopItem
	err := DB(ctx).
		Table("sys_notice n").
		Select(`n.notice_id, n.notice_title, n.notice_type, n.status,
			n.create_by, n.create_time,
			CASE WHEN r.notice_id IS NOT NULL THEN 1 ELSE 0 END AS is_read`).
		Joins("LEFT JOIN sys_notice_read r ON r.notice_id = n.notice_id AND r.user_id = ?", userID).
		Where("n.status = ?", model.StatusNormal).
		Order("n.notice_id DESC").
		Limit(limit).
		Scan(&list).Error
	if err != nil {
		return nil, fmt.Errorf("查询用户 %d 的公告失败: %w", userID, err)
	}
	return list, nil
}

func noticeFilter(db *gorm.DB, query model.NoticeQuery) *gorm.DB {
	if query.NoticeTitle != "" {
		db = db.Where("notice_title LIKE ?", "%"+query.NoticeTitle+"%")
	}
	if query.NoticeType != "" {
		db = db.Where("notice_type = ?", query.NoticeType)
	}
	if query.CreateBy != "" {
		db = db.Where("create_by LIKE ?", "%"+query.CreateBy+"%")
	}
	if query.Status != "" {
		db = db.Where("status = ?", query.Status)
	}
	return db
}

// SelectNoticePage 分页查询公告。
//
// Java 管理列表会返回完整 notice_content，响应契约必须保持一致。
func SelectNoticePage(ctx context.Context, query model.NoticeQuery, pg page.Query) ([]model.SysNotice, int64, error) {
	db := noticeFilter(DB(ctx).Model(&model.SysNotice{}), query)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计公告总数失败: %w", err)
	}
	if total == 0 {
		return []model.SysNotice{}, 0, nil
	}

	orderBy := pg.Stable("notice_id DESC", "notice_id")

	var list []model.SysNotice
	err := db.
		Select("notice_id, notice_title, notice_type, CAST(notice_content AS CHAR) AS notice_content, status, create_by, create_time, update_by, update_time, remark").
		Order(orderBy).
		Offset(pg.Offset()).
		Limit(pg.PageSize).
		Find(&list).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询公告列表失败: %w", err)
	}
	return list, total, nil
}

// SelectNoticeByID 按 ID 查公告（含正文）。
func SelectNoticeByID(ctx context.Context, noticeID int64) (*model.SysNotice, error) {
	var notice model.SysNotice
	err := DB(ctx).Where("notice_id = ?", noticeID).Take(&notice).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询公告 %d 失败: %w", noticeID, err)
	}
	return &notice, nil
}

// InsertNotice 新增公告。
func InsertNotice(ctx context.Context, notice *model.SysNotice) error {
	if err := DB(ctx).Create(notice).Error; err != nil {
		return fmt.Errorf("新增公告失败: %w", err)
	}
	return nil
}

// UpdateNotice 更新公告。
func UpdateNotice(ctx context.Context, notice *model.SysNotice) error {
	updates := map[string]any{
		"notice_title":   notice.NoticeTitle,
		"notice_type":    notice.NoticeType,
		"notice_content": notice.NoticeContent,
		"status":         notice.Status,
		"update_by":      notice.UpdateBy,
		"update_time":    notice.UpdateTime,
	}
	if notice.Remark != nil {
		updates["remark"] = *notice.Remark
	}
	err := DB(ctx).Model(&model.SysNotice{}).
		Where("notice_id = ?", notice.NoticeID).
		Updates(updates).Error
	if err != nil {
		return fmt.Errorf("更新公告 %d 失败: %w", notice.NoticeID, err)
	}
	return nil
}

// DeleteNoticeByIDs 批量删除公告，同时清理已读记录。
func DeleteNoticeByIDs(ctx context.Context, noticeIDs []int64) error {
	if len(noticeIDs) == 0 {
		return nil
	}
	return Transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("notice_id IN ?", noticeIDs).Delete(&model.SysNoticeRead{}).Error; err != nil {
			return fmt.Errorf("清除公告已读记录失败: %w", err)
		}
		if err := tx.Where("notice_id IN ?", noticeIDs).Delete(&model.SysNotice{}).Error; err != nil {
			return fmt.Errorf("删除公告失败: %w", err)
		}
		return nil
	})
}

// MarkPublishedNoticesRead 校验公告存在且已发布，并在同一事务内标记已读。
// FOR UPDATE 保证校验到写入之间公告不能被并发删除或切回草稿。
func MarkPublishedNoticesRead(
	ctx context.Context,
	userID int64,
	noticeIDs []int64,
	at time.Time,
) ([]int64, error) {
	if len(noticeIDs) == 0 {
		return []int64{}, nil
	}

	var published []int64
	err := Transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Model(&model.SysNotice{}).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("notice_id IN ?", noticeIDs).
			Where("status = ?", model.StatusNormal).
			Pluck("notice_id", &published).Error; err != nil {
			return fmt.Errorf("校验可读公告失败: %w", err)
		}
		if len(published) != len(noticeIDs) {
			return nil
		}

		rows := make([]model.SysNoticeRead, 0, len(noticeIDs))
		for _, noticeID := range noticeIDs {
			rows = append(rows, model.SysNoticeRead{
				NoticeID: noticeID,
				UserID:   userID,
				ReadTime: types.Time(at),
			})
		}
		// sys_notice_read 有 (user_id, notice_id) 唯一索引，重复标记直接忽略。
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error; err != nil {
			return fmt.Errorf("标记公告已读失败: %w", err)
		}
		return nil
	})
	return published, err
}

// SelectNoticeReadUserPage 分页查询某条公告的已读用户。
func SelectNoticeReadUserPage(ctx context.Context, query model.NoticeReadUserQuery, pg page.Query) ([]model.NoticeReadUser, int64, error) {
	build := func() *gorm.DB {
		db := DB(ctx).
			Table("sys_notice_read r").
			Joins("INNER JOIN sys_user u ON u.user_id = r.user_id").
			Joins("LEFT JOIN sys_dept d ON d.dept_id = u.dept_id").
			Where("r.notice_id = ?", query.NoticeID).
			Where("u.del_flag = ?", model.DelFlagExist)
		// 前端那个输入框的 placeholder 是"登录名称/用户名称"，两列都要搜，
		// 与 Java 的 selectReadUsersByNoticeId(noticeId, searchValue) 一致
		if query.SearchValue != "" {
			keyword := "%" + query.SearchValue + "%"
			db = db.Where("u.user_name LIKE ? OR u.nick_name LIKE ?", keyword, keyword)
		}
		return db
	}

	var total int64
	if err := build().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计公告已读用户失败: %w", err)
	}
	if total == 0 {
		return []model.NoticeReadUser{}, 0, nil
	}

	var list []model.NoticeReadUser
	err := build().
		Select("u.user_id, u.user_name, u.nick_name, IFNULL(d.dept_name,'') AS dept_name, r.read_time").
		// read_time 精度到秒，一条公告推送后大批人同时点开是常态，
		// 并列几乎必然发生 —— 没有 user_id 兜底翻页就会重复/漏行。
		// 这个查询不接受前端排序，所以不走 pg.Stable
		Order("r.read_time DESC, u.user_id").
		Offset(pg.Offset()).
		Limit(pg.PageSize).
		Scan(&list).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询公告已读用户失败: %w", err)
	}
	return list, total, nil
}
