package gwpool

import (
	"context"
	"runtime"
	"sync/atomic"
)

// IOWorkerPoolRingBuffer - I/O optimized worker pool using ring buffer for main queue only
// Optimized struct alignment: place 8-byte fields first, then smaller fields
type IOWorkerPoolRingBuffer struct {
	runningTasks uint64             // 8 bytes - atomic access, place first
	workers      []*ioWorkerRB      // 24 bytes (slice header)
	taskQueue    *RingBuffer[Task]  // 8 bytes
	cancel       context.CancelFunc // 8 bytes
	ctx          context.Context    // 16 bytes
	maxWorkers   int                // 8 bytes (on 64-bit)
	workerMask   int                // 8 bytes - for bitwise modulo (maxWorkers - 1)
}

// Optimized struct alignment: place pointer first, then smaller fields
type ioWorkerRB struct {
	pool     *IOWorkerPoolRingBuffer // 8 bytes
	taskChan chan Task               // 8 bytes
	id       int                     // 8 bytes (on 64-bit)
	_        [8]byte                 // padding to align to cache line (64 bytes)
}

func NewIOWorkerPoolRingBuffer(maxWorkers int, queueSize int) *IOWorkerPoolRingBuffer {
	if maxWorkers <= 0 {
		panic("maxWorkers must be greater than 0")
	}

	// Ensure maxWorkers is power of 2 for bitwise operations
	maxWorkers = nearestPowerOfTwo(maxWorkers)

	if queueSize <= 0 {
		queueSize = nearestPowerOfTwo(maxWorkers * 2) // Default: 2x worker count, power of 2
	} else {
		queueSize = nearestPowerOfTwo(queueSize) // Ensure power of 2
	}

	ctx, cancel := context.WithCancel(context.Background())
	pool := &IOWorkerPoolRingBuffer{
		maxWorkers: maxWorkers,
		workerMask: maxWorkers - 1,                 // For bitwise modulo: x % maxWorkers = x & workerMask
		taskQueue:  NewRingBuffer[Task](queueSize), // Ring buffer main queue
		ctx:        ctx,
		cancel:     cancel,
	}

	// Create workers with buffered channels (capacity 1)
	pool.workers = make([]*ioWorkerRB, maxWorkers)
	for i := 0; i < maxWorkers; i++ {
		worker := &ioWorkerRB{
			pool:     pool,
			taskChan: make(chan Task, 1), // Buffered channel with capacity 1
			id:       i,
		}
		pool.workers[i] = worker
		go worker.start()
	}

	// Start dispatcher
	go pool.dispatchRingBuffer()
	return pool
}

func (w *ioWorkerRB) start() {
	for {
		select {
		case task := <-w.taskChan:
			atomic.AddUint64(&w.pool.runningTasks, 1)
			task()                                             // Execute directly
			atomic.AddUint64(&w.pool.runningTasks, ^uint64(0)) // Subtract 1
		case <-w.pool.ctx.Done():
			return
		}
	}
}

// Ring buffer dispatcher - optimized with bitwise operations and better locality
func (p *IOWorkerPoolRingBuffer) dispatchRingBuffer() {
	var workerIndex int // Round-robin worker selection (starts at 0)

	for {
		select {
		case <-p.ctx.Done():
			return
		default:
			// Try to get task from ring buffer
			if task, ok := p.taskQueue.Dequeue(); ok {
				// Use bitwise AND for fast modulo (since maxWorkers is power of 2)
				worker := p.workers[workerIndex&p.workerMask]

				select {
				case worker.taskChan <- task:
					// Task delivered successfully
					workerIndex++ // Will wrap around automatically with bitwise AND
				default:
					// Worker busy, try next ones with bitwise operations
					startIndex := workerIndex
					for i := 1; i < p.maxWorkers; i++ {
						nextIndex := (startIndex + i) & p.workerMask
						nextWorker := p.workers[nextIndex]
						select {
						case nextWorker.taskChan <- task:
							workerIndex = nextIndex + 1 // Set next starting point
							goto taskDelivered
						default:
							continue
						}
					}
					// All workers busy, must put task back - keep trying until successful
					for !p.taskQueue.Enqueue(task) {
						// Queue full, wait a bit and retry (never drop the task)
						runtime.Gosched()
					}
				}

			taskDelivered:
			} else {
				// No tasks available, yield to avoid spinning
				runtime.Gosched()
			}
		}
	}
}

func (p *IOWorkerPoolRingBuffer) AddTask(t Task) {
	// Blocking version - guarantees task will be added
	for !p.taskQueue.Enqueue(t) {
		// Queue full, wait and retry (never drop the task)
		runtime.Gosched()
	}
}

func (p *IOWorkerPoolRingBuffer) TryAddTask(t Task) bool {
	return p.taskQueue.Enqueue(t)
}

func (p *IOWorkerPoolRingBuffer) Wait() {
	for {
		runningTasks := atomic.LoadUint64(&p.runningTasks)
		queueLen := p.taskQueue.Len()

		if runningTasks == 0 && queueLen == 0 {
			// Quick check for worker channels using bitwise operations
			hasWorkerTasks := false
			for i := 0; i < p.maxWorkers; i++ {
				if len(p.workers[i].taskChan) > 0 {
					hasWorkerTasks = true
					break
				}
			}

			if !hasWorkerTasks {
				break
			}
		}

		runtime.Gosched() // Yield instead of sleep
	}
}

func (p *IOWorkerPoolRingBuffer) Release() {
	p.cancel()
	// Clear the main queue
	for {
		if _, ok := p.taskQueue.Dequeue(); !ok {
			break
		}
	}
	// Clear worker channels
	for _, worker := range p.workers {
		// Drain the channel
		for {
			select {
			case <-worker.taskChan:
				// Task drained
			default:
				// Channel empty
				goto nextWorker
			}
		}
	nextWorker:
	}
	atomic.StoreUint64(&p.runningTasks, 0)
}

func (p *IOWorkerPoolRingBuffer) Running() int {
	return int(atomic.LoadUint64(&p.runningTasks))
}

func (p *IOWorkerPoolRingBuffer) WorkerCount() int {
	return len(p.workers)
}

func (p *IOWorkerPoolRingBuffer) QueueSize() int {
	return p.taskQueue.Len()
}
