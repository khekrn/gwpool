package gwpool

import (
	"context"
	"runtime"
	"sync/atomic"
	"time"
)

// RingBufferWorkerPool - I/O optimized ringBufferWorker pool using ring buffer for main queue only
// Optimized struct alignment for better cache performance
type RingBufferWorkerPool struct {
	// Hot path fields (frequently accessed together) - First cache line
	runningTasks uint64              // 8 bytes - atomic access, most frequent
	taskQueue    *RingBuffer[Task]   // 8 bytes - hot path for enqueue/dequeue
	maxWorkers   int                 // 8 bytes - used in dispatcher hot path
	workers      []*ringBufferWorker // 24 bytes (slice header) - hot path access

	// Cold path fields (less frequently accessed) - Second cache line
	ctx    context.Context    // 16 bytes - only checked on shutdown
	cancel context.CancelFunc // 8 bytes - only called once
	_      [8]byte            // 8 bytes - padding to separate hot/cold
}

func NewRingBufferWorkerPool(maxWorkers int, queueSize int) *RingBufferWorkerPool {
	if maxWorkers <= 0 {
		panic("maxWorkers must be greater than 0")
	}

	// For RingBufferPool, enforce power-of-2 worker count for optimal bitwise operations
	if !isPowerOfTwo(maxWorkers) {
		maxWorkers = nearestPowerOfTwo(maxWorkers)
	}

	if queueSize <= 0 {
		queueSize = nearestPowerOfTwo(maxWorkers * 2) // Default: 2x worker count, power of 2
	} else {
		queueSize = nearestPowerOfTwo(queueSize) // Ensure power of 2
	}

	ctx, cancel := context.WithCancel(context.Background())
	pool := &RingBufferWorkerPool{
		maxWorkers: maxWorkers,                     // Always power of 2
		taskQueue:  NewRingBuffer[Task](queueSize), // Ring buffer main queue
		ctx:        ctx,
		cancel:     cancel,
	}

	// Create workers with power-of-2 count
	pool.workers = make([]*ringBufferWorker, maxWorkers)
	for i := 0; i < maxWorkers; i++ {
		worker := &ringBufferWorker{
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

// Ring buffer dispatcher - optimized with bitwise operations and better locality
func (p *RingBufferWorkerPool) dispatchRingBuffer() {
	var workerIndex int            // Round-robin worker selection (starts at 0)
	var emptyCount int             // Track consecutive empty iterations
	workerMask := p.maxWorkers - 1 // Precompute mask for bitwise operations

	for {
		select {
		case <-p.ctx.Done():
			return
		default:
			// Try to get task from ring buffer
			if task, ok := p.taskQueue.Dequeue(); ok {
				emptyCount = 0 // Reset empty counter
				// Use fast bitwise operations for worker selection (guaranteed power of 2)
				worker := p.workers[workerIndex&workerMask]

				select {
				case worker.taskChan <- task:
					// Task delivered successfully
					workerIndex++ // Will wrap around with bitwise AND
				default:
					// Worker busy, try next ones with bitwise operations
					startIndex := workerIndex
					for i := 1; i < p.maxWorkers; i++ {
						nextIndex := (startIndex + i) & workerMask // Fast bitwise modulo
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
					backoff := time.Microsecond
					for !p.taskQueue.Enqueue(task) {
						// Queue full, wait a bit and retry (never drop the task)
						if backoff < time.Millisecond {
							runtime.Gosched()
						} else {
							time.Sleep(backoff)
						}
						if backoff < time.Millisecond {
							backoff *= 2
						}
					}
				}

			taskDelivered:
			} else {
				// No tasks available - use backoff to reduce CPU spinning
				emptyCount++
				if emptyCount > 1000 {
					time.Sleep(100 * time.Microsecond)
					emptyCount = 0
				} else {
					runtime.Gosched()
				}
			}
		}
	}
}

func (p *RingBufferWorkerPool) AddTask(t Task) {
	// Blocking version - guarantees task will be added
	backoff := time.Microsecond
	maxBackoff := time.Millisecond

	for !p.taskQueue.Enqueue(t) {
		// Queue full, wait and retry with exponential backoff (never drop the task)
		if backoff < maxBackoff {
			time.Sleep(backoff)
			backoff *= 2
		} else {
			time.Sleep(maxBackoff)
		}
	}
}

func (p *RingBufferWorkerPool) TryAddTask(t Task) bool {
	// Check if pool is still active before attempting to add task
	select {
	case <-p.ctx.Done():
		return false // Pool is shutting down
	default:
		return p.taskQueue.Enqueue(t)
	}
}

func (p *RingBufferWorkerPool) Wait() {
	backoff := time.Microsecond
	maxBackoff := 10 * time.Millisecond

	for {
		runningTasks := atomic.LoadUint64(&p.runningTasks)
		queueLen := p.taskQueue.Len()

		if runningTasks == 0 && queueLen == 0 {
			// Quick check for ringBufferWorker channels using bitwise operations
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

		// Exponential backoff to reduce CPU spinning
		if backoff < maxBackoff {
			if backoff < time.Millisecond {
				runtime.Gosched()
			} else {
				time.Sleep(backoff)
			}
			backoff *= 2
		} else {
			time.Sleep(maxBackoff)
		}
	}
}

func (p *RingBufferWorkerPool) Release() {
	p.cancel()
	// Clear the main queue
	for {
		if _, ok := p.taskQueue.Dequeue(); !ok {
			break
		}
	}
	// Clear ringBufferWorker channels
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

func (p *RingBufferWorkerPool) Running() int {
	return int(atomic.LoadUint64(&p.runningTasks))
}

func (p *RingBufferWorkerPool) WorkerCount() int {
	return len(p.workers)
}

func (p *RingBufferWorkerPool) QueueSize() int {
	return p.taskQueue.Len()
}

// Optimized struct alignment: place pointer first, then smaller fields
type ringBufferWorker struct {
	taskChan chan Task             // 8 bytes
	pool     *RingBufferWorkerPool // 8 bytes
	id       int                   // 8 bytes (on 64-bit)
	_        [5]uint64             // padding to align to cache line (40 bytes + 24 bytes above = 64 bytes)
}

func (w *ringBufferWorker) start() {
	for {
		select {
		case task := <-w.taskChan:
			atomic.AddUint64(&w.pool.runningTasks, 1)
			// Execute task with proper error handling
			func() {
				task()
				atomic.AddUint64(&w.pool.runningTasks, ^uint64(0))
			}()
		case <-w.pool.ctx.Done():
			return
		}
	}
}

func nearestPowerOfTwo(n int) int {
	if n <= 0 {
		return 1
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n++
	return n
}

func isPowerOfTwo(n int) bool {
	return n > 0 && (n&(n-1)) == 0
}
