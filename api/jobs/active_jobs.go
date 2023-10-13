package jobs

import (
	"sync"
)

// It is the resoponsibility of originator to add and remove job from ActiveJobs
type ActiveJobs struct {
	Jobs map[string]*Job `json:"jobs"`
	mu   sync.Mutex
}

func (ac *ActiveJobs) Add(j *Job) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	ac.Jobs[(*j).JobID()] = j
}

func (ac *ActiveJobs) Remove(j *Job) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	delete(ac.Jobs, (*j).JobID())
}

// Revised to kill only currently active jobs
func (ac *ActiveJobs) KillAll() {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	for _, j := range ac.Jobs {
		if (*j).CurrentStatus() == ACCEPTED || (*j).CurrentStatus() == RUNNING {
			// we can't wait for each Kill operation to complete since KillAll will be called during shutdown
			// and limited time is available to gracefully shutdown
			go (*j).Kill()
		}
	}
}
