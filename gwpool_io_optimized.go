package gwpool

import (
	"context"
	"runtime"
	"sync/atomic"
)

// IOWorkerPool - Optimized worker pool specifically for I/O bound tasks
type IOWorkerPool struct {
	workers      []*ioWorker
	taskQueue    chan Task // Simple buffered channel instead of RingBuffer
	cancel       context.CancelFunc
	ctx          context.Context
	runningTasks uint64
	maxWorkers   int
}

type ioWorker struct {
	pool     *IOWorkerPool
	taskChan chan Task // Buffered channel for better throughput
	id       int
}

func NewIOWorkerPool(maxWorkers int, queueSize int) *IOWorkerPool {
	if maxWorkers <= 0 {
		panic("maxWorkers must be greater than 0")
	}

	if queueSize <= 0 {
		queueSize = maxWorkers * 2 // Default: 2x worker count
	}

	ctx, cancel := context.WithCancel(context.Background())
	pool := &IOWorkerPool{
		maxWorkers: maxWorkers,
		taskQueue:  make(chan Task, queueSize), // Buffered main queue
		ctx:        ctx,
		cancel:     cancel,
	}

	// Create workers with buffered channels
	pool.workers = make([]*ioWorker, maxWorkers)
	for i := 0; i < maxWorkers; i++ {
		worker := &ioWorker{
			pool:     pool,
			taskChan: make(chan Task, 1), // Small buffer per worker
			id:       i,
		}
		pool.workers[i] = worker
		go worker.start()
	}

	// Start optimized dispatcher
	go pool.dispatchIO()
	return pool
}

func (w *ioWorker) start() {
	for {
		select {
		case task := <-w.taskChan:
			atomic.AddUint64(&w.pool.runningTasks, 1)
			task()                                             // Execute directly - no retry logic for I/O
			atomic.AddUint64(&w.pool.runningTasks, ^uint64(0)) // Subtract 1
		case <-w.pool.ctx.Done():
			return
		}
	}
}

// I/O optimized dispatcher - no sleep, no complex retry logic
func (p *IOWorkerPool) dispatchIO() {
	workerIndex := 0 // Round-robin worker selection

	for {
		select {
		case task := <-p.taskQueue:
			// Round-robin to avoid worker scanning overhead
			worker := p.workers[workerIndex]

			select {
			case worker.taskChan <- task:
				// Task delivered successfully
			default:
				// Worker busy, try next one
				for i := 0; i < p.maxWorkers; i++ {
					nextWorker := p.workers[(workerIndex+i+1)%p.maxWorkers]
					select {
					case nextWorker.taskChan <- task:
						workerIndex = (workerIndex + i + 1) % p.maxWorkers
						goto taskDelivered
					default:
						continue
					}
				}
				// All workers busy, must put task back - keep trying until successful
				for {
					select {
					case p.taskQueue <- task:
						goto taskDelivered
					default:
						// Queue full, wait and retry (never drop the task)
						runtime.Gosched()
					}
				}
			}

		taskDelivered:
			workerIndex = (workerIndex + 1) % p.maxWorkers

		case <-p.ctx.Done():
			return
		}
	}
}

func (p *IOWorkerPool) AddTask(t Task) {
	// Blocking version - guarantees task will be added
	for {
		select {
		case p.taskQueue <- t:
			return
		default:
			// Queue full, wait and retry (never drop the task)
			runtime.Gosched()
		}
	}
}

func (p *IOWorkerPool) TryAddTask(t Task) bool {
	select {
	case p.taskQueue <- t:
		return true
	default:
		return false // Queue full
	}
}

func (p *IOWorkerPool) Wait() {
	for atomic.LoadUint64(&p.runningTasks) > 0 || len(p.taskQueue) > 0 {
		runtime.Gosched() // Yield instead of sleep
	}
}

func (p *IOWorkerPool) Release() {
	p.cancel()
	close(p.taskQueue)
	atomic.StoreUint64(&p.runningTasks, 0)
}

func (p *IOWorkerPool) Running() int {
	return int(atomic.LoadUint64(&p.runningTasks))
}

func (p *IOWorkerPool) WorkerCount() int {
	return len(p.workers)
}

func (p *IOWorkerPool) QueueSize() int {
	return len(p.taskQueue)
}
