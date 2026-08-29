package service

import (
	"context"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
	"ruoyi-go/pkg/types"
)

// ListPostPage 分页查询岗位。
func ListPostPage(ctx context.Context, query model.PostQuery, pg page.Query) ([]model.SysPost, int64, error) {
	return repository.SelectPostPage(ctx, query, pg)
}

// ListPostAll 查全部岗位。
func ListPostAll(ctx context.Context) ([]model.SysPost, error) {
	return repository.SelectPostAll(ctx)
}

// ListPostExport 按查询条件取全部匹配数据，供导出使用。
func ListPostExport(ctx context.Context, query model.PostQuery) ([]model.SysPost, error) {
	list, err := repository.SelectPostList(ctx, query, MaxExportRows+1)
	if err != nil {
		return nil, err
	}
	if err := checkExportSize(len(list)); err != nil {
		return nil, err
	}
	return list, nil
}

// GetPost 按 ID 查岗位。
func GetPost(ctx context.Context, postID int64) (*model.SysPost, error) {
	post, err := repository.SelectPostByID(ctx, postID)
	if err != nil {
		return nil, err
	}
	if post == nil {
		return nil, errs.New("岗位不存在")
	}
	return post, nil
}

// CreatePost 新增岗位。
func CreatePost(ctx context.Context, post *model.SysPost, operator string) error {
	if err := checkPostUnique(ctx, post, "新增"); err != nil {
		return err
	}
	post.PostID = 0
	post.CreateBy = operator
	post.CreateTime = types.Now()
	return repository.InsertPost(ctx, post)
}

// UpdatePost 修改岗位。
func UpdatePost(ctx context.Context, post *model.SysPost, operator string) error {
	if post.PostID <= 0 {
		return errs.New("岗位ID不能为空")
	}
	existing, err := repository.SelectPostByID(ctx, post.PostID)
	if err != nil {
		return err
	}
	if existing == nil {
		return errs.New("岗位不存在")
	}
	if err := checkPostUnique(ctx, post, "修改"); err != nil {
		return err
	}
	post.UpdateBy = operator
	post.UpdateTime = types.Now()
	return repository.UpdatePost(ctx, post)
}

// DeletePosts 批量删除岗位。
//
// 已分配给用户的岗位不允许删除，与 Java 版行为一致：
// 逐个检查并在第一个冲突处中止，提示里带上岗位名。
func DeletePosts(ctx context.Context, postIDs []int64) error {
	if len(postIDs) == 0 {
		return errs.New("请选择要删除的岗位")
	}
	var err error
	postIDs, err = normalizeRelationIDs(postIDs, "岗位")
	if err != nil {
		return err
	}
	posts, counts, err := repository.SelectPostsForDelete(ctx, postIDs)
	if err != nil {
		return err
	}
	for _, post := range posts {
		if counts[post.PostID] > 0 {
			return errs.Newf("%s已分配,不能删除", post.PostName)
		}
	}
	return repository.DeletePostByIDs(ctx, postIDs)
}

// checkPostUnique 校验岗位名称和编码唯一。
//
// PostID > 0 时视为修改，排除自身。
// action 取 "新增" 或 "修改"，用于拼出与 Java 版一致的提示语：
// "新增岗位'测试'失败，岗位名称已存在"。
func checkPostUnique(ctx context.Context, post *model.SysPost, action string) error {
	nameCount, err := repository.CountPostByName(ctx, post.PostName, post.PostID)
	if err != nil {
		return err
	}
	if nameCount > 0 {
		return errs.Newf("%s岗位'%s'失败，岗位名称已存在", action, post.PostName)
	}

	codeCount, err := repository.CountPostByCode(ctx, post.PostCode, post.PostID)
	if err != nil {
		return err
	}
	if codeCount > 0 {
		return errs.Newf("%s岗位'%s'失败，岗位编码已存在", action, post.PostName)
	}
	return nil
}
