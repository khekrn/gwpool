package gwpool

import (
	"runtime"
	"sync/atomic"
	"unsafe"
)

// LockFreeRingBuffer is a high-performance lock-free MPMC (Multiple Producer Multiple Consumer) ring buffer
// implementation that avoids race conditions and deadlocks through careful atomic operations.
//
// This implementation uses a slot-based approach where each slot has its own state,
// combined with atomic head/tail pointers for coordination.
//
// Key features:
// - True lock-free MPMC operations
// - Power-of-2 capacity requirement for fast modulo operations
// - Memory ordering guarantees through atomic operations
// - No ABA problems through careful state management
type LockFreeRingBuffer[T any] struct {
	buffer []slot[T] // Buffer slots with individual state tracking
	mask   uint64    // capacity - 1 for fast modulo (capacity must be power of 2)

	// Cache-line separated atomic counters to avoid false sharing
	_    [56]byte
	head uint64 // Consumer position (next slot to read from)
	_    [56]byte
	tail uint64 // Producer position (next slot to write to)
	_    [56]byte
}

// slot represents a buffer slot with atomic state management
type slot[T any] struct {
	// Use atomic operations on these fields
	sequence uint64         // Sequence number for coordination
	data     unsafe.Pointer // Pointer to the actual data (*T)
}

// NewLockFreeRingBuffer creates a new lock-free MPMC ring buffer with the specified capacity.
// The capacity must be a power of two for optimal performance.
func NewLockFreeRingBuffer[T any](capacity int) *LockFreeRingBuffer[T] {
	if capacity <= 0 || (capacity&(capacity-1)) != 0 {
		panic("capacity must be a power of two")
	}

	buffer := make([]slot[T], capacity)

	// Initialize each slot's sequence number
	for i := 0; i < capacity; i++ {
		atomic.StoreUint64(&buffer[i].sequence, uint64(i))
	}

	return &LockFreeRingBuffer[T]{
		buffer: buffer,
		mask:   uint64(capacity - 1),
		head:   0,
		tail:   0,
	}
}

// Enqueue adds an item to the ring buffer using lock-free operations.
// Returns true if the item was successfully added, false if the buffer is full.
func (r *LockFreeRingBuffer[T]) Enqueue(item T) bool {
	const maxRetries = 10000 // Prevent infinite loops but allow reasonable retries

	for retries := 0; retries < maxRetries; retries++ {
		// Load current tail position
		tail := atomic.LoadUint64(&r.tail)
		slot := &r.buffer[tail&r.mask]

		// Load the sequence for this slot
		seq := atomic.LoadUint64(&slot.sequence)

		// Check if this slot is ready for writing
		if seq == tail {
			// Try to advance the tail
			if atomic.CompareAndSwapUint64(&r.tail, tail, tail+1) {
				// Successfully claimed this slot, write the data
				data := new(T)
				*data = item
				atomic.StorePointer(&slot.data, unsafe.Pointer(data))

				// Update sequence to indicate data is ready for consumption
				atomic.StoreUint64(&slot.sequence, tail+1)
				return true
			}
			// CAS failed, someone else took this slot, try again
		} else if seq < tail {
			// This slot is still being used by a consumer, buffer might be full
			// Check if we've wrapped around
			head := atomic.LoadUint64(&r.head)
			if tail >= head+uint64(len(r.buffer)) {
				return false // Buffer is full
			}
			// Small backoff to avoid busy waiting
			if retries > 1000 {
				runtime.Gosched() // Yield to other goroutines
			}
		} else {
			// seq > tail, this shouldn't happen in normal operation
			// Some other producer has advanced beyond us, try again
			continue
		}
	}
	return false // Failed after max retries
}

// Dequeue removes an item from the ring buffer using lock-free operations.
// Returns the item and true if successful, or zero value and false if buffer is empty.
func (r *LockFreeRingBuffer[T]) Dequeue() (T, bool) {
	var zero T
	const maxRetries = 10000 // Prevent infinite loops but allow reasonable retries

	for retries := 0; retries < maxRetries; retries++ {
		// Load current head position
		head := atomic.LoadUint64(&r.head)
		slot := &r.buffer[head&r.mask]

		// Load the sequence for this slot
		seq := atomic.LoadUint64(&slot.sequence)

		// Check if this slot has data ready for consumption
		if seq == head+1 {
			// Try to advance the head
			if atomic.CompareAndSwapUint64(&r.head, head, head+1) {
				// Successfully claimed this slot, read the data
				dataPtr := atomic.LoadPointer(&slot.data)
				if dataPtr == nil {
					// This shouldn't happen, but handle gracefully
					continue
				}

				data := *(*T)(dataPtr)

				// Clear the slot data for GC
				atomic.StorePointer(&slot.data, nil)

				// Update sequence to indicate slot is ready for next write
				// The sequence should be head + capacity for the next round
				atomic.StoreUint64(&slot.sequence, head+uint64(len(r.buffer)))

				return data, true
			}
			// CAS failed, someone else took this slot, try again
		} else if seq < head+1 {
			// No data available yet, buffer is empty
			tail := atomic.LoadUint64(&r.tail)
			if head >= tail {
				return zero, false // Buffer is definitely empty
			}
			// Small backoff to avoid busy waiting
			if retries > 1000 {
				runtime.Gosched() // Yield to other goroutines
			}
		} else {
			// seq > head+1, some other consumer has advanced beyond us, try again
			continue
		}
	}
	return zero, false // Failed after max retries
}

// Len returns the approximate number of elements currently in the ring buffer.
// Note: In a lock-free environment, this is a snapshot and may not be perfectly accurate
// due to concurrent operations, but it provides a reasonable estimate.
func (r *LockFreeRingBuffer[T]) Len() int {
	tail := atomic.LoadUint64(&r.tail)
	head := atomic.LoadUint64(&r.head)

	if tail >= head {
		return int(tail - head)
	}
	return 0
}
