package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"ruoyi-go/internal/model"
	"ruoyi-go/pkg/page"
)

// SelectPostPage 分页查询岗位，返回当页数据和总数。
func SelectPostPage(ctx context.Context, query model.PostQuery, pg page.Query) ([]model.SysPost, int64, error) {
	db := postFilter(DB(ctx).Model(&model.SysPost{}), query)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计岗位总数失败: %w", err)
	}
	if total == 0 {
		return []model.SysPost{}, 0, nil
	}

	// Java 的 SysPostMapper.xml 全文没有 order by，但行顺序不属于接口契约，
	// 不该把"翻页可能重复/漏行"这个缺陷一起复刻过来。详见 page.Query.Stable
	var list []model.SysPost
	err := db.Order(pg.Stable("post_id", "post_id")).
		Offset(pg.Offset()).
		Limit(pg.PageSize).
		Find(&list).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询岗位列表失败: %w", err)
	}
	return list, total, nil
}

// SelectPostList 不分页查询，供导出使用。
func SelectPostList(ctx context.Context, query model.PostQuery, limit ...int) ([]model.SysPost, error) {
	var list []model.SysPost
	err := applyOptionalLimit(postFilter(DB(ctx).Model(&model.SysPost{}), query), limit).
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("查询岗位列表失败: %w", err)
	}
	return list, nil
}

func postFilter(db *gorm.DB, query model.PostQuery) *gorm.DB {
	if query.PostCode != "" {
		db = db.Where("post_code LIKE ?", "%"+query.PostCode+"%")
	}
	if query.PostName != "" {
		db = db.Where("post_name LIKE ?", "%"+query.PostName+"%")
	}
	if query.Status != "" {
		db = db.Where("status = ?", query.Status)
	}
	return db
}

// SelectPostByID 按 ID 查岗位，不存在返回 (nil, nil)。
func SelectPostByID(ctx context.Context, postID int64) (*model.SysPost, error) {
	var post model.SysPost
	err := DB(ctx).Where("post_id = ?", postID).Take(&post).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询岗位 %d 失败: %w", postID, err)
	}
	return &post, nil
}

// SelectPostAll 查全部岗位，供下拉选择使用。
func SelectPostAll(ctx context.Context) ([]model.SysPost, error) {
	var list []model.SysPost
	err := DB(ctx).Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("查询全部岗位失败: %w", err)
	}
	return list, nil
}

// InsertPost 新增岗位。
func InsertPost(ctx context.Context, post *model.SysPost) error {
	if err := DB(ctx).Create(post).Error; err != nil {
		return fmt.Errorf("新增岗位失败: %w", err)
	}
	return nil
}

// UpdatePost 更新岗位。
//
// 用 map 而不是 struct：Updates(struct) 会跳过零值，
// 导致"把排序改成 0"这类操作静默失效。
//
// remark 只在非 nil 时才写入，对齐 Java 版 <if test="remark != null">。
// 若无条件写入，请求体不带 remark 时会把已有备注清成 NULL。
func UpdatePost(ctx context.Context, post *model.SysPost) error {
	updates := map[string]any{
		"post_code":   post.PostCode,
		"post_name":   post.PostName,
		"post_sort":   model.IntValue(post.PostSort),
		"status":      post.Status,
		"update_by":   post.UpdateBy,
		"update_time": post.UpdateTime,
	}
	if post.Remark != nil {
		updates["remark"] = *post.Remark
	}

	result := DB(ctx).Model(&model.SysPost{}).
		Where("post_id = ?", post.PostID).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("更新岗位 %d 失败: %w", post.PostID, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("更新岗位 %d 失败: 岗位不存在", post.PostID)
	}
	return nil
}

// DeletePostByIDs 批量删除岗位。
func DeletePostByIDs(ctx context.Context, postIDs []int64) error {
	if len(postIDs) == 0 {
		return nil
	}
	err := DB(ctx).Where("post_id IN ?", postIDs).Delete(&model.SysPost{}).Error
	if err != nil {
		return fmt.Errorf("删除岗位失败: %w", err)
	}
	return nil
}

// CountPostByName 统计同名岗位数量，excludeID 用于修改时排除自身。
func CountPostByName(ctx context.Context, postName string, excludeID int64) (int64, error) {
	return countPost(ctx, "post_name = ?", postName, excludeID)
}

// CountPostByCode 统计同编码岗位数量，excludeID 用于修改时排除自身。
func CountPostByCode(ctx context.Context, postCode string, excludeID int64) (int64, error) {
	return countPost(ctx, "post_code = ?", postCode, excludeID)
}

func countPost(ctx context.Context, cond string, value any, excludeID int64) (int64, error) {
	db := DB(ctx).Model(&model.SysPost{}).Where(cond, value)
	if excludeID > 0 {
		db = db.Where("post_id <> ?", excludeID)
	}
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("校验岗位唯一性失败: %w", err)
	}
	return count, nil
}

// CountUserPostByPostID 统计该岗位下已分配的用户数，用于删除前检查。
func CountUserPostByPostID(ctx context.Context, postID int64) (int64, error) {
	var count int64
	err := DB(ctx).Table("sys_user_post").Where("post_id = ?", postID).Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("统计岗位 %d 的用户数失败: %w", postID, err)
	}
	return count, nil
}
