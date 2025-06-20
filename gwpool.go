package gwpool

import (
	"context"
	"sync/atomic"
	"time"
)

// Task represents a function that can be executed by a worker.
// It returns an error if the task fails.
type Task func() error

type WorkerPool interface {
	TryAddTask(t Task) bool
	Wait()
	Release()
	Running() int
	WorkerCount() int
	QueueSize() int
}

type fixedWorkerPool struct {
	workers       []*worker
	taskQueue     *RingBuffer[Task]
	cancelFunc    context.CancelFunc
	ctx           context.Context
	runningTasks  uint64
	timeout       time.Duration
	queueCapacity int
	maxWorkers    int
	retryCount    int
}

// worker represents a single worker in the worker pool.
// It has a channel to receive tasks and a reference to the worker pool.
type worker struct {
	// Reference to the worker pool
	pool *fixedWorkerPool
	// Channel to receive tasks
	taskChan chan Task
}

func newWorker(pool *fixedWorkerPool) *worker {
	w := &worker{
		pool:     pool,
		taskChan: make(chan Task),
	}

	return w
}

func (w *worker) start() {}

func NewWorkerPool(maxWorkers int, opts ...Option) WorkerPool {
	if maxWorkers <= 0 {
		panic("maxWorkers must be greater than 0")
	}

	ctx, cancel := context.WithCancel(context.Background())
	pool := &fixedWorkerPool{
		maxWorkers: maxWorkers,
		retryCount: 0,
		timeout:    0,
		ctx:        ctx,
		cancelFunc: cancel,
	}

	for _, opt := range opts {
		opt(pool)
	}

	if pool.queueCapacity <= 0 {
		pool.queueCapacity = nearestPowerOfTwo(maxWorkers * 2)
	}

	pool.taskQueue = NewRingBuffer[Task](pool.queueCapacity)

	pool.workers = make([]*worker, maxWorkers)
	for i := 0; i < pool.maxWorkers; i++ {
		w := newWorker(pool)
		pool.workers[i] = w
		w.start()
	}

	go pool.dispatch()
	return pool
}

func (p *fixedWorkerPool) TryAddTask(t Task) bool {
	return false
}

func (p *fixedWorkerPool) Wait() {

}

func (p *fixedWorkerPool) Release() {

}

func (p *fixedWorkerPool) Running() int {
	return int(atomic.LoadUint64(&p.runningTasks))
}

func (p *fixedWorkerPool) WorkerCount() int {
	return len(p.workers)
}

func (p *fixedWorkerPool) QueueSize() int {
	return p.taskQueue.Len()
}

func (p *fixedWorkerPool) dispatch() {}
