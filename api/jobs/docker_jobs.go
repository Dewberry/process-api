package jobs

import (
	"app/controllers"
	"app/utils"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/service/s3"
)

type DockerJob struct {
	ctx       context.Context
	ctxCancel context.CancelFunc
	// Used for monitoring meta data and other routines
	wg sync.WaitGroup
	// Used for monitoring running complete for sync jobs
	wgRun sync.WaitGroup

	UUID           string `json:"jobID"`
	ContainerID    string
	Image          string `json:"image"`
	ProcessName    string `json:"processID"`
	ProcessVersion string `json:"processVersion"`
	EnvVars        []string
	Cmd            []string `json:"commandOverride"`
	UpdateTime     time.Time
	Status         string `json:"status"`
	apiLogs        []string
	containerLogs  []string
	Resources
	DB         *DB
	StorageSvc *s3.S3
	DoneChan   chan Job
}

func (j *DockerJob) WaitForRunCompletion() {
	j.wgRun.Wait()
}

func (j *DockerJob) JobID() string {
	return j.UUID
}

func (j *DockerJob) ProcessID() string {
	return j.ProcessName
}

func (j *DockerJob) ProcessVersionID() string {
	return j.ProcessVersion
}

func (j *DockerJob) CMD() []string {
	return j.Cmd
}

func (j *DockerJob) IMAGE() string {
	return j.Image
}

// Return current logs of the job
func (j *DockerJob) Logs() (JobLogs, error) {
	var logs JobLogs

	logs.JobID = j.UUID
	logs.ProcessID = j.ProcessName
	logs.ContainerLogs = j.containerLogs
	logs.APILogs = j.apiLogs
	return logs, nil
}

func (j *DockerJob) Messages(includeErrors bool) []string {
	return j.apiLogs
}

func (j *DockerJob) NewMessage(m string) {
	j.apiLogs = append(j.apiLogs, m)
}

func (j *DockerJob) LastUpdate() time.Time {
	return j.UpdateTime
}

func (j *DockerJob) NewStatusUpdate(status string, updateTime time.Time) {

	// If old status is one of the terminated status, it should not update status.
	switch j.Status {
	case SUCCESSFUL, DISMISSED, FAILED:
		return
	}

	j.Status = status
	if updateTime.IsZero() {
		j.UpdateTime = time.Now()
	} else {
		j.UpdateTime = updateTime
	}
	j.DB.updateJobRecord(j.UUID, status, j.UpdateTime)
}

func (j *DockerJob) CurrentStatus() string {
	return j.Status
}

func (j *DockerJob) ProviderID() string {
	return j.ContainerID
}

func (j *DockerJob) Equals(job Job) bool {
	switch jj := job.(type) {
	case *DockerJob:
		return j.ctx == jj.ctx
	default:
		return false
	}
}

func (j *DockerJob) Create() error {
	ctx, cancelFunc := context.WithCancel(context.TODO())
	j.ctx = ctx
	j.ctxCancel = cancelFunc

	// At this point job is ready to be added to database
	err := j.DB.addJob(j.UUID, "accepted", time.Now(), "", "local", j.ProcessName)
	if err != nil {
		j.ctxCancel()
		return err
	}

	j.NewStatusUpdate(ACCEPTED, time.Time{})
	j.wgRun.Add(1)
	go j.Run()
	return nil
}

func (j *DockerJob) Run() {

	// Helper function to check if context is cancelled.
	isCancelled := func() bool {
		select {
		case <-j.ctx.Done():
			j.NewMessage("Context cancelled")
			return true
		default:
			return false
		}
	}

	// defers are executed in LIFO order
	// swap the order of following if results are posted/written by the container, and run close as a coroutine
	defer j.wgRun.Done()
	defer func() {
		if !isCancelled() {
			j.Close()
		}
	}()

	c, err := controllers.NewDockerController()
	if err != nil {
		j.NewMessage("Failed creating NewDockerController. Error: " + err.Error())
		j.NewStatusUpdate(FAILED, time.Time{})
		return
	}

	// get environment variables
	envVars := map[string]string{}
	for _, eVar := range j.EnvVars {
		envVars[eVar] = os.Getenv(eVar)
	}

	j.NewMessage(fmt.Sprintf("Registered %v env vars", len(envVars)))
	resources := controllers.DockerResources{}
	resources.NanoCPUs = int64(j.Resources.CPUs * 1e9)         // Docker controller needs cpu in nano ints
	resources.Memory = int64(j.Resources.Memory * 1024 * 1024) // Docker controller needs memory in bytes

	err = c.EnsureImage(j.ctx, j.Image, false)
	if err != nil {
		j.NewMessage(fmt.Sprintf("Could not ensure image %s available", j.Image))
		j.NewStatusUpdate(FAILED, time.Time{})
		return
	}

	// start container
	j.NewStatusUpdate(RUNNING, time.Time{})
	containerID, err := c.ContainerRun(j.ctx, j.Image, j.Cmd, []controllers.VolumeMount{}, envVars, resources)
	if err != nil {
		j.NewMessage("Failed to run container. Error: " + err.Error())
		j.NewStatusUpdate(FAILED, time.Time{})
		return
	}

	j.ContainerID = containerID

	if isCancelled() {
		return
	}

	// wait for process to finish
	exitCode, err := c.ContainerWait(j.ctx, j.ContainerID)
	if err != nil {

		j.NewMessage("Failed waiting for container to finish. Error: " + err.Error())
		j.NewStatusUpdate(FAILED, time.Time{})
		return
	}

	if exitCode != 0 {
		j.NewMessage(fmt.Sprintf("Container failure, exit code: %d ", exitCode))
		j.NewStatusUpdate(FAILED, time.Time{})
		return
	}

	j.NewMessage("Container process finished successfully.")
	j.NewStatusUpdate(SUCCESSFUL, time.Time{})
	go j.WriteMetaData()
}

// kill local container
func (j *DockerJob) Kill() error {
	j.NewMessage("Received dismiss signal")
	switch j.CurrentStatus() {
	case SUCCESSFUL, FAILED, DISMISSED:
		// if these jobs have been loaded from previous snapshot they would not have context etc
		return fmt.Errorf("can't call delete on an already completed, failed, or dismissed job")
	}

	j.NewStatusUpdate(DISMISSED, time.Time{})
	// If a dismiss status is updated the job is considered dismissed at this point
	// Close being graceful or not does not matter.

	defer j.Close()
	return nil
}

// Write metadata at the job's metadata location
func (j *DockerJob) WriteMetaData() {
	j.NewMessage("Starting metadata writing routine.")
	j.wg.Add(1)
	defer j.wg.Done()
	defer j.NewMessage("Finished metadata writing routine.")

	c, err := controllers.NewDockerController()
	if err != nil {
		j.NewMessage("Could not create controller. Error: " + err.Error())
	}

	p := process{j.ProcessID(), j.ProcessVersionID()}
	imageDigest, err := c.GetImageDigest(j.IMAGE())
	if err != nil {
		j.NewMessage(fmt.Sprintf("Error getting imageDigest: %s", err.Error()))
		return
	}

	i := image{j.IMAGE(), imageDigest}

	g, s, e, err := c.GetJobTimes(j.ContainerID)
	if err != nil {
		j.NewMessage(fmt.Sprintf("Error getting jobtimes: %s", err.Error()))
		return
	}

	md := metaData{
		Context:         "https://github.com/Dewberry/process-api/blob/main/context.jsonld",
		JobID:           j.UUID,
		Process:         p,
		Image:           i,
		Commands:        j.Cmd,
		GeneratedAtTime: g,
		StartedAtTime:   s,
		EndedAtTime:     e,
	}

	jsonBytes, err := json.Marshal(md)
	if err != nil {
		j.NewMessage(fmt.Sprintf("Error marshalling metadata: %s", err.Error()))
		return
	}

	metadataDir := os.Getenv("STORAGE_METADATA_DIR")
	mdLocation := fmt.Sprintf("%s/%s.json", metadataDir, j.UUID)
	err = utils.WriteToS3(j.StorageSvc, jsonBytes, mdLocation, "application/json", 0)
	if err != nil {
		return
	}
}

// func (j *DockerJob) WriteResults(data []byte) (err error) {
// 	j.NewMessage("Starting results writing routine.")
// 	defer j.NewMessage("Finished results writing routine.")

// 	resultsDir := os.Getenv("STORAGE_RESULTS_DIR")
// 	resultsLocation := fmt.Sprintf("%s/%s.json", resultsDir, j.UUID)
// 	fmt.Println(resultsLocation)
// 	err = utils.WriteToS3(j.StorageSvc, data, resultsLocation, "application/json", 0)
// 	if err != nil {
// 		j.NewMessage(fmt.Sprintf("error writing results to storage: %v", err.Error()))
// 	}
// 	return
// }

func (j *DockerJob) RunFinished() {
	// do nothing because for local docker jobs decrementing wgRun is handeled by Run Fucntion
	// This prevents wgDone being called twice and causing panics
}

// Write final logs, cancelCtx
func (j *DockerJob) Close() {
	j.ctxCancel() // Signal Run function to terminate if running

	if j.ContainerID != "" { // Container related cleanups if container exists
		c, err := controllers.NewDockerController()
		if err != nil {
			j.NewMessage("Could not create controller. Error: " + err.Error())
		} else {
			containerLogs, err := c.ContainerLog(context.TODO(), j.ContainerID)
			if err != nil {
				j.NewMessage("Could not fetch container logs. Error: " + err.Error())
			}
			j.containerLogs = containerLogs

			err = c.ContainerRemove(context.TODO(), j.ContainerID)
			if err != nil {
				j.NewMessage("Could not remove container. Error: " + err.Error())
			}
		}
	}
	j.wg.Wait() // wait if other routines like metadata are running
	// this should be completed before job is sent to Done because logs are handled differently for active and unactive jobs
	j.DB.upsertLogs(j.UUID, j.ProcessID(), j.apiLogs, j.containerLogs)
	j.DoneChan <- j // At this point job can be safely removed from active jobs
}
