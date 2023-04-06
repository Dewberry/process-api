package jobs

import (
	"app/utils"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
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
	Logs() (map[string][]string, error)
	Kill() error
	LastUpdate() time.Time
	Messages(bool) []string
	NewMessage(string)
	NewStatusUpdate(string)
	Run()
	Create() error
	GetSizeinCache() int
}

type JobStatus struct {
	JobID      string    `json:"jobID"`
	LastUpdate time.Time `json:"updated"`
	Status     string    `json:"status"`
	ProcessID  string    `json:"processID"`
	CMD        []string  `json:"commands,omitempty"`
	Type       string    `default:"process" json:"type"`
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
	Inputs  map[string]interface{} `json:"inputs"`
	EnvVars map[string]string      `json:"environmentVariables"`
}

type JobsCache struct {
	Jobs             map[string]*Job `json:"jobs"`
	MaxSizeBytes     uint64          `json:"maxCacheBytes"`
	TrimThreshold    float64         `json:"cacheTrimThreshold"`
	CurrentSizeBytes uint64          `json:"currentCacheBytes"`
	mu               sync.Mutex
}

func (jc *JobsCache) Add(j *Job) {
	jc.mu.Lock()
	defer jc.mu.Unlock()
	jc.Jobs[(*j).JobID()] = j
}

func (jc *JobsCache) Remove(j *Job) {
	jc.mu.Lock()
	defer jc.mu.Unlock()

	delete(jc.Jobs, (*j).JobID())
}

// Returns an array of all Job statuses in memory
// Most recently updated job first
func (jc *JobsCache) ListJobs() []JobStatus {
	jc.mu.Lock()
	defer jc.mu.Unlock()

	jobs := make([]JobStatus, len(jc.Jobs))

	var i int
	for _, j := range jc.Jobs {
		jobStatus := JobStatus{
			ProcessID:  (*j).ProcessID(),
			JobID:      (*j).JobID(),
			LastUpdate: (*j).LastUpdate(),
			Status:     (*j).CurrentStatus(),
			CMD:        (*j).CMD(),
		}
		jobs[i] = jobStatus
		i++
	}

	// sort the jobs in order with most recent time first
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].LastUpdate.After(jobs[j].LastUpdate)
	})

	return jobs
}

func (jc *JobsCache) DumpCacheToFile() error {
	// create a file to write the serialized data to
	err := os.MkdirAll(".data", os.ModePerm)
	if err != nil {
		return err
	}
	file, err := os.Create(".data/snapshot.gob.tmp")
	if err != nil {
		return err
	}
	defer file.Close()

	gob.Register(&DockerJob{})
	gob.Register(&AWSBatchJob{})

	// create an encoder and use it to serialize the map to the file
	encoder := gob.NewEncoder(file)
	err = encoder.Encode(jc.Jobs)
	if err != nil {
		return err
	}
	file.Close()
	// saving it to tmp is better because
	// if the gob panics then the existing snapshot is still untouched
	err = os.Rename(".data/snapshot.gob.tmp", ".data/snapshot.gob")
	if err != nil {
		return fmt.Errorf("error moving file: %v", err.Error())
	}

	return nil
}

func (jc *JobsCache) LoadCacheFromFile() error {

	jc.Jobs = make(map[string]*Job)

	// create a file to read the serialized data from
	file, err := os.Open(".data/snapshot.gob")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	defer file.Close()

	gob.Register(&DockerJob{})
	gob.Register(&AWSBatchJob{})

	// create a decoder and use it to deserialize the people map from the file
	decoder := gob.NewDecoder(file)
	err = decoder.Decode(&jc.Jobs)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	fmt.Println("Starting from snapshot saved at .data/snapshot.gob")
	return nil
}

func (jc *JobsCache) TrimCache(desiredLength int64) {
	jc.mu.Lock()
	defer jc.mu.Unlock()

	jobIDs := make([]string, len(jc.Jobs))

	var i int
	for k := range jc.Jobs {
		jobIDs[i] = k
		i++
	}

	// sort the jobIDs in reverse order with most recent time first
	sort.Slice(jobIDs, func(i, j int) bool {
		return (*jc.Jobs[jobIDs[i]]).LastUpdate().After((*jc.Jobs[jobIDs[j]]).LastUpdate())
	})

	// delete these records from the map
	for _, jid := range jobIDs[0:desiredLength] {
		delete(jc.Jobs, jid)
	}
}

// Revised to kill only currently active jobs
func (jc *JobsCache) KillAll() error {
	jc.mu.Lock()
	defer jc.mu.Unlock()

	for _, j := range jc.Jobs {
		if (*j).CurrentStatus() == ACCEPTED || (*j).CurrentStatus() == RUNNING {
			if err := (*j).Kill(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (jc *JobsCache) CheckCache() uint64 {
	// jc.mu.Lock()
	// defer jc.mu.Unlock()

	// calculate total size of cache as of now
	var currentSizeBytes uint64
	for _, j := range jc.Jobs {
		currentSizeBytes += uint64((*j).GetSizeinCache())
	}
	jc.CurrentSizeBytes = currentSizeBytes

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
	return currentSizeBytes
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
