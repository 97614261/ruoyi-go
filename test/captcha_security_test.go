package apitest

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/redisx"
)

func TestCaptchaConsumeContract(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	id, answer := newCaptchaForVerify(t, ctx)
	if err := service.VerifyCaptcha(ctx, id, answer+"-wrong"); err == nil ||
		!strings.Contains(err.Error(), "验证码错误") {
		t.Fatalf("错误答案应返回验证码错误，实际=%v", err)
	}
	if err := service.VerifyCaptcha(ctx, id, answer); err == nil ||
		!strings.Contains(err.Error(), "验证码已失效") {
		t.Fatalf("错误尝试后验证码必须已被消费，实际=%v", err)
	}

	id, answer = newCaptchaForVerify(t, ctx)
	if err := service.VerifyCaptcha(ctx, id, strings.ToLower(answer)); err != nil {
		t.Fatalf("正确答案应通过验证：%v", err)
	}
	if err := service.VerifyCaptcha(ctx, id, answer); err == nil ||
		!strings.Contains(err.Error(), "验证码已失效") {
		t.Fatalf("成功验证后验证码不能再次使用，实际=%v", err)
	}
}

func TestCaptchaConcurrentSingleUse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	id, answer := newCaptchaForVerify(t, ctx)
	const attempts = 8
	start := make(chan struct{})
	errs := make([]error, attempts)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			errs[index] = service.VerifyCaptcha(ctx, id, answer)
		}(i)
	}
	close(start)
	wg.Wait()

	succeeded := 0
	expired := 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case strings.Contains(err.Error(), "验证码已失效"):
			expired++
		default:
			t.Errorf("并发消费返回了非预期错误：%v", err)
		}
	}
	if succeeded != 1 || expired != attempts-1 {
		t.Fatalf("同一验证码只能成功消费一次，success=%d expired=%d", succeeded, expired)
	}
}

func newCaptchaForVerify(t *testing.T, ctx context.Context) (string, string) {
	t.Helper()
	id, _, err := service.GenerateCaptcha(ctx)
	if err != nil {
		t.Fatalf("生成测试验证码失败：%v", err)
	}
	key := redisx.CaptchaKey(id)
	trackRedisKey(key)
	answer, err := redisx.C().Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("读取测试验证码答案失败：%v", err)
	}
	return id, answer
}
