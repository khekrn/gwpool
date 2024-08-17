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

// task definition
type task func()

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

func (w *workerPool) AddTask(t task) {
	//TODO implement me
	panic("implement me")
}

func (w *workerPool) TryAddTask(t task) bool {
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

func (w *workerPool) adjustWorkers() {

}

func (w *workerPool) dispatch() {

}
