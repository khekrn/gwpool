package gwpool

// worker represents the actual worker in the pool
type worker struct {
	workerQueue chan task
}

func newWorker() *worker {
	return &worker{workerQueue: make(chan task, 1)}
}

func (w worker) start(pool *workerPool, workerIndex int) {
	//TODO implement me
	panic("implement me")
}
