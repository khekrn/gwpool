package gwpool

import (
	"sync"
	"testing"
	"time"

	"github.com/alitto/pond"
	"github.com/devchat-ai/gopool"
)

func TestAddTask(t *testing.T) {
	// 1. Happy path: Test adding a task to the worker pool's task queue under normal conditions
	t.Run("HappyPath_AddSingleTask", func(t *testing.T) {
		pool := NewWorkerPool(10, WithLock(new(sync.Mutex)))
		defer pool.Release()
		pool.AddTask(func() error {
			time.Sleep(10 * time.Millisecond)
			return nil
		})
		pool.Wait()
	})
}

const (
	PoolSize = 1e4
	TaskNum  = 1e6
)

func BenchmarkGwPool(b *testing.B) {
	pool := NewWorkerPool(PoolSize)
	defer pool.Release()

	taskFunc := func() error {
		time.Sleep(10 * time.Millisecond)
		return nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for num := 0; num < TaskNum; num++ {
			pool.AddTask(taskFunc)
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
				time.Sleep(10 * time.Millisecond)
				wg.Done()
				return nil, nil
			}()
		}
		wg.Wait()
	}
}

func BenchmarkPond(b *testing.B) {
	pool := pond.New(PoolSize, 0, pond.MinWorkers(PoolSize))

	taskFunc := func() {
		time.Sleep(10 * time.Millisecond)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for i := 0; i < TaskNum; i++ {
			pool.Submit(taskFunc)
		}
		pool.StopAndWait()
	}
	b.StopTimer()
}

func BenchmarkGoPool(b *testing.B) {
	pool := gopool.NewGoPool(PoolSize)
	defer pool.Release()

	taskFunc := func() (interface{}, error) {
		time.Sleep(10 * time.Millisecond)
		return nil, nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for num := 0; num < TaskNum; num++ {
			pool.AddTask(taskFunc)
		}
		pool.Wait()
	}
	b.StopTimer()
}
