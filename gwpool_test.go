package gwpool

import (
	"sync"
	"testing"
	"time"
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
