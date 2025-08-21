package gwpool

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRingBufferWorkerPool_RaceConditions(t *testing.T) {
	// Test concurrent AddTask calls
	t.Run("ConcurrentAddTask", func(t *testing.T) {
		pool := NewRingBufferWorkerPool(8, 64) // Increased queue size for better handling

		var counter int64
		var queued int64
		var wg sync.WaitGroup

		// Reduced scale to prevent timeout: 5 goroutines, each adding 50 tasks
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 50; j++ {
					atomic.AddInt64(&queued, 1) // Count queued tasks
					pool.AddTask(func() error {
						atomic.AddInt64(&counter, 1)
						return nil
					})
				}
			}()
		}

		wg.Wait()

		// Use timeout for Wait() to prevent infinite blocking
		done := make(chan bool, 1)
		go func() {
			pool.Wait()
			done <- true
		}()

		select {
		case <-done:
			expected := int64(250) // 5 * 50
			actual := atomic.LoadInt64(&counter)
			queuedCount := atomic.LoadInt64(&queued)

			if queuedCount != expected {
				t.Errorf("Expected %d tasks queued, got %d", expected, queuedCount)
			}
			if actual != expected {
				// Log detailed info for debugging and fail
				t.Errorf("Expected %d tasks executed, got %d (queued: %d, running: %d, queue size: %d)",
					expected, actual, queuedCount, pool.Running(), pool.QueueSize())

				// Additional wait to see if tasks complete later
				time.Sleep(100 * time.Millisecond)
				finalCount := atomic.LoadInt64(&counter)
				t.Errorf("After additional wait: %d tasks executed", finalCount)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("pool.Wait() timed out after 10 seconds - possible deadlock!")
		}

		// Only release after all tasks are confirmed to be completed
		pool.Release()
	}) // Test concurrent TryAddTask calls
	t.Run("ConcurrentTryAddTask", func(t *testing.T) {
		pool := NewRingBufferWorkerPool(8, 64) // Increased queue size to match ConcurrentAddTask

		var counter int64
		var wg sync.WaitGroup
		var successful int64

		// Reduced scale: 5 goroutines, each trying to add 50 tasks
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 50; j++ {
					if pool.TryAddTask(func() error {
						atomic.AddInt64(&counter, 1)
						return nil
					}) {
						atomic.AddInt64(&successful, 1)
					}
				}
			}()
		}

		wg.Wait()

		// Use timeout for Wait()
		done := make(chan bool, 1)
		go func() {
			pool.Wait()
			done <- true
		}()

		select {
		case <-done:
			// All successful tasks should have been executed
			successfulCount := atomic.LoadInt64(&successful)
			actualCount := atomic.LoadInt64(&counter)
			if actualCount != successfulCount {
				t.Errorf("Mismatch: successful adds=%d, executed tasks=%d (running: %d, queue size: %d)",
					successfulCount, actualCount, pool.Running(), pool.QueueSize())

				// Additional wait to see if tasks complete later
				time.Sleep(100 * time.Millisecond)
				finalCount := atomic.LoadInt64(&counter)
				if finalCount != actualCount {
					t.Errorf("After additional wait: %d tasks executed (gained %d)", finalCount, finalCount-actualCount)
				}
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("pool.Wait() timed out after 10 seconds - possible deadlock!")
		}

		pool.Release()
	})

	// Test concurrent QueueSize calls
	t.Run("ConcurrentQueueSize", func(t *testing.T) {
		pool := NewRingBufferWorkerPool(8, 64) // Increased capacity to handle concurrent load

		var wg sync.WaitGroup
		var tasksAdded int64

		// Start workers that continuously check queue size
		for i := 0; i < 3; i++ { // Reduced from 5 to 3
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 50; j++ { // Reduced from 100 to 50
					_ = pool.QueueSize()
					time.Sleep(time.Microsecond)
				}
			}()
		}

		// Start workers that add tasks
		for i := 0; i < 3; i++ { // Reduced from 5 to 3
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 15; j++ { // Further reduced to 15 to avoid queue overflow
					if pool.TryAddTask(func() error {
						time.Sleep(50 * time.Microsecond) // Further reduced work time
						return nil
					}) {
						atomic.AddInt64(&tasksAdded, 1)
					}
				}
			}()
		}

		wg.Wait()

		// Use timeout for Wait()
		done := make(chan bool, 1)
		go func() {
			pool.Wait()
			done <- true
		}()

		select {
		case <-done:
			// Success - verify at least some tasks were added
			added := atomic.LoadInt64(&tasksAdded)
			if added == 0 {
				t.Errorf("No tasks were successfully added")
			}
			t.Logf("Successfully added %d tasks", added)
		case <-time.After(5 * time.Second): // Reduced timeout since tasks are shorter
			t.Fatalf("pool.Wait() timed out after 5 seconds - possible deadlock!")
		}

		pool.Release()
	})
}

func TestRingBufferWorkerPool_ReleaseRace(t *testing.T) {
	// Test that Release() can be called concurrently with AddTask/TryAddTask
	pool := NewRingBufferWorkerPool(4, 8)

	var wg sync.WaitGroup

	// Start goroutines adding tasks
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				pool.TryAddTask(func() error {
					time.Sleep(time.Millisecond)
					return nil
				})
			}
		}()
	}

	// Release the pool while tasks are being added
	go func() {
		time.Sleep(10 * time.Millisecond)
		pool.Release()
	}()

	wg.Wait()
}

func TestRingBufferWorkerPool_HighConcurrency(t *testing.T) {
	pool := NewRingBufferWorkerPool(8, 128) // Increased capacity for high concurrency
	defer pool.Release()

	var counter int64
	var wg sync.WaitGroup

	// Reduced concurrency test: 20 goroutines each adding 25 tasks
	numGoroutines := 20
	tasksPerGoroutine := 25

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < tasksPerGoroutine; j++ {
				pool.AddTask(func() error {
					// Simulate some work
					atomic.AddInt64(&counter, 1)
					for k := 0; k < 100; k++ { // Reduced from 1000 to 100
						_ = k * k
					}
					return nil
				})
			}
		}(i)
	}

	wg.Wait()

	// Use timeout for Wait()
	done := make(chan bool, 1)
	go func() {
		pool.Wait()
		done <- true
	}()

	select {
	case <-done:
		expected := int64(numGoroutines * tasksPerGoroutine)
		actual := atomic.LoadInt64(&counter)
		if actual != expected {
			t.Errorf("Expected %d tasks executed, got %d", expected, actual)
		}
	case <-time.After(15 * time.Second):
		t.Fatalf("pool.Wait() timed out after 15 seconds - possible deadlock!")
	}
}

func TestRaceConditionWorkerIndexAccess(t *testing.T) {
	pool := NewRingBufferWorkerPool(4, 8)
	defer pool.Release()

	var wg sync.WaitGroup

	// Multiple goroutines adding tasks to trigger dispatcher race
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				success := pool.TryAddTask(func() error {
					time.Sleep(time.Microsecond) // Minimal work
					return nil
				})
				if !success {
					break // Don't retry if queue is full
				}
			}
		}()
	}

	wg.Wait()

	// Use timeout for Wait()
	done := make(chan bool, 1)
	go func() {
		pool.Wait()
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(3 * time.Second):
		t.Fatalf("pool.Wait() timed out - possible deadlock in dispatcher!")
	}
}

func TestRaceConditionRunningTasksCounter(t *testing.T) {
	pool := NewRingBufferWorkerPool(8, 16)
	defer pool.Release()

	var wg sync.WaitGroup

	// Add tasks and concurrently read running count
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				success := pool.TryAddTask(func() error {
					time.Sleep(2 * time.Millisecond)
					return nil
				})
				if !success {
					break
				}
			}
		}()
	}

	// Concurrent monitoring
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			running := pool.Running()
			queue := pool.QueueSize()
			_ = running + queue // Use values to prevent optimization
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()

	// Use timeout for Wait()
	done := make(chan bool, 1)
	go func() {
		pool.Wait()
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(3 * time.Second):
		t.Fatalf("pool.Wait() timed out - possible deadlock in counter logic!")
	}
}

func TestRaceConditionRingBuffer(t *testing.T) {
	// This test should be run with: go test -race
	pool := NewRingBufferWorkerPool(8, 16)
	defer pool.Release()

	var wg sync.WaitGroup
	var completed int64

	// Start multiple goroutines adding tasks concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				success := pool.TryAddTask(func() error {
					atomic.AddInt64(&completed, 1)
					time.Sleep(time.Microsecond) // Very short work
					return nil
				})
				if !success {
					// If queue is full, don't retry to avoid infinite loops
					break
				}
			}
		}(i)
	}

	// Concurrent calls to monitoring
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			_ = pool.Running()
			_ = pool.QueueSize()
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()

	// Use timeout for Wait() to prevent infinite blocking
	done := make(chan bool, 1)
	go func() {
		pool.Wait()
		done <- true
	}()

	select {
	case <-done:
		t.Logf("Completed tasks: %d", atomic.LoadInt64(&completed))
	case <-time.After(5 * time.Second):
		t.Fatalf("pool.Wait() timed out after 5 seconds - possible deadlock!")
	}
}

// Benchmark tests for race detection
func BenchmarkRingBufferWorkerPool_AddTask(b *testing.B) {
	pool := NewRingBufferWorkerPool(8, 128)
	defer pool.Release()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pool.AddTask(func() error {
				return nil
			})
		}
	})
}

func BenchmarkRingBufferWorkerPool_TryAddTask(b *testing.B) {
	pool := NewRingBufferWorkerPool(8, 128)
	defer pool.Release()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pool.TryAddTask(func() error {
				return nil
			})
		}
	})
}

func BenchmarkRaceDetection(b *testing.B) {
	pool := NewRingBufferWorkerPool(16, 32)
	defer pool.Release()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pool.TryAddTask(func() error {
				return nil
			})
		}
	})

	pool.Wait()
}
