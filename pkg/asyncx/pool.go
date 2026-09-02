// Package asyncx provides small bounded executors for fire-and-forget work.
package asyncx

import (
	"context"
	"log/slog"
	"sync"
)

type Pool struct {
	queue     chan func()
	mu        sync.RWMutex
	closeOnce sync.Once
	workers   sync.WaitGroup
	done      chan struct{}
	closed    bool
}

func NewPool(workers, capacity int) *Pool {
	if workers < 1 {
		workers = 1
	}
	if capacity < 1 {
		capacity = 1
	}
	p := &Pool{
		queue: make(chan func(), capacity),
		done:  make(chan struct{}),
	}
	p.workers.Add(workers)
	for range workers {
		go p.run()
	}
	go func() {
		p.workers.Wait()
		close(p.done)
	}()
	return p
}

// Submit queues work without blocking. False means the bounded queue is full.
func (p *Pool) Submit(work func()) bool {
	if work == nil {
		return false
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return false
	}
	select {
	case p.queue <- work:
		return true
	default:
		return false
	}
}

// Shutdown stops admission and waits for accepted work to drain. A timed-out
// caller may return while workers finish in the background, but no work is lost
// merely because shutdown started.
func (p *Pool) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closed = true
		close(p.queue)
		p.mu.Unlock()
	})

	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *Pool) run() {
	defer p.workers.Done()
	for work := range p.queue {
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					slog.Error("异步任务发生 panic", "panic", recovered)
				}
			}()
			work()
		}()
	}
}
