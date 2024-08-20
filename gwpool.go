package gwpool

import (
	"context"
	"sort"
	"sync"
	"time"
)

// WorkerPool implementation contract for new go-routine pool.
type WorkerPool interface {
	AddTask(t task)
	TryAddTask(t task) bool
	Wait()
	Release()
	Running() int
	WorkerCount() int
	QueueSize() int
}

// task definition function that will be executed by a worker
type task func() error

// workerPool represents a pool of worker
type workerPool struct {
	// Each worker with a channel of size 1.
	workers []*worker
	// Available workers are defined via slice.
	workerStack []int

	// Tasks are added to this channel first and later dispatched to workers.
	taskQueue chan task
	ctx       context.Context
	// Used to cancel the context and it's ued in Release function.
	cancelFunc context.CancelFunc
	// Set by WithTimeout(), used to set a timeout for a task. Default is 0, which means no timeout.
	timeout time.Duration
	// adjustInterval is the interval to adjust the number of workers. Default is 1 second.
	adjustInterval time.Duration

	lock sync.Locker
	cond *sync.Cond

	maxWorkers int
	// Set by WithMinWorkers(), used to adjust the number of workers. Default equals to maxWorkers.
	minWorkers int
	// Set by WithTaskQueue(), used to set the size of the task queue. Default is 1e6.
	taskQueueSize int
	// Set by WithRetryCount(), used to retry a task when it fails. Default is 0.
	retryCount int
	// Set by WithDynamicScaling(), used to adjust the workers scale up and down
	autoScaling bool
}

// NewWorkerPool creates a new pool of workers
func NewWorkerPool(maxWorkers int, opts ...Option) WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	pool := &workerPool{
		workers:        nil,
		workerStack:    nil,
		taskQueue:      nil,
		ctx:            ctx,
		cancelFunc:     cancel,
		timeout:        0,
		adjustInterval: 1 * time.Second,
		lock:           new(sync.Mutex),
		maxWorkers:     maxWorkers,
		taskQueueSize:  1e6,
		retryCount:     0,
	}

	for _, opt := range opts {
		opt(pool)
	}

	totalWorkers := pool.minWorkers
	if !pool.autoScaling {
		totalWorkers = pool.maxWorkers
	}

	pool.taskQueue = make(chan task, pool.taskQueueSize)
	pool.workers = make([]*worker, totalWorkers)
	pool.workerStack = make([]int, totalWorkers)

	if pool.cond == nil {
		pool.cond = sync.NewCond(pool.lock)
	}
	// Create workers with the minimum number. Don't use pushWorker() here.
	for i := 0; i < totalWorkers; i++ {
		worker := newWorker()
		pool.workers[i] = worker
		pool.workerStack[i] = i
		worker.start(pool, i)
	}
	if pool.autoScaling {
		go pool.adjustWorkers()
	}
	go pool.dispatch()
	return pool
}

// AddTask adds a task to the pool.
func (w *workerPool) AddTask(t task) {
	w.taskQueue <- t
}

// TryAddTask adds a task to the pool only if enough workers are available.
func (w *workerPool) TryAddTask(t task) bool {
	w.lock.Lock()
	currentLength := len(w.workerStack)
	w.lock.Unlock()
	result := false
	if currentLength != 0 {
		result = true
		w.AddTask(t)
	}
	return result
}

// Wait waits for all tasks to be dispatched and completed.
func (w *workerPool) Wait() {
	for {
		w.lock.Lock()
		currentLength := len(w.workerStack)
		w.lock.Unlock()
		if len(w.taskQueue) == 0 && currentLength == len(w.workers) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Release stops all workers and releases resources.
func (w *workerPool) Release() {
	close(w.taskQueue)
	w.cancelFunc()

	// Wait for all tasks to be processed
	w.Wait()

	w.cond.L.Lock()
	// Close all worker queues
	for _, worker := range w.workers {
		close(worker.workerQueue)
	}
	w.cond.L.Unlock()

	// Wait for all workers to finish
	w.cond.L.Lock()
	for len(w.workerStack) != len(w.workers) {
		w.cond.Wait()
	}
	w.cond.L.Unlock()

	w.workers = nil
	w.workerStack = nil
}

// Running returns the number of workers that are currently working.
func (w *workerPool) Running() int {
	w.lock.Lock()
	defer w.lock.Unlock()
	return len(w.workers) - len(w.workerStack)
}

// WorkerCount returns the number of workers in the pool.
func (w *workerPool) WorkerCount() int {
	w.lock.Lock()
	defer w.lock.Unlock()
	return len(w.workers)
}

// QueueSize returns the size of the task queue.
func (w *workerPool) QueueSize() int {
	w.lock.Lock()
	defer w.lock.Unlock()
	return len(w.taskQueue)
}

func (w *workerPool) pushWorker(workerIndex int) {
	w.lock.Lock()
	w.workerStack = append(w.workerStack, workerIndex)
	w.lock.Unlock()
	w.cond.Signal()
}

func (w *workerPool) popWorker() int {
	w.lock.Lock()
	defer w.lock.Unlock()
	workerIndex := w.workerStack[len(w.workerStack)-1]
	w.workerStack = w.workerStack[:len(w.workerStack)-1]
	return workerIndex
}

// adjustWorkers adjusts the number of workers according to the number of tasks in the queue.
func (w *workerPool) adjustWorkers() {
	ticker := time.NewTicker(w.adjustInterval)
	defer ticker.Stop()

	var adjustFlag bool
	for {
		adjustFlag = false
		select {
		case <-ticker.C:
			w.cond.L.Lock()
			if len(w.taskQueue) > len(w.workers)*3/4 && len(w.workers) < w.maxWorkers {
				adjustFlag = true
				// Double the number of workers until it reaches the maximum
				newWorkers := min(len(w.workers)*2, w.maxWorkers) - len(w.workers)
				for i := 0; i < newWorkers; i++ {
					worker := newWorker()
					w.workers = append(w.workers, worker)
					w.workerStack = append(w.workerStack, len(w.workers)-1)
					worker.start(w, len(w.workers)-1)
				}
			} else if len(w.taskQueue) == 0 && len(w.workerStack) == len(w.workers) && len(w.workers) > w.minWorkers {
				adjustFlag = true
				// Halve the number of workers until it reaches the minimum
				removeWorkers := (len(w.workers) - w.minWorkers + 1) / 2
				// Sort the workerStack before removing workers.
				sort.Ints(w.workerStack)
				w.workers = w.workers[:len(w.workers)-removeWorkers]
				w.workerStack = w.workerStack[:len(w.workerStack)-removeWorkers]
			}
			w.cond.L.Unlock()
			if adjustFlag {
				w.cond.Broadcast()
			}
		case <-w.ctx.Done():
			return
		}
	}
}

// dispatch dispatches tasks to workers.
func (w *workerPool) dispatch() {
	for t := range w.taskQueue {
		w.cond.L.Lock()
		for len(w.workerStack) == 0 {
			w.cond.Wait()
		}
		w.cond.L.Unlock()
		workerIndex := w.popWorker()
		w.workers[workerIndex].workerQueue <- t
	}
}
