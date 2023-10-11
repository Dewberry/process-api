package jobs

import (
	"app/controllers"
	"app/utils"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/sirupsen/logrus"
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

	logger  *logrus.Logger
	logFile *os.File

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

// Update container logs
func (j *DockerJob) UpdateContainerLogs() (err error) {
	// If old status is one of the terminated status, close has already been called and container logs fetched, container killed
	switch j.Status {
	case SUCCESSFUL, DISMISSED, FAILED:
		return
	}

	j.logger.Debug("Updating container logss")
	containerLogs, err := j.fetchContainerLogs()
	if err != nil {
		j.logger.Error(err.Error())
		return
	}

	if len(containerLogs) == 0 || containerLogs == nil {
		return
	}

	// Create a new file or overwrite if it exists
	file, err := os.Create(fmt.Sprintf("%s/%s.container.jsonl", os.Getenv("LOCAL_LOGS_DIR"), j.UUID))
	if err != nil {
		return
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	for i, line := range containerLogs {
		if i != len(containerLogs)-1 {
			_, err = writer.WriteString(line + "\n")
		} else {
			_, err = writer.WriteString(line)
		}
	}

	return
}

func (j *DockerJob) LogMessage(m string, level logrus.Level) {
	switch level {
	// case 0:
	// 	j.logger.Panic(m)
	// case 1:
	// 	j.logger.Fatal(m)
	case 2:
		j.logger.Error(m)
	case 3:
		j.logger.Warn(m)
	case 4:
		j.logger.Info(m)
	case 5:
		j.logger.Debug(m)
	case 6:
		j.logger.Trace(m)
	default:
		j.logger.Info(m) // default to Info level if level is out of range
	}
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
	j.logger.Infof("Status changed to %s.", status)
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

func (j *DockerJob) initLogger() error {
	// Create a place holder file for container logs
	file, err := os.OpenFile(fmt.Sprintf("%s/%s.container.jsonl", os.Getenv("LOCAL_LOGS_DIR"), j.UUID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("failed to open log file: %s", err.Error())
	}
	file.Close()

	// Create logger for server logs
	j.logger = logrus.New()

	file, err = os.OpenFile(fmt.Sprintf("%s/%s.server.jsonl", os.Getenv("LOCAL_LOGS_DIR"), j.UUID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("failed to open log file: %s", err.Error())
	}

	j.logger.SetOutput(file)
	j.logger.SetFormatter(&logrus.JSONFormatter{})
	j.logger.SetLevel(logrus.DebugLevel)
	return nil
}

func (j *DockerJob) Create() error {

	err := j.initLogger()
	if err != nil {
		return err
	}
	j.logger.Info("Container Commands: ", j.CMD())

	ctx, cancelFunc := context.WithCancel(context.TODO())
	j.ctx = ctx
	j.ctxCancel = cancelFunc

	// At this point job is ready to be added to database
	err = j.DB.addJob(j.UUID, "accepted", time.Now(), "", "local", j.ProcessName)
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
			j.logger.Info("Context cancelled.")
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
		j.logger.Errorf("Failed creating NewDockerController. Error: %s", err.Error())
		j.NewStatusUpdate(FAILED, time.Time{})
		return
	}

	// get environment variables
	envVars := map[string]string{}
	for _, eVar := range j.EnvVars {
		envVars[eVar] = os.Getenv(eVar)
	}

	j.logger.Infof("Registered %v env vars", len(envVars))
	resources := controllers.DockerResources{}
	resources.NanoCPUs = int64(j.Resources.CPUs * 1e9)         // Docker controller needs cpu in nano ints
	resources.Memory = int64(j.Resources.Memory * 1024 * 1024) // Docker controller needs memory in bytes

	err = c.EnsureImage(j.ctx, j.Image, false)
	if err != nil {
		j.logger.Infof("Could not ensure image %s available", j.Image)
		j.NewStatusUpdate(FAILED, time.Time{})
		return
	}

	// start container
	containerID, err := c.ContainerRun(j.ctx, j.Image, j.Cmd, []controllers.VolumeMount{}, envVars, resources)
	if err != nil {
		j.logger.Errorf("Failed to run container. Error: %s", err.Error())
		j.NewStatusUpdate(FAILED, time.Time{})
		return
	}
	j.NewStatusUpdate(RUNNING, time.Time{})

	j.ContainerID = containerID

	if isCancelled() {
		return
	}

	// wait for process to finish
	exitCode, err := c.ContainerWait(j.ctx, j.ContainerID)
	if err != nil {

		j.logger.Errorf("Failed waiting for container to finish. Error: %s", err.Error())
		j.NewStatusUpdate(FAILED, time.Time{})
		return
	}

	if exitCode != 0 {
		j.logger.Errorf("Container failure, exit code: %d", exitCode)
		j.NewStatusUpdate(FAILED, time.Time{})
		return
	}

	j.logger.Info("Container process finished successfully.")
	j.NewStatusUpdate(SUCCESSFUL, time.Time{})
	go j.WriteMetaData()
}

// kill local container
func (j *DockerJob) Kill() error {
	j.logger.Info("Received dismiss signal.")
	switch j.CurrentStatus() {
	case SUCCESSFUL, FAILED, DISMISSED:
		// if these jobs have been loaded from previous snapshot they would not have context etc
		return fmt.Errorf("can't call delete on an already completed, failed, or dismissed job")
	}

	j.NewStatusUpdate(DISMISSED, time.Time{})
	// If a dismiss status is updated the job is considered dismissed at this point
	// Close being graceful or not does not matter.

	defer func() {
		go j.Close()
	}()
	return nil
}

// Write metadata at the job's metadata location
func (j *DockerJob) WriteMetaData() {
	j.logger.Info("Starting metadata writing routine.")
	j.wg.Add(1)
	defer j.wg.Done()
	defer j.logger.Info("Finished metadata writing routine.")

	c, err := controllers.NewDockerController()
	if err != nil {
		j.logger.Errorf("Could not create controller. Error: %s", err.Error())
	}

	p := process{j.ProcessID(), j.ProcessVersionID()}
	imageDigest, err := c.GetImageDigest(j.IMAGE())
	if err != nil {
		j.logger.Errorf("Error getting Image Digest: %s", err.Error())
		return
	}

	i := image{j.IMAGE(), imageDigest}

	g, s, e, err := c.GetJobTimes(j.ContainerID)
	if err != nil {
		j.logger.Errorf("Error getting job times: %s", err.Error())
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
		j.logger.Errorf("Error marshalling metadata to JSON bytes: %s", err.Error())
		return
	}

	metadataDir := os.Getenv("STORAGE_METADATA_PREFIX")
	mdLocation := fmt.Sprintf("%s/%s.json", metadataDir, j.UUID)
	err = utils.WriteToS3(j.StorageSvc, jsonBytes, mdLocation, "application/json", 0)
	if err != nil {
		return
	}
}

// func (j *DockerJob) WriteResults(data []byte) (err error) {
// 	j.logger.Info("Starting results writing routine.")
// 	defer j.logger.Info("Finished results writing routine.")

// 	resultsDir := os.Getenv("STORAGE_RESULTS_PREFIX")
// 	resultsLocation := fmt.Sprintf("%s/%s.json", resultsDir, j.UUID)
// 	fmt.Println(resultsLocation)
// 	err = utils.WriteToS3(j.StorageSvc, data, resultsLocation, "application/json", 0)
// 	if err != nil {
// 		j.logger.Info(fmt.Sprintf("error writing results to storage: %v", err.Error()))
// 	}
// 	return
// }

func (j *DockerJob) fetchContainerLogs() ([]string, error) {
	c, err := controllers.NewDockerController()
	if err != nil {
		return nil, fmt.Errorf("could not create controller to fetch container logs")
	}
	containerLogs, err := c.ContainerLog(context.TODO(), j.ContainerID)
	if err != nil {
		return nil, fmt.Errorf("could not fetch container logs")
	}
	return containerLogs, nil
}

func (j *DockerJob) RunFinished() {
	// do nothing because for local docker jobs decrementing wgRun is handeled by Run Fucntion
	// This prevents wgDone being called twice and causing panics
}

// Write final logs, cancelCtx
func (j *DockerJob) Close() {

	j.logger.Info("Starting closing routine.")
	// to do: add panic recover to remove job from active jobs even if following panics
	j.ctxCancel() // Signal Run function to terminate if running

	if j.ContainerID != "" { // Container related cleanups if container exists
		c, err := controllers.NewDockerController()
		if err != nil {
			j.logger.Errorf("Could not create controller. Error: %s", err.Error())
		} else {
			containerLogs, err := c.ContainerLog(context.TODO(), j.ContainerID)
			if err != nil {
				j.logger.Errorf("Could not fetch container logs. Error: %s", err.Error())
			}

			file, err := os.Create(fmt.Sprintf("%s/%s.container.jsonl", os.Getenv("LOCAL_LOGS_DIR"), j.UUID))
			if err != nil {
				j.logger.Errorf("Could not create container logs file. Error: %s", err.Error())
				return
			}

			writer := bufio.NewWriter(file)

			for i, line := range containerLogs {
				if i != len(containerLogs)-1 {
					_, err = writer.WriteString(line + "\n")
				} else {
					_, err = writer.WriteString(line)
				}
				if err != nil {
					j.logger.Errorf("Could not write log %s to file.", line)
				}
			}

			writer.Flush()
			file.Close()

			err = c.ContainerRemove(context.TODO(), j.ContainerID)
			if err != nil {
				j.logger.Errorf("Could not remove container. Error: %s", err.Error())
			}
		}
	}
	j.DoneChan <- j // At this point job can be safely removed from active jobs

	go func() {
		j.wg.Wait() // wait if other routines like metadata are running
		j.logFile.Close()
		UploadLogsToStorage(j.StorageSvc, j.UUID, j.ProcessName)
		// It is expected that logs will be requested multiple times for a recently finished job
		// so we are waiting for one hour to before deleting the local copy
		// so that we can avoid repetitive request to storage service.
		// If the server shutdown, these files would need to be manually deleted
		time.Sleep(time.Hour)
		DeleteLocalLogs(j.StorageSvc, j.UUID, j.ProcessName)
	}()
}
