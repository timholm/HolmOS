package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// TaskStatus represents the current state of a task
type TaskStatus string

const (
	StatusPending   TaskStatus = "pending"
	StatusRunning   TaskStatus = "running"
	StatusCompleted TaskStatus = "completed"
	StatusFailed    TaskStatus = "failed"
)

// Task represents a task in the queue
type Task struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Priority    int               `json:"priority"` // Higher number = higher priority
	Status      TaskStatus        `json:"status"`
	Payload     map[string]any    `json:"payload,omitempty"`
	Result      map[string]any    `json:"result,omitempty"`
	Error       string            `json:"error,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	StartedAt   *time.Time        `json:"started_at,omitempty"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
	WorkerID    string            `json:"worker_id,omitempty"`
}

// Worker represents a worker that processes tasks
type Worker struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Status       string    `json:"status"`
	CurrentTask  string    `json:"current_task,omitempty"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	TasksCompleted int     `json:"tasks_completed"`
	TasksFailed    int     `json:"tasks_failed"`
}

// TaskQueue manages the in-memory task queue
type TaskQueue struct {
	mu      sync.RWMutex
	tasks   map[string]*Task
	workers map[string]*Worker
}

// NewTaskQueue creates a new task queue
func NewTaskQueue() *TaskQueue {
	return &TaskQueue{
		tasks:   make(map[string]*Task),
		workers: make(map[string]*Worker),
	}
}

// AddTask adds a new task to the queue
func (q *TaskQueue) AddTask(name string, priority int, payload map[string]any) *Task {
	q.mu.Lock()
	defer q.mu.Unlock()

	task := &Task{
		ID:        uuid.New().String(),
		Name:      name,
		Priority:  priority,
		Status:    StatusPending,
		Payload:   payload,
		CreatedAt: time.Now(),
	}
	q.tasks[task.ID] = task
	return task
}

// GetTask returns a task by ID
func (q *TaskQueue) GetTask(id string) *Task {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.tasks[id]
}

// GetPendingTasks returns all pending tasks sorted by priority (highest first)
func (q *TaskQueue) GetPendingTasks() []*Task {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var pending []*Task
	for _, task := range q.tasks {
		if task.Status == StatusPending {
			pending = append(pending, task)
		}
	}

	// Sort by priority (highest first), then by creation time (oldest first)
	sort.Slice(pending, func(i, j int) bool {
		if pending[i].Priority != pending[j].Priority {
			return pending[i].Priority > pending[j].Priority
		}
		return pending[i].CreatedAt.Before(pending[j].CreatedAt)
	})

	return pending
}

// GetAllTasks returns all tasks
func (q *TaskQueue) GetAllTasks() []*Task {
	q.mu.RLock()
	defer q.mu.RUnlock()

	tasks := make([]*Task, 0, len(q.tasks))
	for _, task := range q.tasks {
		tasks = append(tasks, task)
	}

	// Sort by creation time (newest first)
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt.After(tasks[j].CreatedAt)
	})

	return tasks
}

// CompleteTask marks a task as completed
func (q *TaskQueue) CompleteTask(id string, result map[string]any) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	task, exists := q.tasks[id]
	if !exists {
		return &NotFoundError{Message: "task not found"}
	}

	now := time.Now()
	task.Status = StatusCompleted
	task.Result = result
	task.CompletedAt = &now

	// Update worker stats if task has a worker
	if task.WorkerID != "" {
		if worker, ok := q.workers[task.WorkerID]; ok {
			worker.TasksCompleted++
			worker.CurrentTask = ""
			worker.Status = "idle"
		}
	}

	return nil
}

// FailTask marks a task as failed
func (q *TaskQueue) FailTask(id string, errorMsg string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	task, exists := q.tasks[id]
	if !exists {
		return &NotFoundError{Message: "task not found"}
	}

	now := time.Now()
	task.Status = StatusFailed
	task.Error = errorMsg
	task.CompletedAt = &now

	// Update worker stats if task has a worker
	if task.WorkerID != "" {
		if worker, ok := q.workers[task.WorkerID]; ok {
			worker.TasksFailed++
			worker.CurrentTask = ""
			worker.Status = "idle"
		}
	}

	return nil
}

// RegisterWorker registers a new worker
func (q *TaskQueue) RegisterWorker(name string) *Worker {
	q.mu.Lock()
	defer q.mu.Unlock()

	worker := &Worker{
		ID:            uuid.New().String(),
		Name:          name,
		Status:        "idle",
		LastHeartbeat: time.Now(),
	}
	q.workers[worker.ID] = worker
	return worker
}

// GetWorkers returns all workers
func (q *TaskQueue) GetWorkers() []*Worker {
	q.mu.RLock()
	defer q.mu.RUnlock()

	workers := make([]*Worker, 0, len(q.workers))
	for _, worker := range q.workers {
		workers = append(workers, worker)
	}
	return workers
}

// NotFoundError represents a not found error
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return e.Message
}

// Server handles HTTP requests
type Server struct {
	queue  *TaskQueue
	router *mux.Router
}

// NewServer creates a new server
func NewServer(queue *TaskQueue) *Server {
	s := &Server{
		queue:  queue,
		router: mux.NewRouter(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.router.HandleFunc("/health", s.handleHealth).Methods("GET")
	s.router.HandleFunc("/tasks", s.handleAddTask).Methods("POST")
	s.router.HandleFunc("/tasks", s.handleListTasks).Methods("GET")
	s.router.HandleFunc("/task/{id}", s.handleGetTask).Methods("GET")
	s.router.HandleFunc("/task/{id}/complete", s.handleCompleteTask).Methods("POST")
	s.router.HandleFunc("/task/{id}/fail", s.handleFailTask).Methods("POST")
	s.router.HandleFunc("/workers", s.handleListWorkers).Methods("GET")
	s.router.HandleFunc("/workers", s.handleRegisterWorker).Methods("POST")
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":    "healthy",
		"service":   "task-queue",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

type addTaskRequest struct {
	Name     string         `json:"name"`
	Priority int            `json:"priority"`
	Payload  map[string]any `json:"payload"`
}

func (s *Server) handleAddTask(w http.ResponseWriter, r *http.Request) {
	var req addTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	task := s.queue.AddTask(req.Name, req.Priority, req.Payload)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(task)
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")

	var tasks []*Task
	if status == "pending" {
		tasks = s.queue.GetPendingTasks()
	} else {
		tasks = s.queue.GetAllTasks()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"tasks": tasks,
		"count": len(tasks),
	})
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	task := s.queue.GetTask(id)
	if task == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

type completeTaskRequest struct {
	Result map[string]any `json:"result"`
}

func (s *Server) handleCompleteTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var req completeTaskRequest
	json.NewDecoder(r.Body).Decode(&req) // Optional body

	if err := s.queue.CompleteTask(id, req.Result); err != nil {
		if _, ok := err.(*NotFoundError); ok {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	task := s.queue.GetTask(id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

type failTaskRequest struct {
	Error string `json:"error"`
}

func (s *Server) handleFailTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var req failTaskRequest
	json.NewDecoder(r.Body).Decode(&req) // Optional body

	errorMsg := req.Error
	if errorMsg == "" {
		errorMsg = "task failed"
	}

	if err := s.queue.FailTask(id, errorMsg); err != nil {
		if _, ok := err.(*NotFoundError); ok {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	task := s.queue.GetTask(id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func (s *Server) handleListWorkers(w http.ResponseWriter, r *http.Request) {
	workers := s.queue.GetWorkers()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"workers": workers,
		"count":   len(workers),
	})
}

type registerWorkerRequest struct {
	Name string `json:"name"`
}

func (s *Server) handleRegisterWorker(w http.ResponseWriter, r *http.Request) {
	var req registerWorkerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		req.Name = "worker-" + uuid.New().String()[:8]
	}

	worker := s.queue.RegisterWorker(req.Name)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(worker)
}

func main() {
	queue := NewTaskQueue()
	server := NewServer(queue)

	port := ":8080"
	log.Printf("Task Queue service starting on port %s", port)
	log.Printf("Endpoints:")
	log.Printf("  POST /tasks - Add task to queue")
	log.Printf("  GET  /tasks - List tasks (use ?status=pending for pending only)")
	log.Printf("  GET  /task/{id} - Get task status")
	log.Printf("  POST /task/{id}/complete - Mark task complete")
	log.Printf("  POST /task/{id}/fail - Mark task failed")
	log.Printf("  GET  /workers - List workers")
	log.Printf("  POST /workers - Register worker")
	log.Printf("  GET  /health - Health check")

	if err := http.ListenAndServe(port, server); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
