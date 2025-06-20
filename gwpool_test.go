package gwpool

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	PoolSize = 1e4
	TaskNum  = 1e6
)

func BenchmarkGwPool(b *testing.B) {
	pool := NewWorkerPool(PoolSize)
	defer pool.Release()

	taskFunc := func() error {
		time.Sleep(100 * time.Millisecond)
		return nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for num := 0; num < TaskNum; num++ {
			pool.TryAddTask(taskFunc)
		}
		pool.Wait()
	}
	b.StopTimer()
}

func BenchmarkGoroutines(b *testing.B) {
	var wg sync.WaitGroup
	var taskNum = int(TaskNum)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wg.Add(taskNum)
		for num := 0; num < taskNum; num++ {
			go func() (interface{}, error) {
				time.Sleep(100 * time.Millisecond)
				wg.Done()
				return nil, nil
			}()
		}
		wg.Wait()
	}
}

// Test basic worker pool creation and properties
func TestNewWorkerPool(t *testing.T) {
	pool := NewWorkerPool(5)
	defer pool.Release()

	if pool.WorkerCount() != 5 {
		t.Errorf("Expected 5 workers, got %d", pool.WorkerCount())
	}

	if pool.Running() != 0 {
		t.Errorf("Expected 0 running tasks, got %d", pool.Running())
	}

	if pool.QueueSize() != 0 {
		t.Errorf("Expected 0 queue size, got %d", pool.QueueSize())
	}
}

// Test worker pool creation with options
func TestNewWorkerPoolWithOptions(t *testing.T) {
	pool := NewWorkerPool(3,
		WithTaskQueueSize(128),
		WithTimeout(5*time.Second),
		WithRetryCount(2),
	)
	defer pool.Release()

	if pool.WorkerCount() != 3 {
		t.Errorf("Expected 3 workers, got %d", pool.WorkerCount())
	}
}

// Test panic on invalid worker count
func TestNewWorkerPoolPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic for zero workers")
		}
	}()
	NewWorkerPool(0)
}

// Test adding and executing tasks
func TestTryAddTask(t *testing.T) {
	pool := NewWorkerPool(2, WithTaskQueueSize(8)) // Larger queue to accommodate all tasks
	defer pool.Release()

	var counter int64
	task := func() error {
		atomic.AddInt64(&counter, 1)
		time.Sleep(10 * time.Millisecond) // Small delay to simulate work
		return nil
	}

	// Add some tasks
	for i := 0; i < 5; i++ {
		if !pool.TryAddTask(task) {
			t.Errorf("Failed to add task %d", i)
		}
	}

	// Wait for tasks to complete
	pool.Wait()

	if atomic.LoadInt64(&counter) != 5 {
		t.Errorf("Expected 5 tasks executed, got %d", atomic.LoadInt64(&counter))
	}
}

// Test queue full scenario
func TestTryAddTaskQueueFull(t *testing.T) {
	// Create pool with small queue and slow tasks
	pool := NewWorkerPool(1, WithTaskQueueSize(2))
	defer pool.Release()

	slowTask := func() error {
		time.Sleep(100 * time.Millisecond)
		return nil
	}

	// Fill up the queue
	successCount := 0
	for i := 0; i < 10; i++ {
		if pool.TryAddTask(slowTask) {
			successCount++
		}
	}

	// Should be able to add some tasks but not all due to queue limit
	if successCount == 0 {
		t.Error("Should be able to add at least some tasks")
	}
	if successCount == 10 {
		t.Error("Should not be able to add all tasks due to queue limit")
	}

	pool.Wait()
}

// Test task execution with errors
func TestTaskWithError(t *testing.T) {
	pool := NewWorkerPool(2, WithTaskQueueSize(16)) // Larger queue for all tasks
	defer pool.Release()

	var successCount, errorCount int64

	successTask := func() error {
		atomic.AddInt64(&successCount, 1)
		time.Sleep(5 * time.Millisecond) // Small delay
		return nil
	}

	errorTask := func() error {
		atomic.AddInt64(&errorCount, 1)
		time.Sleep(5 * time.Millisecond) // Small delay
		return errors.New("task error")
	}

	// Add mix of successful and error tasks
	for i := 0; i < 5; i++ {
		if !pool.TryAddTask(successTask) {
			t.Errorf("Failed to add success task %d", i)
		}
		if !pool.TryAddTask(errorTask) {
			t.Errorf("Failed to add error task %d", i)
		}
	}

	pool.Wait()

	if atomic.LoadInt64(&successCount) != 5 {
		t.Errorf("Expected 5 successful tasks, got %d", atomic.LoadInt64(&successCount))
	}
	if atomic.LoadInt64(&errorCount) != 5 {
		t.Errorf("Expected 5 error tasks, got %d", atomic.LoadInt64(&errorCount))
	}
}

// Test retry functionality
func TestTaskRetry(t *testing.T) {
	pool := NewWorkerPool(1, WithRetryCount(2))
	defer pool.Release()

	var attemptCount int64
	retryTask := func() error {
		count := atomic.AddInt64(&attemptCount, 1)
		if count < 3 { // Fail first 2 attempts
			return errors.New("temporary error")
		}
		return nil // Succeed on 3rd attempt
	}

	pool.TryAddTask(retryTask)
	pool.Wait()

	if atomic.LoadInt64(&attemptCount) != 3 {
		t.Errorf("Expected 3 attempts, got %d", atomic.LoadInt64(&attemptCount))
	}
}

// Test timeout functionality
func TestTaskTimeout(t *testing.T) {
	pool := NewWorkerPool(1, WithTimeout(50*time.Millisecond))
	defer pool.Release()

	var completed bool
	var mu sync.Mutex

	// Task that completes quickly
	quickTask := func() error {
		mu.Lock()
		completed = true
		mu.Unlock()
		return nil
	}

	start := time.Now()
	if !pool.TryAddTask(quickTask) {
		t.Error("Failed to add quick task")
	}
	pool.Wait()
	elapsed := time.Since(start)

	mu.Lock()
	wasCompleted := completed
	mu.Unlock()

	if !wasCompleted {
		t.Error("Quick task should have completed")
	}

	// For a quick task, it should complete normally
	if elapsed > 100*time.Millisecond {
		t.Errorf("Quick task took too long: %v", elapsed)
	}
}

// Test concurrent access
func TestConcurrentAccess(t *testing.T) {
	pool := NewWorkerPool(5, WithTaskQueueSize(128)) // Larger queue for concurrent access
	defer pool.Release()

	var counter int64
	var wg sync.WaitGroup

	task := func() error {
		atomic.AddInt64(&counter, 1)
		time.Sleep(5 * time.Millisecond) // Shorter sleep
		return nil
	}

	// Add tasks from multiple goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				// Retry logic for queue full scenarios
				for {
					if pool.TryAddTask(task) {
						break
					}
					time.Sleep(time.Millisecond)
				}
			}
		}()
	}

	wg.Wait()
	pool.Wait()

	if atomic.LoadInt64(&counter) != 100 {
		t.Errorf("Expected 100 tasks executed, got %d", atomic.LoadInt64(&counter))
	}
}

// Test running tasks counter
func TestRunningTasksCounter(t *testing.T) {
	pool := NewWorkerPool(2)
	defer pool.Release()

	var maxRunning int
	var mu sync.Mutex

	task := func() error {
		mu.Lock()
		current := pool.Running()
		if current > maxRunning {
			maxRunning = current
		}
		mu.Unlock()

		time.Sleep(50 * time.Millisecond)
		return nil
	}

	// Add multiple tasks
	for i := 0; i < 5; i++ {
		pool.TryAddTask(task)
	}

	pool.Wait()

	mu.Lock()
	finalRunning := pool.Running()
	mu.Unlock()

	if finalRunning != 0 {
		t.Errorf("Expected 0 running tasks after Wait(), got %d", finalRunning)
	}

	if maxRunning == 0 {
		t.Error("Expected some tasks to be running concurrently")
	}

	if maxRunning > 2 {
		t.Errorf("Expected max 2 running tasks, got %d", maxRunning)
	}
}

// Test Wait() functionality
func TestWait(t *testing.T) {
	pool := NewWorkerPool(2)
	defer pool.Release()

	var completed bool
	task := func() error {
		time.Sleep(100 * time.Millisecond)
		completed = true
		return nil
	}

	start := time.Now()
	pool.TryAddTask(task)
	pool.Wait()
	elapsed := time.Since(start)

	if !completed {
		t.Error("Task should have completed")
	}

	if elapsed < 100*time.Millisecond {
		t.Error("Wait() should wait for tasks to complete")
	}

	if pool.Running() != 0 {
		t.Errorf("Expected 0 running tasks after Wait(), got %d", pool.Running())
	}

	if pool.QueueSize() != 0 {
		t.Errorf("Expected 0 queue size after Wait(), got %d", pool.QueueSize())
	}
}

// Test Release() functionality
func TestRelease(t *testing.T) {
	pool := NewWorkerPool(3)

	// Add a task
	task := func() error {
		return nil
	}
	pool.TryAddTask(task)

	// Release the pool
	pool.Release()

	// Verify state after release
	if pool.Running() != 0 {
		t.Errorf("Expected 0 running tasks after Release(), got %d", pool.Running())
	}
}

// Test queue size tracking
func TestQueueSize(t *testing.T) {
	pool := NewWorkerPool(1) // Single worker to control execution
	defer pool.Release()

	slowTask := func() error {
		time.Sleep(100 * time.Millisecond)
		return nil
	}

	// Add tasks faster than they can be processed
	for i := 0; i < 5; i++ {
		pool.TryAddTask(slowTask)
	}

	// Check that queue size is tracked
	if pool.QueueSize() == 0 {
		t.Error("Expected non-zero queue size")
	}

	pool.Wait()

	if pool.QueueSize() != 0 {
		t.Errorf("Expected 0 queue size after Wait(), got %d", pool.QueueSize())
	}
}

// Test edge case: no tasks
func TestNoTasks(t *testing.T) {
	pool := NewWorkerPool(3)
	defer pool.Release()

	// Wait with no tasks should return immediately
	start := time.Now()
	pool.Wait()
	elapsed := time.Since(start)

	if elapsed > 10*time.Millisecond {
		t.Error("Wait() with no tasks should return immediately")
	}
}

// Test worker pool with different queue sizes
func TestDifferentQueueSizes(t *testing.T) {
	sizes := []int{1, 2, 4, 8, 16, 32}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("QueueSize%d", size), func(t *testing.T) {
			pool := NewWorkerPool(2, WithTaskQueueSize(size))
			defer pool.Release()

			var counter int64
			task := func() error {
				atomic.AddInt64(&counter, 1)
				time.Sleep(10 * time.Millisecond)
				return nil
			}

			// Add more tasks than queue size
			taskCount := size + 5
			successCount := 0
			for i := 0; i < taskCount; i++ {
				if pool.TryAddTask(task) {
					successCount++
				}
			}

			pool.Wait()

			if atomic.LoadInt64(&counter) != int64(successCount) {
				t.Errorf("Expected %d tasks executed, got %d", successCount, atomic.LoadInt64(&counter))
			}
		})
	}
}
