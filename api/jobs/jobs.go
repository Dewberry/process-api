package jobs

import (
	"app/utils"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
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
	ProcessVersionID() string

	// Logs must first fetch the current logs before returning
	Logs() (JobLogs, error)
	Kill() error
	LastUpdate() time.Time
	Messages(bool) []string
	NewMessage(string)

	// NewStatusUpdate must update the status of the AWSBatchJob to the provided status string.
	// If a zero-value time is provided as updateTime, the current time (time.Now()) should be set as the UpdateTime.
	// Otherwise, the provided updateTime should be set as the UpdateTime.
	// This function should also update the job record in the database with the new status and UpdateTime.
	// If old status is one of the terminated status, it should not update status.
	NewStatusUpdate(string, time.Time)

	// Create must change job status to accepted
	// At this point job should be ready to be processed and added to database
	Create() error

	WriteMetaData()
	// WriteResults([]byte) error

	// WaitForRunCompletion must wait until all job is completed.
	WaitForRunCompletion()

	// Decrement Run Waitgroup
	RunFinished()

	// Pefrom any cleanup such as cancelling context etc
	Close()
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
	ProcessID     string   `json:"processID"`
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
	ACCEPTED   string = "accepted"
	RUNNING    string = "running"
	SUCCESSFUL string = "successful"
	FAILED     string = "failed"
	DISMISSED  string = "dismissed"
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

// FetchResults by parsing logs
// Assumes last log will be results always
func FetchResults(db *DB, jid string) (interface{}, error) {

	logs, err := db.GetLogs(jid)
	if err != nil {
		return nil, err
	}

	containerLogs := logs.ContainerLogs
	lastLogIdx := len(containerLogs) - 1
	if lastLogIdx < 0 {
		return nil, fmt.Errorf("no contnainer logs available")
	}

	lastLog := containerLogs[lastLogIdx]
	lastLog = strings.ReplaceAll(lastLog, "'", "\"")

	var data map[string]interface{}
	err = json.Unmarshal([]byte(lastLog), &data)
	if err != nil {
		return nil, fmt.Errorf(`unable to parse results, expected {"plugin_results": {....}}, found : %s. Error: %s`, lastLog, err.Error())
	}

	pluginResults, ok := data["plugin_results"]
	if !ok {
		return nil, fmt.Errorf("'plugin_results' key not found")
	}

	return pluginResults, nil
}

// // If JobID exists but results file doesn't then it raises an error
// // Assumes jobID is valid
// func FetchResults(svc *s3.S3, jid string) (interface{}, error) {
// 	key := fmt.Sprintf("%s/%s.json", os.Getenv("STORAGE_RESULTS_DIR"), jid)

// 	exist, err := utils.KeyExists(key, svc)
// 	if err != nil {
// 		return nil, err
// 	}

// 	if !exist {
// 		return nil, fmt.Errorf("not found")
// 	}

// 	data, err := utils.GetS3JsonData(key, svc)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return data, nil
// }

// If JobID exists but metadata file doesn't then it raises an error
// Assumes jobID is valid
func FetchMeta(svc *s3.S3, jid string) (interface{}, error) {
	key := fmt.Sprintf("%s/%s.json", os.Getenv("STORAGE_METADATA_DIR"), jid)

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
