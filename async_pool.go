package gwpool

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// AsyncWorkerPool implements a battle-tested worker pool using Go channels
// Based on proven Kotlin coroutine implementation but adapted for Go idioms
//
// Key features:
//   - Buffered channel for task queue (configurable size)
//   - Graceful start/stop with context cancellation
//   - Worker goroutines that consume from shared channel
//   - Non-blocking task submission with TryAddTask
//   - Blocking task submission with AddTask
type AsyncWorkerPool struct {
	// Core components
	taskChan chan Task          // Buffered channel for task distribution
	ctx      context.Context    // Context for cancellation
	cancel   context.CancelFunc // Cancel function for graceful shutdown

	// Worker management
	maxWorkers int            // Number of worker goroutines
	wg         sync.WaitGroup // WaitGroup to track worker completion

	// State tracking
	runningTasks uint64 // Atomic counter for running tasks
}

// NewAsyncWorkerPool creates a new channel-based worker pool
//
// Parameters:
//   - maxWorkers: number of worker goroutines to create
//   - queueSize: buffered channel capacity (0 = unbuffered)
//
// Returns a new AsyncWorkerPool instance
func NewAsyncWorkerPool(maxWorkers int, queueSize int) *AsyncWorkerPool {
	if maxWorkers <= 0 {
		panic("maxWorkers must be greater than 0")
	}

	if queueSize < 0 {
		queueSize = 0 // Default to unbuffered if negative
	}

	ctx, cancel := context.WithCancel(context.Background())

	pool := &AsyncWorkerPool{
		taskChan:   make(chan Task, queueSize),
		ctx:        ctx,
		cancel:     cancel,
		maxWorkers: maxWorkers,
	}

	// Auto-start workers like other pool implementations
	pool.Start()

	return pool
}

// Start launches the worker goroutines
// This is equivalent to the start() function in the Kotlin implementation
func (p *AsyncWorkerPool) Start() {
	// Launch worker goroutines (equivalent to repeat(maxWorkers) in Kotlin)
	for i := 0; i < p.maxWorkers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
}

// worker is the main worker goroutine function
// Equivalent to the inner launch block in Kotlin implementation
func (p *AsyncWorkerPool) worker(workerID int) {
	defer p.wg.Done()

	// Process tasks from channel until context is cancelled
	// Equivalent to "for (workflowRequestDeferred in channel)" in Kotlin
	for {
		select {
		case task, ok := <-p.taskChan:
			if !ok {
				// Channel closed, worker should exit
				return
			}

			// Increment running tasks counter
			atomic.AddUint64(&p.runningTasks, 1)

			// Execute task with error recovery (equivalent to try-catch in Kotlin)
			func() {
				defer func() {
					// Decrement running tasks counter when done
					atomic.AddUint64(&p.runningTasks, ^uint64(0))

					// Recover from panics (equivalent to catch block)
					if r := recover(); r != nil {
						// Task panicked, but we continue
						_ = r
					}
				}()

				// Execute the task
				_ = task()
			}()

		case <-p.ctx.Done():
			// Context cancelled, worker should exit
			return
		}
	}
}

// AddTask submits a task for execution (blocking)
// Equivalent to executeWorkflow but with blocking semantics
//
// This method blocks until the task is successfully queued, retrying if necessary
// Only returns if the pool is permanently shut down (context cancelled)
func (p *AsyncWorkerPool) AddTask(task Task) {
	for {
		select {
		case p.taskChan <- task:
			// Task successfully queued
			return
		case <-p.ctx.Done():
			// Pool is shutting down, cannot queue task
			return
		default:
			// Channel is full, retry after a short delay
			select {
			case <-p.ctx.Done():
				// Pool shut down while waiting
				return
			case <-time.After(100 * time.Millisecond):
				// Retry after short delay
				continue
			}
		}
	}
}

// TryAddTask attempts to submit a task for execution (non-blocking)
// Returns true if task was queued, false if queue is full
//
// This is the non-blocking equivalent of AddTask
func (p *AsyncWorkerPool) TryAddTask(task Task) bool {
	select {
	case p.taskChan <- task:
		return true // Task successfully queued
	case <-p.ctx.Done():
		return false // Pool is shutting down
	default:
		return false // Queue is full
	}
}

// Wait blocks until all queued and running tasks complete
// Similar to joining all coroutines in Kotlin
func (p *AsyncWorkerPool) Wait() {
	// Keep checking until both conditions are stable
	for {
		// Check multiple times to ensure stability
		stable := 0
		for i := 0; i < 3; i++ {
			running := atomic.LoadUint64(&p.runningTasks)
			queueLen := len(p.taskChan)

			if running == 0 && queueLen == 0 {
				stable++
			} else {
				stable = 0
				break
			}
			time.Sleep(1 * time.Millisecond)
		}

		if stable == 3 {
			// All checks passed, we're stable
			break
		}

		// Small sleep to avoid busy waiting
		select {
		case <-p.ctx.Done():
			return
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

// Stop gracefully shuts down the worker pool
// Equivalent to the stop() function in Kotlin implementation
func (p *AsyncWorkerPool) Stop() {
	// Cancel context to signal workers to stop
	p.cancel()

	// Close the task channel to prevent new tasks
	close(p.taskChan)

	// Wait for all workers to complete
	p.wg.Wait()
}

// Release is an alias for Stop for compatibility with WorkerPool interface
func (p *AsyncWorkerPool) Release() {
	p.Stop()
}

// Running returns the current number of executing tasks
func (p *AsyncWorkerPool) Running() int {
	return int(atomic.LoadUint64(&p.runningTasks))
}

// WorkerCount returns the number of worker goroutines
func (p *AsyncWorkerPool) WorkerCount() int {
	return p.maxWorkers
}

// QueueSize returns the current number of tasks in the queue
func (p *AsyncWorkerPool) QueueSize() int {
	return len(p.taskChan)
}
