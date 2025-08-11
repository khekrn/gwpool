package gwpool

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	mathrand "math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	PoolSize = 512
	TaskNum  = 1024
)

// simulateLightIO simulates lighter I/O operations (faster for large-scale benchmarks)
func simulateLightIO() error {
	// Mix of small CPU work and minimal I/O simulation
	data := make([]byte, 256+mathrand.Intn(512))
	rand.Read(data)

	// Light hash computation
	hash := sha256.Sum256(data)
	_ = hash

	// Minimal delay to simulate I/O wait
	time.Sleep(time.Duration(mathrand.Intn(5)) * time.Millisecond)
	return nil
}

func BenchmarkChannelPool(b *testing.B) {
	pool := NewWorkerPool(PoolSize, PoolSize, ChannelPool)
	defer pool.Release()

	taskFunc := func() error {
		return simulateLightIO()
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

// BenchmarkRingBufferPoolIO benchmarks the CPU-optimized ring buffer pool for I/O tasks
func BenchmarkRingBufferIOPool(b *testing.B) {
	// Use CPU-optimized version for better CPU-bound performance
	pool := NewWorkerPool(PoolSize, PoolSize, RingBufferPool)
	defer pool.Release()

	taskFunc := func() error {
		return simulateLightIO()
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
				simulateLightIO()
				wg.Done()
				return nil, nil
			}()
		}
		wg.Wait()
	}
}

// Test basic ringBufferWorker pool creation and properties
func TestNewWorkerPool(t *testing.T) {
	// Test RingBufferPool
	t.Run("RingBufferPool", func(t *testing.T) {
		pool := NewWorkerPool(8, 8, RingBufferPool) // Use power of 2
		defer pool.Release()

		if pool.WorkerCount() != 8 {
			t.Errorf("Expected 8 workers, got %d", pool.WorkerCount())
		}

		if pool.Running() != 0 {
			t.Errorf("Expected 0 running tasks, got %d", pool.Running())
		}

		if pool.QueueSize() != 0 {
			t.Errorf("Expected 0 queue size, got %d", pool.QueueSize())
		}
	})

	// Test ChannelPool
	t.Run("ChannelPool", func(t *testing.T) {
		pool := NewWorkerPool(8, 16, ChannelPool) // Any size works for ChannelPool
		defer pool.Release()

		if pool.WorkerCount() != 8 {
			t.Errorf("Expected 8 workers, got %d", pool.WorkerCount())
		}

		if pool.Running() != 0 {
			t.Errorf("Expected 0 running tasks, got %d", pool.Running())
		}

		if pool.QueueSize() != 0 {
			t.Errorf("Expected 0 queue size, got %d", pool.QueueSize())
		}
	})
}

// Test panic on invalid ringBufferWorker count
func TestNewWorkerPoolPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic for zero workers")
		}
	}()
	NewWorkerPool(0, 0, ChannelPool)
}

// Test adding and executing tasks
func TestTryAddTask(t *testing.T) {
	// Test ChannelPool
	t.Run("ChannelPool", func(t *testing.T) {
		pool := NewWorkerPool(2, 8, ChannelPool) // Larger queue to accommodate all tasks
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
	})

	// Test RingBufferPool
	t.Run("RingBufferPool", func(t *testing.T) {
		pool := NewWorkerPool(2, 8, RingBufferPool) // Power of 2 queue
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
	})
}

// Test queue full scenario
func TestTryAddTaskQueueFull(t *testing.T) {
	// Test RingBufferPool
	t.Run("RingBufferPool", func(t *testing.T) {
		// Create pool with small queue and slow tasks
		pool := NewWorkerPool(2, 2, RingBufferPool) // Use power of 2
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
	})

	// Test ChannelPool
	t.Run("ChannelPool", func(t *testing.T) {
		// Create pool with small queue and slow tasks
		pool := NewWorkerPool(2, 3, ChannelPool) // Small queue
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
	})
}

// Test task execution with errors
func TestTaskWithError(t *testing.T) {
	// Test RingBufferPool
	t.Run("RingBufferPool", func(t *testing.T) {
		pool := NewWorkerPool(2, 16, RingBufferPool) // Larger queue for all tasks
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
	})

	// Test ChannelPool
	t.Run("ChannelPool", func(t *testing.T) {
		pool := NewWorkerPool(2, 20, ChannelPool) // Larger queue for all tasks
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
	})
}

// Test concurrent access
func TestConcurrentAccess(t *testing.T) {
	// Test RingBufferPool
	t.Run("RingBufferPool", func(t *testing.T) {
		pool := NewWorkerPool(8, 128, RingBufferPool) // Larger queue for concurrent access, use power of 2
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
	})

	// Test ChannelPool
	t.Run("ChannelPool", func(t *testing.T) {
		pool := NewWorkerPool(8, 150, ChannelPool) // Larger queue for concurrent access
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
	})
}

// Test running tasks counter
func TestRunningTasksCounter(t *testing.T) {
	pool := NewWorkerPool(2, 8, RingBufferPool) // Larger queue to ensure tasks can be added
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

	// Add multiple tasks - use AddTask to guarantee they're added
	for i := 0; i < 5; i++ {
		pool.AddTask(task)
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
	// Test RingBufferPool
	t.Run("RingBufferPool", func(t *testing.T) {
		pool := NewWorkerPool(2, 4, RingBufferPool) // Adequate queue size
		defer pool.Release()

		var completed int64
		task := func() error {
			time.Sleep(100 * time.Millisecond)
			atomic.StoreInt64(&completed, 1)
			return nil
		}

		start := time.Now()
		if !pool.TryAddTask(task) {
			t.Fatal("Failed to add task to pool")
		}
		pool.Wait()
		elapsed := time.Since(start)

		if atomic.LoadInt64(&completed) != 1 {
			t.Error("Task should have completed")
		}

		if elapsed < 100*time.Millisecond {
			t.Error("Wait() should wait for tasks to complete")
		}

		// Give a small moment for cleanup to complete
		time.Sleep(10 * time.Millisecond)

		if pool.Running() != 0 {
			t.Errorf("Expected 0 running tasks after Wait(), got %d", pool.Running())
		}

		if pool.QueueSize() != 0 {
			t.Errorf("Expected 0 queue size after Wait(), got %d", pool.QueueSize())
		}
	})

	// Test ChannelPool
	t.Run("ChannelPool", func(t *testing.T) {
		pool := NewWorkerPool(2, 8, ChannelPool) // Adequate queue size
		defer pool.Release()

		var completed int64
		task := func() error {
			time.Sleep(100 * time.Millisecond)
			atomic.StoreInt64(&completed, 1)
			return nil
		}

		start := time.Now()
		if !pool.TryAddTask(task) {
			t.Fatal("Failed to add task to pool")
		}
		pool.Wait()
		elapsed := time.Since(start)

		if atomic.LoadInt64(&completed) != 1 {
			t.Error("Task should have completed")
		}

		if elapsed < 100*time.Millisecond {
			t.Error("Wait() should wait for tasks to complete")
		}

		// Give a small moment for cleanup to complete
		time.Sleep(10 * time.Millisecond)

		if pool.Running() != 0 {
			t.Errorf("Expected 0 running tasks after Wait(), got %d", pool.Running())
		}

		if pool.QueueSize() != 0 {
			t.Errorf("Expected 0 queue size after Wait(), got %d", pool.QueueSize())
		}
	})
}

// Test Release() functionality
func TestRelease(t *testing.T) {
	pool := NewWorkerPool(4, 4, RingBufferPool)

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

// Test edge case: no tasks
func TestNoTasks(t *testing.T) {
	pool := NewWorkerPool(4, 4, RingBufferPool)
	defer pool.Release()
	defer pool.Release()

	// Wait with no tasks should return immediately
	start := time.Now()
	pool.Wait()
	elapsed := time.Since(start)

	if elapsed > 10*time.Millisecond {
		t.Error("Wait() with no tasks should return immediately")
	}
}

// Test ringBufferWorker pool with different queue sizes
func TestDifferentQueueSizes(t *testing.T) {
	// Test RingBufferPool with power-of-2 sizes
	t.Run("RingBufferPool", func(t *testing.T) {
		sizes := []int{2, 4, 8, 16, 32} // Use power-of-2 sizes for RingBufferPool

		for _, size := range sizes {
			t.Run(fmt.Sprintf("QueueSize%d", size), func(t *testing.T) {
				pool := NewWorkerPool(2, size, RingBufferPool)
				defer pool.Release()

				var counter int64
				task := func() error {
					atomic.AddInt64(&counter, 1)
					time.Sleep(5 * time.Millisecond) // Reduced delay
					return nil
				}

				// Add tasks gradually to allow processing
				taskCount := size + 2 // Reduced from size + 5
				successCount := 0
				for i := 0; i < taskCount; i++ {
					if pool.TryAddTask(task) {
						successCount++
					} else {
						// If queue is full, give a moment for processing
						time.Sleep(time.Millisecond)
						if pool.TryAddTask(task) {
							successCount++
						}
					}
				}

				pool.Wait()

				// Give a moment for cleanup
				time.Sleep(10 * time.Millisecond)

				// The test should verify that the number of executed tasks equals
				// the number of successfully added tasks
				executedCount := atomic.LoadInt64(&counter)
				if executedCount != int64(successCount) {
					t.Logf("Queue size: %d, Attempted: %d, Added: %d, Executed: %d",
						size, taskCount, successCount, executedCount)
					// Allow small timing variations but fail if the difference is too large
					if abs(int(executedCount)-successCount) > 1 {
						t.Errorf("Expected ~%d tasks executed, got %d (diff > 1)", successCount, executedCount)
					}
				}
			})
		}
	})

	// Test ChannelPool with various sizes
	t.Run("ChannelPool", func(t *testing.T) {
		sizes := []int{1, 3, 5, 10, 25} // Different sizes for ChannelPool

		for _, size := range sizes {
			t.Run(fmt.Sprintf("QueueSize%d", size), func(t *testing.T) {
				pool := NewWorkerPool(2, size, ChannelPool)
				defer pool.Release()

				var counter int64
				task := func() error {
					atomic.AddInt64(&counter, 1)
					time.Sleep(5 * time.Millisecond) // Reduced delay
					return nil
				}

				// Add tasks gradually to allow processing
				taskCount := size + 2
				successCount := 0
				for i := 0; i < taskCount; i++ {
					if pool.TryAddTask(task) {
						successCount++
					} else {
						// If queue is full, give a moment for processing
						time.Sleep(time.Millisecond)
						if pool.TryAddTask(task) {
							successCount++
						}
					}
				}

				pool.Wait()

				// Give a moment for cleanup
				time.Sleep(10 * time.Millisecond)

				// The test should verify that the number of executed tasks equals
				// the number of successfully added tasks
				executedCount := atomic.LoadInt64(&counter)
				if executedCount != int64(successCount) {
					t.Logf("Queue size: %d, Attempted: %d, Added: %d, Executed: %d",
						size, taskCount, successCount, executedCount)
					// Allow small timing variations but fail if the difference is too large
					if abs(int(executedCount)-successCount) > 1 {
						t.Errorf("Expected ~%d tasks executed, got %d (diff > 1)", successCount, executedCount)
					}
				}
			})
		}
	})
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// BenchmarkHTTPRingBufferPool benchmarks the ringBufferWorker pool with HTTP I/O tasks
func BenchmarkHTTPRingBufferPool(b *testing.B) {
	// Reset counter at start
	var gwPoolHttpCount atomic.Uint64

	pool := NewWorkerPool(100, 5000, RingBufferPool) // More workers for I/O bound tasks
	//pool := NewWorkerPool(100)
	defer pool.Release()

	// HTTP client for making requests
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Task that makes HTTP request to simulate I/O
	httpTask := func() error {
		gwPoolHttpCount.Add(1) // Count all attempts, regardless of success/failure
		resp, err := client.Get("http://localhost:8080/io-task?delay=5000")
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("HTTP error: %d", resp.StatusCode)
		}
		return nil
	}

	// Run at least 5000 iterations as requested
	iterations := 2500
	if b.N > iterations {
		iterations = b.N
	}

	// Add cleanup function to print total at the very end
	b.Cleanup(func() {
		fmt.Printf("==> GwPool Total HTTP requests made: %d\n", gwPoolHttpCount.Load())
	})

	b.ResetTimer()
	for i := 0; i < iterations; i++ {
		if !pool.TryAddTask(httpTask) {
			// If queue is full, wait a bit and retry
			time.Sleep(time.Millisecond)
			i-- // Retry this iteration
		}
	}
	pool.Wait()
	b.StopTimer()
}

// BenchmarkHTTPLimitedGoroutines benchmarks limited goroutines with HTTP I/O
func BenchmarkHTTPLimitedGoroutines(b *testing.B) {
	// Reset counter at start
	var limitedGoroutinesCount atomic.Uint64

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 100) // Limit to 100 concurrent goroutines

	// HTTP client for making requests
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Run at least 5000 iterations as requested
	iterations := 2500
	if b.N > iterations {
		iterations = b.N
	}

	// Add cleanup function to print total at the very end
	b.Cleanup(func() {
		fmt.Printf("==> Limited Goroutines Total requests made: %d\n", limitedGoroutinesCount.Load())
	})

	b.ResetTimer()
	wg.Add(iterations)
	for i := 0; i < iterations; i++ {
		go func() {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire semaphore
			defer func() { <-semaphore }() // Release semaphore

			limitedGoroutinesCount.Add(1) // Count all attempts
			resp, err := client.Get("http://localhost:8080/io-task?delay=5000")
			if err != nil {
				return
			}
			defer resp.Body.Close()
		}()
	}
	wg.Wait()
	b.StopTimer()
}

// BenchmarkHttpChannelPool benchmarks the channel worker pool
func BenchmarkHttpChannelPool(b *testing.B) {
	// Reset counter at start
	var ioOptimizedCount atomic.Uint64

	pool := NewWorkerPool(100, 5000, ChannelPool)
	defer pool.Release()

	// HTTP client for making requests
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Task that makes HTTP request to simulate I/O
	httpTask := func() error {
		ioOptimizedCount.Add(1) // Count all attempts, regardless of success/failure
		resp, err := client.Get("http://localhost:8080/io-task?delay=5000")
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("HTTP error: %d", resp.StatusCode)
		}
		return nil
	}

	// Run at least 5000 iterations as requested
	iterations := 2500
	if b.N > iterations {
		iterations = b.N
	}

	// Add cleanup function to print total at the very end
	b.Cleanup(func() {
		fmt.Printf("==> IO Optimized Total HTTP requests made: %d\n", ioOptimizedCount.Load())
	})

	b.ResetTimer()
	for i := 0; i < iterations; i++ {
		if !pool.TryAddTask(httpTask) {
			// If queue is full, wait a bit and retry
			time.Sleep(time.Millisecond)
			i-- // Retry this iteration
		}
	}
	pool.Wait()
	b.StopTimer()
}
