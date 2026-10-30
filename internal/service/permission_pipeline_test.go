package service

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/redis/go-redis/v9"
)

// A cold script cache only reports NOSCRIPT after the pipeline executes. No Redis instance is touched.
type coldPermissionHook struct{ versions map[string]int64 }

func (h *coldPermissionHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(context.Context, string, string) (net.Conn, error) { return nil, errors.New("test must not dial") }
}
func (h *coldPermissionHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(context.Context, redis.Cmder) error { return errors.New("unexpected non-pipeline command") }
}
func (h *coldPermissionHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, commands []redis.Cmder) error {
		for _, command := range commands {
			if command.Name() == "evalsha" {
				err := errors.New("NOSCRIPT No matching script. Please use EVAL.")
				command.SetErr(err)
				return err
			}
			if command.Name() != "eval" {
				return errors.New("unexpected pipeline command")
			}
			key := command.Args()[3].(string)
			h.versions[key]++
			command.(*redis.Cmd).SetVal(h.versions[key])
		}
		return nil
	}
}
func TestPermissionVersionPipelineWorksWithColdScriptCache(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	defer client.Close()
	client.AddHook(&coldPermissionHook{versions: map[string]int64{}})
	first, err := advancePermissionVersionsClient(context.Background(), client, []int64{10, 11, 10})
	if err != nil {
		t.Fatal(err)
	}
	second, err := advancePermissionVersionsClient(context.Background(), client, []int64{10, 11})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || first[10] != 1 || first[11] != 1 || second[10] != 2 || second[11] != 2 {
		t.Fatal("cold-cache version advancement lost or duplicated changes")
	}
}
