package upload

import (
	"fmt"
	"testing"
)

func TestClientMessageHidesInternalErrors(t *testing.T) {
	_, validationErr := Save(nil, Options{})
	if message, ok := ClientMessage(validationErr); !ok || message != "未选择文件" {
		t.Fatalf("validation error should be safe: message=%q ok=%v", message, ok)
	}

	internalErr := fmt.Errorf("创建上传目录失败: C:\\secret\\upload: access denied")
	if message, ok := ClientMessage(internalErr); ok || message != "" {
		t.Fatalf("internal error leaked as client message: message=%q ok=%v", message, ok)
	}
}
