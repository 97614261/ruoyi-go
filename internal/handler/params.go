package handler

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"ruoyi-go/internal/middleware"
	"ruoyi-go/pkg/errs"
)

// maxBatchIDs 批量操作的 ID 数量上限，防止一次删几万行把库拖死。
const maxBatchIDs = 200

// parseIDs 解析路径里逗号分隔的 ID 列表，如 /system/post/1,2,3。
func parseIDs(raw string) ([]int64, error) {
	parts := strings.Split(raw, ",")
	if len(parts) > maxBatchIDs {
		return nil, errs.Newf("一次最多操作%d条数据", maxBatchIDs)
	}

	ids := make([]int64, 0, len(parts))
	seen := make(map[int64]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id <= 0 {
			return nil, errs.New("参数格式错误")
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, errs.New("参数不能为空")
	}
	return ids, nil
}

// parseID 解析路径里的单个 ID。
func parseID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, errs.New("参数格式错误")
	}
	return id, nil
}

// currentUsername 取当前登录账号，用于填 create_by / update_by。
func currentUsername(c *gin.Context) string {
	loginUser := middleware.CurrentUser(c)
	if loginUser == nil || loginUser.User == nil {
		return ""
	}
	return loginUser.User.UserName
}
