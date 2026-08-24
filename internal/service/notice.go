package service

import (
	"context"
	"time"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

// noticeTopLimit 顶栏展示的公告条数，与 Java 版一致。
const noticeTopLimit = 5

// ListNoticeTop 取顶栏公告列表及未读数。
func ListNoticeTop(ctx context.Context, userID int64) ([]model.NoticeTopItem, int64, error) {
	list, err := repository.SelectNoticeTopWithReadStatus(ctx, userID, noticeTopLimit)
	if err != nil {
		return nil, 0, err
	}

	var unread int64
	for _, item := range list {
		if !item.IsRead {
			unread++
		}
	}
	return list, unread, nil
}

// ListNoticePage 分页查询公告。
func ListNoticePage(ctx context.Context, query model.NoticeQuery, pg page.Query) ([]model.SysNotice, int64, error) {
	return repository.SelectNoticePage(ctx, query, pg)
}

// GetNotice 按 ID 查公告。
func GetNotice(ctx context.Context, noticeID int64) (*model.SysNotice, error) {
	notice, err := repository.SelectNoticeByID(ctx, noticeID)
	if err != nil {
		return nil, err
	}
	if notice == nil {
		return nil, errs.New("公告不存在")
	}
	return notice, nil
}

// CreateNotice 新增公告。
func CreateNotice(ctx context.Context, notice *model.SysNotice, operator string) error {
	notice.NoticeID = 0
	if notice.Status == "" {
		notice.Status = model.StatusNormal
	}
	notice.CreateBy = operator
	notice.CreateTime = types.Now()
	return repository.InsertNotice(ctx, notice)
}

// UpdateNotice 修改公告。
func UpdateNotice(ctx context.Context, notice *model.SysNotice, operator string) error {
	if notice.NoticeID == 0 {
		return errs.New("公告ID不能为空")
	}
	existing, err := repository.SelectNoticeByID(ctx, notice.NoticeID)
	if err != nil {
		return err
	}
	if existing == nil {
		return errs.New("公告不存在")
	}

	notice.UpdateBy = operator
	notice.UpdateTime = types.Now()
	return repository.UpdateNotice(ctx, notice)
}

// DeleteNotices 批量删除公告。
func DeleteNotices(ctx context.Context, noticeIDs []int64) error {
	if len(noticeIDs) == 0 {
		return errs.New("请选择要删除的公告")
	}
	return repository.DeleteNoticeByIDs(ctx, noticeIDs)
}

// MarkNoticeRead 标记单条公告已读。
func MarkNoticeRead(ctx context.Context, userID, noticeID int64) error {
	if noticeID == 0 {
		return errs.New("公告ID不能为空")
	}
	return repository.MarkNoticeRead(ctx, userID, []int64{noticeID}, time.Now())
}

// MarkNoticeReadBatch 批量标记已读，noticeIDs 为空时标记全部未读。
func MarkNoticeReadBatch(ctx context.Context, userID int64, noticeIDs []int64) error {
	if len(noticeIDs) == 0 {
		unread, err := repository.SelectUnreadNoticeIDs(ctx, userID)
		if err != nil {
			return err
		}
		noticeIDs = unread
	}
	if len(noticeIDs) == 0 {
		return nil // 本来就没有未读，不算错误
	}
	return repository.MarkNoticeRead(ctx, userID, noticeIDs, time.Now())
}

// ListNoticeReadUsers 查某条公告的已读用户。
func ListNoticeReadUsers(ctx context.Context, query model.NoticeReadUserQuery, pg page.Query) ([]model.NoticeReadUser, int64, error) {
	if query.NoticeID == 0 {
		return nil, 0, errs.New("公告ID不能为空")
	}
	return repository.SelectNoticeReadUserPage(ctx, query, pg)
}
