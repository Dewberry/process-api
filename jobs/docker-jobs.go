package jobs

import (
	"app/controllers"
	"app/utils"
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"
	"unsafe"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

type DockerJob struct {
	Ctx         context.Context
	ctxCancel   context.CancelFunc
	UUID        string `json:"jobID"`
	ContainerID string
	Repository  string `json:"repository"` // for local repositories leave empty
	ImgTag      string `json:"imageAndTag"`
	ProcessName string `json:"processID"`
	EnvVars     []string
	Cmd         []string `json:"commandOverride"`
	UpdateTime  time.Time
	Status      string `json:"status"`
	APILogs     []string
	Links       []Link `json:"links"`
}

func (j *DockerJob) JobID() string {
	return j.UUID
}

func (j *DockerJob) ProcessID() string {
	return j.ProcessName
}

func (j *DockerJob) CMD() []string {
	return j.Cmd
}

func (j *DockerJob) IMGTAG() string {
	return j.ImgTag
}

// Fetches Container logs from S3 and API logs from cache
func (j *DockerJob) Logs() (JobLogs, error) {
	var logs JobLogs
	cl, err := j.FetchLogs()
	if err != nil {
		return JobLogs{}, err
	}
	logs.ContainerLog = cl
	logs.APILog = j.APILogs
	return logs, nil
}

func (j *DockerJob) Messages(includeErrors bool) []string {
	return j.APILogs
}

func (j *DockerJob) NewMessage(m string) {
	j.APILogs = append(j.APILogs, m)
}

func (j *DockerJob) HandleError(m string) {
	j.APILogs = append(j.APILogs, m)
	j.NewStatusUpdate(FAILED)
	j.ctxCancel()
}

func (j *DockerJob) LastUpdate() time.Time {
	return j.UpdateTime
}

func (j *DockerJob) NewStatusUpdate(s string) {
	j.Status = s
	j.UpdateTime = time.Now()
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
		return j.Ctx == jj.Ctx
	default:
		return false
	}
}

func (j *DockerJob) Create() error {
	ctx, cancelFunc := context.WithCancel(j.Ctx)
	j.Ctx = ctx
	j.ctxCancel = cancelFunc

	c, err := controllers.NewDockerController()
	if err != nil {
		j.HandleError(err.Error())
		return err
	}

	// verify command in body
	if j.Cmd == nil {
		j.HandleError(err.Error())
		return err
	}

	// pull image
	if j.Repository != "" {
		err = c.EnsureImage(ctx, j.ImgTag, false)
		if err != nil {
			j.HandleError(err.Error())
			return err
		}
	}

	j.NewStatusUpdate(ACCEPTED)
	return nil
}

func (j *DockerJob) Run() {
	c, err := controllers.NewDockerController()
	if err != nil {
		j.HandleError(err.Error())
		return
	}

	// get environment variables
	envVars := map[string]string{}
	for _, eVar := range j.EnvVars {
		envVars[eVar] = os.Getenv(eVar)
	}

	// start container
	j.NewStatusUpdate(RUNNING)
	containerID, err := c.ContainerRun(j.Ctx, j.ImgTag, j.Cmd, []controllers.VolumeMount{}, envVars)
	if err != nil {
		j.HandleError(err.Error())
		return
	}

	j.ContainerID = containerID

	// wait for process to finish
	statusCode, errWait := c.ContainerWait(j.Ctx, j.ContainerID)
	logs, errLog := c.ContainerLog(j.Ctx, j.ContainerID)

	// Creating new routine so that failure of writing logs does not mean failure of job
	// This function does not panic
	go utils.WriteToS3(strings.Join(logs, "\n"), fmt.Sprintf("%s/%s.txt", os.Getenv("S3_LOGS_DIR"), j.UUID), &j.APILogs, "text/plain")

	// If there are error messages remove container before cancelling context inside Handle Error
	for _, err := range []error{errWait, errLog} {
		if err != nil {
			errRem := c.ContainerRemove(j.Ctx, j.ContainerID)
			if errRem != nil {
				j.HandleError(err.Error() + " " + errRem.Error())
				return
			}
			j.HandleError(err.Error())
			return
		}
	}

	if statusCode != 0 {
		errRem := c.ContainerRemove(j.Ctx, j.ContainerID)
		if errRem != nil {
			j.HandleError(fmt.Sprintf("container exit code: %d", statusCode) + " " + errRem.Error())
			return
		}
		j.HandleError(fmt.Sprintf("container exit code: %d", statusCode))
		return
	} else if statusCode == 0 {
		j.NewStatusUpdate(SUCCESSFUL)
	}

	// clean up the finished job
	err = c.ContainerRemove(j.Ctx, j.ContainerID)
	if err != nil {
		j.HandleError(err.Error())
		return
	}

	j.ctxCancel()
}

// kill local container
func (j *DockerJob) Kill() error {
	switch j.CurrentStatus() {
	case SUCCESSFUL, FAILED, DISMISSED:
		// if these jobs have been loaded from previous snapshot they would not have context etc
		return fmt.Errorf("can't call delete on an already completed, failed, or dismissed job")
	}

	c, err := controllers.NewDockerController()
	if err != nil {
		j.NewMessage(err.Error())
	}

	err = c.ContainerKillAndRemove(j.Ctx, j.ContainerID, "KILL")
	if err != nil {
		j.HandleError(err.Error())
	}

	j.NewStatusUpdate(DISMISSED)
	j.ctxCancel()
	return nil
}

func (j *DockerJob) GetSizeinCache() int {
	cmdData := int(unsafe.Sizeof(j.Cmd))
	for _, item := range j.Cmd {
		cmdData += len(item)
	}

	messageData := int(unsafe.Sizeof(j.APILogs))
	for _, item := range j.APILogs {
		messageData += len(item)
	}

	// not calculated appropriately, add method...
	linkData := int(unsafe.Sizeof(j.Links))

	totalMemory := cmdData + messageData + linkData +
		int(unsafe.Sizeof(j.Ctx)) +
		int(unsafe.Sizeof(j.ctxCancel)) +
		int(unsafe.Sizeof(j.UUID)) + len(j.UUID) +
		int(unsafe.Sizeof(j.ContainerID)) + len(j.ContainerID) +
		int(unsafe.Sizeof(j.ImgTag)) + len(j.ImgTag) +
		int(unsafe.Sizeof(j.UpdateTime)) +
		int(unsafe.Sizeof(j.Status))
	return totalMemory
}

// If JobID exists but log file doesn't then it raises an error
// Assumes jobID is valid and the process is sync
func (j *DockerJob) FetchLogs() ([]string, error) {
	// Set up a session with AWS credentials and region
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	svc := s3.New(sess)

	bucket := os.Getenv("S3_BUCKET")
	key := fmt.Sprintf("%s/%s.txt", os.Getenv("S3_LOGS_DIR"), j.UUID)

	exist, err := utils.KeyExists(key, svc)
	if err != nil {
		return nil, err
	}

	if !exist {
		return nil, fmt.Errorf("not found")
	}

	// Create a new S3GetObjectInput object to specify the file to read
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

	scanner := bufio.NewScanner(resp.Body)
	var logs []string

	for scanner.Scan() {
		logs = append(logs, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return logs, nil
}
