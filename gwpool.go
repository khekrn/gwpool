package gwpool

// Task represents a function that can be executed by a ringBufferWorker.
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

// PoolType represents the type of worker pool to create
type PoolType int

const (
	// ChannelPool uses channels for task distribution
	ChannelPool PoolType = iota
	// RingBufferPool uses a ring buffer for task distribution
	RingBufferPool
)

// NewWorkerPool creates a new worker pool of the specified type
// maxWorkers: the maximum number of workers to create
// queueSize: the size of the task queue (if <= 0, defaults to 2*maxWorkers)
// poolType: the type of worker pool to create (ChannelPool or RingBufferPool)
func NewWorkerPool(maxWorkers int, queueSize int, poolType PoolType) WorkerPool {
	switch poolType {
	case ChannelPool:
		return NewChannelWorkerPool(maxWorkers, queueSize)
	case RingBufferPool:
		return NewRingBufferWorkerPool(maxWorkers, queueSize)
	default:
		// Default to channel-based pool
		return NewChannelWorkerPool(maxWorkers, queueSize)
	}
}
