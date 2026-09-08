package service

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	permissionRevokeTimeout    = 2 * time.Second
	permissionRetryBatchSize   = 200
	permissionErrorDetailLimit = 20
	permissionPendingLimit     = 100000
)

var permissionRetryDelays = [...]time.Duration{
	250 * time.Millisecond,
	500 * time.Millisecond,
	time.Second,
	2 * time.Second,
	5 * time.Second,
}

// permissionRevocationQueue is a bounded best-effort cleanup queue. Security
// does not depend on this in-memory state: the persistent Redis permission
// version rejects stale snapshots across process restarts. The queue only
// accelerates removal of stale session keys.
type permissionRevocationQueue struct {
	mu               sync.RWMutex
	pending          map[int64]time.Time
	oldest           permissionPendingHeap
	order            []int64
	wake             chan struct{}
	stop             chan struct{}
	done             chan struct{}
	stopOne          sync.Once
	attemptMu        sync.Mutex
	retryAttempts    uint64
	maxBatchObserved int
	dropped          uint64
	revoke           func(context.Context, []int64) map[int64]error
}

type permissionPendingEntry struct {
	userID   int64
	queuedAt time.Time
}

type permissionPendingHeap []permissionPendingEntry

func (h permissionPendingHeap) Len() int           { return len(h) }
func (h permissionPendingHeap) Less(i, j int) bool { return h[i].queuedAt.Before(h[j].queuedAt) }
func (h permissionPendingHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *permissionPendingHeap) Push(value any) {
	*h = append(*h, value.(permissionPendingEntry))
}
func (h *permissionPendingHeap) Pop() any {
	old := *h
	last := len(old) - 1
	value := old[last]
	*h = old[:last]
	return value
}

type permissionRevocationStats struct {
	Pending          int
	RetryAttempts    uint64
	OldestWait       time.Duration
	MaxBatchObserved int
	Dropped          uint64
}

func newPermissionRevocationQueue(revoke func(context.Context, []int64) map[int64]error) *permissionRevocationQueue {
	q := &permissionRevocationQueue{
		pending: make(map[int64]time.Time),
		wake:    make(chan struct{}, 1),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
		revoke:  revoke,
	}
	go q.run()
	return q
}

var permissionRevocations = newPermissionRevocationQueue(revokeUserSessionsBatch)

func (q *permissionRevocationQueue) mark(userIDs ...int64) uint64 {
	now := time.Now()
	var dropped uint64
	q.mu.Lock()
	for _, userID := range uniquePositiveIDs(userIDs) {
		if _, exists := q.pending[userID]; exists {
			continue
		}
		if len(q.pending) >= permissionPendingLimit {
			q.dropped++
			dropped++
			continue
		}
		q.pending[userID] = now
		heap.Push(&q.oldest, permissionPendingEntry{userID: userID, queuedAt: now})
		q.order = append(q.order, userID)
	}
	q.mu.Unlock()
	return dropped
}

func (q *permissionRevocationQueue) clear(userID int64) {
	q.mu.Lock()
	delete(q.pending, userID)
	q.mu.Unlock()
}

func (q *permissionRevocationQueue) isPending(userID int64) bool {
	q.mu.RLock()
	_, pending := q.pending[userID]
	q.mu.RUnlock()
	return pending
}

func (q *permissionRevocationQueue) takeBatch(limit int) []int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	if limit <= 0 {
		return nil
	}
	batch := make([]int64, 0, min(limit, len(q.pending)))
	for len(q.order) > 0 && len(batch) < limit {
		userID := q.order[0]
		q.order = q.order[1:]
		if _, exists := q.pending[userID]; exists {
			batch = append(batch, userID)
		}
	}
	if len(batch) > q.maxBatchObserved {
		q.maxBatchObserved = len(batch)
	}
	return batch
}

func (q *permissionRevocationQueue) finishBatch(batch []int64, failures map[int64]error) {
	q.mu.Lock()
	for _, userID := range batch {
		if _, failed := failures[userID]; failed {
			if _, pending := q.pending[userID]; pending {
				q.order = append(q.order, userID)
			}
			continue
		}
		delete(q.pending, userID)
	}
	q.retryAttempts++
	q.mu.Unlock()
}

func (q *permissionRevocationQueue) stats() permissionRevocationStats {
	q.mu.Lock()
	defer q.mu.Unlock()
	stats := permissionRevocationStats{
		Pending:          len(q.pending),
		RetryAttempts:    q.retryAttempts,
		MaxBatchObserved: q.maxBatchObserved,
		Dropped:          q.dropped,
	}
	for q.oldest.Len() > 0 {
		entry := q.oldest[0]
		queuedAt, pending := q.pending[entry.userID]
		if pending && queuedAt.Equal(entry.queuedAt) {
			stats.OldestWait = time.Since(queuedAt)
			break
		}
		heap.Pop(&q.oldest)
	}
	return stats
}

func (q *permissionRevocationQueue) signal() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *permissionRevocationQueue) processOneBatch() (int, map[int64]error) {
	q.attemptMu.Lock()
	defer q.attemptMu.Unlock()
	batch := q.takeBatch(permissionRetryBatchSize)
	if len(batch) == 0 {
		return 0, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), permissionRevokeTimeout)
	failures := q.revoke(ctx, batch)
	cancel()
	if failures == nil {
		failures = map[int64]error{}
	}
	q.finishBatch(batch, failures)
	return len(batch), failures
}

// compensate performs at most one fixed-size batch synchronously. All
// remaining IDs stay queued; the returned error contains at most 20 details.
func (q *permissionRevocationQueue) compensate(userIDs ...int64) error {
	dropped := q.mark(userIDs...)
	_, failures := q.processOneBatch()
	stats := q.stats()
	if stats.Pending > 0 {
		q.signal()
	}
	if len(failures) == 0 && stats.Pending == 0 && dropped == 0 {
		return nil
	}
	return permissionRevokeSummary(failures, stats.Pending, dropped)
}

func permissionRevokeSummary(failures map[int64]error, pending int, dropped uint64) error {
	details := make([]error, 0, min(len(failures), permissionErrorDetailLimit)+1)
	for userID, err := range failures {
		if len(details) >= permissionErrorDetailLimit {
			break
		}
		details = append(details, fmt.Errorf("撤销用户 %d 会话失败: %w", userID, err))
	}
	details = append(details, fmt.Errorf("会话清理汇总: 本批失败=%d, 等待重试=%d, 因队列上限未入队=%d",
		len(failures), pending, dropped))
	return errors.Join(details...)
}

func (q *permissionRevocationQueue) run() {
	defer close(q.done)
	for {
		select {
		case <-q.wake:
		case <-q.stop:
			return
		}

		backoff := 0
		for {
			processed, failures := q.processOneBatch()
			stats := q.stats()
			if processed == 0 || stats.Pending == 0 {
				break
			}
			if len(failures) == 0 {
				backoff = 0
				continue
			}
			slog.Warn("权限会话补偿尚未完成",
				"batch", processed, "failed", len(failures), "pending", stats.Pending,
				"retryAttempts", stats.RetryAttempts, "oldestWaitMs", stats.OldestWait.Milliseconds(),
				"dropped", stats.Dropped)
			delay := permissionRetryDelays[min(backoff, len(permissionRetryDelays)-1)]
			if backoff < len(permissionRetryDelays)-1 {
				backoff++
			}
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
			case <-q.stop:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return
			}
		}
	}
}

func (q *permissionRevocationQueue) shutdown(ctx context.Context) error {
	q.stopOne.Do(func() { close(q.stop) })
	select {
	case <-q.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ShutdownPermissionRevocationQueue stops the single compensation worker.
func ShutdownPermissionRevocationQueue(ctx context.Context) error {
	return permissionRevocations.shutdown(ctx)
}
