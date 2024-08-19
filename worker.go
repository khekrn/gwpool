package gwpool

// worker represents the actual worker in the pool
type worker struct {
	workerQueue chan task
}

func newWorker() *worker {
	return &worker{workerQueue: make(chan task, 1)}
}

func (w *worker) start(pool *workerPool, workerIndex int) {
	go func() {
		for t := range w.workerQueue {
			if t != nil {
				w.executeTask(pool, t)
			}
			pool.pushWorker(workerIndex)
		}
	}()
}

func (w *worker) executeTask(pool *workerPool, t task) {
	for i := 0; i <= pool.retryCount; i++ {
		err := t()
		if err == nil || i == pool.retryCount {
			break
		}
	}
}
