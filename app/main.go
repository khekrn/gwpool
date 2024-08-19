package main

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/khekrn/gwpool"
)

func main() {
	// Create a new worker pool with 5 maximum workers
	pool := gwpool.NewWorkerPool(256,
		gwpool.WithMinWorkers(64),
		gwpool.WithTaskQueue(512),
		gwpool.WithRetryCount(0),
	)

	// Defer the release of the pool
	defer pool.Release()

	// Add 20 sample tasks to the pool
	for i := 0; i < 2048; i++ {
		taskID := i
		pool.AddTask(func() error {
			// Simulate work with a random duration
			duration := time.Duration(rand.Intn(500)) * time.Millisecond
			time.Sleep(duration)
			log.Printf("Task %d completed after %v", taskID, duration)
			fmt.Println("Total Workers = ", pool.WorkerCount(), " and Current Running = ", pool.Running())
			return nil
		})
	}

	fmt.Println("Loop Completed")

	// Add a task that might fail and be retried
	pool.AddTask(func() error {
		if rand.Float32() < 0.5 {
			log.Println("Task failed, will be retried")
			return fmt.Errorf("random failure")
		}
		log.Println("Potentially failing task completed successfully")
		return nil
	})

	// Wait for all tasks to complete
	pool.Wait()

	fmt.Println("All tasks have been processed")
}
