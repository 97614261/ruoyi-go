package service

import (
	"context"
	"github.com/redis/go-redis/v9"
	"testing"
)

func TestPermissionVersionPipelineUsesColdCacheSafeEval(t *testing.T) {
	// Queue only: no network or shared Redis script-cache mutation.
	c := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	defer c.Close()
	p := c.Pipeline()
	defer p.Discard()
	cmd := queuePermissionVersionAdvance(context.Background(), p, "test-only:permission:1")
	if cmd.Name() != "eval" || p.Len() != 1 {
		t.Fatal("pipelined script must not rely on Run/EVALSHA fallback")
	}
	args := cmd.Args()
	if len(args) != 4 || args[2] != 1 || args[3] != "test-only:permission:1" {
		t.Fatal("unexpected script key shape")
	}
}
