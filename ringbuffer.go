package gwpool

import "sync/atomic"

// CacheLinePad ensures that each field resides on a separate cache line.
type CacheLinePad struct {
	_ [8]uint64 // 64 bytes padding to avoid false sharing
}

// RingBuffer represents a lock-free MPMC ring buffer with cache padding
// is a lock-free, thread-safe ring buffer implementation. It uses a fixed-size array
// to store elements and allows concurrent access without locks.
type RingBuffer[T any] struct {
	buf     []T          // The buffer to hold the elements
	capMask uint64       // The capacity of the buffer (capacity - 1)
	_       CacheLinePad // Padding to avoid false sharing
	head    uint64       // The index of the next element to be read
	_       CacheLinePad // Padding to avoid false sharing
	tail    uint64       // The index of the next element to be written
	_       CacheLinePad // Padding to avoid false sharing
}

// NewRingBuffer creates a new RingBuffer with the specified capacity.
// The capacity must be a power of two.
// If the capacity is not a power of two, it panics.
// The buffer is initialized with zero values of type T.
func NewRingBuffer[T any](capacity int) *RingBuffer[T] {
	if capacity <= 0 || (capacity&(capacity-1)) != 0 {
		panic("capacity must be a power of two")
	}
	return &RingBuffer[T]{
		buf:     make([]T, capacity),
		capMask: uint64(capacity - 1),
	}
}

// Len returns the number of elements currently in the ring buffer.
// It calculates the length by comparing the head and tail indices.
// The length is computed as the difference between tail and head,
// masked by the capacity to ensure it wraps correctly.
func (r *RingBuffer[T]) Len() int {
	head := atomic.LoadUint64(&r.head)
	tail := atomic.LoadUint64(&r.tail)
	return int((tail - head) & r.capMask)
}

// Enqueue adds an item to the ring buffer.
// It atomically checks if there is space available in the buffer.
// If the buffer is full, it returns false.
// If there is space, it increments the tail index and stores the item.
func (r *RingBuffer[T]) Enqueue(item T) bool {
	var tail uint64
	for {
		// Load the current tail index atomically
		tail = atomic.LoadUint64(&r.tail)

		// Load the current head index atomically
		head := atomic.LoadUint64(&r.head)

		// Check if the buffer is full
		// If (tail + 1) & capMask == head & capMask, the buffer is full
		if (tail+1)&r.capMask == head&r.capMask {
			// Buffer is full, cannot enqueue
			return false
		}

		// Try to automatically increment the tail index
		if atomic.CompareAndSwapUint64(&r.tail, tail, tail+1) {
			break
		}
	}

	// Store the item in the buffer at the tail index
	r.buf[tail&r.capMask] = item
	return true
}

func (r *RingBuffer[T]) Dequeue() (T, bool) {
	var head uint64
	var zero T // Zero value of type T to return when the buffer is empty
	for {
		// Load the current head index atomically
		head = atomic.LoadUint64(&r.head)

		// Load the current tail index atomically
		tail := atomic.LoadUint64(&r.tail)

		// Check if the buffer is empty
		// If head == tail, the buffer is empty
		if head == tail {
			// Buffer is empty, cannot dequeue
			return zero, false
		}

		// Try to automatically increment the head index
		if atomic.CompareAndSwapUint64(&r.head, head, head+1) {
			break
		}
	}
	value := r.buf[head&r.capMask]
	return value, true
}
