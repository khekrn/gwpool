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
type task func() error

type workerPool struct {
	workers        []*worker
	workerStack    []int
	taskQueue      chan task
	ctx            context.Context
	cancelFunc     context.CancelFunc
	timeout        time.Duration
	adjustInterval time.Duration

	maxWorkers    int
	minWorkers    int
	taskQueueSize int
	retryCount    int

	lock sync.Locker
	cond *sync.Cond
}

func NewWorkerPool(minWorkers, maxWorkers, queueSize int) WorkerPool {

}
