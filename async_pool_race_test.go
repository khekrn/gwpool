package gwpool

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAsyncWorkerPool_RaceConditions(t *testing.T) {
	pool := NewAsyncWorkerPool(4, 100)
	defer pool.Stop()

	// Test concurrent AddTask calls
	t.Run("ConcurrentAddTask", func(t *testing.T) {
		var counter int64
		var wg sync.WaitGroup

		// Start 10 goroutines, each adding 100 tasks
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					pool.AddTask(func() error {
						atomic.AddInt64(&counter, 1)
						return nil
					})
				}
			}()
		}

		wg.Wait()

		// Wait for all tasks to complete
		for pool.QueueSize() > 0 {
			time.Sleep(10 * time.Millisecond)
		}

		expected := int64(1000)
		if atomic.LoadInt64(&counter) != expected {
			t.Errorf("Expected %d tasks executed, got %d", expected, atomic.LoadInt64(&counter))
		}
	})

	// Test concurrent TryAddTask calls
	t.Run("ConcurrentTryAddTask", func(t *testing.T) {
		var counter int64
		var wg sync.WaitGroup
		var successful int64

		// Start 10 goroutines, each trying to add 100 tasks
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 100; j++ {
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

		// Use the proper Wait() method to ensure all tasks complete
		pool.Wait()

		// All successful tasks should have been executed
		successfulCount := atomic.LoadInt64(&successful)
		actualCount := atomic.LoadInt64(&counter)
		if actualCount != successfulCount {
			t.Errorf("Mismatch: successful adds=%d, executed tasks=%d",
				successfulCount, actualCount)
		}
	})

	// Test concurrent QueueSize calls
	t.Run("ConcurrentQueueSize", func(t *testing.T) {
		var wg sync.WaitGroup

		// Start workers that continuously check queue size
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					_ = pool.QueueSize()
					time.Sleep(time.Microsecond)
				}
			}()
		}

		// Start workers that add tasks
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 50; j++ {
					pool.TryAddTask(func() error {
						time.Sleep(time.Millisecond)
						return nil
					})
				}
			}()
		}

		wg.Wait()
	})
}

func TestAsyncWorkerPool_StopRace(t *testing.T) {
	// Test that Stop() can be called concurrently with AddTask/TryAddTask
	pool := NewAsyncWorkerPool(2, 50)

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

	// Stop the pool while tasks are being added
	go func() {
		time.Sleep(10 * time.Millisecond)
		pool.Stop()
	}()

	wg.Wait()
}

func TestAsyncWorkerPool_HighConcurrency(t *testing.T) {
	pool := NewAsyncWorkerPool(8, 1000)
	defer pool.Stop()

	var counter int64
	var wg sync.WaitGroup

	// High concurrency test: 100 goroutines each adding 50 tasks
	numGoroutines := 100
	tasksPerGoroutine := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < tasksPerGoroutine; j++ {
				pool.AddTask(func() error {
					// Simulate some work
					atomic.AddInt64(&counter, 1)
					for k := 0; k < 1000; k++ {
						_ = k * k
					}
					return nil
				})
			}
		}(i)
	}

	wg.Wait()

	// Wait for all tasks to complete
	pool.Wait()

	expected := int64(numGoroutines * tasksPerGoroutine)
	actual := atomic.LoadInt64(&counter)
	if actual != expected {
		t.Errorf("Expected %d tasks executed, got %d", expected, actual)
	}
}

func BenchmarkAsyncWorkerPool_AddTask(b *testing.B) {
	pool := NewAsyncWorkerPool(4, 1000)
	defer pool.Stop()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pool.AddTask(func() error {
				return nil
			})
		}
	})
}

func BenchmarkAsyncWorkerPool_TryAddTask(b *testing.B) {
	pool := NewAsyncWorkerPool(4, 1000)
	defer pool.Stop()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pool.TryAddTask(func() error {
				return nil
			})
		}
	})
}
