package jobs

import (
	"app/controllers"
	"context"
	"os"
	"time"
	"unsafe"
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
	MessageList []string
	LogInfo     string
	Links       []Link        `json:"links"`
	Outputs     []interface{} `json:"outputs"`

	JobDef   string `json:"jobDefinition"`
	JobQueue string `json:"jobQueue"`
	JobName  string `json:"jobName"`
	EnvVars  map[string]string
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

func (j *AWSBatchJob) JobLogs() string {
	return j.LogInfo
}

func (j *AWSBatchJob) JobOutputs() []interface{} {
	return j.Outputs
}

func (j *AWSBatchJob) ClearOutputs() {
	// method not invoked for aysnc jobs
}

func (j *AWSBatchJob) Messages(includeErrors bool) []string {
	return j.MessageList
}

func (j *AWSBatchJob) NewMessage(m string) {
	j.MessageList = append(j.MessageList, m)
}

func (j *AWSBatchJob) NewErrorMessage(m string) {
	j.MessageList = append(j.MessageList, m)
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

// set withLogs to false for batch jobs, logs can be retrieved from cloudwatch
func (j *AWSBatchJob) Create() error {
	ctx, cancelFunc := context.WithCancel(j.Ctx)
	j.Ctx = ctx
	j.CtxCancel = cancelFunc

	// _, err := controllers.NewAWSBatchController(os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY"), os.Getenv("AWS_DEFAULT_REGION"))
	// if err != nil {
	// 	j.NewErrorMessage(err.Error())
	// 	return err
	// }

	// // verify command in body
	// if j.Cmd == nil {
	// 	j.NewErrorMessage(err.Error())
	// 	return err
	// }

	j.NewStatusUpdate(ACCEPTED)
	return nil
}

// set withLogs to false for batch jobs, logs can be retrieved from cloudwatch
func (j *AWSBatchJob) Run() {
	c, err := controllers.NewAWSBatchController(os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY"), os.Getenv("AWS_DEFAULT_REGION"))
	if err != nil {
		j.NewErrorMessage(err.Error())
		return
	}

	j.AWSBatchID, err = c.JobCreate(j.Ctx, j.JobDef, j.JobName, j.JobQueue, j.Cmd, j.EnvVars)
	if err != nil {
		j.NewErrorMessage(err.Error())
		return
	}

	for {
		status, logStream, err := c.JobMonitor(j.AWSBatchID)

		if err != nil {
			j.NewErrorMessage(err.Error())
			return
		}

		switch status {
		case "ACCEPTED":
			j.NewStatusUpdate(ACCEPTED)
		case "RUNNING":
			j.Outputs = []interface{}{logStream}
			j.NewStatusUpdate(RUNNING)
		case "SUCCEEDED":
			j.Outputs = []interface{}{logStream}
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
		time.Sleep(10 * time.Second)
	}
}

func (j *AWSBatchJob) Kill() error {
	c, err := controllers.NewAWSBatchController(os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY"), os.Getenv("AWS_DEFAULT_REGION"))
	if err != nil {
		j.NewErrorMessage(err.Error())
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

	messageData := int(unsafe.Sizeof(j.MessageList))
	for _, item := range j.MessageList {
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
		int(unsafe.Sizeof(j.LogInfo)) + len(j.LogInfo) +
		int(unsafe.Sizeof(j.JobDef)) + len(j.JobDef) +
		int(unsafe.Sizeof(j.JobQueue)) + len(j.JobQueue) +
		int(unsafe.Sizeof(j.JobName)) + len(j.JobName) +
		int(unsafe.Sizeof(j.EnvVars)) + len(j.EnvVars)

	return totalMemory
}
