package jobs

import (
	"app/controllers"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
)

// Fields are exported so that gob can access it
type AWSBatchJob struct {
	ctx           context.Context // not exported because unsupported by gob
	ctxCancel     context.CancelFunc
	UUID          string `json:"jobID"`
	AWSBatchID    string
	ProcessName   string   `json:"processID"`
	Image         string   `json:"image"`
	Cmd           []string `json:"commandOverride"`
	UpdateTime    time.Time
	Status        string `json:"status"`
	apiLogs       []string
	containerLogs []string

	JobDef   string `json:"jobDefinition"`
	JobQueue string `json:"jobQueue"`

	// Job Name in Batch for this job
	JobName       string `json:"jobName"`
	EnvVars       map[string]string
	batchContext  *controllers.AWSBatchController
	logStreamName string
	// MetaData
	MetaDataLocation string
	ProcessVersion   string
	DB               *DB
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

func (j *AWSBatchJob) IMAGE() string {
	return j.Image
}

// Return current logs of the job.
// Fetches Container logs from CloudWatch.
func (j *AWSBatchJob) Logs() (JobLogs, error) {
	var logs JobLogs
	// we are fetching logs here and not in run function because we only want to fetch logs when needed
	if j.logStreamName != "" {
		err := j.fetchCloudWatchLogs()
		if err != nil {
			return logs, fmt.Errorf("error while fetching cloud watch logs for: %s: %s", j.logStreamName, err.Error())
		}
	}

	logs.JobID = j.UUID
	logs.ContainerLogs = j.containerLogs
	logs.APILogs = j.apiLogs
	return logs, nil
}

func (j *AWSBatchJob) ClearOutputs() {
	// method not invoked for aysnc jobs
}

func (j *AWSBatchJob) Messages(includeErrors bool) []string {
	return j.apiLogs
}

func (j *AWSBatchJob) NewMessage(m string) {
	j.apiLogs = append(j.apiLogs, m)
}

// Append error to apiLogs, cancelCtx, update Status, and time, write logs to database
func (j *AWSBatchJob) HandleError(m string) {
	j.apiLogs = append(j.apiLogs, m)
	j.ctxCancel()
	if j.Status != DISMISSED { // if job dismissed then the error is because of dismissing job
		j.NewStatusUpdate(FAILED)
		j.fetchCloudWatchLogs()
		go j.DB.upsertLogs(j.UUID, j.apiLogs, j.containerLogs)
	}
}

func (j *AWSBatchJob) LastUpdate() time.Time {
	return j.UpdateTime
}

func (j *AWSBatchJob) NewStatusUpdate(s string) {
	j.Status = s
	now := time.Now()
	j.UpdateTime = now
	j.DB.updateJobRecord(j.UUID, s, now)
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
		return j.ctx == jj.ctx
	default:
		return false
	}
}

func (j *AWSBatchJob) Create() error {
	ctx, cancelFunc := context.WithCancel(context.TODO())
	j.ctx = ctx
	j.ctxCancel = cancelFunc

	batchContext, err := controllers.NewAWSBatchController(os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY"), os.Getenv("AWS_DEFAULT_REGION"))
	if err != nil {
		j.ctxCancel()
		return err
	}

	aWSBatchID, err := batchContext.JobCreate(j.ctx, j.JobDef, j.JobName, j.JobQueue, j.Cmd, j.EnvVars)
	if err != nil {
		j.ctxCancel()
		return err
	}

	j.AWSBatchID = aWSBatchID
	j.batchContext = batchContext

	// verify command in body
	if j.Cmd == nil {
		j.ctxCancel()
		return err
	}

	// At this point job is ready to be added to database
	err = j.DB.addJob(j.UUID, "accepted", time.Now(), "", "aws-batch", j.ProcessName)
	if err != nil {
		j.ctxCancel()
		return err
	}

	j.NewStatusUpdate(ACCEPTED)
	return nil
}

// Thid actually does not run a job but only monitors it
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
			j.logStreamName = logStreamName
			switch status {
			case "ACCEPTED":
				j.NewStatusUpdate(ACCEPTED)
			case "RUNNING":
				j.NewStatusUpdate(RUNNING)
			case "SUCCEEDED":
				// fetch results here // todo
				j.NewStatusUpdate(SUCCESSFUL)
				j.ctxCancel()
				j.fetchCloudWatchLogs()
				go j.DB.upsertLogs(j.UUID, j.apiLogs, j.containerLogs)
				go j.WriteMeta(c)
				return
			case "DISMISSED":
				j.NewStatusUpdate(DISMISSED)
				j.ctxCancel()
				j.fetchCloudWatchLogs()
				go j.DB.upsertLogs(j.UUID, j.apiLogs, j.containerLogs)
				return
			case "FAILED":
				j.HandleError("Batch API returned failed status")
				return
			}
		}
		oldStatus = status
		time.Sleep(10 * time.Second)
	}
}

func (j *AWSBatchJob) Kill() error {
	switch j.CurrentStatus() {
	case SUCCESSFUL, FAILED, DISMISSED:
		// if these jobs have been loaded from previous snapshot they would not have context etc
		return fmt.Errorf("can't call delete on an already completed, failed, or dismissed job")
	}

	c, err := controllers.NewAWSBatchController(os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY"), os.Getenv("AWS_DEFAULT_REGION"))
	if err != nil {
		j.HandleError(err.Error())
		return err
	}

	_, err = c.JobKill(j.AWSBatchID)
	if err != nil {
		j.HandleError(err.Error())
		return err
	}

	j.NewStatusUpdate(DISMISSED)
	// this would be redundent in most cases because the run function will also update status and add logs
	// but leaving it here in case run function fails
	j.fetchCloudWatchLogs()
	go j.DB.upsertLogs(j.UUID, j.apiLogs, j.containerLogs)
	j.ctxCancel()
	return nil
}

// Fetches logs from CloudWatch using the AWS Go SDK
func (j *AWSBatchJob) fetchCloudWatchLogs() error {
	// Create a new session in the desired region
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(os.Getenv("AWS_DEFAULT_REGION")),
	})
	if err != nil {
		return fmt.Errorf("Error creating session: " + err.Error())
	}

	// Create a CloudWatchLogs client
	svc := cloudwatchlogs.New(sess)

	if j.logStreamName == "" {
		return fmt.Errorf("logStreamName is empty. If you just ran your job, retry in few seconds")
	}

	// Define the parameters for the log stream you want to read
	params := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(os.Getenv("BATCH_LOG_STREAM_GROUP")),
		LogStreamName: aws.String(j.logStreamName),
		StartFromHead: aws.Bool(true),
	}

	// Call the GetLogEvents API to read the log events
	resp, err := svc.GetLogEvents(params)
	if err != nil {
		if err.Error() == "ResourceNotFoundException: The specified log stream does not exist." {
			return nil
		} else {
			return err
		}
	}

	// Print the log events
	logs := make([]string, len(resp.Events))
	for i, event := range resp.Events {
		logs[i] = *event.Message
	}
	j.containerLogs = logs
	return nil
}
