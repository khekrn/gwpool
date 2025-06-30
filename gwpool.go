package gwpool

import (
	"context"
	"runtime"
	"sync/atomic"
	"time"
)

// Task represents a function that can be executed by a worker.
// It returns an error if the task fails.
type Task func() error

type WorkerPool interface {
	AddTask(t Task)         // Blocking version - guarantees task delivery
	TryAddTask(t Task) bool // Non-blocking version - may fail if queue full
	Wait()
	Release()
	Running() int
	WorkerCount() int
	QueueSize() int
}

type fixedWorkerPool struct {
	workers       []*worker
	taskQueue     *RingBuffer[Task]
	cancel        context.CancelFunc
	ctx           context.Context
	runningTasks  uint64
	timeout       time.Duration
	queueCapacity int
	maxWorkers    int
	retryCount    int
}

func NewWorkerPool(maxWorkers int, opts ...Option) WorkerPool {
	if maxWorkers <= 0 {
		panic("maxWorkers must be greater than 0")
	}

	ctx, cancel := context.WithCancel(context.Background())
	pool := &fixedWorkerPool{
		maxWorkers: maxWorkers,
		retryCount: 0,
		timeout:    0,
		ctx:        ctx,
		cancel:     cancel,
	}

	for _, opt := range opts {
		opt(pool)
	}

	if pool.queueCapacity <= 0 {
		pool.queueCapacity = nearestPowerOfTwo(maxWorkers * 2)
	}

	pool.taskQueue = NewRingBuffer[Task](pool.queueCapacity)

	pool.workers = make([]*worker, maxWorkers)
	for i := 0; i < pool.maxWorkers; i++ {
		w := newWorker(pool)
		pool.workers[i] = w
		w.start()
	}

	go pool.dispatch()
	return pool
}

func (p *fixedWorkerPool) AddTask(t Task) {
	// Blocking version - guarantees task will be added
	for !p.taskQueue.Enqueue(t) {
		// Queue full, wait and retry (never drop the task)
		runtime.Gosched()
	}
}

func (p *fixedWorkerPool) TryAddTask(t Task) bool {
	// Try to enqueue the task to the ring buffer
	// If the queue is full, return false
	return p.taskQueue.Enqueue(t)
}

func (p *fixedWorkerPool) Wait() {
	for atomic.LoadUint64(&p.runningTasks) > 0 || p.taskQueue.Len() > 0 {
		time.Sleep(50 * time.Millisecond) // Wait for tasks to complete
	}
}

func (p *fixedWorkerPool) Release() {
	p.cancel()
	for _, w := range p.workers {
		w.stop()
	}
	p.workers = nil
	p.taskQueue = nil
	atomic.StoreUint64(&p.runningTasks, 0)
}

func (p *fixedWorkerPool) Running() int {
	return int(atomic.LoadUint64(&p.runningTasks))
}

func (p *fixedWorkerPool) WorkerCount() int {
	return len(p.workers)
}

func (p *fixedWorkerPool) QueueSize() int {
	return p.taskQueue.Len()
}

func (p *fixedWorkerPool) dispatch() {
	for {
		select {
		case <-p.ctx.Done():
			return // Exit if the context is cancelled
		default:
			task, ok := p.taskQueue.Dequeue()
			if !ok {
				time.Sleep(10 * time.Millisecond) // Sleep briefly if no task is available
				continue
			}

			delivered := false
		outerLoop:
			for _, w := range p.workers {
				select {
				case w.taskChan <- task:
					delivered = true
					break outerLoop // Task delivered to a worker
				default:
					// Worker is busy, try the next one
					continue
				}
			}
			if !delivered {
				// If no worker was available, re-enqueue the task
				p.taskQueue.Enqueue(task)
			}
		}
	}
	// for {
	// 	select {
	// 	case <-p.ctx.Done():
	// 		return
	// 	default:
	// 		task, ok := p.taskQueue.Dequeue()
	// 		if !ok {
	// 			time.Sleep(10 * time.Millisecond)
	// 			continue
	// 		}

	// 		// YOUR OPTIMIZATION: Check before send
	// 		delivered := false
	// 		for _, w := range p.workers {
	// 			if len(w.taskChan) == 0 { // Channel has space
	// 				w.taskChan <- task // Guaranteed to succeed!
	// 				delivered = true
	// 				break
	// 			}
	// 		}

	// 		if !delivered {
	// 			p.taskQueue.Enqueue(task)
	// 		}
	// 	}
	// }
}
