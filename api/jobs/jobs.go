package jobs

import (
	"app/utils"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go/service/s3"
)

type Resources struct {
	CPUs   float32
	Memory int
}

// Job refers to any process that has been created through
// the processes/{processID}/execution endpoint
type Job interface {
	CMD() []string
	CurrentStatus() string
	Equals(Job) bool
	IMAGE() string
	JobID() string
	ProcessID() string
	Logs() (JobLogs, error)
	Kill() error
	LastUpdate() time.Time
	Messages(bool) []string
	NewMessage(string)
	NewStatusUpdate(string)
	Run()
	Create() error
	Results() (map[string]interface{}, error)
}

// JobRecord contains details about a job
type JobRecord struct {
	JobID      string    `json:"jobID"`
	LastUpdate time.Time `json:"updated"`
	Status     string    `json:"status"`
	ProcessID  string    `json:"processID"`
	Type       string    `default:"process" json:"type"`
	Host       string    `json:"host,omitempty"`
	Mode       string    `json:"mode,omitempty"`
}

// JobLogs describes logs for the job
type JobLogs struct {
	JobID         string   `json:"jobID"`
	ContainerLogs []string `json:"container_logs"`
	APILogs       []string `json:"api_logs"`
}

// Prettify JobLogs by replacing nil with empty []string{}
func (jl *JobLogs) Prettify() {
	if jl.ContainerLogs == nil {
		jl.ContainerLogs = []string{}
	}
	if jl.APILogs == nil {
		jl.APILogs = []string{}
	}
}

// OGCStatusCodes
const (
	ACCEPTED   = "accepted"
	RUNNING    = "running"
	SUCCESSFUL = "successful"
	FAILED     = "failed"
	DISMISSED  = "dismissed"
)

// Returns an array of all Job statuses in memory
// Most recently updated job first
func (ac *ActiveJobs) ListJobs() []JobRecord {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	jobs := make([]JobRecord, len(ac.Jobs))

	var i int
	for _, j := range ac.Jobs {
		js := JobRecord{
			ProcessID:  (*j).ProcessID(),
			JobID:      (*j).JobID(),
			LastUpdate: (*j).LastUpdate(),
			Status:     (*j).CurrentStatus(),
		}
		jobs[i] = js
		i++
	}

	// sort the jobs in order with most recent time first
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].LastUpdate.After(jobs[j].LastUpdate)
	})

	return jobs
}

// TODO: Add to db
func FetchResults(svc *s3.S3, jid string) (interface{}, error) {
	return nil, nil
}

// TODO: Add to db
func FetchMeta(svc *s3.S3, jid string) (interface{}, error) {
	key := fmt.Sprintf("%s/%s.json", os.Getenv("S3_META_DIR"), jid)

	exist, err := utils.KeyExists(key, svc)
	if err != nil {
		return nil, err
	}

	if !exist {
		return nil, fmt.Errorf("not found")
	}

	data, err := utils.GetS3JsonData(key, svc)
	if err != nil {
		return nil, err
	}

	return data, nil
}
