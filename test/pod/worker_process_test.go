package podtest

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hibiken/asynq"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/repository"
	"ruoyi-go/internal/service"
	"ruoyi-go/pkg/redisx"
	"strconv"
	"testing"
	"time"
)

// The child uses the same verified bootstrap as cmd/worker, with a test-only queue.
func TestPODWorkerProcessHelper(t *testing.T) {
	if os.Getenv("POD_WORKER_PROCESS_HELPER") != "1" {
		t.Skip("subprocess helper")
	}
	cfg, err := config.Load(os.Getenv("POD_TEST_CONFIG"))
	if err != nil {
		t.Fatal("helper config unavailable")
	}
	if err := config.ValidatePOD(cfg); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := repository.Init(cfg.MySQL); err != nil {
		t.Fatal("helper DB unavailable")
	}
	defer repository.Close()
	if err := repository.VerifyPODSchema(ctx, cfg.POD.InstanceID); err != nil {
		t.Fatal(err)
	}
	if err := redisx.Init(ctx, redisx.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB}); err != nil {
		t.Fatal("helper Redis unavailable")
	}
	defer redisx.Close()
	owner, err := redisx.C().Get(ctx, redisx.PODInstanceKey).Result()
	if err != nil || owner != cfg.POD.InstanceID {
		t.Fatal("helper instance mismatch")
	}
	cfg.POD.InstanceID = os.Getenv("POD_WORKER_QUEUE_INSTANCE")
	stopPath := os.Getenv("POD_WORKER_STOP_PATH")
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := os.Stat(stopPath); err == nil {
					cancel()
					return
				}
			}
		}
	}()
	if err := service.RunPODWorker(ctx, cfg); err != nil {
		t.Fatal(err)
	}
}

type podTestWorker struct {
	command  *exec.Cmd
	done     chan error
	stopPath string
}

func startPODTestWorker(t *testing.T, ctx context.Context, cfg *config.Config, instance string) *podTestWorker {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	stopPath := filepath.Join(t.TempDir(), "worker.stop")
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestPODWorkerProcessHelper$", "-test.timeout=100s")
	cmd.Env = append(os.Environ(), "POD_WORKER_PROCESS_HELPER=1", "POD_WORKER_QUEUE_INSTANCE="+instance, "POD_WORKER_STOP_PATH="+stopPath)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal("cannot start worker helper", err)
	}
	w := &podTestWorker{command: cmd, done: make(chan error, 1), stopPath: stopPath}
	go func() { w.done <- cmd.Wait() }()
	t.Cleanup(func() {
		select {
		case <-w.done:
			return
		default:
			_ = cmd.Process.Kill()
			<-w.done
		}
	})
	return w
}
func (w *podTestWorker) stop(t *testing.T, kill bool) {
	t.Helper()
	if kill {
		if err := w.command.Process.Kill(); err != nil {
			t.Fatal(err)
		}
	} else {
		if err := os.WriteFile(w.stopPath, []byte("stop"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case err := <-w.done:
		if !kill && err != nil {
			t.Fatal("worker failed to drain")
		}
		w.done <- nil
	case <-time.After(20 * time.Second):
		t.Fatal("worker did not stop")
	}
}
func enqueueRecovery(t *testing.T, ctx context.Context, cfg *config.Config, instance string, id int64) {
	t.Helper()
	c := asynq.NewClient(asynq.RedisClientOpt{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	defer c.Close()
	body, _ := json.Marshal(map[string]string{"runId": strconv.FormatInt(id, 10)})
	_, err := c.EnqueueContext(ctx, asynq.NewTask("pod:run:wake:v1", body), asynq.Queue(service.PODQueueName(instance)), asynq.TaskID(fmt.Sprintf("recovery-%d", id)), asynq.Timeout(60*time.Second), asynq.MaxRetry(0))
	if err != nil {
		t.Fatal("could not enqueue recovery hint")
	}
}
