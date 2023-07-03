package gwpool

import (
	"sync"
)

type WorkerPool interface {
	TrySubmit(task func()) bool

	StopAndWait()
}

type bufferWorkerPool struct {
	maxWorkers   int
	maxCapacity  int
	queue        chan func()
	wg           sync.WaitGroup
	stopChannels []chan struct{}
}

// StopAndWait implements WorkerPool.
func (b *bufferWorkerPool) StopAndWait() {
	for _, stop := range b.stopChannels {
		close(stop)
	}
	b.wg.Wait()
	close(b.queue)
}

// TrySubmit implements WorkerPool.
func (b *bufferWorkerPool) TrySubmit(task func()) bool {
	if len(b.queue) >= b.maxCapacity {
		return false
	}

	b.queue <- task
	return true
}

func (b *bufferWorkerPool) startWorker(id int) {
	b.wg.Add(1)
	stop := make(chan struct{})
	b.stopChannels[id] = stop

	go func() {
		defer b.wg.Done()
		for {
			select {
			case function, ok := <-b.queue:
				if !ok {
					return
				}
				function()
			case <-stop:
				return
			}
		}
	}()

}

func New(maxWorkers, maxCapacity int) WorkerPool {
	instance := &bufferWorkerPool{
		maxWorkers:   maxWorkers,
		maxCapacity:  maxCapacity,
		queue:        make(chan func(), maxCapacity),
		stopChannels: make([]chan struct{}, maxWorkers),
	}

	for i := 1; i <= maxWorkers; i++ {
		instance.startWorker(i)
	}

	return instance
}
