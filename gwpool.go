package gwpool

import "sync"

type WPool interface {
	Initiate()
	TrySubmit(task func()) bool
	StopAndWait()
}

type bufferWorkerPool struct {
	wg          sync.WaitGroup
	queue       chan func()
	maxWorkers  int
	maxCapacity int
}

func NewPool(maxWorkers, maxCapacity int) WPool {
	return &bufferWorkerPool{
		maxWorkers:  maxWorkers,
		maxCapacity: maxCapacity,
		queue:       make(chan func(), maxCapacity),
	}
}

func (b *bufferWorkerPool) Initiate() {
	for i := 0; i < b.maxWorkers; i++ {
		b.startWorker()
	}
}

func (b *bufferWorkerPool) startWorker() {
	b.wg.Add(1)

	go func() {
		defer b.wg.Done()
		for {
			select {
			case function, ok := <-b.queue:
				if !ok {
					return
				}
				function()
			default:
				return
			}
		}
	}()
}

func (b *bufferWorkerPool) StopAndWait() {
	close(b.queue)
	b.wg.Wait()
}

func (b *bufferWorkerPool) TrySubmit(task func()) bool {
	select {
	case b.queue <- task:
		return true
	default:
		return false
	}
}
