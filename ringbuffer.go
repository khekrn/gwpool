package gwpool

import (
	"sync"
)

// MutexRingBuffer is a thread-safe ring buffer using mutex for simplicity and correctness
type MutexRingBuffer[T any] struct {
	buf     []T          // The buffer to hold the elements
	capMask uint64       // The capacity of the buffer (capacity - 1)
	mu      sync.RWMutex // Mutex for thread safety
	head    uint64       // The index of the next element to be read
	tail    uint64       // The index of the next element to be written
}

// NewMutexRingBuffer creates a new MutexRingBuffer with the specified capacity.
// The capacity must be a power of two.
func NewMutexRingBuffer[T any](capacity int) *MutexRingBuffer[T] {
	if capacity <= 0 || (capacity&(capacity-1)) != 0 {
		panic("capacity must be a power of two")
	}
	return &MutexRingBuffer[T]{
		buf:     make([]T, capacity),
		capMask: uint64(capacity - 1),
	}
}

// Len returns the number of elements currently in the ring buffer.
func (r *MutexRingBuffer[T]) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return int((r.tail - r.head) & r.capMask)
}

// Enqueue adds an item to the ring buffer.
func (r *MutexRingBuffer[T]) Enqueue(item T) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if buffer is full
	if (r.tail+1)&r.capMask == r.head&r.capMask {
		return false
	}

	// Store the item and advance tail
	r.buf[r.tail&r.capMask] = item
	r.tail++
	return true
}

// Dequeue removes an item from the ring buffer.
func (r *MutexRingBuffer[T]) Dequeue() (T, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var zero T

	// Check if buffer is empty
	if r.head == r.tail {
		return zero, false
	}

	// Get the item and advance head
	value := r.buf[r.head&r.capMask]
	r.buf[r.head&r.capMask] = zero // Clear for GC
	r.head++
	return value, true
}
