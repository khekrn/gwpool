package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

// Response structure for the API
type APIResponse struct {
	Message   string        `json:"message"`
	Delay     time.Duration `json:"delay"`
	Timestamp time.Time     `json:"timestamp"`
	TaskID    string        `json:"task_id"`
}

// ErrorResponse structure for error cases
type ErrorResponse struct {
	Error string `json:"error"`
}

// simulateIOTask simulates an I/O bounded task with the given delay
func simulateIOTask(delay time.Duration, taskID string) APIResponse {
	start := time.Now()

	// Simulate I/O operation (database query, file read, network call, etc.)
	time.Sleep(delay)

	fmt.Println("Processed I/O task:", taskID)

	return APIResponse{
		Message:   "I/O task completed successfully",
		Delay:     delay,
		Timestamp: start,
		TaskID:    taskID,
	}
}

// ioTaskHandler handles the GET /io-task endpoint
func ioTaskHandler(w http.ResponseWriter, r *http.Request) {
	// Only allow GET requests
	if r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error: "Method not allowed. Use GET.",
		})
		return
	}

	// Parse delay query parameter
	delayStr := r.URL.Query().Get("delay")
	if delayStr == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error: "Missing 'delay' query parameter. Example: ?delay=1000 (milliseconds)",
		})
		return
	}

	// Convert delay to integer (milliseconds)
	delayMs, err := strconv.Atoi(delayStr)
	if err != nil || delayMs < 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error: "Invalid 'delay' parameter. Must be a positive integer (milliseconds).",
		})
		return
	}

	// Convert to duration
	delay := time.Duration(delayMs) * time.Millisecond

	// Limit maximum delay to prevent abuse
	maxDelay := 10 * time.Second
	if delay > maxDelay {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error: fmt.Sprintf("Delay too large. Maximum allowed: %v", maxDelay),
		})
		return
	}

	// Generate task ID
	taskID := fmt.Sprintf("task_%d", time.Now().UnixNano())

	// Execute the I/O task directly (no worker pool)
	result := simulateIOTask(delay, taskID)

	// Return successful response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

func main() {
	// Setup route
	http.HandleFunc("/io-task", ioTaskHandler)

	// Root handler with API documentation
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		apiDoc := map[string]interface{}{
			"name":        "Simple I/O Simulation Server",
			"description": "HTTP server for simulating I/O bounded tasks",
			"endpoints": map[string]interface{}{
				"GET /io-task": map[string]interface{}{
					"description": "Simulate I/O bounded task with specified delay",
					"parameters": map[string]string{
						"delay": "Delay in milliseconds (required, max: 10000)",
					},
					"example": "/io-task?delay=1500",
				},
			},
		}

		json.NewEncoder(w).Encode(apiDoc)
	})

	port := ":8080"
	log.Printf("Starting simple I/O server on port %s", port)
	log.Printf("API Documentation: http://localhost%s", port)
	log.Printf("Example: http://localhost%s/io-task?delay=1000", port)

	log.Fatal(http.ListenAndServe(port, nil))
}
