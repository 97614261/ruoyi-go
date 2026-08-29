// Package asyncx provides small bounded executors for fire-and-forget work.
package asyncx

import "log/slog"

type Pool struct {
	queue chan func()
}

func NewPool(workers, capacity int) *Pool {
	if workers < 1 {
		workers = 1
	}
	if capacity < 1 {
		capacity = 1
	}
	p := &Pool{queue: make(chan func(), capacity)}
	for range workers {
		go p.run()
	}
	return p
}

// Submit queues work without blocking. False means the bounded queue is full.
func (p *Pool) Submit(work func()) bool {
	if work == nil {
		return false
	}
	select {
	case p.queue <- work:
		return true
	default:
		return false
	}
}

func (p *Pool) run() {
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
