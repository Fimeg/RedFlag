package scheduler

import (
	"container/heap"
	"fmt"
	"sync"
	"time"

	"github.com/gofrs/uuid/v5"
)

// SubsystemJob represents a scheduled subsystem scan
type SubsystemJob struct {
	AgentID         uuid.UUID
	AgentHostname   string // For logging/debugging
	Subsystem       string
	IntervalMinutes int
	NextRunAt       time.Time
	Enabled         bool
	index           int // Heap index (managed by heap.Interface)
}

// String returns a human-readable representation of the job
func (j *SubsystemJob) String() string {
	return fmt.Sprintf("[%s/%s] next_run=%s interval=%dm",
		j.AgentHostname, j.Subsystem,
		j.NextRunAt.Format("15:04:05"), j.IntervalMinutes)
}

// jobHeap implements heap.Interface for SubsystemJob priority queue
// Jobs are ordered by NextRunAt (earliest first)
type jobHeap []*SubsystemJob

func (h jobHeap) Len() int { return len(h) }

func (h jobHeap) Less(i, j int) bool {
	return h[i].NextRunAt.Before(h[j].NextRunAt)
}

func (h jobHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *jobHeap) Push(x interface{}) {
	n := len(*h)
	job := x.(*SubsystemJob)
	job.index = n
	*h = append(*h, job)
}

func (h *jobHeap) Pop() interface{} {
	old := *h
	n := len(old)
	job := old[n-1]
	old[n-1] = nil  // Avoid memory leak
	job.index = -1  // Mark as removed
	*h = old[0 : n-1]
	return job
}

// PriorityQueue is a thread-safe priority queue for subsystem jobs
// Jobs are ordered by their NextRunAt timestamp (earliest first)
type PriorityQueue struct {
	heap jobHeap
	mu   sync.RWMutex

	// Index for fast lookups by agent_id + subsystem
	index map[string]*SubsystemJob // key: "agent_id:subsystem"
}

// NewPriorityQueue creates a new empty priority queue
func NewPriorityQueue() *PriorityQueue {
	pq := &PriorityQueue{
		heap:  make(jobHeap, 0),
		index: make(map[string]*SubsystemJob),
	}
	heap.Init(&pq.heap)
	return pq
}

// Push adds a job to the queue
// If a job with the same agent_id + subsystem already exists, it's updated
func (pq *PriorityQueue) Push(job *SubsystemJob) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	key := makeKey(job.AgentID, job.Subsystem)

	// Check if job already exists
	if existing, exists := pq.index[key]; exists {
		// Update existing job
		existing.NextRunAt = job.NextRunAt
		existing.IntervalMinutes = job.IntervalMinutes
		existing.Enabled = job.Enabled
		existing.AgentHostname = job.AgentHostname
		heap.Fix(&pq.heap, existing.index)
		return
	}

	// Add new job
	heap.Push(&pq.heap, job)
	pq.index[key] = job
}

// Pop removes and returns the job with the earliest NextRunAt
// Returns nil if queue is empty
func (pq *PriorityQueue) Pop() *SubsystemJob {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if pq.heap.Len() == 0 {
		return nil
	}

	job := heap.Pop(&pq.heap).(*SubsystemJob)
	key := makeKey(job.AgentID, job.Subsystem)
	delete(pq.index, key)

	return job
}

// Peek returns the job with the earliest NextRunAt without removing it
// Returns nil if queue is empty
func (pq *PriorityQueue) Peek() *SubsystemJob {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	if pq.heap.Len() == 0 {
		return nil
	}

	return pq.heap[0]
}

// Remove removes a specific job from the queue
// Returns true if job was found and removed, false otherwise
func (pq *PriorityQueue) Remove(agentID uuid.UUID, subsystem string) bool {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	key := makeKey(agentID, subsystem)
	job, exists := pq.index[key]
	if !exists {
		return false
	}

	heap.Remove(&pq.heap, job.index)
	delete(pq.index, key)

	return true
}

// Get retrieves a specific job without removing it
// Returns nil if not found
func (pq *PriorityQueue) Get(agentID uuid.UUID, subsystem string) *SubsystemJob {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	key := makeKey(agentID, subsystem)
	return pq.index[key]
}

// PopBefore returns all jobs with NextRunAt <= before, up to limit
// Jobs are removed from the queue
// If limit <= 0, all matching jobs are returned
func (pq *PriorityQueue) PopBefore(before time.Time, limit int) []*SubsystemJob {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	var jobs []*SubsystemJob

	for pq.heap.Len() > 0 {
		// Peek at next job
		next := pq.heap[0]

		// Stop if next job is after our cutoff
		if next.NextRunAt.After(before) {
			break
		}

		// Stop if we've hit the limit
		if limit > 0 && len(jobs) >= limit {
			break
		}

		// Pop and collect the job
		job := heap.Pop(&pq.heap).(*SubsystemJob)
		key := makeKey(job.AgentID, job.Subsystem)
		delete(pq.index, key)

		jobs = append(jobs, job)
	}

	return jobs
}

// PeekBefore returns all jobs with NextRunAt <= before without removing them
// If limit <= 0, all matching jobs are returned
func (pq *PriorityQueue) PeekBefore(before time.Time, limit int) []*SubsystemJob {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	var jobs []*SubsystemJob

	for i := 0; i < pq.heap.Len(); i++ {
		job := pq.heap[i]

		if job.NextRunAt.After(before) {
			// Since heap is sorted by NextRunAt, we can break early
			// Note: This is only valid because we peek in order
			break
		}

		if limit > 0 && len(jobs) >= limit {
			break
		}

		jobs = append(jobs, job)
	}

	return jobs
}

// Len returns the number of jobs in the queue
func (pq *PriorityQueue) Len() int {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	return pq.heap.Len()
}

// Clear removes all jobs from the queue
func (pq *PriorityQueue) Clear() {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	pq.heap = make(jobHeap, 0)
	pq.index = make(map[string]*SubsystemJob)
	heap.Init(&pq.heap)
}

// GetStats returns statistics about the queue
func (pq *PriorityQueue) GetStats() QueueStats {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	stats := QueueStats{
		Size: pq.heap.Len(),
	}

	if pq.heap.Len() > 0 {
		stats.NextRunAt = &pq.heap[0].NextRunAt
		stats.OldestJob = pq.heap[0].String()
	}

	// Count jobs by subsystem
	stats.JobsBySubsystem = make(map[string]int)
	for _, job := range pq.heap {
		stats.JobsBySubsystem[job.Subsystem]++
	}

	return stats
}

// QueueStats holds statistics about the priority queue
type QueueStats struct {
	Size             int
	NextRunAt        *time.Time
	OldestJob        string
	JobsBySubsystem  map[string]int
}

// String returns a human-readable representation of stats
func (s QueueStats) String() string {
	nextRun := "empty"
	if s.NextRunAt != nil {
		nextRun = s.NextRunAt.Format("15:04:05")
	}
	return fmt.Sprintf("size=%d next=%s oldest=%s", s.Size, nextRun, s.OldestJob)
}

// makeKey creates a unique key for agent_id + subsystem
func makeKey(agentID uuid.UUID, subsystem string) string {
	return agentID.String() + ":" + subsystem
}
