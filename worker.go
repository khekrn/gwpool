package gwpool

// worker represents the actual worker in the pool
type worker struct {
	workerQueue chan task
}
