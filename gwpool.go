package gwpool

import (
	"sync"
)

type WorkerPool interface {
	Initiate()

	TrySubmit(task func()) bool

	StopAndWait()
}

type bufferWorkerPool struct {
	wg           sync.WaitGroup
	stopChannels []chan struct{}
	queue        chan func()
	maxWorkers   int
	maxCapacity  int
}

func New(maxWorkers, maxCapacity int) WorkerPool {
	instance := &bufferWorkerPool{
		maxWorkers:   maxWorkers,
		maxCapacity:  maxCapacity,
		queue:        make(chan func(), maxCapacity),
		stopChannels: make([]chan struct{}, maxWorkers),
	}
	return instance
}

func (b *bufferWorkerPool) Initiate() {
	for i := 0; i < b.maxWorkers; i++ {
		b.startWorker(i)
	}
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
