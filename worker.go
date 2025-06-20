package gwpool

import (
	"context"
	"sync/atomic"
	"time"
)

// worker represents a single worker in the worker pool.
// It has a channel to receive tasks and a reference to the worker pool.
type worker struct {
	// Reference to the worker pool
	pool *fixedWorkerPool
	// Channel to receive tasks
	taskChan chan Task
}

func newWorker(pool *fixedWorkerPool) *worker {
	return &worker{
		pool:     pool,
		taskChan: make(chan Task),
	}
}

func (w *worker) start() {
	go func() {
		for t := range w.taskChan {
			atomic.AddUint64(&w.pool.runningTasks, 1)
			w.executeTask(t)
			atomic.AddUint64(&w.pool.runningTasks, ^uint64(0)) //
		}
	}()
}

func (w *worker) stop() {
	close(w.taskChan) // Close the task channel to stop the worker
	for range w.taskChan {
	}
}

func (w *worker) executeTask(t Task) {
	ctx, cancel := context.WithTimeout(context.Background(), w.pool.timeout)
	defer cancel()

	for i := 0; i <= w.pool.retryCount; i++ {
		select {
		case <-ctx.Done():
			return // Task execution timed out
		default:
			err := t()
			if err == nil {
				return // Task executed successfully
			}
			if i == w.pool.retryCount {
				return
			}
			time.Sleep(time.Duration(1<<i) * time.Millisecond)
		}
	}
}
