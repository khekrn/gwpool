package gwpool

import (
	"context"
	"sync"
	"time"
)

type Option func(*workerPool)

func WithLock(lock sync.Locker) Option {
	return func(pool *workerPool) {
		pool.lock = lock
	}
}
func MinWorkers(minWorkers int) Option {
	return func(pool *workerPool) {
		pool.minWorkers = minWorkers
	}
}

func MaxWorkers(maxWorkers int) Option {
	return func(pool *workerPool) {
		pool.maxWorkers = maxWorkers
	}
}

func WithTimeOut(timeout time.Duration) Option {
	return func(pool *workerPool) {
		pool.ctx, pool.cancelFunc = context.WithTimeout(pool.ctx, timeout)
	}
}

func WithRetryCount(retryCount int) Option {
	return func(pool *workerPool) {
		pool.retryCount = retryCount
	}
}

func WithTaskQueueSize(size int) Option {
	return func(pool *workerPool) {
		pool.taskQueueSize = size
	}
}

func WithAdjustInterval(interval time.Duration) Option {
	return func(pool *workerPool) {
		pool.adjustInterval = interval
	}
}
