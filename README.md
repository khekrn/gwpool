# gwpool

A high-performance, production-ready worker pool library for Go applications offering two optimized implementations for different use cases.

[![Go Version](https://img.shields.io/badge/Go-%3E%3D%201.23-blue)](https://golang.org/doc/install)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/khekrn/gwpool)](https://github.com/khekrn/gwpool/releases)

## 🚀 Features

- **Two High-Performance Implementations**:

  - **ChannelPool**: Flexible, general-purpose worker pool using Go channels
  - **RingBufferPool**: 20% faster implementation with ring buffer implementation

- **Dual Task Submission Methods**:

  - **`AddTask()`**: Blocking method that guarantees task execution
  - **`TryAddTask()`**: Non-blocking method for high-throughput scenarios

- **Production-Ready**:
  - Comprehensive error handling and recovery
  - Graceful shutdown with proper resource cleanup
  - Thread-safe operations with minimal contention
  - Cache-aligned data structures for optimal performance

## 📊 Performance Comparison

Based on HTTP benchmark tests with 2,500 concurrent requests:

| Implementation     | Execution Time | Performance Gain |
| ------------------ | -------------- | ---------------- |
| **RingBufferPool** | 100.12s        | **20% faster**   |
| ChannelPool        | 125.24s        | Baseline         |
| Limited Goroutines | 125.44s        | -0.2%            |

_RingBufferPool is optimized for maximum throughput with power-of-2 optimizations._

## 📦 Installation

```bash
go get github.com/khekrn/gwpool@latest
```

## 🎯 Quick Start

```go
package main

import (
    "fmt"
    "log"
    "time"
    "github.com/khekrn/gwpool"
)

func main() {
    // Create a worker pool - choose implementation based on your needs
    pool := gwpool.NewWorkerPool(64, 128, gwpool.RingBufferPool)
    defer pool.Release()

    // Submit a task that will definitely execute (blocking)
    pool.AddTask(func() error {
        fmt.Println("This task is guaranteed to execute")
        return nil
    })

    // Submit a task only if queue has space (non-blocking)
    if pool.TryAddTask(func() error {
        fmt.Println("This task executes only if queue isn't full")
        return nil
    }) {
        fmt.Println("Task queued successfully")
    } else {
        fmt.Println("Queue is full, task not queued")
    }

    // Wait for all tasks to complete
    pool.Wait()
}
```

## 📋 Pool Types Comparison

### ChannelPool - General Purpose

```go
pool := gwpool.NewWorkerPool(50, 100, gwpool.ChannelPool)
```

**Best for:**

- Normal I/O-bound tasks (HTTP requests, database operations, file I/O)
- Mixed workloads with varying task durations
- Applications where simplicity and reliability are priorities
- Debugging and profiling scenarios

**Characteristics:**

- Uses native Go channels for task distribution
- Accepts any positive worker count
- Lower memory overhead
- Simpler architecture, easier debugging

### RingBufferPool - High Performance

```go
pool := gwpool.NewWorkerPool(64, 128, gwpool.RingBufferPool)  // Workers auto-rounded to 64
```

**Best for:**

- High-throughput scenarios requiring maximum performance
- Tasks with consistent execution times
- Applications where the 20% performance improvement justifies complexity
- Microservices with strict latency requirements

**Characteristics:**

- Ring buffer implementation
- Worker count automatically rounded to nearest power-of-2
- Cache-aligned data structures
- Bitwise optimizations for worker selection

## 🔄 Task Submission Methods

### `AddTask()` - Blocking Submission (Guaranteed Execution)

The `AddTask()` method **blocks until the task is successfully queued**, guaranteeing that your task will eventually execute.

```go
// ✅ GUARANTEED EXECUTION - Will block until task is queued
pool.AddTask(func() error {
    // Critical task that must not be lost
    err := saveUserData(userData)
    if err != nil {
        log.Printf("Failed to save user data: %v", err)
        return err
    }
    return nil
})

// This line executes only after the task above is queued
fmt.Println("Task queued successfully, continuing...")
```

**Use Cases for AddTask():**

- Critical operations that cannot be lost
- Database transactions and data persistence
- User-initiated actions requiring confirmation
- Sequential workflows where task completion order matters

### `TryAddTask()` - Non-Blocking Submission (Best Effort)

The `TryAddTask()` method **returns immediately** with a boolean indicating success/failure, never blocking the caller.

```go
// ⚡ NON-BLOCKING - Returns immediately with success status
success := pool.TryAddTask(func() error {
    // This task may or may not execute depending on queue availability
    updateMetrics()
    return nil
})

if success {
    fmt.Println("Task queued successfully")
} else {
    fmt.Println("Queue full - task dropped")
    // Handle the failure case
    handleTaskDropped()
}

// This line executes immediately regardless of queue status
fmt.Println("Continuing without waiting...")
```

**Use Cases for TryAddTask():**

- High-frequency operations (metrics, logging, analytics)
- Background tasks where occasional drops are acceptable
- Real-time systems where blocking is not acceptable
- Bulk operations where some failures are tolerable

## 🔄 Real-World Examples

### Example 1: Web Server Request Handler

```go
func handleUpload(w http.ResponseWriter, r *http.Request) {
    // Critical: Save file metadata (must not be lost)
    pool.AddTask(func() error {
        return saveFileMetadata(r.Header.Get("filename"))
    })

    // Best effort: Update analytics (can be dropped if overloaded)
    pool.TryAddTask(func() error {
        updateUploadMetrics(r.RemoteAddr)
        return nil
    })

    w.WriteHeader(http.StatusOK)
}
```

### Example 2: Batch Processing with Different Priorities

```go
func processBatch(criticalTasks, backgroundTasks []Task) {
    // Process critical tasks first (blocking)
    for _, task := range criticalTasks {
        pool.AddTask(task)  // Guarantees execution
    }

    // Process background tasks (non-blocking)
    dropped := 0
    for _, task := range backgroundTasks {
        if !pool.TryAddTask(task) {
            dropped++
        }
    }

    log.Printf("Queued %d critical tasks, dropped %d background tasks",
               len(criticalTasks), dropped)
}
```

### Example 3: Queue Full Handling Strategies

```go
// Strategy 1: Retry with backoff
func submitWithRetry(task gwpool.Task, maxRetries int) bool {
    for i := 0; i < maxRetries; i++ {
        if pool.TryAddTask(task) {
            return true
        }
        time.Sleep(time.Duration(i*100) * time.Millisecond)
    }
    return false
}

// Strategy 2: Fallback to synchronous execution
func submitOrExecuteSync(task gwpool.Task) {
    if !pool.TryAddTask(task) {
        // Queue full - execute synchronously
        log.Println("Queue full, executing task synchronously")
        task()
    }
}

// Strategy 3: Circuit breaker pattern
func submitWithCircuitBreaker(task gwpool.Task) bool {
    if circuitBreaker.IsOpen() {
        return false  // Fail fast when system is overloaded
    }

    success := pool.TryAddTask(task)
    if !success {
        circuitBreaker.RecordFailure()
    }
    return success
}
```

## 📊 Pool Monitoring and Observability

```go
func monitorPool(pool gwpool.WorkerPool) {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    for range ticker.C {
        fmt.Printf("Pool Stats - Workers: %d, Queue: %d, Running: %d\n",
            pool.WorkerCount(),
            pool.QueueSize(),
            pool.Running())
    }
}

// Usage
go monitorPool(pool)
```

## ⚙️ Configuration Guidelines

### Choosing Worker Count

```go
// CPU-intensive tasks
workers := runtime.NumCPU()

// I/O-intensive tasks
workers := runtime.NumCPU() * 2

// High-latency I/O (database, external APIs)
workers := runtime.NumCPU() * 4
```

### Choosing Queue Size

```go
// Conservative (low memory usage)
queueSize := workers * 2

// Balanced (good for most applications)
queueSize := workers * 4

// High throughput (accepts more memory usage)
queueSize := workers * 8
```

### Pool Type Selection

```go
// For maximum performance (20% faster)
pool := gwpool.NewWorkerPool(64, 256, gwpool.RingBufferPool)

// For general use and debugging ease
pool := gwpool.NewWorkerPool(50, 200, gwpool.ChannelPool)
```

## 🛡️ Best Practices

### 1. Resource Management

```go
// Always defer Release() immediately after creation
pool := gwpool.NewWorkerPool(50, 100, gwpool.ChannelPool)
defer pool.Release()  // Ensures cleanup even if panic occurs
```

### 2. Error Handling in Tasks

```go
pool.AddTask(func() error {
    defer func() {
        if r := recover(); r != nil {
            log.Printf("Task panicked: %v", r)
        }
    }()

    // Your task logic here
    return doWork()
})
```

### 3. Graceful Shutdown

```go
func gracefulShutdown(pool gwpool.WorkerPool) {
    log.Println("Starting graceful shutdown...")

    // Stop accepting new tasks
    // (implement your application logic)

    // Wait for current tasks to complete
    pool.Wait()

    // Release resources
    pool.Release()

    log.Println("Shutdown complete")
}
```

### 4. Task Design Guidelines

```go
// ✅ Good: Small, focused tasks
pool.TryAddTask(func() error {
    return processEvent(event)
})

// ✅ Better: Break into smaller chunks
for _, chunk := range chunks {
    pool.TryAddTask(func() error {
        return processChunk(chunk)
    })
}
```

## 🧪 Testing Your Integration

```go
func TestWorkerPool(t *testing.T) {
    pool := gwpool.NewWorkerPool(4, 8, gwpool.ChannelPool)
    defer pool.Release()

    var counter int64

    // Test AddTask (blocking)
    pool.AddTask(func() error {
        atomic.AddInt64(&counter, 1)
        return nil
    })

    // Test TryAddTask (non-blocking)
    success := pool.TryAddTask(func() error {
        atomic.AddInt64(&counter, 1)
        return nil
    })

    assert.True(t, success)
    pool.Wait()
    assert.Equal(t, int64(2), atomic.LoadInt64(&counter))
}
```

## 📈 Performance Tuning

### Benchmark Your Configuration

```go
func BenchmarkWorkerPool(b *testing.B) {
    pool := gwpool.NewWorkerPool(100, 1000, gwpool.RingBufferPool)
    defer pool.Release()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        pool.TryAddTask(func() error {
            // Your typical task
            return nil
        })
    }
    pool.Wait()
}
```

## 📚 Advanced Usage

### Custom Task Types

```go
type DatabaseTask struct {
    Query string
    Args  []interface{}
}

func (dt DatabaseTask) Execute() error {
    return executeQuery(dt.Query, dt.Args...)
}

// Wrap in gwpool.Task
pool.AddTask(func() error {
    return DatabaseTask{
        Query: "INSERT INTO users VALUES (?, ?)",
        Args:  []interface{}{"john", "doe"},
    }.Execute()
})
```

### Pool Composition

```go
type ServiceManager struct {
    criticalPool    gwpool.WorkerPool  // For critical tasks
    backgroundPool  gwpool.WorkerPool  // For background tasks
}

func NewServiceManager() *ServiceManager {
    return &ServiceManager{
        criticalPool:   gwpool.NewWorkerPool(32, 64, gwpool.RingBufferPool),
        backgroundPool: gwpool.NewWorkerPool(16, 128, gwpool.ChannelPool),
    }
}
```

## 🔧 Troubleshooting

### Common Issues

**Queue Full Errors:**

```go
// Monitor queue usage
if pool.QueueSize() > pool.WorkerCount() * 3 {
    log.Warn("Queue size growing, consider scaling workers")
}
```

**Memory Usage:**

```go
// RingBufferPool uses more memory due to cache alignment
// Use ChannelPool if memory is constrained
pool := gwpool.NewWorkerPool(workers, queueSize, gwpool.ChannelPool)
```

**Worker Starvation:**

```go
// Ensure tasks don't block indefinitely
pool.AddTask(func() error {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    return longRunningOperation(ctx)
})
```

## 📄 License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) file for details.

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## 📞 Support

- **Issues**: [GitHub Issues](https://github.com/khekrn/gwpool/issues)
- **Documentation**: [pkg.go.dev](https://pkg.go.dev/github.com/khekrn/gwpool)
