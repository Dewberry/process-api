package jobs

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/labstack/gommon/log"
)

// Job refers to any process that has been created through
// the processes/{processID}/execution endpoint
type Job interface {
	CMD() []string
	CurrentStatus() string
	Equals(Job) bool
	IMGTAG() string
	JobID() string
	ProcessID() string
	JobLogs() string
	Kill() error
	LastUpdate() time.Time
	Messages(bool) []string
	NewMessage(string)
	NewStatusUpdate(string)
	Run()
	Create() error
	GetSizeinCache() int
	JobOutputs() []interface{}
	ClearOutputs()
}

type Jobs []Job

type JobStatus struct {
	JobID      string   `json:"jobID"`
	LastUpdate string   `json:"last_update"`
	Status     string   `json:"status"`
	ProcessID  string   `json:"processID"`
	Details    []string `json:"details"`
}

// OGCStatusCode
const (
	ACCEPTED   = "accepted"
	RUNNING    = "running"
	SUCCESSFUL = "successful"
	FAILED     = "failed"
	DISMISSED  = "dismissed"
)

// RunRequestBody provides the required inputs for containerized processes
type RunRequestBody struct {
	Inputs  []map[string]string `json:"inputs"`
	EnvVars map[string]string   `json:"environmentVariables"`
}

type JobsCache struct {
	Jobs             `json:"jobs"`
	mu               sync.Mutex
	MaxSizeBytes     uint64  `json:"maxCacheBytes"`
	TrimThreshold    float64 `json:"cacheTrimThreshold"`
	CurrentSizeBytes uint64  `json:"currentCacheBytes"`
}

func (jc *JobsCache) Add(j ...Job) {
	jc.mu.Lock()
	defer jc.mu.Unlock()
	jc.Jobs = append(jc.Jobs, j...)
}

func (jc *JobsCache) Remove(j Job) {
	jc.mu.Lock()
	defer jc.mu.Unlock()

	newJobs := make([]Job, 0)
	for _, j := range jc.Jobs {

		if !j.Equals(j) {
			newJobs = append(newJobs, j)
		}
	}
	jc.Jobs = newJobs
}

func (jc *JobsCache) ListJobs(includeErrorMessages bool) []JobStatus {
	jc.mu.Lock()
	defer jc.mu.Unlock()

	output := make([]JobStatus, len(jc.Jobs))

	for i, j := range jc.Jobs {

		jobStatus := JobStatus{j.JobID(), j.LastUpdate().String(), j.CurrentStatus(),
			j.ProcessID(), j.CMD()}

		// if includeErrorMessages {
		// 	result["error_messages"] = j.Messages(true)
		// }
		output[i] = jobStatus
	}
	return output
}

func (jc *JobsCache) DumpCacheToFile(fileName string) error {
	// Create a file
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()

	// Write the map to the file
	b, err := json.Marshal(jc.ListJobs(true))
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err != nil {
		return err
	}
	return nil
}

func (jc *JobsCache) TrimCache(desiredLength int64) {
	jc.mu.Lock()
	defer jc.mu.Unlock()
	jobs := jc.Jobs
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].LastUpdate().After(jobs[j].LastUpdate())
	})
	jc.Jobs = jobs[0:desiredLength]
}

func (jc *JobsCache) KillAll() error {
	jc.mu.Lock()
	defer jc.mu.Unlock()

	for _, j := range jc.Jobs {
		if err := j.Kill(); err != nil {
			return err
		}
	}
	jc.Jobs = make([]Job, 0)

	return nil
}

func (jc *JobsCache) CheckCache() uint64 {
	// jc.mu.Lock()
	// defer jc.mu.Unlock()

	var jobSize uint64
	for _, j := range jc.Jobs {
		jobSize += uint64(j.GetSizeinCache())
	}
	jc.CurrentSizeBytes = jobSize

	pctCacheFull := float64(jc.CurrentSizeBytes) / float64(jc.MaxSizeBytes)
	log.Info("cache_pct_full=", pctCacheFull, " current_size=", float64(jc.CurrentSizeBytes), " jobs=", len(jc.Jobs), " (max cache=", float64(jc.MaxSizeBytes), ")")
	// set default auto-trim to 95%....
	if pctCacheFull > 0.95 {
		currentLenth := len(jc.Jobs)
		desiredLength := int64(jc.TrimThreshold * float64(currentLenth))
		message := fmt.Sprintf("trimming cache from %d jobs to %d jobs", currentLenth, desiredLength)
		log.Info(message)
		jc.TrimCache(desiredLength)
	}
	return jobSize
}

// func (jc *JobsCache) ClearCache(desiredLength int64) {
// 	jobs := jc.Jobs
// 	sort.Slice(jobs, func(i, j int) bool {
// 		return jobs[i].LastUpdate().After(jobs[j].LastUpdate())
// 	})
// 	jc.Jobs = make(Jobs, 0)
// }
