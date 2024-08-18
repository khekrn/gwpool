package gwpool

import (
	"sync"
	"time"
)

type Option func(*workerPool)

func WithLock(lock sync.Locker) Option {
	return func(wp *workerPool) {
		wp.lock = lock
	}
}

func WithTimeout(timeout time.Duration) Option {
	return func(wp *workerPool) {
		wp.timeout = timeout
	}
}

func WithAdjustInterval(adjustInterval time.Duration) Option {
	return func(wp *workerPool) {
		wp.adjustInterval = adjustInterval
	}
}

func WithMinWorkers(minWorkers int) Option {
	return func(wp *workerPool) {
		wp.minWorkers = minWorkers
	}
}

func WithMaxWorkers(maxWorkers int) Option {
	return func(wp *workerPool) {
		wp.maxWorkers = maxWorkers
	}
}

func WithTaskQueue(size int) Option {
	return func(wp *workerPool) {
		wp.taskQueueSize = size
	}
}

func WithRetryCount(retryCount int) Option {
	return func(wp *workerPool) {
		wp.retryCount = retryCount
	}
}
