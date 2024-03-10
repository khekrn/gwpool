package gwpool

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type WorkerPool interface {
	AddTask(t task)
	Wait()
	Release()
	WorkerCount() int
	QueueSize() int
}

type task func(ctx context.Context) error

type workerPool struct {
	lock           sync.Mutex
	cond           *sync.Cond
	taskQueue      chan task
	ctx            context.Context
	cancelFunc     context.CancelFunc
	adjustInterval time.Duration
	workerCount    int32
	minWorkers     int
	maxWorkers     int
	activeTasks    int32
}

func NewWorkerPool(minWorkers, maxWorkers, queueSize int) WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	pool := &workerPool{
		taskQueue:      make(chan task, queueSize),
		ctx:            ctx,
		cancelFunc:     cancel,
		adjustInterval: 1 * time.Second,
		minWorkers:     minWorkers,
		maxWorkers:     maxWorkers,
	}
	pool.cond = sync.NewCond(&pool.lock)

	for i := 0; i < minWorkers; i++ {
		pool.addWorker()
	}

	go pool.adjustWorkers()

	return pool
}

func (w *workerPool) addWorker() {
	atomic.AddInt32(&w.workerCount, 1)
	go func() {
		defer atomic.AddInt32(&w.workerCount, -1)
		for {
			select {
			case task, ok := <-w.taskQueue:
				if !ok {
					return
				}
				atomic.AddInt32(&w.activeTasks, 1)
				if err := task(w.ctx); err != nil {
					// Log error or handle failed task scenario
				}
				atomic.AddInt32(&w.activeTasks, -1)
				w.cond.Signal()
			case <-w.ctx.Done():
				return
			}
		}
	}()
}

func (w *workerPool) AddTask(t task) {
	w.taskQueue <- t
}

func (w *workerPool) Wait() {
	w.lock.Lock()
	defer w.lock.Unlock()
	for atomic.LoadInt32(&w.activeTasks) > 0 || len(w.taskQueue) > 0 {
		w.cond.Wait()
	}
}

func (w *workerPool) Release() {
	w.cancelFunc()
	close(w.taskQueue)
}

func (w *workerPool) WorkerCount() int {
	return int(atomic.LoadInt32(&w.workerCount))
}

func (w *workerPool) QueueSize() int {
	return len(w.taskQueue)
}

func (w *workerPool) adjustWorkers() {
	ticker := time.NewTicker(w.adjustInterval)
	defer ticker.Stop()

	for range ticker.C {
		if w.ctx.Err() != nil {
			return
		}

		currentWorkers := w.WorkerCount()
		queueLen := w.QueueSize()

		if queueLen > 0 && currentWorkers < w.maxWorkers {
			w.addWorker()
		}
	}
}
