package gwpool

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

// TestNewRingBuffer tests the creation of ring buffers
func TestNewRingBuffer(t *testing.T) {
	tests := []struct {
		name      string
		capacity  int
		wantPanic bool
	}{
		{"Valid power of 2 - 1", 1, false},
		{"Valid power of 2 - 2", 2, false},
		{"Valid power of 2 - 4", 4, false},
		{"Valid power of 2 - 8", 8, false},
		{"Valid power of 2 - 16", 16, false},
		{"Valid power of 2 - 1024", 1024, false},
		{"Invalid - zero", 0, true},
		{"Invalid - negative", -1, true},
		{"Invalid - not power of 2", 3, true},
		{"Invalid - not power of 2", 5, true},
		{"Invalid - not power of 2", 6, true},
		{"Invalid - not power of 2", 7, true},
		{"Invalid - not power of 2", 9, true},
		{"Invalid - not power of 2", 100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("NewRingBuffer() should have panicked for capacity %d", tt.capacity)
					}
				}()
			}

			rb := NewRingBuffer[int](tt.capacity)
			if !tt.wantPanic {
				if rb == nil {
					t.Errorf("NewRingBuffer() returned nil")
					return
				}
				if len(rb.buf) != tt.capacity {
					t.Errorf("NewRingBuffer() buffer length = %d, want %d", len(rb.buf), tt.capacity)
				}
				if rb.capMask != uint64(tt.capacity-1) {
					t.Errorf("NewRingBuffer() capMask = %d, want %d", rb.capMask, tt.capacity-1)
				}
				if rb.head != 0 {
					t.Errorf("NewRingBuffer() head = %d, want 0", rb.head)
				}
				if rb.tail != 0 {
					t.Errorf("NewRingBuffer() tail = %d, want 0", rb.tail)
				}
			}
		})
	}
}

// TestRingBufferBasicOperations tests basic enqueue and dequeue operations
func TestRingBufferBasicOperations(t *testing.T) {
	rb := NewRingBuffer[int](4)

	// Test initial state
	if rb.Len() != 0 {
		t.Errorf("Initial length should be 0, got %d", rb.Len())
	}

	// Test dequeue from empty buffer
	val, ok := rb.Dequeue()
	if ok {
		t.Errorf("Dequeue from empty buffer should return false")
	}
	if val != 0 {
		t.Errorf("Dequeue from empty buffer should return zero value, got %d", val)
	}

	// Test single enqueue/dequeue
	if !rb.Enqueue(42) {
		t.Errorf("Enqueue should succeed on empty buffer")
	}
	if rb.Len() != 1 {
		t.Errorf("Length should be 1 after one enqueue, got %d", rb.Len())
	}

	val, ok = rb.Dequeue()
	if !ok {
		t.Errorf("Dequeue should succeed after enqueue")
	}
	if val != 42 {
		t.Errorf("Dequeued value should be 42, got %d", val)
	}
	if rb.Len() != 0 {
		t.Errorf("Length should be 0 after dequeue, got %d", rb.Len())
	}
}

// TestRingBufferFIFO tests that the ring buffer maintains FIFO order
func TestRingBufferFIFO(t *testing.T) {
	rb := NewRingBuffer[int](8)

	// Enqueue several items
	items := []int{1, 2, 3, 4, 5}
	for _, item := range items {
		if !rb.Enqueue(item) {
			t.Errorf("Enqueue should succeed for item %d", item)
		}
	}

	if rb.Len() != len(items) {
		t.Errorf("Length should be %d, got %d", len(items), rb.Len())
	}

	// Dequeue and verify order
	for i, expected := range items {
		val, ok := rb.Dequeue()
		if !ok {
			t.Errorf("Dequeue should succeed for item %d", i)
		}
		if val != expected {
			t.Errorf("Expected %d, got %d at position %d", expected, val, i)
		}
	}

	if rb.Len() != 0 {
		t.Errorf("Length should be 0 after all dequeues, got %d", rb.Len())
	}
}

// TestRingBufferFullCapacity tests behavior when buffer reaches full capacity
func TestRingBufferFullCapacity(t *testing.T) {
	rb := NewRingBuffer[int](4)

	// Fill the buffer (capacity is 4, but can only store 3 items due to full/empty distinction)
	for i := 0; i < 3; i++ {
		if !rb.Enqueue(i) {
			t.Errorf("Enqueue should succeed for item %d", i)
		}
	}

	// Buffer should now be full - next enqueue should fail
	if rb.Enqueue(100) {
		t.Errorf("Enqueue should fail when buffer is full")
	}

	// Dequeue one item and try again
	val, ok := rb.Dequeue()
	if !ok {
		t.Errorf("Dequeue should succeed")
	}
	if val != 0 {
		t.Errorf("First dequeued value should be 0, got %d", val)
	}

	// Now enqueue should succeed again
	if !rb.Enqueue(100) {
		t.Errorf("Enqueue should succeed after dequeue")
	}
}

// TestRingBufferWrapAround tests the wrap-around behavior
func TestRingBufferWrapAround(t *testing.T) {
	rb := NewRingBuffer[int](4)

	// Fill, empty, and fill again to test wrap-around
	for cycle := 0; cycle < 3; cycle++ {
		// Fill the buffer
		for i := 0; i < 3; i++ {
			value := cycle*10 + i
			if !rb.Enqueue(value) {
				t.Errorf("Enqueue should succeed for value %d", value)
			}
		}

		// Empty the buffer
		for i := 0; i < 3; i++ {
			expected := cycle*10 + i
			val, ok := rb.Dequeue()
			if !ok {
				t.Errorf("Dequeue should succeed for cycle %d, item %d", cycle, i)
			}
			if val != expected {
				t.Errorf("Expected %d, got %d in cycle %d", expected, val, cycle)
			}
		}
	}
}

// TestRingBufferStringType tests the ring buffer with string type
func TestRingBufferStringType(t *testing.T) {
	rb := NewRingBuffer[string](8)

	items := []string{"hello", "world", "foo", "bar"}

	// Enqueue strings
	for _, item := range items {
		if !rb.Enqueue(item) {
			t.Errorf("Enqueue should succeed for string %s", item)
		}
	}

	// Dequeue and verify
	for _, expected := range items {
		val, ok := rb.Dequeue()
		if !ok {
			t.Errorf("Dequeue should succeed")
		}
		if val != expected {
			t.Errorf("Expected %s, got %s", expected, val)
		}
	}
}

// TestRingBufferStructType tests the ring buffer with struct type
func TestRingBufferStructType(t *testing.T) {
	type TestStruct struct {
		ID   int
		Name string
	}

	rb := NewRingBuffer[TestStruct](4)

	items := []TestStruct{
		{ID: 1, Name: "Alice"},
		{ID: 2, Name: "Bob"},
		{ID: 3, Name: "Charlie"},
	}

	// Enqueue structs
	for _, item := range items {
		if !rb.Enqueue(item) {
			t.Errorf("Enqueue should succeed for struct %+v", item)
		}
	}

	// Dequeue and verify
	for _, expected := range items {
		val, ok := rb.Dequeue()
		if !ok {
			t.Errorf("Dequeue should succeed")
		}
		if val != expected {
			t.Errorf("Expected %+v, got %+v", expected, val)
		}
	}
}

// TestRingBufferConcurrentProducers tests multiple goroutines producing concurrently
func TestRingBufferConcurrentProducers(t *testing.T) {
	rb := NewRingBuffer[int](1024)
	numProducers := 4
	itemsPerProducer := 200 // Reduced to fit in buffer: 4 * 200 = 800 < 1023
	var wg sync.WaitGroup

	// Start producer goroutines
	for i := 0; i < numProducers; i++ {
		wg.Add(1)
		go func(producerID int) {
			defer wg.Done()
			for j := 0; j < itemsPerProducer; j++ {
				value := producerID*itemsPerProducer + j
				for !rb.Enqueue(value) {
					runtime.Gosched() // Yield to other goroutines
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify all items were enqueued
	expectedCount := numProducers * itemsPerProducer
	actualCount := rb.Len()
	if actualCount != expectedCount {
		t.Errorf("Expected %d items, got %d", expectedCount, actualCount)
	}
}

// TestRingBufferConcurrentConsumers tests multiple goroutines consuming concurrently
func TestRingBufferConcurrentConsumers(t *testing.T) {
	rb := NewRingBuffer[int](1024)
	numItems := 800 // Reduced to fit in buffer: 800 < 1023
	numConsumers := 5

	// Fill the buffer first
	for i := 0; i < numItems; i++ {
		for !rb.Enqueue(i) {
			runtime.Gosched()
		}
	}

	var wg sync.WaitGroup
	var consumedCount int64
	consumedItems := make(map[int]bool)
	var mu sync.Mutex

	// Start consumer goroutines
	for i := 0; i < numConsumers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if val, ok := rb.Dequeue(); ok {
					atomic.AddInt64(&consumedCount, 1)
					mu.Lock()
					if consumedItems[val] {
						t.Errorf("Item %d consumed multiple times", val)
					}
					consumedItems[val] = true
					mu.Unlock()
				} else {
					// Buffer might be empty, but check if we're done
					if atomic.LoadInt64(&consumedCount) >= int64(numItems) {
						break
					}
					runtime.Gosched()
				}
			}
		}()
	}

	wg.Wait()

	// Verify all items were consumed
	if int(consumedCount) != numItems {
		t.Errorf("Expected to consume %d items, consumed %d", numItems, consumedCount)
	}

	// Verify no duplicates and all items present
	mu.Lock()
	for i := 0; i < numItems; i++ {
		if !consumedItems[i] {
			t.Errorf("Item %d was not consumed", i)
		}
	}
	mu.Unlock()
}

// TestRingBufferConcurrentProducersConsumers tests concurrent producers and consumers
func TestRingBufferConcurrentProducersConsumers(t *testing.T) {
	rb := NewRingBuffer[int](256)
	numProducers := 3
	numConsumers := 16
	itemsPerProducer := 100
	totalItems := numProducers * itemsPerProducer

	var producedCount, consumedCount int64
	var producerWg, consumerWg sync.WaitGroup

	// Start producers
	for i := 0; i < numProducers; i++ {
		producerWg.Add(1)
		go func(producerID int) {
			defer producerWg.Done()
			for j := 0; j < itemsPerProducer; j++ {
				value := producerID*itemsPerProducer + j
				for !rb.Enqueue(value) {
					runtime.Gosched()
				}
				atomic.AddInt64(&producedCount, 1)
			}
		}(i)
	}

	// Start consumers
	for i := 0; i < numConsumers; i++ {
		consumerWg.Add(1)
		go func() {
			defer consumerWg.Done()
			for atomic.LoadInt64(&consumedCount) < int64(totalItems) {
				if _, ok := rb.Dequeue(); ok {
					atomic.AddInt64(&consumedCount, 1)
				} else {
					runtime.Gosched()
				}
			}
		}()
	}

	// Wait for all producers to finish
	producerWg.Wait()

	// Wait for all items to be consumed
	for atomic.LoadInt64(&consumedCount) < int64(totalItems) {
		runtime.Gosched()
	}

	// Wait for consumers to finish
	consumerWg.Wait()

	// Verify counts
	produced := atomic.LoadInt64(&producedCount)
	consumed := atomic.LoadInt64(&consumedCount)

	t.Logf("Produced: %d, Consumed: %d, Buffer length: %d", produced, consumed, rb.Len())

	if produced != int64(totalItems) {
		t.Errorf("Expected to produce %d items, produced %d", totalItems, produced)
	}

	if consumed != int64(totalItems) {
		t.Errorf("Expected to consume %d items, consumed %d", totalItems, consumed)
	}
}

// TestRingBufferMemoryOrdering tests memory ordering guarantees
func TestRingBufferMemoryOrdering(t *testing.T) {
	rb := NewRingBuffer[int](1024)
	iterations := 500 // Reduced for faster execution

	// This test ensures that items enqueued by one goroutine
	// are properly visible to another goroutine
	var wg sync.WaitGroup
	errors := make(chan error, 2)

	// Producer
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			for !rb.Enqueue(i) {
				runtime.Gosched()
			}
		}
	}()

	// Consumer
	wg.Add(1)
	go func() {
		defer wg.Done()
		expected := 0
		for expected < iterations {
			if val, ok := rb.Dequeue(); ok {
				if val != expected {
					errors <- fmt.Errorf("expected %d, got %d", expected, val)
					return
				}
				expected++
			} else {
				runtime.Gosched()
			}
		}
	}()

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Error(err)
	}
}

// BenchmarkRingBufferEnqueue benchmarks enqueue operations
func BenchmarkRingBufferEnqueue(b *testing.B) {
	rb := NewRingBuffer[int](1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Use modulo to avoid buffer overflow in benchmark
		if !rb.Enqueue(i) {
			// If buffer is full, dequeue some items to make space
			for j := 0; j < 100 && rb.Len() > 0; j++ {
				rb.Dequeue()
			}
			rb.Enqueue(i) // Try again
		}
	}
}

// BenchmarkRingBufferDequeue benchmarks dequeue operations
func BenchmarkRingBufferDequeue(b *testing.B) {
	rb := NewRingBuffer[int](1024)

	// Pre-fill the buffer
	for i := 0; i < 500; i++ {
		rb.Enqueue(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := rb.Dequeue(); !ok {
			// If empty, refill some items
			for j := 0; j < 100; j++ {
				rb.Enqueue(j)
			}
		}
	}
}

// BenchmarkRingBufferMixed benchmarks mixed enqueue/dequeue operations
func BenchmarkRingBufferMixed(b *testing.B) {
	rb := NewRingBuffer[int](1024)

	// Pre-fill half the buffer
	for i := 0; i < 250; i++ {
		rb.Enqueue(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			if !rb.Enqueue(i) {
				// Buffer full, dequeue one
				rb.Dequeue()
				rb.Enqueue(i)
			}
		} else {
			if _, ok := rb.Dequeue(); !ok {
				// Buffer empty, enqueue one
				rb.Enqueue(i)
			}
		}
	}
}
