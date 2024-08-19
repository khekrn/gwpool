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
	workers        []*worker
	workerStack    []int
	taskQueue      chan task
	ctx            context.Context
	cancelFunc     context.CancelFunc
	timeout        time.Duration
	adjustInterval time.Duration

	lock sync.Locker
	cond *sync.Cond

	maxWorkers    int
	minWorkers    int
	taskQueueSize int
	retryCount    int
}

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

	pool.taskQueue = make(chan task, pool.taskQueueSize)
	pool.workers = make([]*worker, pool.minWorkers)
	pool.workerStack = make([]int, pool.minWorkers)

	if pool.cond == nil {
		pool.cond = sync.NewCond(pool.lock)
	}
	// Create workers with the minimum number. Don't use pushWorker() here.
	for i := 0; i < pool.minWorkers; i++ {
		worker := newWorker()
		pool.workers[i] = worker
		pool.workerStack[i] = i
		worker.start(pool, i)
	}
	go pool.adjustWorkers()
	go pool.dispatch()
	return pool
}

func (w *workerPool) AddTask(t task) {
	w.taskQueue <- t
}

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

func (w *workerPool) Release() {
	close(w.taskQueue)
	w.cancelFunc()

	w.cond.L.Lock()
	for len(w.workerStack) != w.minWorkers {
		w.cond.Wait()
	}
	w.cond.L.Unlock()

	for _, worker := range w.workers {
		close(worker.workerQueue)
	}

	w.workers = nil
	w.workerStack = nil

	// close(w.taskQueue)
	// w.cancelFunc()

	// // Wait for all tasks to be processed
	// w.Wait()

	// w.cond.L.Lock()
	// // Close all worker queues
	// for _, worker := range w.workers {
	// 	close(worker.workerQueue)
	// }
	// w.cond.L.Unlock()

	// // Wait for all workers to finish
	// w.cond.L.Lock()
	// for len(w.workerStack) != len(w.workers) {
	// 	w.cond.Wait()
	// }
	// w.cond.L.Unlock()

	// w.workers = nil
	// w.workerStack = nil
}

func (w *workerPool) Running() int {
	w.lock.Lock()
	defer w.lock.Unlock()
	return len(w.workers) - len(w.workerStack)
}

func (w *workerPool) WorkerCount() int {
	w.lock.Lock()
	defer w.lock.Unlock()
	return len(w.workers)
}

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
				newWorkers := min(len(w.workers)*2, w.maxWorkers) - len(w.workers)
				for i := 0; i < newWorkers; i++ {
					worker := newWorker()
					w.workers = append(w.workers, worker)
					w.workerStack = append(w.workerStack, len(w.workers)-1)
					worker.start(w, len(w.workers)-1)
				}
			} else if len(w.taskQueue) == 0 && len(w.workerStack) == len(w.workers) && len(w.workers) > w.minWorkers {
				adjustFlag = true
				removeWorkers := (len(w.workers) - w.minWorkers + 1) / 2
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
