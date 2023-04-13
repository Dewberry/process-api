package jobs

import (
	"app/controllers"
	"context"
	"fmt"
	"os"
	"time"
	"unsafe"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/labstack/gommon/log"
)

type AWSBatchJob struct {
	Ctx         context.Context
	CtxCancel   context.CancelFunc
	UUID        string `json:"jobID"`
	AWSBatchID  string
	ProcessName string   `json:"processID"`
	ImgTag      string   `json:"imageAndTag"`
	Cmd         []string `json:"commandOverride"`
	UpdateTime  time.Time
	Status      string `json:"status"`
	APILogs     []string
	Links       []Link `json:"links"`

	JobDef        string `json:"jobDefinition"`
	JobQueue      string `json:"jobQueue"`
	JobName       string `json:"jobName"`
	EnvVars       map[string]string
	BatchContext  *controllers.AWSBatchController
	LogStreamName string
}

func (j *AWSBatchJob) JobID() string {
	return j.UUID
}

func (j *AWSBatchJob) ProcessID() string {
	return j.ProcessName
}

func (j *AWSBatchJob) CMD() []string {
	return j.Cmd
}

func (j *AWSBatchJob) IMGTAG() string {
	return j.ImgTag
}

// Fetches Container logs from CloudWatch and API logs from cache
func (j *AWSBatchJob) Logs() (JobLog, error) {
	var log JobLog
	cl, err := j.FetchLogs()
	if err != nil {
		return log, err
	}
	log.ContainerLog = cl
	log.APILog = j.APILogs
	return log, nil
}

func (j *AWSBatchJob) ClearOutputs() {
	// method not invoked for aysnc jobs
}

func (j *AWSBatchJob) Messages(includeErrors bool) []string {
	return j.APILogs
}

func (j *AWSBatchJob) NewMessage(m string) {
	j.APILogs = append(j.APILogs, m)
}

func (j *AWSBatchJob) HandleError(m string) {
	j.APILogs = append(j.APILogs, m)
	j.NewStatusUpdate(FAILED)
	j.CtxCancel()
}

func (j *AWSBatchJob) LastUpdate() time.Time {
	return j.UpdateTime
}

func (j *AWSBatchJob) NewStatusUpdate(s string) {
	j.Status = s
	j.UpdateTime = time.Now()
}

func (j *AWSBatchJob) CurrentStatus() string {
	return j.Status
}

func (j *AWSBatchJob) ProviderID() string {
	return j.AWSBatchID
}

func (j *AWSBatchJob) Equals(job Job) bool {
	switch jj := job.(type) {
	case *AWSBatchJob:
		return j.Ctx == jj.Ctx
	default:
		return false
	}
}

func (j *AWSBatchJob) Create() error {
	ctx, cancelFunc := context.WithCancel(j.Ctx)
	j.Ctx = ctx
	j.CtxCancel = cancelFunc

	batchContext, err := controllers.NewAWSBatchController(os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY"), os.Getenv("AWS_DEFAULT_REGION"))
	if err != nil {
		j.HandleError(err.Error())
		return err
	}

	log.Debug("j.JobDef | ", j.JobDef)
	log.Debug("j.JobQueue | ", j.JobQueue)
	log.Debug("j.JobName  | ", j.JobName)
	aWSBatchID, err := batchContext.JobCreate(j.Ctx, j.JobDef, j.JobName, j.JobQueue, j.Cmd, j.EnvVars)
	if err != nil {
		j.HandleError(err.Error())
		return err
	}

	j.AWSBatchID = aWSBatchID
	j.BatchContext = batchContext

	// verify command in body
	if j.Cmd == nil {
		j.HandleError(err.Error())
		return err
	}

	j.NewStatusUpdate(ACCEPTED)
	return nil
}

func (j *AWSBatchJob) Run() {
	c, err := controllers.NewAWSBatchController(os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY"), os.Getenv("AWS_DEFAULT_REGION"))
	if err != nil {
		j.HandleError(err.Error())
		return
	}

	if j.AWSBatchID == "" {
		j.HandleError("AWSBatchID empty")
		return
	}

	var oldStatus string
	for {
		status, logStreamName, err := c.JobMonitor(j.AWSBatchID)
		if err != nil {
			j.HandleError(err.Error())
			return
		}

		if status != oldStatus {
			j.LogStreamName = logStreamName
			switch status {
			case "ACCEPTED":
				j.NewStatusUpdate(ACCEPTED)
			case "RUNNING":
				j.NewStatusUpdate(RUNNING)
			case "SUCCEEDED":
				// fetch results here // todo
				j.NewStatusUpdate(SUCCESSFUL)
				j.CtxCancel()
				return
			case "DISMISSED":
				j.NewStatusUpdate(DISMISSED)
				j.CtxCancel()
				return
			case "FAILED":
				j.NewStatusUpdate(FAILED)
				j.CtxCancel()
				return
			}
		}
		oldStatus = status
		time.Sleep(10 * time.Second)
	}
}

func (j *AWSBatchJob) Kill() error {
	c, err := controllers.NewAWSBatchController(os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY"), os.Getenv("AWS_DEFAULT_REGION"))
	if err != nil {
		j.HandleError(err.Error())
		return err
	}

	_, err = c.JobKill(j.AWSBatchID)
	if err != nil {
		return err
	}

	j.NewStatusUpdate(DISMISSED)
	j.CtxCancel()
	return nil
}

// Placeholder
func (j *AWSBatchJob) GetSizeinCache() int {
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
		int(unsafe.Sizeof(j.CtxCancel)) +
		int(unsafe.Sizeof(j.UUID)) + len(j.UUID) +
		int(unsafe.Sizeof(j.AWSBatchID)) + len(j.AWSBatchID) +
		int(unsafe.Sizeof(j.ImgTag)) + len(j.ImgTag) +
		int(unsafe.Sizeof(j.UpdateTime)) +
		int(unsafe.Sizeof(j.Status)) +
		int(unsafe.Sizeof(j.LogStreamName)) + len(j.LogStreamName) +
		int(unsafe.Sizeof(j.JobDef)) + len(j.JobDef) +
		int(unsafe.Sizeof(j.JobQueue)) + len(j.JobQueue) +
		int(unsafe.Sizeof(j.JobName)) + len(j.JobName) +
		int(unsafe.Sizeof(j.EnvVars)) + len(j.EnvVars)

	return totalMemory
}

// Fetches logs from CloudWatch using the AWS Go SDK
func (j *AWSBatchJob) FetchLogs() (logs []string, err error) {
	// Create a new session in the desired region
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(os.Getenv("AWS_DEFAULT_REGION")),
	})
	if err != nil {
		return logs, fmt.Errorf("Error creating session: " + err.Error())
	}

	// Create a CloudWatchLogs client
	svc := cloudwatchlogs.New(sess)

	if j.LogStreamName == "" {
		return nil, fmt.Errorf("LogStreamName is empty. If you just ran your job, retry in few seconds")
	}

	// Define the parameters for the log stream you want to read
	params := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(os.Getenv("BATCH_LOG_STREAM_GROUP")),
		LogStreamName: aws.String(j.LogStreamName),
		StartFromHead: aws.Bool(true),
	}

	// Call the GetLogEvents API to read the log events
	resp, err := svc.GetLogEvents(params)
	if err != nil {
		return logs, fmt.Errorf("Error reading log events: " + err.Error())
	}

	// Print the log events
	logs = make([]string, len(resp.Events))
	for i, event := range resp.Events {
		logs[i] = *event.Message
	}
	return logs, nil
}
