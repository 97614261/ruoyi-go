package service

import (
	"context"
	"fmt"
	"os"
	"ruoyi-go/internal/config"
	"ruoyi-go/pkg/redisx"
	"testing"
	"time"
)

func TestInquiryProofAndLimits(t *testing.T) {
	if os.Getenv("PORTAL_INTEGRATION") != "1" {
		t.Skip("explicit local portal integration")
	}
	cfg, e := config.Load("../../configs/portal.local.yml")
	if e != nil || cfg.Redis.DB != 14 || cfg.Redis.Addr != "127.0.0.1:6379" {
		t.Fatal("local portal Redis required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if e = redisx.Init(ctx, redisx.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: 14}); e != nil {
		t.Fatal(e)
	}
	defer redisx.Close()
	key := fmt.Sprintf("portal_inquiry:test:%d", time.Now().UnixNano())
	defer redisx.C().Del(context.Background(), key)
	for i := 0; i < 2; i++ {
		if e = inquiryLimit(ctx, key, 2, time.Minute); e != nil {
			t.Fatal(e)
		}
	}
	if e = inquiryLimit(ctx, key, 2, time.Minute); e == nil {
		t.Fatal("rate cap not enforced")
	}
	key += "-proof"
	defer redisx.C().Del(context.Background(), key)
	now := time.Now().Unix()
	check := func(want int, ip string) {
		v, e := inquiryConsume.Run(ctx, redisx.C(), []string{key}, ip, now).Int()
		if e != nil || v != want {
			t.Fatal("one-use proof result", v, want, e)
		}
	}
	if e = redisx.C().Set(ctx, key, fmt.Sprintf("%d:hash", now), time.Minute).Err(); e != nil {
		t.Fatal(e)
	}
	check(0, "hash")
	if e = redisx.C().Set(ctx, key, fmt.Sprintf("%d:hash", now-3), time.Minute).Err(); e != nil {
		t.Fatal(e)
	}
	check(0, "other-ip")
	check(1, "hash")
	check(0, "hash")
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, e = CMSInquiryProof(canceled, "test"); e == nil {
		t.Fatal("Redis failure/canceled request must not issue proof")
	}
	t.Log("PASS rate cap, age gate, IP binding, atomic consume/replay rejection, fail-closed proof issuance")
}
