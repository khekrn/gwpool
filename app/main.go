package main

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/khekrn/gwpool"
)

func main() {
	// Create a new worker pool with 256 maximum workers
	pool := gwpool.NewWorkerPool(64,
		gwpool.WithTaskQueueSize(128),
		gwpool.WithRetryCount(0),
	)

	fmt.Printf("Created worker pool with %d workers and queue size %d\n", pool.WorkerCount(), 512)

	// Defer the release of the pool
	defer pool.Release()

	// Add 512 sample tasks to the pool
	tasksAdded := 0
	for i := 0; i < 512; i++ {
		taskID := i
		for {
			result := pool.TryAddTask(func() error {
				// Simulate work with a random duration
				duration := time.Duration(rand.Intn(1000)) * time.Millisecond
				time.Sleep(duration)
				log.Printf("Task %d completed after %v, workers = %d and current =%d\n", taskID, duration, pool.WorkerCount(), pool.Running())
				return nil
			})
			if result {
				tasksAdded++
				if tasksAdded%100 == 0 {
					fmt.Printf("Added %d tasks so far, queue size: %d, running: %d\n", tasksAdded, pool.QueueSize(), pool.Running())
				}
				break // Task was successfully added
			}
			// Queue is full, wait a bit and retry
			fmt.Printf("Queue full for task %d, retrying...\n", taskID)
			time.Sleep(10 * time.Millisecond)
		}
	}

	fmt.Printf("Loop Completed - Added %d tasks total\n", tasksAdded)

	// Add a task that might fail and be retried
	pool.TryAddTask(func() error {
		if rand.Float32() < 0.5 {
			log.Println("Task failed, will be retried")
			return fmt.Errorf("random failure")
		}
		log.Println("Potentially failing task completed successfully")
		return nil
	})

	// Wait for all tasks to complete
	fmt.Printf("Waiting for tasks to complete. Queue size: %d, Running: %d\n", pool.QueueSize(), pool.Running())
	pool.Wait()

	fmt.Printf("All tasks have been processed. Final stats - Queue: %d, Running: %d\n", pool.QueueSize(), pool.Running())
}
