package main

import (
	"fmt"
	v3 "github.com/khekrn/gwpool/v4"
	"time"
)

func main() {
	maxWorkers := 10
	//minWorkers := 2
	taskQueueSize := 100

	// Create a new worker pool
	pool := v3.NewGoPool(maxWorkers)
	//pool := gwpool.NewWorkerPool(minWorkers, maxWorkers, taskQueueSize)
	defer pool.Release()

	// Create a slice to store the results
	results := make([]int, taskQueueSize)

	// Add tasks to the pool
	for i := 0; i < taskQueueSize; i++ {
		taskID := i
		pool.AddTask(func() (interface{}, error) {
			time.Sleep(100 * time.Millisecond)

			// Store the result
			results[taskID] = taskID

			fmt.Printf("Task %d completed\n", taskID)
			return nil, nil
		})
	}

	// Wait for all tasks to be completed
	pool.Wait()

	// Print the results
	fmt.Println("Results:", results)

	// Get pool statistics
	fmt.Printf("Worker Count: %d\n", pool.WorkerCount())
	fmt.Printf("Task Queue Size: %d\n", pool.TaskQueueSize())

}
