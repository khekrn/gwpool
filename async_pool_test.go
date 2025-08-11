package gwpool

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestAsyncWorkerPoolBasic tests basic functionality
func TestAsyncWorkerPoolBasic(t *testing.T) {
	pool := NewAsyncWorkerPool(4, 10)
	pool.Start()
	defer pool.Stop()

	var counter int64
	var wg sync.WaitGroup

	// Add some tasks
	for i := 0; i < 10; i++ {
		wg.Add(1)
		pool.AddTask(func() error {
			atomic.AddInt64(&counter, 1)
			wg.Done()
			return nil
		})
	}

	wg.Wait()

	if atomic.LoadInt64(&counter) != 10 {
		t.Errorf("Expected 10 tasks executed, got %d", atomic.LoadInt64(&counter))
	}
}

// TestAsyncWorkerPoolTryAddTask tests non-blocking task submission
func TestAsyncWorkerPoolTryAddTask(t *testing.T) {
	// Create pool with small buffer to test queue full scenario
	pool := NewAsyncWorkerPool(2, 2)
	pool.Start()
	defer pool.Stop()

	var executed int64

	// Add slow tasks to fill the queue
	for i := 0; i < 4; i++ {
		success := pool.TryAddTask(func() error {
			time.Sleep(100 * time.Millisecond)
			atomic.AddInt64(&executed, 1)
			return nil
		})
		if !success && i < 2 {
			t.Errorf("Should be able to add task %d", i)
		}
	}

	// Try to add one more task - should fail due to full queue
	extraTaskAdded := pool.TryAddTask(func() error {
		atomic.AddInt64(&executed, 1)
		return nil
	})

	// Wait for tasks to complete
	pool.Wait()

	t.Logf("Executed %d tasks, extra task added: %v", atomic.LoadInt64(&executed), extraTaskAdded)
}

// TestAsyncWorkerPoolConcurrentAccess tests concurrent access
func TestAsyncWorkerPoolConcurrentAccess(t *testing.T) {
	pool := NewAsyncWorkerPool(8, 50)
	pool.Start()
	defer pool.Stop()

	var counter int64
	var wg sync.WaitGroup

	// Multiple goroutines adding tasks concurrently
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				pool.AddTask(func() error {
					atomic.AddInt64(&counter, 1)
					time.Sleep(time.Millisecond) // Small delay
					return nil
				})
			}
		}()
	}

	wg.Wait()
	pool.Wait()

	expected := int64(5 * 20)
	if atomic.LoadInt64(&counter) != expected {
		t.Errorf("Expected %d tasks executed, got %d", expected, atomic.LoadInt64(&counter))
	}
}

// TestAsyncWorkerPoolGracefulShutdown tests graceful shutdown
func TestAsyncWorkerPoolGracefulShutdown(t *testing.T) {
	pool := NewAsyncWorkerPool(4, 20)
	pool.Start()

	var started, completed int64

	// Add some tasks
	for i := 0; i < 10; i++ {
		pool.AddTask(func() error {
			atomic.AddInt64(&started, 1)
			time.Sleep(50 * time.Millisecond)
			atomic.AddInt64(&completed, 1)
			return nil
		})
	}

	// Wait a bit for tasks to start
	time.Sleep(20 * time.Millisecond)

	// Stop the pool
	pool.Stop()

	// All started tasks should complete
	startedCount := atomic.LoadInt64(&started)
	completedCount := atomic.LoadInt64(&completed)

	t.Logf("Started: %d, Completed: %d", startedCount, completedCount)

	if completedCount != startedCount {
		t.Errorf("All started tasks should complete. Started: %d, Completed: %d", startedCount, completedCount)
	}
}

// TestAsyncWorkerPoolRaceConditions tests for race conditions
func TestAsyncWorkerPoolRaceConditions(t *testing.T) {
	pool := NewAsyncWorkerPool(10, 100)
	pool.Start()
	defer pool.Stop()

	var counter int64
	var wg sync.WaitGroup

	// Concurrent task submission and monitoring
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				pool.TryAddTask(func() error {
					atomic.AddInt64(&counter, 1)
					return nil
				})
			}
		}()
	}

	// Concurrent monitoring
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_ = pool.Running()
			_ = pool.QueueSize()
			_ = pool.WorkerCount()
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()
	pool.Wait()

	t.Logf("Completed %d tasks", atomic.LoadInt64(&counter))
}

// TestAsyncWorkerPoolErrorHandling tests error handling
func TestAsyncWorkerPoolErrorHandling(t *testing.T) {
	pool := NewAsyncWorkerPool(4, 10)
	defer pool.Stop()

	var successCount, errorCount int64

	// Add mix of successful and error tasks
	for i := 0; i < 10; i++ {
		if i%2 == 0 {
			pool.AddTask(func() error {
				atomic.AddInt64(&successCount, 1)
				return nil
			})
		} else {
			pool.AddTask(func() error {
				atomic.AddInt64(&errorCount, 1)
				return &TaskError{"Test error"}
			})
		}
	}

	pool.Wait()

	// Add small delay to ensure all tasks are fully processed
	time.Sleep(10 * time.Millisecond)

	actualSuccess := atomic.LoadInt64(&successCount)
	actualError := atomic.LoadInt64(&errorCount)

	if actualSuccess != 5 {
		t.Errorf("Expected 5 successful tasks, got %d", actualSuccess)
	}
	if actualError != 5 {
		t.Errorf("Expected 5 error tasks, got %d", actualError)
	}
}

// TaskError is a simple error type for testing
type TaskError struct {
	Message string
}

func (e *TaskError) Error() string {
	return e.Message
}
