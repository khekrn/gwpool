package gwpool

import (
	"context"
	"sync"
	"time"
)

// WorkerPool implementation contract for new go-routine pool.
type WorkerPool interface {
	AddTask(t task)
	TryAddTask(t task) bool
	Wait()
	Release()
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
	//go pool.adjustWorkers()
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
}

func (w *workerPool) WorkerCount() int {
	//TODO implement me
	panic("implement me")
}

func (w *workerPool) QueueSize() int {
	//TODO implement me
	panic("implement me")
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
	for {
		time.Sleep(1 * time.Second)
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
