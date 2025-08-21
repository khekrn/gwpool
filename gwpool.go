// Package gwpool provides high-performance worker pool implementations for Go applications.
//
// This package offers two optimized worker pool types:
//   - ChannelPool: Best for normal I/O-bound and tasks, flexible worker counts
//   - RingBufferPool: Best for High-throughput I/O tasks, power-of-2 optimizations
//
// Example usage:
//
//	// For I/O-bound tasks (HTTP requests, database operations)
//	pool := gwpool.NewWorkerPool(64, 128, gwpool.ChannelPool)
//	defer pool.Release()
//
//	// For CPU-intensive tasks (data processing, calculations)
//	pool := gwpool.NewWorkerPool(64, 128, gwpool.RingBufferPool)
//	defer pool.Release()
//
//	pool.AddTask(func() error {
//		// Your work here
//		return nil
//	})
//	pool.Wait()
package gwpool

// Task represents a function that can be executed by a ringBufferWorker.
// It returns an error if the task fails.
type Task func() error

// WorkerPool defines the interface for all worker pool implementations.
type WorkerPool interface {
	AddTask(t Task)         // Blocking version - guarantees task delivery
	TryAddTask(t Task) bool // Non-blocking version - may fail if queue full
	Wait()
	Release()
	Running() int
	WorkerCount() int
	QueueSize() int
}

// PoolType represents the type of worker pool to create
type PoolType int

const (
	// ChannelPool uses channels for task distribution
	ChannelPool PoolType = iota
	// RingBufferPool uses a ring buffer for task distribution
	RingBufferPool
)

// NewWorkerPool creates a new worker pool of the specified type.
//
// Parameters:
//   - maxWorkers: number of worker goroutines (RingBufferPool rounds to power-of-2)
//   - queueSize: task queue capacity (if <= 0, defaults to 2*maxWorkers)
//   - poolType: ChannelPool or RingBufferPool
//
// Returns a WorkerPool interface for the chosen implementation.
func NewWorkerPool(maxWorkers int, queueSize int, poolType PoolType) WorkerPool {
	switch poolType {
	case ChannelPool:
		return NewAsyncWorkerPool(maxWorkers, queueSize)
	case RingBufferPool:
		return NewRingBufferWorkerPool(maxWorkers, queueSize)
	default:
		// Default to channel-based pool
		return NewAsyncWorkerPool(maxWorkers, queueSize)
	}
}
