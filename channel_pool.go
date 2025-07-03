package gwpool

import (
	"context"
	"runtime"
	"sync/atomic"
	"time"
)

// ChannelWorkerPool implements a simple worker pool
//
// Technical features:
//   - Native Go channels for task distribution
//   - Flexible worker counts (any positive integer)
//   - Lower memory overhead than RingBufferPool
//   - Simple, reliable architecture
//
// Performance characteristics:
//   - Excellent for I/O-bound workloads (HTTP, database, file operations)
//   - Lower latency for mixed workloads
//   - Easier debugging and profiling
//
// Best choice for most applications unless maximum performance is required.
type ChannelWorkerPool struct {
	workers      []*channelWorker
	taskQueue    chan Task // Simple buffered channel instead of RingBuffer
	cancel       context.CancelFunc
	ctx          context.Context
	runningTasks uint64
	maxWorkers   int
}

// NewChannelWorkerPool creates a channel-based worker pool.
//
// Accepts any positive worker count (no power-of-2 requirement).
//
// Parameters:
//   - maxWorkers: exact number of workers to create
//   - queueSize: task queue capacity
//
// Panics if maxWorkers <= 0.
func NewChannelWorkerPool(maxWorkers int, queueSize int) *ChannelWorkerPool {
	if maxWorkers <= 0 {
		panic("maxWorkers must be greater than 0")
	}

	if queueSize <= 0 {
		queueSize = maxWorkers * 2 // Default: 2x channelWorker count
	}

	ctx, cancel := context.WithCancel(context.Background())
	pool := &ChannelWorkerPool{
		maxWorkers: maxWorkers,
		taskQueue:  make(chan Task, queueSize), // Buffered main queue
		ctx:        ctx,
		cancel:     cancel,
	}

	// Create workers with buffered channels
	pool.workers = make([]*channelWorker, maxWorkers)
	for i := 0; i < maxWorkers; i++ {
		worker := &channelWorker{
			pool:     pool,
			taskChan: make(chan Task, 1), // Small buffer per channelWorker
			id:       i,
		}
		pool.workers[i] = worker
		go worker.start()
	}

	// Start optimized dispatcher
	go pool.dispatchIO()
	return pool
}

// I/O optimized dispatcher - no sleep, no complex retry logic
func (p *ChannelWorkerPool) dispatchIO() {
	workerIndex := 0 // Round-robin channelWorker selection

	for {
		select {
		case task := <-p.taskQueue:
			// Round-robin to avoid channelWorker scanning overhead
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
				backoff := time.Microsecond
				for {
					select {
					case p.taskQueue <- task:
						goto taskDelivered
					default:
						// Queue full, wait and retry (never drop the task)
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
			}

		taskDelivered:
			workerIndex = (workerIndex + 1) % p.maxWorkers

		case <-p.ctx.Done():
			return
		}
	}
}

// AddTask blocks until the task is queued, guaranteeing execution.
// Uses exponential backoff if the queue is full to avoid CPU spinning
// while ensuring the task is never dropped.
//
// Parameters:
//   - t: the task function to execute
func (p *ChannelWorkerPool) AddTask(t Task) {
	// Blocking version - guarantees task will be added
	backoff := time.Microsecond
	maxBackoff := time.Millisecond

	for {
		select {
		case p.taskQueue <- t:
			return
		default:
			// Queue full, wait and retry with exponential backoff (never drop the task)
			if backoff < maxBackoff {
				time.Sleep(backoff)
				backoff *= 2
			} else {
				time.Sleep(maxBackoff)
			}
		}
	}
}

// TryAddTask attempts to add a task without blocking.
// Returns immediately with false if the queue is full or pool is shutting down.
//
// Parameters:
//   - t: the task function to execute
//
// Returns:
//   - true if task was queued successfully
//   - false if queue is full or pool is shutting down
func (p *ChannelWorkerPool) TryAddTask(t Task) bool {
	// Check if pool is still active before attempting to add task
	select {
	case <-p.ctx.Done():
		return false // Pool is shutting down
	default:
		select {
		case p.taskQueue <- t:
			return true
		default:
			return false // Queue full
		}
	}
}

// Wait blocks until all queued and running tasks have completed.
// Checks the main queue, running task counter, and individual worker
// channels to ensure no tasks are pending.
//
// Uses exponential backoff to efficiently wait without excessive CPU usage.
func (p *ChannelWorkerPool) Wait() {
	backoff := time.Microsecond
	maxBackoff := 10 * time.Millisecond

	for {
		runningTasks := atomic.LoadUint64(&p.runningTasks)
		queueLen := len(p.taskQueue)

		if runningTasks == 0 && queueLen == 0 {
			// Check worker channels for pending tasks
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

// Release gracefully shuts down the worker pool, stopping all workers
// and clearing the task queue.
//
// After calling Release, no new tasks should be added to the pool.
// The main task queue is closed, which will cause workers to exit
// when they finish their current tasks.
//
// Should typically be called with defer after pool creation:
//
//	pool := NewChannelWorkerPool(50, 128, gwpool.ChannelPool)
//	defer pool.Release()
func (p *ChannelWorkerPool) Release() {
	p.cancel()
	close(p.taskQueue)
	atomic.StoreUint64(&p.runningTasks, 0)
}

// Running returns the current number of tasks being executed by workers.
// This count reflects tasks that have been dequeued from worker channels
// and are actively running.
//
// Returns the number of currently executing tasks.
func (p *ChannelWorkerPool) Running() int {
	return int(atomic.LoadUint64(&p.runningTasks))
}

// WorkerCount returns the total number of workers in the pool.
// This is the exact number specified during pool creation (no rounding).
//
// Returns the number of worker goroutines.
func (p *ChannelWorkerPool) WorkerCount() int {
	return len(p.workers)
}

// QueueSize returns the current number of tasks waiting in the main queue.
// This does not include tasks that are queued in individual worker channels.
//
// Returns the number of tasks waiting to be distributed to workers.
func (p *ChannelWorkerPool) QueueSize() int {
	return len(p.taskQueue)
}

type channelWorker struct {
	pool     *ChannelWorkerPool
	taskChan chan Task // Buffered channel for better throughput
	id       int
}

func (w *channelWorker) start() {
	for {
		select {
		case task := <-w.taskChan:
			atomic.AddUint64(&w.pool.runningTasks, 1)
			// Execute task with proper error handling
			func() {
				defer func() {
					// Subtract 1 from running tasks when done
					atomic.AddUint64(&w.pool.runningTasks, ^uint64(0))
				}()
				// Execute directly
				task()
			}()
		case <-w.pool.ctx.Done():
			return
		}
	}
}
