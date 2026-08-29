package apitest

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
)

// TestOperLogPasswordRedaction 验证 JSON 大小写宽松绑定不会绕过操作日志脱敏。
func TestOperLogPasswordRedaction(t *testing.T) {
	body := newUserPayload("operlog_password")
	id := createUser(t, body)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var baseline int64
	if err := repository.DB(ctx).Model(&model.SysOperLog{}).
		Select("COALESCE(MAX(oper_id), 0)").Scan(&baseline).Error; err != nil {
		t.Fatalf("读取操作日志基线失败：%v", err)
	}

	const secret = "CaseSecret987"
	// encoding/json 会把 Password 绑定到 password 字段；旧正则却只认小写 password。
	mustOK(t, doPut(t, "/system/user/resetPwd", map[string]any{
		"userId": id, "Password": secret,
	}), "使用大小写变体字段重置密码")

	var record model.SysOperLog
	userIDPattern := fmt.Sprintf("%%\"userId\":%d%%", id)
	for {
		result := repository.DB(ctx).
			Where("oper_id > ? AND oper_url = ? AND oper_param LIKE ?", baseline,
				"/system/user/resetPwd", userIDPattern).
			Order("oper_id DESC").Limit(1).Find(&record)
		if result.Error != nil {
			t.Fatalf("读取重置密码操作日志失败：%v", result.Error)
		}
		if result.RowsAffected > 0 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("等待异步操作日志写入超时")
		case <-time.After(20 * time.Millisecond):
		}
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = repository.DB(cleanupCtx).Delete(&model.SysOperLog{}, record.OperID).Error
	})

	if strings.Contains(record.OperParam, secret) {
		t.Fatalf("操作日志泄漏了明文密码：%s", record.OperParam)
	}
	if !strings.Contains(record.OperParam, `"Password":"******"`) {
		t.Errorf("操作日志应保留字段结构并替换密码，实际=%s", record.OperParam)
	}
}
