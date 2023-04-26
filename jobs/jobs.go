package jobs

import (
	"app/utils"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
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
	Logs() (JobLogs, error)
	Kill() error
	LastUpdate() time.Time
	Messages(bool) []string
	NewMessage(string)
	NewStatusUpdate(string)
	Run()
	Create() error
	GetSizeinCache() int
}

// JobStatus describes status of a job
type JobStatus struct {
	JobID      string    `json:"jobID"`
	LastUpdate time.Time `json:"updated"`
	Status     string    `json:"status"`
	ProcessID  string    `json:"processID"`
	CMD        []string  `json:"commands,omitempty"`
	Type       string    `default:"process" json:"type"`
}

// JobLogs describes logs for the job
type JobLogs struct {
	ContainerLog []string `json:"container_log"`
	APILog       []string `json:"api_log"`
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
func (jc *JobsCache) ListJobs() []JobStatus {
	jc.mu.Lock()
	defer jc.mu.Unlock()

	jobs := make([]JobStatus, len(jc.Jobs))

	var i int
	for _, j := range jc.Jobs {
		js := JobStatus{
			ProcessID:  (*j).ProcessID(),
			JobID:      (*j).JobID(),
			LastUpdate: (*j).LastUpdate(),
			Status:     (*j).CurrentStatus(),
			CMD:        (*j).CMD(),
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

// If JobID exists but results file doesn't then it raises an error
// Assumes jobID is valid
func FetchResults(svc *s3.S3, jid string) (interface{}, error) {
	bucket := os.Getenv("S3_BUCKET")
	key := fmt.Sprintf("%s/%s.json", os.Getenv("S3_RESULTS_DIR"), jid)

	exist, err := utils.KeyExists(key, svc)
	if err != nil {
		return nil, err
	}

	if !exist {
		return nil, fmt.Errorf("not found")
	}

	// Create a new S3GetObjectInput object to specify the file you want to read
	params := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	// Use the S3 service object to download the file into a byte slice
	resp, err := svc.GetObject(params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read the file contents into a byte slice
	jsonBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Declare an empty interface{} value to hold the unmarshalled data
	var data interface{}

	// Unmarshal the JSON data into the interface{} value
	err = json.Unmarshal(jsonBytes, &data)
	if err != nil {
		return nil, err
	}

	return data, nil
}
