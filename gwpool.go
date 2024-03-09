package gwpool

import (
	"context"
	"sync"
	"time"
)

// WorkerPool represents a pool of workers
type WorkerPool interface {
	// AddTask adds a task to the pool
	AddTask(t task)

	// Wait waits for all the task to be complete
	Wait()

	// Release releases the pool and all it's workers
	Release()

	// WorkerCount returns total no of workers
	WorkerCount() int

	// QueueSize returns the size of the queue
	QueueSize() int
}

// task represents a function that will be executed by a worker
type task func(ctx context.Context) error

// workerPool represents a pool of workers
type workerPool struct {
	// Pointers and interfaces first (assume all are 8 bytes on a 64-bit system)
	lock      sync.Locker
	cond      *sync.Cond
	workers   []chan task
	taskQueue chan task
	ctx       context.Context
	// cancelFunc is used to cancel the context. It is called when Release() is called.
	cancelFunc context.CancelFunc

	// 64-bit integers (8 bytes each)
	// adjustInterval is the interval to adjust the number of workers. Default is 1 second.
	adjustInterval time.Duration
	workerCount    int
	minWorkers     int
	maxWorkers     int
	taskQueueSize  int
	retryCount     int
}

func (w *workerPool) AddTask(t task) {
	//TODO implement me
	panic("implement me")
}

func (w *workerPool) Wait() {
	//TODO implement me
	panic("implement me")
}

func (w *workerPool) Release() {
	//TODO implement me
	panic("implement me")
}

func (w *workerPool) WorkerCount() int {
	//TODO implement me
	panic("implement me")
}

func (w *workerPool) QueueSize() int {
	//TODO implement me
	panic("implement me")
}

func NewWorkerPool(minWorkers, maxWorkers, queueSize int, options ...Option) WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	pool := &workerPool{
		lock:           &sync.Mutex{},
		cond:           nil,
		workers:        make([]chan task, minWorkers),
		taskQueue:      make(chan task, queueSize),
		ctx:            ctx,
		cancelFunc:     cancel,
		adjustInterval: 1 * time.Second,
		workerCount:    minWorkers,
		minWorkers:     minWorkers,
		maxWorkers:     maxWorkers,
		taskQueueSize:  queueSize,
		retryCount:     0,
	}

	if pool.cond == nil {
		pool.cond = sync.NewCond(pool.lock)
	}

	// Applying options
	for _, opt := range options {
		opt(pool)
	}

	return pool
}
