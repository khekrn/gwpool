package gwpool

import (
	"context"
	"sync"
	"time"
)

type WorkerPoolBuilder struct {
	lock           sync.Locker
	timeout        time.Duration
	adjustInterval time.Duration

	minWorkers    int
	maxWorkers    int
	taskQueueSize int
	retryCount    int
}

func NewWorkerPoolBuilder() *WorkerPoolBuilder {
	return &WorkerPoolBuilder{
		lock:           new(sync.Mutex),
		timeout:        0,
		adjustInterval: 30 * time.Second,
		minWorkers:     64,
		maxWorkers:     512,
		taskQueueSize:  1e6,
		retryCount:     0,
	}
}

func (wb *WorkerPoolBuilder) Build() WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	pool := &workerPool{
		ctx:            ctx,
		cancelFunc:     cancel,
		timeout:        wb.timeout,
		adjustInterval: wb.adjustInterval,
		lock:           wb.lock,
		maxWorkers:     wb.maxWorkers,
		minWorkers:     wb.minWorkers,
		taskQueueSize:  wb.taskQueueSize,
		retryCount:     wb.retryCount,
	}

	pool.workers = make([]*worker, pool.minWorkers)
	pool.workerStack = make([]int, pool.minWorkers)
	pool.taskQueue = make(chan task, pool.taskQueueSize)

	pool.cond = sync.NewCond(pool.lock)

	for i := 0; i < pool.minWorkers; i++ {
		workerInstance := newWorker()
		pool.workers[i] = workerInstance
		pool.workerStack[i] = i
		workerInstance.start(pool, i)
	}

	go pool.adjustWorkers()
	go pool.dispatch()

	return pool
}

func (wb *WorkerPoolBuilder) WithLock(lock sync.Locker) *WorkerPoolBuilder {
	wb.lock = lock
	return wb
}

func (wb *WorkerPoolBuilder) WithTimeout(timeout time.Duration) *WorkerPoolBuilder {
	wb.timeout = timeout
	return wb
}

func (wb *WorkerPoolBuilder) WithAdjustInterval(adjustInterval time.Duration) *WorkerPoolBuilder {
	wb.adjustInterval = adjustInterval
	return wb
}

func (wb *WorkerPoolBuilder) WithMinWorkers(minWorkers int) *WorkerPoolBuilder {
	wb.minWorkers = minWorkers
	return wb
}

func (wb *WorkerPoolBuilder) WithMaxWorkers(maxWorkers int) *WorkerPoolBuilder {
	wb.maxWorkers = maxWorkers
	return wb
}

func (wb *WorkerPoolBuilder) WithTaskQueue(size int) *WorkerPoolBuilder {
	wb.taskQueueSize = size
	return wb
}

func (wb *WorkerPoolBuilder) WithRetryCount(retryCount int) *WorkerPoolBuilder {
	wb.retryCount = retryCount
	return wb
}
