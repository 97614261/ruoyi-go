package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/hibiken/asynq"
	"ruoyi-go/internal/config"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/executor"
	"ruoyi-go/pkg/redisx"
)

const podTaskType = "pod:run:wake:v1"
const PODNodeLease = 30 * time.Second

func PODQueueName(instance string) string { return "pod-" + instance }
func podQueueOptions(cfg *config.Config) asynq.RedisClientOpt {
	return asynq.RedisClientOpt{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB, DialTimeout: 3 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second, PoolSize: 8}
}

// DispatchPODRuns is bounded; delayed or lost enqueues remain recoverable in MySQL.
func DispatchPODRuns(ctx context.Context, cfg *config.Config) (int, error) {
	rows, err := repository.DuePODOutbox(ctx, 5*time.Second)
	if err != nil {
		return 0, err
	}
	client := asynq.NewClient(podQueueOptions(cfg))
	defer client.Close()
	inspector := asynq.NewInspector(podQueueOptions(cfg))
	defer inspector.Close()
	for i, r := range rows {
		payload, _ := json.Marshal(struct {
			RunID string `json:"runId"`
		}{strconv.FormatInt(r.RunID, 10)})
		enqueueCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		taskID := fmt.Sprintf("run-%d", r.RunID)
		_, err := client.EnqueueContext(enqueueCtx, asynq.NewTask(podTaskType, payload), asynq.Queue(PODQueueName(cfg.POD.InstanceID)), asynq.TaskID(taskID), asynq.Timeout(420*time.Second), asynq.MaxRetry(3))
		if errors.Is(err, asynq.ErrTaskIDConflict) {
			info, lookupErr := inspector.GetTaskInfo(PODQueueName(cfg.POD.InstanceID), taskID)
			if lookupErr == nil && (info.State == asynq.TaskStateArchived || info.State == asynq.TaskStateCompleted) {
				_ = inspector.DeleteTask(PODQueueName(cfg.POD.InstanceID), taskID)
			}
		}
		cancel()
		if err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) {
			return i, fmt.Errorf("POD 队列暂不可用，任务已保留待重新投递: %w", err)
		}
	}
	return len(rows), nil
}

func ExecutePODStep(ctx context.Context, id int64, limits model.PODExecutionLimits, provider executor.Provider, images ...*PODImageExecutor) (bool, error) {
	return executePODStep(ctx, id, limits, 60*time.Second, func(callCtx context.Context, claim *repository.PODClaim) executor.Result {
		if podImageNode(claim.Definition.Type) && len(images) > 0 {
			return images[0].Execute(callCtx, claim)
		}
		request := executor.Request{Key: claim.Node.SubmissionKey, Scenario: claim.Run.TestScenario, Attempt: claim.Node.Attempt, Node: claim.Definition, Input: claim.Input}
		if claim.Run.ExecutionMode == "qwen" || claim.Run.ExecutionMode == "managed" {
			return qwenPending("providerUnavailable", "真实调用不能使用测试执行器；等待恢复千问模式")
		}
		if claim.Reconcile {
			return provider.Query(callCtx, request)
		}
		return provider.Submit(callCtx, request)
	})
}

func ExecutePODQwenStep(ctx context.Context, id int64, limits model.PODExecutionLimits, provider *PODQwenExecutor) (bool, error) {
	return executePODStep(ctx, id, limits, 300*time.Second, func(callCtx context.Context, claim *repository.PODClaim) executor.Result {
		if podImageNode(claim.Definition.Type) && provider != nil {
			return (&PODImageExecutor{Assets: provider.Assets, AIURL: limits.ImageAIURL}).Execute(callCtx, claim)
		}
		if claim.Run.ExecutionMode == "qwen" || claim.Run.ExecutionMode == "managed" {
			return provider.Execute(callCtx, claim)
		}
		request := executor.Request{Key: claim.Node.SubmissionKey, Scenario: claim.Run.TestScenario, Attempt: claim.Node.Attempt, Node: claim.Definition, Input: claim.Input}
		if claim.Reconcile {
			return executor.Simulation{}.Query(callCtx, request)
		}
		return executor.Simulation{}.Submit(callCtx, request)
	})
}

func executePODStep(ctx context.Context, id int64, limits model.PODExecutionLimits, timeout time.Duration, execute func(context.Context, *repository.PODClaim) executor.Result) (bool, error) {
	claimCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	claim, err := repository.ClaimPODNode(claimCtx, id, limits, PODNodeLease)
	cancel()
	if err != nil || claim == nil {
		return false, err
	}
	callCtx, cancelCall := context.WithTimeout(ctx, timeout)
	defer cancelCall()
	heartbeatDone := make(chan struct{})
	stopHeartbeat := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(PODNodeLease / 3)
		defer ticker.Stop()
		for {
			select {
			case <-stopHeartbeat:
				return
			case <-callCtx.Done():
				return
			case <-ticker.C:
				beatCtx, cancel := context.WithTimeout(callCtx, 3*time.Second)
				err := repository.HeartbeatPODNode(beatCtx, claim, PODNodeLease)
				cancel()
				if err != nil {
					cancelCall()
					return
				}
			}
		}
	}()
	result := execute(callCtx, claim)
	close(stopHeartbeat)
	<-heartbeatDone
	// Completion uses a new bounded context: an interrupted call still needs its uncertain state recorded.
	finishCtx, cancelFinish := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelFinish()
	err = repository.FinishPODNode(finishCtx, claim, result.State, result.Output, result.Code, result.Message)
	if errors.Is(err, repository.ErrPODLease) {
		return false, nil
	}
	return result.State != "awaitingReconciliation", err
}

type podQueueLogger struct{}

func (podQueueLogger) Debug(...interface{}) {}
func (podQueueLogger) Info(...interface{})  {}
func (podQueueLogger) Warn(...interface{}) {
	slog.Warn("POD Worker 队列暂时异常，保留数据库任务")
}
func (podQueueLogger) Error(...interface{}) {
	slog.Error("POD Worker 队列操作失败，保留数据库任务")
}
func (podQueueLogger) Fatal(...interface{}) { slog.Error("POD Worker 队列停止") }

func RunPODWorker(ctx context.Context, cfg *config.Config) error {
	limits := PODExecutionLimits(cfg.POD.Execution)
	if limits.Mode != "simulation" && limits.Mode != "qwen" && limits.Mode != "managed" {
		return errors.New("Worker 需要显式配置 pod.execution.mode: simulation、qwen 或 managed")
	}
	var qwen *PODQwenExecutor
	if limits.Mode == "qwen" || limits.Mode == "managed" {
		var err error
		qwen, err = NewPODQwenExecutor(cfg)
		if err != nil {
			return err
		}
		defer qwen.Images.CloseIdleConnections()
	}
	server := asynq.NewServer(podQueueOptions(cfg), asynq.Config{Concurrency: limits.GlobalConcurrency, Queues: map[string]int{PODQueueName(cfg.POD.InstanceID): 1}, ShutdownTimeout: 15 * time.Second, BaseContext: func() context.Context { return context.WithoutCancel(ctx) }, Logger: podQueueLogger{}})
	mux := asynq.NewServeMux()
	mux.HandleFunc(podTaskType, func(taskCtx context.Context, t *asynq.Task) error {
		var body struct {
			RunID string `json:"runId"`
		}
		if json.Unmarshal(t.Payload(), &body) != nil {
			return asynq.SkipRetry
		}
		id, err := PODPositiveID(body.RunID)
		if err != nil {
			return asynq.SkipRetry
		}
		for step := 0; step < 100; step++ {
			var more bool
			var err error
			if limits.Mode == "qwen" || limits.Mode == "managed" {
				more, err = ExecutePODQwenStep(taskCtx, id, limits, qwen)
			} else {
				more, err = ExecutePODStep(taskCtx, id, limits, executor.Simulation{}, &PODImageExecutor{Assets: NewPODAssetService(cfg.POD.StorageRoot, model.PODAssetLimits{MaxUploadBytes: cfg.POD.MaxUploadBytes, MaxPixels: cfg.POD.MaxImagePixels}), AIURL: limits.ImageAIURL})
			}
			if err != nil {
				return errors.New("POD 任务暂未完成，等待重投")
			}
			if !more {
				return nil
			}
			if ctx.Err() != nil {
				return nil
			}
		}
		return nil
	})
	if err := server.Start(mux); err != nil {
		return errors.New("POD Worker 启动失败")
	}
	defer server.Shutdown()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	slog.Info("POD Worker 已启动", "mode", limits.Mode, "queue", PODQueueName(cfg.POD.InstanceID))
	for {
		tickCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, batchErr := repository.DispatchPODBatch(tickCtx, limits, model.PODGraphLimits{MaxNodes: cfg.POD.MaxNodes, MaxEdges: cfg.POD.MaxEdges, MaxGraphBytes: cfg.POD.MaxGraphBytes})
		if batchErr != nil && ctx.Err() == nil {
			slog.Warn("POD 批量输入保留，稍后派发")
		}
		_, err := DispatchPODRuns(tickCtx, cfg)
		if err != nil && ctx.Err() == nil {
			slog.Warn("POD 待投递任务保留，稍后重试")
		}
		_ = redisx.C().Set(tickCtx, "pod:worker:heartbeat:"+cfg.POD.InstanceID, time.Now().Unix(), 10*time.Second).Err()
		cancel()
		select {
		case <-ctx.Done():
			server.Stop()
			return nil
		case <-ticker.C:
		}
	}
}
